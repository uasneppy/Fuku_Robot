package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/uasneppy/Fuku_Robot/fuku/db/command_center"
	"github.com/uasneppy/Fuku_Robot/fuku/db/lang"
	"github.com/uasneppy/Fuku_Robot/fuku/db/models"
	"github.com/uasneppy/Fuku_Robot/fuku/i18n"
	"github.com/uasneppy/Fuku_Robot/fuku/utils/chat_status"
	"github.com/uasneppy/Fuku_Robot/fuku/utils/error_handling"
	"github.com/uasneppy/Fuku_Robot/fuku/utils/extraction"
	"github.com/uasneppy/Fuku_Robot/fuku/utils/formatting"
	"github.com/uasneppy/Fuku_Robot/fuku/utils/helpers"
)

// Command center action types.
const (
	actionTypeMute   = "mute"
	actionTypeUnmute = "unmute"
	actionTypeBan    = "ban"
	actionTypeUnban  = "unban"
	actionTypeKick   = "kick"
)

// ccMaxParallelChats bounds how many connected chats are acted on concurrently, so a
// command center with MaxConnectedChats does not fire 50 simultaneous Telegram calls.
const ccMaxParallelChats = 5

var commandCenterModule = moduleStruct{
	moduleName: "CommandCenter",
}

func init() {
	RegisterLegacyModule("CommandCenter", 280, LoadCommandCenter)
}

// ccChatResult is the outcome of one action against one connected chat.
type ccChatResult struct {
	chatID int64
	err    error
}

// ccResolveHostCenter returns the command center hosted in the current chat, requiring
// the caller to be an admin of it. It is the single authorization gate for every
// cross-chat action: the command must run inside the command center's own chat, and
// only that chat's admins may drive it.
func ccResolveHostCenter(b *gotgbot.Bot, ctx *ext.Context) (*models.CommandCenter, bool) {
	chat := ctx.EffectiveChat
	if !chat_status.RequireGroup(b, ctx, chat) {
		return nil, false
	}

	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return nil, false
	}

	cc, err := command_center.GetCommandCenterByChatID(chat.Id)
	if err != nil {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("command_center_not_a_center")
		replyHTML(b, ctx.EffectiveMessage, text)
		return nil, false
	}

	if !chat_status.RequireUserAdmin(b, ctx, chat, user.Id) {
		return nil, false
	}

	return cc, true
}

