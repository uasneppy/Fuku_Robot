package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// lockdownModule groups the lockdown commands. Its handlerGroup is the guard group
// a later plan uses for the join guard.
var lockdownModule = moduleStruct{
	moduleName:   "Lockdown",
	handlerGroup: -7,
}

const (
	// lockdownReasonMaxRunes caps the reason an admin types.
	lockdownReasonMaxRunes = 300
	// lockdownNameMaxRunes caps a name stored on a lockdown row.
	lockdownNameMaxRunes = 64
	// lockdownLiveCheckTimeout bounds the live getChatMember of the command's sender.
	lockdownLiveCheckTimeout = 5 * time.Second
)

// The reason and the names are user-controlled, and the translator runs a
// printf-style pass over interpolated text, so they are spliced in after the
// translation, in place of these tokens.
const (
	lockdownReasonToken = "<<lockdown-reason>>"
	lockdownNameToken   = "<<lockdown-name>>"
	// lockdownDetailToken stands in for Telegram's already escaped error text.
	lockdownDetailToken = "<<lockdown-detail>>"
)

var (
	// lockdownDesc and unlockdownDesc are deliberately not Disableable: a chat
	// cannot switch off its raid tool. Their authority check is a live getChatMember
	// of the sender, never the admin cache.
	lockdownDesc = helpers.CommandDescriptor{
		Name:           "lockdown",
		RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup(), requireLockdownAuthority(true)},
	}
	unlockdownDesc = helpers.CommandDescriptor{
		Name:           "unlockdown",
		RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup(), requireLockdownAuthority(true)},
	}
)

// lockdownLiveMember asks Telegram, live and uncached, for one member of a group.
func lockdownLiveMember(b *gotgbot.Bot, chatID, userID int64) (gotgbot.MergedChatMember, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lockdownLiveCheckTimeout)
	defer cancel()

	member, err := b.GetChatMemberWithContext(ctx, chatID, userID, nil)
	if err != nil {
		return gotgbot.MergedChatMember{}, err
	}
	if member == nil {
		return gotgbot.MergedChatMember{}, errors.New("getChatMember returned no member")
	}
	return member.MergeChatMember(), nil
}

// requireLockdownAuthority admits only the group's creator, or an administrator
// holding can_restrict_members when needRestrict is set, judged by a live
// getChatMember of the sender. The admin cache, the Telegram service IDs and the
// chat's AnonAdmin setting never authorize: a failed lookup refuses. It replies
// through the permission responder, so it is valid only inside the command pipeline.
func requireLockdownAuthority(needRestrict bool) helpers.CheckFunc {
	return func(c *helpers.CommandContext) bool {
		if c.User == nil || c.Chat == nil || c.Ctx == nil {
			return false
		}
		refuse := func(key string) bool {
			chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, key, "", chat_status.WithReply())
			return false
		}

		// A message sent as another chat or channel cannot be tied to one person.
		if sender := c.Ctx.EffectiveSender; sender != nil && sender.Chat != nil && sender.Chat.Id != c.Chat.Id {
			return refuse("chat_status_user_admin_cmd_error")
		}

		member, err := lockdownLiveMember(c.Bot, c.Chat.Id, c.User.Id)
		if err != nil {
			log.Warnf("[Lockdown] live check of user %d in chat %d failed: %v", c.User.Id, c.Chat.Id, err)
			return refuse("lockdown_check_failed")
		}

		if needRestrict {
			switch staffIssuerSkipReason(member) {
			case "":
				return true
			case staffReasonSkipIssuerNoRight:
				return refuse("chat_status_restrict_cmd_error")
			}
			return refuse("chat_status_user_admin_cmd_error")
		}
		if member.Status == gotgbot.ChatMemberStatusCreator || member.Status == gotgbot.ChatMemberStatusAdministrator {
			return true
		}
		return refuse("chat_status_user_admin_cmd_error")
	}
}

// lockdownHasPermissions reports whether a getChat permissions member is present
// and is a JSON object. A group whose permissions cannot be read is never locked,
// because the lift could not put them back.
func lockdownHasPermissions(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed)
}

// lockdownCapRunes cuts s to at most n runes.
func lockdownCapRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// lockdownReasonFromArgs joins the words after the command into the reason, trimmed
// and cut to lockdownReasonMaxRunes.
func lockdownReasonFromArgs(args []string) string {
	return lockdownCapRunes(strings.TrimSpace(strings.Join(args, " ")), lockdownReasonMaxRunes)
}

// lockdownCommandArgs is the words after the command itself, empty when the message
// has no words at all.
func lockdownCommandArgs(ctx *ext.Context) []string {
	args := ctx.Args()
	if len(args) == 0 {
		return nil
	}
	return args[1:]
}

// lockdownTime renders a moment the way the staff history does, in UTC.
func lockdownTime(t time.Time) string {
	return t.UTC().Format("2 Jan 15:04") + " UTC"
}

