package modules

import (
	"fmt"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/actionlog"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/extraction"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// moderationCtx holds the decomposed context for a moderation command.
// It provides a minimal surface that every moderation handler needs.
type moderationCtx struct {
	Bot    *gotgbot.Bot
	Chat   *gotgbot.Chat
	Msg    *gotgbot.Message
	User   *gotgbot.User
	Ctx    *ext.Context
	Tr     *i18n.Translator
	Module *moduleStruct
}

// moderationReasonLine renders the reason line of a punishment reply: the locale template key
// formatted with the HTML-escaped reason, or "" for an empty reason or a missing template. The
// template comes from the locale file only; the reason is never part of a format string, so a '%'
// in it is printed as typed and a '<' cannot make Telegram reject the reply after the punishment
// already happened. The default reply builders (each an eventDefaultReply) call it.
func moderationReasonLine(tr *i18n.Translator, key, reason string) string {
	if reason == "" {
		return ""
	}
	temp, _ := tr.GetString(key)
	if temp == "" {
		return ""
	}
	return fmt.Sprintf(temp, formatting.HtmlEscape(reason))
}

// replySuccess sends the success reply of a punishment command as a reply to the command and, when
// it was sent, queues it for deletion under the manual punishment kind (D-13). Validation and error
// replies, lists and question cards never go through it. The error is exactly what Msg.Reply returned.
func (c *moderationCtx) replySuccess(text string, opts *gotgbot.SendMessageOpts) error {
	_, err := c.sendSuccess(text, opts)
	return err
}

// sendSuccess is replySuccess returning the message it sent: the same Msg.Reply and the same queueing
// under the manual punishment kind. The message is nil when the send failed.
func (c *moderationCtx) sendSuccess(text string, opts *gotgbot.SendMessageOpts) (*gotgbot.Message, error) {
	sent, err := c.Msg.Reply(c.Bot, text, opts)
	if err != nil {
		return nil, err
	}
	scheduleBotMessage(sent, deletion.KindBotManual)
	return sent, nil
}

// replyHint sends a usage hint (a "reply to a message" answer) as a reply to the command. Unlike
// replySuccess it never queues the message for deletion: validation and error replies are not
// covered by the manual punishment kind (D-13, IN-01). The error is exactly what Msg.Reply returned.
func (c *moderationCtx) replyHint(text string, opts *gotgbot.SendMessageOpts) error {
	_, err := c.Msg.Reply(c.Bot, text, opts)
	return err
}

// gateFn is a permission gate. Returns true if the gate passes.
// On failure, it should already have sent any required error messages.
type gateFn func(c *moderationCtx) bool

// standardModGates are the most common permission checks for moderation commands.
// RequireGroup -> RequireUserAdmin -> RequireBotAdmin -> CanUserRestrict -> CanBotRestrict
func standardModGates(c *moderationCtx) bool {
	if !chat_status.RequireGroup(c.Bot, c.Ctx, nil) {
		chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, "chat_status_group_only_error", "", chat_status.WithReply())
		return false
	}
	if !chat_status.RequireUserAdmin(c.Bot, c.Ctx, nil, c.User.Id) {
		chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return false
	}
	if !chat_status.RequireBotAdmin(c.Bot, c.Ctx, nil) {
		chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, "chat_status_bot_not_admin", "", chat_status.WithReply())
		return false
	}
	if !chat_status.CanUserRestrict(c.Bot, c.Ctx, nil, c.User.Id) {
		chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, "chat_status_restrict_cmd_error", "chat_status_restrict_button_error")
		return false
	}
	if !chat_status.CanBotRestrict(c.Bot, c.Ctx, nil) {
		chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, "chat_status_bot_restrict_group_error", "chat_status_bot_restrict_error")
		return false
	}
	return true
}

// deleteModGates extends standardModGates with delete permissions.
// Used by purge-like commands.
func deleteModGates(c *moderationCtx) bool {
	if !standardModGates(c) {
		return false
	}
	if !chat_status.CanBotDelete(c.Bot, c.Ctx, nil) {
		chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, "chat_status_bot_delete_error", "", chat_status.WithReply())
		return false
	}
	if !chat_status.CanUserDelete(c.Bot, c.Ctx, c.Chat, c.User.Id) {
		chat_status.NewPermissionResponder(c.Bot).Respond(c.Ctx, "chat_status_delete_cmd_error", "chat_status_delete_button_error", chat_status.WithReply())
		return false
	}
	return true
}

