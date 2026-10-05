package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/db/user"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// staffCommandSpec is one staff command: the command word and the action it runs
// across the linked groups.
type staffCommandSpec struct {
	Name     string
	Kind     staffActionKind
	Duration staffDurationMode
}

// staffActionCommands lists the commands the StaffActions module intercepts. It
// is a table so the command names reach the dispatcher through a loop variable
// and the docs generator, which reads only literal registrations, does not list
// them a second time.
var staffActionCommands = []staffCommandSpec{
	{Name: "ban", Kind: staffKindBan, Duration: staffDurationOptional},
	{Name: "mute", Kind: staffKindMute, Duration: staffDurationOptional},
	{Name: "kick", Kind: staffKindKick, Duration: staffDurationNone},
	{Name: "unban", Kind: staffKindUnban, Duration: staffDurationNone},
	{Name: "unmute", Kind: staffKindUnmute, Duration: staffDurationNone},
}

// LoadStaffActions registers the Staff Group command interceptors. They are raw
// command handlers on purpose: helpers.WrapCommand would build a command context
// first, and that replies to sender-less updates in every chat. Outside a Staff
// Group each interceptor returns ext.ContinueGroups without a reply, a write or a
// Telegram call, so the per-group commands of Bans and Mutes behave as before.
// Help stays under the Staff module, so SetModuleEnabled is not called.
func LoadStaffActions(dispatcher *ext.Dispatcher) {
	for _, spec := range staffActionCommands {
		dispatcher.AddHandler(handlers.NewCommand(spec.Name, staffActionEntry(spec)))
	}
}

// staffActionEntry is the interceptor of one command.
func staffActionEntry(spec staffCommandSpec) handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if !isStaffChatCached(ctx.EffectiveChat) {
			return ext.ContinueGroups
		}
		return staffModule.handleStaffAction(b, ctx, spec)
	}
}

// isStaffChatCached is the cheap gate: a group or supergroup the cached lookup
// knows as a Staff Group. It can be stale, so handleStaffAction confirms with a
// fresh read before doing anything.
func isStaffChatCached(chat *gotgbot.Chat) bool {
	if chat == nil || (chat.Type != "group" && chat.Type != "supergroup") {
		return false
	}
	return staff.GetStaffGroup(chat.Id) != nil
}

// replyStaffAction replies to the command message in HTML and logs a failure.
func replyStaffAction(b *gotgbot.Bot, msg *gotgbot.Message, text string) {
	if msg == nil || text == "" {
		return
	}
	if _, err := msg.Reply(b, text, formatting.Shtml()); err != nil {
		log.Errorf("[StaffActions] reply: %v", err)
	}
}

// handleStaffAction runs a staff command up to the confirm card. Nothing is
// applied here: the card's Confirm button, pressed by the issuer, does the work.
// Every path inside a Staff Group ends the update with ext.EndGroups, except a
// stale gate, which falls through to the per-group command.
func (m moduleStruct) handleStaffAction(b *gotgbot.Bot, ctx *ext.Context, spec staffCommandSpec) error {
	defer error_handling.RecoverFromPanic("handleStaffAction", "StaffActions")

	msg := ctx.EffectiveMessage
	tr := ctxTr(ctx)
	reply := func(key string) error {
		text, _ := tr.GetString(key)
		replyStaffAction(b, msg, text)
		return ext.EndGroups
	}

	group, err := staff.GetStaffGroupFresh(ctx.EffectiveChat.Id)
	if err != nil {
		return reply("staff_act_abort_check_failed")
	}
	if group == nil {
		return ext.ContinueGroups
	}

	// Before any parsing or Telegram lookup: an identity that cannot be proven
	// gets no card and no anonymous-admin proof keyboard.
	var sender *gotgbot.User
	if ctx.EffectiveSender != nil {
		sender = ctx.EffectiveSender.User
	}
	if helpers.IsAnonymousSender(ctx, sender) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "staff_post_as_yourself", "", chat_status.WithReply())
		return ext.EndGroups
	}

	if cache.GetRedisClient() == nil {
		return reply("staff_act_redis_unavailable")
	}

	req, parsed := parseStaffActionArgs(msg, spec)
	switch parsed {
	case staffParseNoTarget:
		return reply("staff_act_hint_need_target")
	case staffParseBadTarget:
		return reply("staff_act_hint_bad_target")
	}

	links, err := staff.ListLinksByStaffFresh(group.ChatID)
	if err != nil {
		return reply("staff_act_abort_check_failed")
	}
	if len(links) == 0 {
		return reply("staff_act_no_links")
	}

	card := &staffActionCard{
		Issuer:     sender.Id,
		StaffChat:  group.ChatID,
		Kind:       req.Kind,
		Target:     req.Target.UserID,
		TargetName: staffStoredName(req.Target.UserID),
		Reason:     req.Reason,
		GroupCount: len(links),
	}
	if err := saveStaffActionCard(card); err != nil {
		log.Errorf("[StaffActions] save card: %v", err)
		return reply("staff_act_abort_check_failed")
	}
	keyboard, ok := staffActionCardKeyboard(tr, card.Token)
	if !ok {
		deleteStaffActionCard(card.Token)
		return reply("staff_act_abort_check_failed")
	}
	opts := formatting.Shtml()
	opts.ReplyMarkup = keyboard
	if _, err := msg.Reply(b, staffActionCardText(tr, card), opts); err != nil {
		log.Errorf("[StaffActions] send card: %v", err)
		deleteStaffActionCard(card.Token)
	}
	return ext.EndGroups
}

// staffStoredName is the display name the bot has stored for a user: the name,
// else "@username", else empty (never seen).
func staffStoredName(userID int64) string {
	username, name, found := user.GetUserInfoById(userID)
	switch {
	case !found:
		return ""
	case name != "":
		return name
	case username != "":
		return "@" + username
	}
	return ""
}

func init() {
	RegisterLegacyModule("StaffActions", 65, LoadStaffActions)
}