// lockdownSplice replaces each token of text with its value, in token, value order.
// The values are already HTML-escaped; they go in after translation.
func lockdownSplice(text string, pairs ...string) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		text = strings.ReplaceAll(text, pairs[i], pairs[i+1])
	}
	return text
}

// lockdownJoinLines joins the non-empty lines of a reply with newlines.
func lockdownJoinLines(lines []string) string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// lockdownMention renders a person for a group message: a mention that escapes the
// name, or the escaped name alone when there is no user ID.
func lockdownMention(userID int64, name string) string {
	if userID == 0 {
		return html.EscapeString(name)
	}
	return formatting.MentionHtml(userID, name)
}

// replyLockdown sends an HTML reply to the command message and logs a failure.
func replyLockdown(b *gotgbot.Bot, msg *gotgbot.Message, text string) {
	if msg == nil || text == "" {
		return
	}
	if _, err := msg.Reply(b, text, formatting.Shtml()); err != nil {
		log.Errorf("[Lockdown] reply: %v", err)
	}
}

// lockdownText translates key with the given params and logs a missing key.
func lockdownText(tr *i18n.Translator, key string, params ...i18n.TranslationParams) string {
	text, err := tr.GetString(key, params...)
	if err != nil {
		log.Errorf("[Lockdown] translate %s: %v", key, err)
	}
	return text
}

// lockdownReasonLine is the "Reason: ..." line, empty when there is no reason.
func lockdownReasonLine(tr *i18n.Translator, reason string) string {
	if reason == "" {
		return ""
	}
	line := lockdownText(tr, "lockdown_reason_line", i18n.TranslationParams{"reason": lockdownReasonToken})
	return lockdownSplice(line, lockdownReasonToken, html.EscapeString(reason))
}

// lockdownAlreadyActiveText tells the existing lockdown: since when, who started it
// and the reason.
func lockdownAlreadyActiveText(tr *i18n.Translator, row *models.ChatLockdown) string {
	since := row.CreatedAt
	if row.LockedAt != nil {
		since = *row.LockedAt
	}
	text := lockdownText(tr, "lockdown_already_active", i18n.TranslationParams{
		"since": lockdownTime(since),
		"name":  lockdownNameToken,
	})
	text = lockdownSplice(text, lockdownNameToken, lockdownMention(row.StartedBy, row.StartedByName))
	if line := lockdownReasonLine(tr, row.Reason); line != "" {
		text += "\n" + line
	}
	return strings.TrimSpace(text)
}