// target holds the resolved target user and optional reason text.
type target struct {
	userID    int64
	reason    string
	timeVal   string // used by time-based commands (e.g., tban)
	isChannel bool   // true if original target was a channel ID
	untilDate int64  // the Telegram until_date a time-based command computed; 0 for a permanent one
}

// eventUntil is the end of a timed punishment as the event message's {until} value: the Telegram
// until_date as a time, or the zero time for a permanent one, which renders {until} empty (D-09).
func eventUntil(untilDate int64) time.Time {
	if untilDate <= 0 {
		return time.Time{}
	}
	return time.Unix(untilDate, 0)
}

// extractFromArgs resolves the target from command arguments using ExtractUserAndText.
// Returns a sentinel userID of -1 when extraction itself signals an error reply
// has already been sent by the extractor (e.g. bad mention).
func extractFromArgs(c *moderationCtx) (target, error) {
	uid, reason := extraction.ExtractUserAndText(c.Bot, c.Ctx)
	if uid == -1 {
		// extraction already sent an error message
		return target{}, fmt.Errorf("extraction failed")
	}
	if chat_status.IsChannelId(uid) {
		anonKey := "bans_anonymous_ban_only_error"
		text, _ := c.Tr.GetString(anonKey)
		if text == "" {
			text, _ = c.Tr.GetString("common_anonymous_user_error")
		}
		_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return target{}, err
		}
		return target{}, fmt.Errorf("anonymous user")
	}
	if uid == 0 {
		noUserKey := "common_no_user_specified"
		text, _ := c.Tr.GetString(noUserKey)
		_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return target{}, err
		}
		return target{}, fmt.Errorf("no user")
	}
	return target{userID: uid, reason: reason}, nil
}

// extractFromReply resolves the target from the replied-to message.
func extractFromReply(c *moderationCtx) (target, error) {
	reply := extraction.ReplyTarget(c.Msg)
	if reply == nil {
		text, _ := c.Tr.GetString("bans_dkick_no_reply")
		if text == "" {
			text, _ = c.Tr.GetString("common_no_reply_to_message")
		}
		_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return target{}, err
		}
		return target{}, fmt.Errorf("no reply")
	}
	if reply.From == nil {
		text, _ := c.Tr.GetString("bans_cannot_identify_user")
		_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return target{}, err
		}
		return target{}, fmt.Errorf("nil from")
	}
	uid := reply.From.Id
	if chat_status.IsChannelId(uid) {
		anonKey := "bans_anonymous_ban_only_error"
		text, _ := c.Tr.GetString(anonKey)
		if text == "" {
			text, _ = c.Tr.GetString("common_anonymous_user_error")
		}
		_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return target{}, err
		}
		return target{}, fmt.Errorf("anonymous user")
	}
	if uid == 0 {
		noUserKey := "common_no_user_specified"
		text, _ := c.Tr.GetString(noUserKey)
		_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return target{}, err
		}
		return target{}, fmt.Errorf("no user")
	}
	return target{userID: uid}, nil
}

// validateTargetFn checks the resolved target before executing the action.
// The function receives the moderationCtx and resolved target; on failure
// it sends the i18n error message and returns a non-nil error.
type validateTargetFn func(c *moderationCtx, t *target) error

// actionFn performs the moderation API call.
// It receives the moderationCtx and the validated target.
type actionFn func(c *moderationCtx, t *target) error

// replyFn builds and sends the success reply for a moderation command.
// It receives the moderationCtx, the validated target, and the API action result.
type replyFn func(c *moderationCtx, t *target) error

// moderationCommand wires the fixed-order pipeline for a moderation handler:
//
//	RequireUser -> gates -> extractTarget -> validate -> execute -> reply.
//
// It eliminates the 30+ lines of boilerplate that every moderation command repeats.
type moderationCommand struct {
	gates     []gateFn
	extract   func(*moderationCtx) (target, error)
	validate  validateTargetFn
	execute   actionFn
	reply     replyFn
	module    *moduleStruct
	logAction string
	logUser   bool
}

