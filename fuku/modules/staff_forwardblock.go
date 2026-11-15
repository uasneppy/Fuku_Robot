package modules

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// The forward blocklist of a Staff Group: the channels or groups whose forwards the bot deletes in
// every linked group, and the one action applied to the member who forwarded. Writes are the Staff
// Group's live creator only (the creator is, by the link invariant, the creator of every linked
// group); any administrator of the Staff Group can read. The apply functions below judge no
// person's rights, the doors (the commands and the settings page) do.

const (
	// forwardBlockResolveTimeout bounds the one live getChat of a source being added.
	forwardBlockResolveTimeout = 8 * time.Second
	// forwardBlockTitleMaxRunes caps the stored source title.
	forwardBlockTitleMaxRunes = 64
	// forwardBlockListMax is how many sources /fwdblocklist shows before its more-line.
	forwardBlockListMax = 40
	// forwardBlockRefMaxLen bounds a typed ref before it is parsed.
	forwardBlockRefMaxLen = 64
)

var (
	addFwdBlockDesc = helpers.CommandDescriptor{
		Name:           "addfwdblock",
		RequiredChecks: forwardBlockChecks(true),
	}
	rmFwdBlockDesc = helpers.CommandDescriptor{
		Name:           "rmfwdblock",
		RequiredChecks: forwardBlockChecks(true),
	}
	fwdBlockListDesc = helpers.CommandDescriptor{
		Name:           "fwdblocklist",
		RequiredChecks: forwardBlockChecks(false),
	}
	fwdBlockActionDesc = helpers.CommandDescriptor{
		Name:           "fwdblockaction",
		RequiredChecks: forwardBlockChecks(false),
	}
)

// forwardBlockChecks is the check chain every forward block command shares: a named sender, a group
// chat, a Staff Group and an administrator of it. A write (creator true) also needs the Staff Group's
// live creator, the one check that is never the cached admin list.
func forwardBlockChecks(creator bool) []helpers.CheckFunc {
	checks := []helpers.CheckFunc{
		helpers.RejectAnonymousSender(),
		helpers.RequireGroup(),
		requireStaffGroup(),
		helpers.RequireUserAdmin(),
	}
	if creator {
		checks = append(checks, requireStaffGroupCreator())
	}
	return checks
}

// requireStaffGroup replies and returns false unless the chat is a Staff Group, read fresh. An
// unreadable row replies that the check failed and writes nothing.
func requireStaffGroup() helpers.CheckFunc {
	return func(c *helpers.CommandContext) bool {
		group, err := staff.GetStaffGroupFresh(c.Chat.Id)
		if err != nil {
			text, _ := c.Tr.GetString("staff_check_failed")
			replyStaff(c, text)
			return false
		}
		if group == nil {
			text, _ := c.Tr.GetString("staff_unset_not_staff")
			replyStaff(c, text)
			return false
		}
		return true
	}
}

// requireStaffGroupCreator replies and returns false unless Telegram, asked live, lists the sender as
// the creator of the Staff Group. An unknown answer refuses with the check-failed text and a
// mismatch with the owner-only text; nothing is written either way.
func requireStaffGroupCreator() helpers.CheckFunc {
	return func(c *helpers.CommandContext) bool {
		result, _, err := chat_status.CheckOwner(c.Bot, c.Chat.Id, c.User.Id)
		switch result {
		case chat_status.OwnerMatch:
			return true
		case chat_status.OwnerUnknown:
			log.Warnf("[Staff] forward block: owner check for chat %d failed: %v", c.Chat.Id, err)
			text, _ := c.Tr.GetString("staff_check_failed")
			replyStaff(c, text)
		default:
			text, _ := c.Tr.GetString("staff_fwdblock_owner_only")
			replyStaff(c, text)
		}
		return false
	}
}

// forwardBlockWriteSeams are the repository writes the apply functions call, a variable so a test
// can make one fail.
type forwardBlockWriteSeams struct {
	Add            func(ctx context.Context, staffChatID, sourceChatID int64, title string, userID int64) (bool, error)
	RemoveByID     func(ctx context.Context, staffChatID, id int64) (*models.StaffForwardBlock, error)
	RemoveBySource func(ctx context.Context, staffChatID, sourceChatID int64) (bool, error)
	SetAction      func(ctx context.Context, staffChatID, userID int64, action string) error
}

// forwardBlockWrites is the one place the commands and the settings page reach the repository's
// writes through.
var forwardBlockWrites = forwardBlockWriteSeams{
	Add:            staff.AddForwardBlock,
	RemoveByID:     staff.RemoveForwardBlockByID,
	RemoveBySource: staff.RemoveForwardBlockBySource,
	SetAction:      staff.SetForwardBlockAction,
}