// setupCommandCenter turns the current chat into a command center.
func (m moduleStruct) setupCommandCenter(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	if !chat_status.RequireGroup(b, ctx, chat) {
		return ext.EndGroups
	}

	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	// Only a chat admin may designate the chat as a command center.
	if !chat_status.RequireUserAdmin(b, ctx, chat, user.Id) {
		return ext.EndGroups
	}

	name := strings.TrimSpace(strings.Join(ctx.Args()[1:], " "))
	if name == "" {
		name = chat.Title
	}

	cc, err := command_center.CreateCommandCenter(chat.Id, user.Id, name)
	if err != nil {
		text, _ := tr.GetString(ccErrorKey(err, "command_center_setup_failed"))
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	text, _ := tr.GetString("command_center_created", i18n.TranslationParams{
		"name": formatting.HtmlEscape(cc.Name),
		"id":   strconv.FormatUint(uint64(cc.ID), 10),
	})
	replyHTML(b, msg, text)
	return ext.EndGroups
}

// deleteCommandCenter removes the command center hosted in the current chat.
func (m moduleStruct) deleteCommandCenter(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	if !chat_status.RequireGroup(b, ctx, chat) {
		return ext.EndGroups
	}

	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	cc, err := command_center.GetCommandCenterByChatID(chat.Id)
	if err != nil {
		text, _ := tr.GetString("command_center_not_a_center")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	// Deleting is destructive for every connected chat, so restrict it to the owner.
	if user.Id != cc.OwnerID {
		text, _ := tr.GetString("command_center_owner_only")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	if err := command_center.DeleteCommandCenter(cc.ID); err != nil {
		log.Errorf("[CommandCenter] Delete failed: %v", err)
		text, _ := tr.GetString("command_center_delete_failed")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	text, _ := tr.GetString("command_center_deleted")
	replyHTML(b, msg, text)
	return ext.EndGroups
}

// connectChat connects the current chat to a command center by ID.
func (m moduleStruct) connectChat(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	if !chat_status.RequireGroup(b, ctx, chat) {
		return ext.EndGroups
	}

	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	// The caller must be an admin here, so a stranger cannot attach this chat.
	if !chat_status.RequireUserAdmin(b, ctx, chat, user.Id) {
		return ext.EndGroups
	}

	args := ctx.Args()
	if len(args) < 2 {
		text, _ := tr.GetString("command_center_no_id")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	id, err := strconv.ParseUint(strings.TrimSpace(args[1]), 10, 64)
	if err != nil || id == 0 {
		text, _ := tr.GetString("command_center_invalid_id")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	cc, err := command_center.GetCommandCenterByID(uint(id))
	if err != nil {
		text, _ := tr.GetString("command_center_not_found")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	// Connecting grants the command center power over this chat, so require the
	// caller to also own that command center rather than merely know its ID.
	if user.Id != cc.OwnerID {
		text, _ := tr.GetString("command_center_connect_owner_only")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	if err := command_center.ConnectChat(cc.ID, chat.Id); err != nil {
		text, _ := tr.GetString(ccErrorKey(err, "command_center_connect_failed"))
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	text, _ := tr.GetString("command_center_connected", i18n.TranslationParams{
		"name": formatting.HtmlEscape(cc.Name),
	})
	replyHTML(b, msg, text)
	return ext.EndGroups
}

// disconnectChat detaches the current chat from its command center.
func (m moduleStruct) disconnectChat(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	if !chat_status.RequireGroup(b, ctx, chat) {
		return ext.EndGroups
	}

	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	cc, err := command_center.GetCommandCenterForChat(chat.Id)
	if err != nil {
		text, _ := tr.GetString("command_center_not_connected")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	// Either side of the link may break it: an admin here, or the center's owner.
	if user.Id != cc.OwnerID && !chat_status.RequireUserAdmin(b, ctx, chat, user.Id) {
		return ext.EndGroups
	}

	if err := command_center.DisconnectChat(cc.ID, chat.Id); err != nil {
		log.Errorf("[CommandCenter] Disconnect failed: %v", err)
		text, _ := tr.GetString("command_center_disconnect_failed")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	text, _ := tr.GetString("command_center_disconnected", i18n.TranslationParams{
		"name": formatting.HtmlEscape(cc.Name),
	})
	replyHTML(b, msg, text)
	return ext.EndGroups
}

// listConnectedChats lists the chats managed by the command center in this chat.
func (m moduleStruct) listConnectedChats(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	cc, ok := ccResolveHostCenter(b, ctx)
	if !ok {
		return ext.EndGroups
	}

	connected, err := command_center.GetConnectedChats(cc.ID)
	if err != nil {
		log.Errorf("[CommandCenter] List connected failed: %v", err)
		text, _ := tr.GetString("command_center_list_failed")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	if len(connected) == 0 {
		text, _ := tr.GetString("command_center_no_chats", i18n.TranslationParams{
			"id": strconv.FormatUint(uint64(cc.ID), 10),
		})
		replyHTML(b, msg, text)
		return ext.EndGroups
	}

	header, _ := tr.GetString("command_center_list_header", i18n.TranslationParams{
		"name":  formatting.HtmlEscape(cc.Name),
		"count": strconv.Itoa(len(connected)),
	})

	var sb strings.Builder
	sb.WriteString(header)
	for _, conn := range connected {
		title := strconv.FormatInt(conn.ChatID, 10)
		if info, err := b.GetChat(conn.ChatID, nil); err == nil && info.Title != "" {
			title = fmt.Sprintf("%s (<code>%d</code>)", formatting.HtmlEscape(info.Title), conn.ChatID)
		}
		fmt.Fprintf(&sb, "\n - %s", title)
	}

	replyHTML(b, msg, sb.String())
	return ext.EndGroups
}

// ccModerationHandler builds a handler that applies actionType across every chat
// connected to the command center hosted in the current chat.
func ccModerationHandler(actionType string) func(*gotgbot.Bot, *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		msg := ctx.EffectiveMessage
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

		cc, ok := ccResolveHostCenter(b, ctx)
		if !ok {
			return ext.EndGroups
		}

		// ExtractUserAndText returns -1 when it has already replied with the reason.
		targetID, reason := extraction.ExtractUserAndText(b, ctx)
		if targetID == -1 {
			return ext.EndGroups
		}
		if targetID == 0 {
			text, _ := tr.GetString("command_center_no_target")
			replyHTML(b, msg, text)
			return ext.EndGroups
		}

		// Never let the command center act on its own owner or on the bot.
		if targetID == cc.OwnerID || targetID == b.Id {
			text, _ := tr.GetString("command_center_protected_target")
			replyHTML(b, msg, text)
			return ext.EndGroups
		}

		connected, err := command_center.GetConnectedChats(cc.ID)
		if err != nil {
			log.Errorf("[CommandCenter] List connected failed: %v", err)
			text, _ := tr.GetString("command_center_list_failed")
			replyHTML(b, msg, text)
			return ext.EndGroups
		}
		if len(connected) == 0 {
			text, _ := tr.GetString("command_center_no_chats", i18n.TranslationParams{
				"id": strconv.FormatUint(uint64(cc.ID), 10),
			})
			replyHTML(b, msg, text)
			return ext.EndGroups
		}

		results := ccApplyAcrossChats(b, connected, targetID, actionType)

		var succeeded int
		var failures []string
		for _, res := range results {
			if res.err == nil {
				succeeded++
				continue
			}
			failures = append(failures,
				fmt.Sprintf("<code>%d</code>: %s", res.chatID, formatting.HtmlEscape(res.err.Error())))
		}

		if succeeded > 0 {
			if err := command_center.LogAction(cc.ID, cc.ChatID, targetID,
				actionType, reason, msg.MessageId); err != nil {
				log.Errorf("[CommandCenter] Log action failed: %v", err)
			}
		}

		summary, _ := tr.GetString("command_center_action_summary", i18n.TranslationParams{
			"action":  formatting.HtmlEscape(actionType),
			"user":    strconv.FormatInt(targetID, 10),
			"success": strconv.Itoa(succeeded),
			"total":   strconv.Itoa(len(connected)),
		})

		var sb strings.Builder
		sb.WriteString(summary)
		if len(failures) > 0 {
			failHeader, _ := tr.GetString("command_center_action_failures")
			sb.WriteString("\n\n")
			sb.WriteString(failHeader)
			for _, f := range failures {
				fmt.Fprintf(&sb, "\n - %s", f)
			}
		}

		replyHTML(b, msg, sb.String())
		return ext.EndGroups
	}
}

// ccApplyAcrossChats runs the action against every connected chat, bounded to
// ccMaxParallelChats concurrent Telegram calls.
func ccApplyAcrossChats(b *gotgbot.Bot, connected []models.CommandCenterChat,
	targetID int64, actionType string,
) []ccChatResult {
	results := make([]ccChatResult, len(connected))
	sem := make(chan struct{}, ccMaxParallelChats)
	var wg sync.WaitGroup

	for i, conn := range connected {
		wg.Add(1)
		go func(idx int, chatID int64) {
			defer wg.Done()
			defer error_handling.RecoverFromPanic("ccApplyAcrossChats", "CommandCenter")

			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = ccChatResult{
				chatID: chatID,
				err:    ccApplyInChat(b, chatID, targetID, actionType),
			}
		}(i, conn.ChatID)
	}

	wg.Wait()
	return results
}

// ccApplyInChat performs a single moderation action in one connected chat.
func ccApplyInChat(b *gotgbot.Bot, chatID, targetID int64, actionType string) error {
	// Skip chats where the target is an admin; the bot cannot restrict them and
	// attempting it would fail noisily for every such chat.
	if chat_status.IsUserAdmin(b, chatID, targetID) {
		return fmt.Errorf("target is an admin here")
	}

	switch actionType {
	case actionTypeBan:
		_, err := b.BanChatMember(chatID, targetID, nil)
		return err
	case actionTypeUnban:
		_, err := b.UnbanChatMember(chatID, targetID,
			&gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
		return err
	case actionTypeKick:
		// One-call removal; see kickMember in bans.go for why this avoids ban/unban.
		return kickMember(b, chatID, targetID)
	case actionTypeMute:
		_, err := b.RestrictChatMember(chatID, targetID, MutedPermissions, nil)
		return err
	case actionTypeUnmute:
		perms := defaultUnmutePermissions()
		if info, err := b.GetChat(chatID, nil); err == nil {
			perms = resolveUnmutePermissions(info)
		}
		_, err := b.RestrictChatMember(chatID, targetID, perms, nil)
		return err
	default:
		return fmt.Errorf("unknown action type: %s", actionType)
	}
}

// ccErrorKey maps a repository sentinel error to its locale key.
func ccErrorKey(err error, fallback string) string {
	switch {
	case errors.Is(err, command_center.ErrAlreadyOwnsCC):
		return "command_center_already_owns"
	case errors.Is(err, command_center.ErrChatIsCommandCenter):
		return "command_center_already_center"
	case errors.Is(err, command_center.ErrAlreadyConnected):
		return "command_center_already_connected"
	case errors.Is(err, command_center.ErrLimitReached):
		return "command_center_limit_reached"
	case errors.Is(err, command_center.ErrNotFound):
		return "command_center_not_found"
	case errors.Is(err, command_center.ErrNotConnected):
		return "command_center_not_connected"
	default:
		return fallback
	}
}

// LoadCommandCenter registers the command center commands.
func LoadCommandCenter(dispatcher *ext.Dispatcher) {
	DefaultHelpRegistry().AbleMap[commandCenterModule.moduleName] = true

	// Setup and membership.
	dispatcher.AddHandler(handlers.NewCommand("ccsetup", commandCenterModule.setupCommandCenter))
	dispatcher.AddHandler(handlers.NewCommand("ccdelete", commandCenterModule.deleteCommandCenter))
	dispatcher.AddHandler(handlers.NewCommand("ccconnect", commandCenterModule.connectChat))
	dispatcher.AddHandler(handlers.NewCommand("ccdisconnect", commandCenterModule.disconnectChat))
	dispatcher.AddHandler(handlers.NewCommand("cclist", commandCenterModule.listConnectedChats))

	// Cross-chat moderation, usable only inside the command center's own chat.
	dispatcher.AddHandler(handlers.NewCommand("ccban", ccModerationHandler(actionTypeBan)))
	dispatcher.AddHandler(handlers.NewCommand("ccunban", ccModerationHandler(actionTypeUnban)))
	dispatcher.AddHandler(handlers.NewCommand("cckick", ccModerationHandler(actionTypeKick)))
	dispatcher.AddHandler(handlers.NewCommand("ccmute", ccModerationHandler(actionTypeMute)))
	dispatcher.AddHandler(handlers.NewCommand("ccunmute", ccModerationHandler(actionTypeUnmute)))

	for _, cmd := range []string{
		"ccsetup", "ccdelete", "ccconnect", "ccdisconnect", "cclist",
		"ccban", "ccunban", "cckick", "ccmute", "ccunmute",
	} {
		helpers.AddCmdToDisableable(cmd)
	}
}