// buildModerationCtx creates the common decomposed context from a gotgbot update.
func buildModerationCtx(m *moduleStruct, b *gotgbot.Bot, ctx *ext.Context) (*moderationCtx, error) {
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		chat_status.NewPermissionResponder(b).Respond(ctx, "common_cannot_identify_user", "", chat_status.WithReply())
		return nil, ext.EndGroups //nolint:goerr113 // EndGroups is not a real error
	}
	return &moderationCtx{
		Bot:    b,
		Chat:   ctx.EffectiveChat,
		Msg:    ctx.EffectiveMessage,
		User:   user,
		Ctx:    ctx,
		Tr:     i18n.MustNewTranslator(lang.GetLanguage(ctx)),
		Module: m,
	}, nil
}

// run executes the moderation pipeline.
func (cmd *moderationCommand) run(b *gotgbot.Bot, ctx *ext.Context) error {
	mc, err := buildModerationCtx(cmd.module, b, ctx)
	if err != nil {
		return ext.EndGroups
	}

	// Run all permission gates in order.
	for _, g := range cmd.gates {
		if !g(mc) {
			return ext.EndGroups
		}
	}

	// Extract the target user (+ optional reason).
	tgt, err := cmd.extract(mc)
	if err != nil {
		return ext.EndGroups
	}

	// Validate the resolved target.
	if cmd.validate != nil {
		if err := cmd.validate(mc, &tgt); err != nil {
			return ext.EndGroups
		}
	}

	// Execute the moderation action.
	if err := cmd.execute(mc, &tgt); err != nil {
		log.Error(err)
		return err
	}

	// Build and send the success reply.
	if cmd.reply != nil {
		if err := cmd.reply(mc, &tgt); err != nil {
			log.Error(err)
			return err
		}
	}

	if cmd.logAction != "" {
		if cmd.logUser {
			actionlog.User(mc.Bot, mc.Chat, mc.User, cmd.logAction)
		} else {
			actionlog.Admin(mc.Bot, mc.Chat, mc.User, cmd.logAction, tgt.userID, tgt.reason)
		}
	}
	return ext.EndGroups
}

// Sentinel errors used by moderation command templates.
var (
	errUserNotInChat = fmt.Errorf("target user is not in chat")
	errAdminTarget   = fmt.Errorf("target is a protected admin")
	errTargetIsBot   = fmt.Errorf("target is the bot itself")
)

// errReplyTargetMismatch is returned when a delete-and-punish command names an
// explicit target that did not send the message it replies to.
var errReplyTargetMismatch = fmt.Errorf("explicit target is not the reply's sender")

// replyTargetMismatch reports whether msg replies (extraction.ReplyTarget) to a
// message whose sender is not userID. A message with no reply target never
// mismatches; a reply whose sender cannot be read always does.
func replyTargetMismatch(msg *gotgbot.Message, userID int64) bool {
	reply := extraction.ReplyTarget(msg)
	if reply == nil {
		return false
	}
	sender := reply.GetSender()
	return sender == nil || sender.Id() != userID
}

// refuseReplyMismatch answers common_reply_target_mismatch: /dban, /dmute and
// /dwarn delete the replied message, so they act only when the explicit target
// wrote it, and otherwise delete and punish nothing.
func refuseReplyMismatch(c *moderationCtx) error {
	text, _ := c.Tr.GetString("common_reply_target_mismatch")
	if err := replyValidationError(c, text); err != nil {
		return err
	}
	return errReplyTargetMismatch
}

// liveTargetState is what a target's live member record says before a button
// kicks, mutes or unmutes them.
type liveTargetState int

const (
	// liveTargetInChat: a member, an administrator, the creator, or a restricted
	// member who is still a member.
	liveTargetInChat liveTargetState = iota
	// liveTargetGone: kicked, left, or a restricted non-member.
	liveTargetGone
	// liveTargetUnknown: the record could not be read.
	liveTargetUnknown
)

// memberRecordInChat reports whether a live member record shows someone still in
// the chat. Kicked and left are not, and neither is a restricted non-member.
func memberRecordInChat(member gotgbot.ChatMember) bool {
	if member == nil {
		return false
	}
	merged := member.MergeChatMember()
	switch merged.Status {
	case "member", "administrator", "creator":
		return true
	case "restricted":
		return merged.IsMember
	default:
		return false
	}
}