// forwardBlockChat is what a live getChat answers for a source: its ID, type and title.
type forwardBlockChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

// forwardBlockResolve looks a source up with one live getChat, a variable so a test can script the
// answer. chat is an int64 for a numeric ref and the @name for a name.
var forwardBlockResolve = forwardBlockResolveChat

// forwardBlockResolveChat is the default of forwardBlockResolve: a raw getChat (gotgbot's GetChat
// takes only an int64) bounded by forwardBlockResolveTimeout.
func forwardBlockResolveChat(ctx context.Context, b *gotgbot.Bot, chat any) (forwardBlockChat, error) {
	callCtx, cancel := context.WithTimeout(ctx, forwardBlockResolveTimeout)
	defer cancel()
	raw, err := b.RequestWithContext(callCtx, "getChat", map[string]any{"chat_id": chat}, nil)
	if err != nil {
		return forwardBlockChat{}, err
	}
	var out forwardBlockChat
	if err := json.Unmarshal(raw, &out); err != nil {
		return forwardBlockChat{}, err
	}
	return out, nil
}

// forwardBlockRef is a parsed source ref: a negative numeric chat ID or an @username.
type forwardBlockRef struct {
	ID       int64
	Username string
}

// parseForwardBlockRef is pure and strict: a negative numeric ID through parseStaffChatIDArg, an
// @name through staffUsernameRe, nothing else.
func parseForwardBlockRef(raw string) (forwardBlockRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > forwardBlockRefMaxLen {
		return forwardBlockRef{}, false
	}
	if id, ok := parseStaffChatIDArg(raw); ok {
		return forwardBlockRef{ID: id}, true
	}
	if staffUsernameRe.MatchString(raw) {
		return forwardBlockRef{Username: raw}, true
	}
	return forwardBlockRef{}, false
}

// forwardBlockAddOutcome is what applyForwardBlockAdd did; its zero value is added.
type forwardBlockAddOutcome int

const (
	// forwardBlockAdded means the source was stored.
	forwardBlockAdded forwardBlockAddOutcome = iota
	// forwardBlockAlready means the source is already listed.
	forwardBlockAlready
	// forwardBlockInvalidRef means the ref is neither a negative ID nor an @username.
	forwardBlockInvalidRef
	// forwardBlockNotFound means Telegram does not know the @username.
	forwardBlockNotFound
	// forwardBlockNotSource means the answer is not a channel or a group.
	forwardBlockNotSource
	// forwardBlockCheckFailed means Telegram could not answer, so nothing was stored.
	forwardBlockCheckFailed
	// forwardBlockFull means the list holds staff.ForwardBlockMax sources.
	forwardBlockFull
	// forwardBlockNoStaffGroup means the chat is no longer a Staff Group.
	forwardBlockNoStaffGroup
	// forwardBlockSaveFailed means a database error, logged where it happened.
	forwardBlockSaveFailed
)

// forwardBlockAddResult is the typed result of applyForwardBlockAdd.
type forwardBlockAddResult struct {
	Outcome      forwardBlockAddOutcome
	SourceChatID int64
	Title        string
}

// forwardBlockTitle trims a title and cuts it at forwardBlockTitleMaxRunes code points.
func forwardBlockTitle(title string) string {
	title = strings.TrimSpace(title)
	runes := []rune(title)
	if len(runes) > forwardBlockTitleMaxRunes {
		return string(runes[:forwardBlockTitleMaxRunes])
	}
	return title
}

// forwardBlockIsSourceType reports whether a getChat type can be a forward source.
func forwardBlockIsSourceType(chatType string) bool {
	return chatType == "channel" || chatType == "supergroup" || chatType == "group"
}

// forwardBlockLookup resolves a parsed ref to the numeric ID and title to store, or the outcome that
// refuses (point A): a name that Telegram does not know is not found, an answer that is not a
// negative channel or group ID is not a source, a numeric ID Telegram refuses with a client error
// is stored unverified with no title, and any other failure stores nothing.
func forwardBlockLookup(ctx context.Context, b *gotgbot.Bot, ref forwardBlockRef) (id int64, title string, outcome forwardBlockAddOutcome) {
	var query any = ref.ID
	if ref.Username != "" {
		query = ref.Username
	}
	chat, err := forwardBlockResolve(ctx, b, query)
	if err != nil {
		switch {
		case !isTelegramClientError(err):
			return 0, "", forwardBlockCheckFailed
		case ref.Username != "":
			return 0, "", forwardBlockNotFound
		default:
			return ref.ID, "", forwardBlockAdded
		}
	}
	if chat.ID >= 0 || !forwardBlockIsSourceType(chat.Type) || (ref.Username == "" && chat.ID != ref.ID) {
		return 0, "", forwardBlockNotSource
	}
	return chat.ID, forwardBlockTitle(chat.Title), forwardBlockAdded
}