// lockdown locks the group the command was sent in: it stores the group's raw
// permissions, then sets every default permission off, and only after Telegram
// confirmed that announces the lockdown. A second /lockdown makes no Telegram call
// and reports the lockdown that already runs.
func (m moduleStruct) lockdown(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	actor := chat_status.GetEffectiveUser(ctx)
	if chat == nil || msg == nil || actor == nil {
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	reply := func(text string) error {
		replyLockdown(b, msg, text)
		return ext.EndGroups
	}
	failed := func() error { return reply(lockdownText(tr, "lockdown_state_failed")) }
	// refuse answers a refusal whose text carries Telegram's own words; nothing has
	// been written when it runs.
	refuse := func(key, detail string) error {
		text := lockdownText(tr, key, i18n.TranslationParams{"detail": lockdownDetailToken})
		return reply(strings.TrimSpace(lockdownSplice(text, lockdownDetailToken, detail)))
	}

	// A basic group cannot honour "banned until the lift", so it is refused before
	// anything is read or written.
	if chat.Type != "supergroup" {
		return refuse("lockdown_basic_group", "")
	}

	active, err := lockdown.GetActiveFresh(chat.Id)
	if err != nil {
		return failed()
	}
	if active != nil {
		return reply(lockdownAlreadyActiveText(tr, active))
	}

	// The bot's own rights are read live. An unknown answer fails closed.
	botMember, botResult, botErr := chat_status.FetchBotMember(b, chat.Id)
	if botResult == chat_status.BotMemberUnknown {
		log.Warnf("[Lockdown] bot rights check in chat %d failed: %v", chat.Id, botErr)
		return refuse("lockdown_bot_check_failed", "")
	}
	if botResult == chat_status.BotMemberMissing ||
		botMember.Status != gotgbot.ChatMemberStatusAdministrator || !botMember.CanRestrictMembers {
		return refuse("lockdown_bot_cannot_restrict", "")
	}

	bg := context.Background()
	chatInfo, err := fetchLockdownChat(bg, b, chat.Id)
	if err != nil {
		log.Warnf("[Lockdown] getChat for chat %d failed: %v", chat.Id, err)
		return refuse("lockdown_permissions_unreadable", telegramErrorDetail(err))
	}
	if !lockdownHasPermissions(chatInfo.Permissions) {
		return refuse("lockdown_permissions_unreadable", "")
	}

	name := lockdownCapRunes(staffFullName(actor), lockdownNameMaxRunes)
	row := &models.ChatLockdown{
		ChatID:            chat.Id,
		TriggerKind:       models.LockdownTriggerManual,
		Reason:            lockdownReasonFromArgs(lockdownCommandArgs(ctx)),
		StartedBy:         actor.Id,
		StartedByName:     name,
		PrePermissions:    string(chatInfo.Permissions),
		LockedPermissions: lockdownLockedPermissions,
	}
	started, err := lockdown.Start(row)
	if err != nil {
		return failed()
	}
	if !started {
		existing, err := lockdown.GetActiveFresh(chat.Id)
		if err != nil || existing == nil {
			return failed()
		}
		return reply(lockdownAlreadyActiveText(tr, existing))
	}

	if err := setLockdownPermissions(bg, b, chat.Id, lockdownLockedPermissions); err != nil {
		log.Warnf("[Lockdown] lock call for chat %d failed: %v", chat.Id, err)
		if _, delErr := lockdown.DeleteUnconfirmed(row.ID); delErr != nil {
			log.Errorf("[Lockdown] lockdown %d could not be removed after a refused lock: %v", row.ID, delErr)
		}
		return refuse("lockdown_lock_failed", telegramErrorDetail(err))
	}
	if _, err := lockdown.ConfirmLocked(row.ID); err != nil {
		if _, retryErr := lockdown.ConfirmLocked(row.ID); retryErr != nil {
			log.Errorf("[Lockdown] lockdown %d is locked but not confirmed: %v", row.ID, retryErr)
		}
	}

	parts := []string{
		lockdownText(tr, "lockdown_started"),
		lockdownText(tr, "lockdown_note_approved_muted"),
		lockdownSplice(
			lockdownText(tr, "lockdown_started_by", i18n.TranslationParams{"name": lockdownNameToken}),
			lockdownNameToken, lockdownMention(actor.Id, name),
		),
	}
	if line := lockdownReasonLine(tr, row.Reason); line != "" {
		parts = append(parts, line)
	}
	// A missing delete or invite right never blocks the lock; it only adds a note.
	if !botMember.CanDeleteMessages {
		parts = append(parts, lockdownText(tr, "lockdown_note_no_delete"))
	}
	if !botMember.CanInviteUsers {
		parts = append(parts, lockdownText(tr, "lockdown_note_no_invite"))
	}
	return reply(strings.TrimSpace(strings.Join(parts, "\n")))
}

// unlockdown lifts the group's lockdown: it sends the stored raw permissions back,
// marks the lockdown lifting with who lifted it, finishes it at once when no joiner
// is left to unban, and only then announces the lift.
func (m moduleStruct) unlockdown(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	actor := chat_status.GetEffectiveUser(ctx)
	if chat == nil || msg == nil || actor == nil {
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	reply := func(text string) error {
		replyLockdown(b, msg, text)
		return ext.EndGroups
	}

	row, err := lockdown.GetActiveFresh(chat.Id)
	if err != nil {
		return reply(lockdownText(tr, "lockdown_state_failed"))
	}
	if row == nil {
		return reply(lockdownText(tr, "lockdown_not_active"))
	}

	if err := setLockdownPermissions(context.Background(), b, chat.Id, row.PrePermissions); err != nil {
		log.Warnf("[Lockdown] restore for chat %d failed: %v", chat.Id, err)
		return reply(lockdownText(tr, "lockdown_state_failed"))
	}

	name := lockdownCapRunes(staffFullName(actor), lockdownNameMaxRunes)
	won, err := lockdown.BeginLift(row.ID, actor.Id, name, false)
	if err != nil {
		return reply(lockdownText(tr, "lockdown_state_failed"))
	}
	if !won {
		return reply(lockdownText(tr, "lockdown_not_active"))
	}
	if _, err := lockdown.FinishLift(row.ID); err != nil {
		log.Errorf("[Lockdown] lockdown %d lift was not finished: %v", row.ID, err)
	}

	text := lockdownText(tr, "lockdown_lifted", i18n.TranslationParams{"name": lockdownNameToken})
	return reply(lockdownSplice(text, lockdownNameToken, lockdownMention(actor.Id, name)))
}

// LoadLockdown registers /lockdown, /unlockdown and /lockdownstatus.
func LoadLockdown(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(lockdownModule.moduleName, true)

	helpers.WrapCommand(dispatcher, lockdownDesc, pipelineHandler(lockdownModule.lockdown))
	helpers.WrapCommand(dispatcher, unlockdownDesc, pipelineHandler(lockdownModule.unlockdown))
	helpers.WrapCommand(dispatcher, lockdownStatusDesc, pipelineHandler(lockdownModule.lockdownStatus))
}

func init() {
	RegisterLegacyModule("Lockdown", 238, LoadLockdown)
}