// readLiveTarget reads the target's live member record. A member-removing
// unbanChatMember on a kicked user lifts the ban and a restrictChatMember
// replaces it, so the Kick, Mute and Unmute buttons and /zombies act only on
// someone this answers liveTargetInChat for.
func readLiveTarget(b *gotgbot.Bot, chatID, userID int64) liveTargetState {
	member, err := b.GetChatMember(chatID, userID, nil)
	if err != nil {
		log.Warnf("[Moderation] live member read of user %d in chat %d failed: %v", userID, chatID, err)
		return liveTargetUnknown
	}
	if memberRecordInChat(member) {
		return liveTargetInChat
	}
	return liveTargetGone
}

// answerLiveTargetRefusal answers a button press whose target is not in the
// chat (common_user_not_in_chat) or could not be checked
// (common_member_check_failed); nothing was changed.
func answerLiveTargetRefusal(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator, state liveTargetState) {
	key := "common_user_not_in_chat"
	if state == liveTargetUnknown {
		key = "common_member_check_failed"
	}
	text, _ := tr.GetString(key)
	if _, err := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: text, ShowAlert: true}); err != nil {
		log.Error(err)
	}
}

// replyValidationError sends a validation failure reply, logging send errors.
func replyValidationError(c *moderationCtx, text string) error {
	_, err := c.Msg.Reply(c.Bot, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}

// validateTarget runs the shared target checks: optional in-chat membership,
// admin-protection, and bot-self. Each key falls back to the shared common_*
// string when the module-specific key is empty. Returns the sentinel for the
// first failing check, nil when the target is actionable.
func validateTarget(c *moderationCtx, userID int64, checkInChat bool, notInChatKey, adminKey, selfKey string) error {
	if checkInChat && !chat_status.IsUserInChat(c.Bot, c.Chat, userID) {
		text, _ := c.Tr.GetString(notInChatKey)
		if text == "" {
			text, _ = c.Tr.GetString("common_user_not_in_chat")
		}
		if err := replyValidationError(c, text); err != nil {
			return err
		}
		return errUserNotInChat
	}
	if chat_status.IsUserBanProtected(c.Bot, c.Ctx, nil, userID) {
		text, _ := c.Tr.GetString(adminKey)
		if text == "" {
			text, _ = c.Tr.GetString("common_cannot_target_admin")
		}
		if err := replyValidationError(c, text); err != nil {
			return err
		}
		return errAdminTarget
	}
	if userID == c.Bot.Id {
		text, _ := c.Tr.GetString(selfKey)
		if text == "" {
			text, _ = c.Tr.GetString("common_cannot_target_self")
		}
		if err := replyValidationError(c, text); err != nil {
			return err
		}
		return errTargetIsBot
	}
	return nil
}

// extractTemporalTarget parses the duration from the target reason via
// ExtractTime, storing the normalized timeVal/reason back on the target.
// Returns the Telegram until_date, or -1 when extraction already replied.
func extractTemporalTarget(c *moderationCtx, t *target) int64 {
	_time, timeVal, reason := extraction.ExtractTime(c.Bot, c.Ctx, t.reason)
	if _time == -1 {
		return -1
	}
	t.timeVal = timeVal
	t.reason = reason
	t.untilDate = _time
	return _time
}

// pipelineHandler adapts a legacy (bot,ctx) moderation handler to the command
// pipeline. WrapCommand already ran desc.RequiredChecks; the domain handler
// keeps its legacy gates as a redundant safety net during migration.
func pipelineHandler(domain func(*gotgbot.Bot, *ext.Context) error) func(*helpers.CommandContext) error {
	return func(c *helpers.CommandContext) error {
		return domain(c.Bot, c.Ctx)
	}
}

// anonPipelineHandler is the post-proof anonymous-admin entry point for migrated
// commands. It re-runs the same pipeline checks via helpers.RunChecks, making
// migrated commands anon-safe by default without per-module re-enforcement.
func anonPipelineHandler(desc helpers.CommandDescriptor, domain func(*gotgbot.Bot, *ext.Context) error) AnonymousAdminHandler {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		c, err := helpers.BuildCommandContext(b, ctx)
		if err != nil {
			return ext.EndGroups
		}
		if !helpers.RunChecks(c, desc.RequiredChecks) {
			return ext.EndGroups
		}
		return domain(b, ctx)
	}
}