// applyForwardBlockAdd lists one source for the Staff Group. It parses the ref strictly first (no
// Telegram call for a bad one), resolves it live, stores the numeric ID and title and maps the
// repository's refusals. It judges no person's rights.
func applyForwardBlockAdd(ctx context.Context, b *gotgbot.Bot, staffChatID, userID int64, rawRef string) forwardBlockAddResult {
	ref, ok := parseForwardBlockRef(rawRef)
	if !ok {
		return forwardBlockAddResult{Outcome: forwardBlockInvalidRef}
	}
	id, title, outcome := forwardBlockLookup(ctx, b, ref)
	if outcome != forwardBlockAdded {
		return forwardBlockAddResult{Outcome: outcome}
	}
	result := forwardBlockAddResult{SourceChatID: id, Title: title}
	added, err := forwardBlockWrites.Add(ctx, staffChatID, id, title, userID)
	switch {
	case errors.Is(err, staff.ErrStaffGroupMissing):
		result.Outcome = forwardBlockNoStaffGroup
	case errors.Is(err, staff.ErrForwardBlockFull):
		result.Outcome = forwardBlockFull
	case err != nil:
		log.Errorf("[Staff] forward block add in chat %d: %v", staffChatID, err)
		result.Outcome = forwardBlockSaveFailed
	case !added:
		result.Outcome = forwardBlockAlready
	default:
		log.Infof("[Staff] forward block source added: staff chat %d by user %d", staffChatID, userID)
	}
	return result
}

// forwardBlockRemoveOutcome is what a removal did; its zero value is removed.
type forwardBlockRemoveOutcome int

const (
	// forwardBlockRemoved means a listed source went.
	forwardBlockRemoved forwardBlockRemoveOutcome = iota
	// forwardBlockGone means no such source was listed: a missing, foreign or already removed row all
	// answer alike.
	forwardBlockGone
	// forwardBlockRemoveFailed means a database error, logged where it happened.
	forwardBlockRemoveFailed
)

// forwardBlockRemoveResult is the typed result of a removal.
type forwardBlockRemoveResult struct {
	Outcome      forwardBlockRemoveOutcome
	SourceChatID int64
	Title        string
}

// applyForwardBlockRemove removes a source by its chat ID. A source that is not listed is gone. The
// title of the removed row is not known on this path.
func applyForwardBlockRemove(ctx context.Context, staffChatID, sourceChatID int64) forwardBlockRemoveResult {
	result := forwardBlockRemoveResult{SourceChatID: sourceChatID}
	removed, err := forwardBlockWrites.RemoveBySource(ctx, staffChatID, sourceChatID)
	switch {
	case err != nil:
		log.Errorf("[Staff] forward block remove in chat %d: %v", staffChatID, err)
		result.Outcome = forwardBlockRemoveFailed
	case !removed:
		result.Outcome = forwardBlockGone
	}
	return result
}

// applyForwardBlockRemoveByID removes a source by its row ID, only a row of this Staff Group (the
// menu's delete). The removed row's source and title are returned for the notice.
func applyForwardBlockRemoveByID(ctx context.Context, staffChatID, id int64) forwardBlockRemoveResult {
	var result forwardBlockRemoveResult
	row, err := forwardBlockWrites.RemoveByID(ctx, staffChatID, id)
	switch {
	case err != nil:
		log.Errorf("[Staff] forward block remove by ID in chat %d: %v", staffChatID, err)
		result.Outcome = forwardBlockRemoveFailed
	case row == nil:
		result.Outcome = forwardBlockGone
	default:
		result.SourceChatID, result.Title = row.SourceChatID, row.SourceTitle
	}
	return result
}

// forwardBlockActionOutcome is what applyForwardBlockAction did; its zero value is applied.
type forwardBlockActionOutcome int

const (
	// forwardBlockActionApplied means the action was stored.
	forwardBlockActionApplied forwardBlockActionOutcome = iota
	// forwardBlockActionUnknown means the action is not one of blacklistActionValues; nothing was written.
	forwardBlockActionUnknown
	// forwardBlockActionNoStaffGroup means the chat is no longer a Staff Group.
	forwardBlockActionNoStaffGroup
	// forwardBlockActionSaveFailed means a database error, logged where it happened.
	forwardBlockActionSaveFailed
)

// forwardBlockActionResult is the typed result of applyForwardBlockAction.
type forwardBlockActionResult struct {
	Outcome forwardBlockActionOutcome
	Action  string
}

// applyForwardBlockAction stores the list's one action. It checks the action against the blacklist's
// vocabulary before any write and maps the repository's refusals.
func applyForwardBlockAction(ctx context.Context, staffChatID, userID int64, action string) forwardBlockActionResult {
	result := forwardBlockActionResult{Action: action}
	if !slices.Contains(blacklistActionValues, action) {
		result.Outcome = forwardBlockActionUnknown
		return result
	}
	err := forwardBlockWrites.SetAction(ctx, staffChatID, userID, action)
	switch {
	case errors.Is(err, staff.ErrForwardBlockAction):
		result.Outcome = forwardBlockActionUnknown
	case errors.Is(err, staff.ErrStaffGroupMissing):
		result.Outcome = forwardBlockActionNoStaffGroup
	case err != nil:
		log.Errorf("[Staff] forward block action in chat %d: %v", staffChatID, err)
		result.Outcome = forwardBlockActionSaveFailed
	default:
		log.Infof("[Staff] forward block action changed: staff chat %d by user %d", staffChatID, userID)
	}
	return result
}

// forwardBlockEntryText is the plain text a source is shown as: its chat ID, then its title. It is
// spliced into a translation through settingsListEntryText, which collapses it to one line, cuts and
// escapes it.
func forwardBlockEntryText(sourceChatID int64, title string) string {
	id := strconv.FormatInt(sourceChatID, 10)
	if strings.TrimSpace(title) == "" {
		return id
	}
	return id + " " + title
}

// forwardBlockAddText words the outcome of an add, shared by the commands and the settings page.
func forwardBlockAddText(tr *i18n.Translator, r forwardBlockAddResult) string {
	entry := forwardBlockEntryText(r.SourceChatID, r.Title)
	switch r.Outcome {
	case forwardBlockAdded:
		return settingsListEntryText(tr, "staff_fwdblock_added", "source", entry)
	case forwardBlockAlready:
		return settingsListEntryText(tr, "staff_fwdblock_already", "source", entry)
	case forwardBlockInvalidRef:
		return settingsText(tr, "staff_fwdblock_usage_add")
	case forwardBlockNotFound:
		return settingsText(tr, "staff_fwdblock_not_found")
	case forwardBlockNotSource:
		return settingsText(tr, "staff_fwdblock_not_source")
	case forwardBlockCheckFailed:
		return settingsText(tr, "staff_fwdblock_check_failed")
	case forwardBlockFull:
		return settingsText(tr, "staff_fwdblock_full", i18n.TranslationParams{"max": staff.ForwardBlockMax})
	case forwardBlockNoStaffGroup:
		return settingsText(tr, "staff_unset_not_staff")
	default:
		return settingsText(tr, "common_settings_save_failed")
	}
}

// forwardBlockRemoveText words the outcome of a removal, shared by the commands and the page.
func forwardBlockRemoveText(tr *i18n.Translator, r forwardBlockRemoveResult) string {
	switch r.Outcome {
	case forwardBlockRemoved:
		return settingsListEntryText(tr, "staff_fwdblock_removed", "source", forwardBlockEntryText(r.SourceChatID, r.Title))
	case forwardBlockGone:
		return settingsText(tr, "staff_fwdblock_not_listed")
	default:
		return settingsText(tr, "common_settings_save_failed")
	}
}

// forwardBlockActionText words the outcome of an action change, shared by the commands and the page.
func forwardBlockActionText(tr *i18n.Translator, r forwardBlockActionResult) string {
	switch r.Outcome {
	case forwardBlockActionApplied:
		return settingsText(tr, "staff_fwdblock_action_changed", i18n.TranslationParams{"action": settingsBlacklistActionWord(tr, r.Action)})
	case forwardBlockActionUnknown:
		return settingsText(tr, "staff_fwdblock_action_usage")
	case forwardBlockActionNoStaffGroup:
		return settingsText(tr, "staff_unset_not_staff")
	default:
		return settingsText(tr, "common_settings_save_failed")
	}
}

// forwardBlockArgs returns the words after the command name.
func forwardBlockArgs(c *helpers.CommandContext) []string {
	args := c.Ctx.Args()
	if len(args) <= 1 {
		return nil
	}
	return args[1:]
}

// addFwdBlock is /addfwdblock <ID or @username>: one source per command.
func (moduleStruct) addFwdBlock(c *helpers.CommandContext) error {
	args := forwardBlockArgs(c)
	if len(args) != 1 {
		replyStaff(c, settingsText(c.Tr, "staff_fwdblock_usage_add"))
		return ext.EndGroups
	}
	result := applyForwardBlockAdd(context.Background(), c.Bot, c.Chat.Id, c.User.Id, args[0])
	replyStaff(c, forwardBlockAddText(c.Tr, result))
	return ext.EndGroups
}

// rmFwdBlock is /rmfwdblock <ID>: the ID is the source chat ID /fwdblocklist shows.
func (moduleStruct) rmFwdBlock(c *helpers.CommandContext) error {
	args := forwardBlockArgs(c)
	if len(args) != 1 {
		replyStaff(c, settingsText(c.Tr, "staff_fwdblock_usage_rm"))
		return ext.EndGroups
	}
	id, ok := parseStaffChatIDArg(args[0])
	if !ok {
		replyStaff(c, settingsText(c.Tr, "staff_fwdblock_usage_rm"))
		return ext.EndGroups
	}
	replyStaff(c, forwardBlockRemoveText(c.Tr, applyForwardBlockRemove(context.Background(), c.Chat.Id, id)))
	return ext.EndGroups
}

// forwardBlockListText builds the /fwdblocklist reply: the header with the action, up to
// forwardBlockListMax sources (the ID in code, then the escaped title) and a more-line.
func forwardBlockListText(tr *i18n.Translator, rows []models.StaffForwardBlock, action string) string {
	if len(rows) == 0 {
		return settingsText(tr, "staff_fwdblock_list_empty")
	}
	lines := []string{settingsText(tr, "staff_fwdblock_list_header", i18n.TranslationParams{"action": settingsBlacklistActionWord(tr, action)})}
	for i, row := range rows {
		if i == forwardBlockListMax {
			lines = append(lines, settingsText(tr, "staff_fwdblock_list_more", i18n.TranslationParams{"n": len(rows) - forwardBlockListMax}))
			break
		}
		line := "• <code>" + strconv.FormatInt(row.SourceChatID, 10) + "</code>"
		if title := settingsListLabel(row.SourceTitle, forwardBlockTitleMaxRunes); title != "" {
			line += " " + title
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// fwdBlockList is /fwdblocklist: readable by any administrator of the Staff Group.
func (moduleStruct) fwdBlockList(c *helpers.CommandContext) error {
	ctx := context.Background()
	rows, err := staff.ListForwardBlocksFresh(ctx, c.Chat.Id)
	if err != nil {
		log.Errorf("[Staff] forward block list in chat %d: %v", c.Chat.Id, err)
		replyStaff(c, settingsText(c.Tr, "settings_check_failed"))
		return ext.EndGroups
	}
	rules, err := staff.GetForwardBlockRulesFresh(ctx, c.Chat.Id)
	if err != nil {
		log.Errorf("[Staff] forward block list action in chat %d: %v", c.Chat.Id, err)
		replyStaff(c, settingsText(c.Tr, "settings_check_failed"))
		return ext.EndGroups
	}
	replyStaff(c, forwardBlockListText(c.Tr, rows, rules.Action))
	return ext.EndGroups
}

// fwdBlockAction is /fwdblockaction [action]: with no argument any administrator reads the action,
// with one only the Staff Group's live creator changes it.
func (moduleStruct) fwdBlockAction(c *helpers.CommandContext) error {
	args := forwardBlockArgs(c)
	ctx := context.Background()
	if len(args) == 0 {
		rules, err := staff.GetForwardBlockRulesFresh(ctx, c.Chat.Id)
		if err != nil {
			log.Errorf("[Staff] forward block action read in chat %d: %v", c.Chat.Id, err)
			replyStaff(c, settingsText(c.Tr, "settings_check_failed"))
			return ext.EndGroups
		}
		replyStaff(c, settingsText(c.Tr, "staff_fwdblock_action_current", i18n.TranslationParams{"action": settingsBlacklistActionWord(c.Tr, rules.Action)}))
		return ext.EndGroups
	}
	if !requireStaffGroupCreator()(c) {
		return ext.EndGroups
	}
	if len(args) != 1 {
		replyStaff(c, settingsText(c.Tr, "staff_fwdblock_action_usage"))
		return ext.EndGroups
	}
	replyStaff(c, forwardBlockActionText(c.Tr, applyForwardBlockAction(ctx, c.Chat.Id, c.User.Id, strings.ToLower(args[0]))))
	return ext.EndGroups
}
