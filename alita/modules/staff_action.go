package modules

import (
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/db/user"
	"github.com/divkix/Alita_Robot/alita/i18n"
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
	// Refused marks a per-group variant (sban, dmute and the others) that has no
	// meaning in a Staff Group: it gets a hint and never acts.
	Refused bool
}

// staffUsernameMatchLimit caps how many users sharing one username are listed.
const staffUsernameMatchLimit = 10

// Placeholders for user-controlled text while a hint is translated; the real text
// is spliced in afterwards, so it never goes through the translator.
const (
	staffCommandToken   = "<<staff-command>>"
	staffUsernameToken  = "<<staff-username>>"
	staffMatchListToken = "<<staff-match-list>>"
	staffDateToken      = "<<staff-date>>"
)

// staffUserLookup resolves an @username against the users table. It is a package
// variable only so a test can make the lookup fail.
var staffUserLookup = user.FindUsersByUsername

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
	// tban and tmute are the same actions as ban and mute with the duration required.
	{Name: "tban", Kind: staffKindBan, Duration: staffDurationRequired},
	{Name: "tmute", Kind: staffKindMute, Duration: staffDurationRequired},
	// The silent and delete-the-message variants have no meaning in a Staff Group. They
	// are intercepted only to be refused with a hint, so they never act locally and
	// never fan out (D-02).
	{Name: "sban", Refused: true},
	{Name: "dban", Refused: true},
	{Name: "skick", Refused: true},
	{Name: "dkick", Refused: true},
	{Name: "smute", Refused: true},
	{Name: "dmute", Refused: true},
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

	// A variant with no meaning here: a hint, no parsing, no card and no Telegram
	// write. The anonymous check above comes first, so an anonymous /sban also gets
	// "post as yourself".
	if spec.Refused {
		text, _ := tr.GetString("staff_act_hint_variant", i18n.TranslationParams{"command": staffCommandToken})
		replyStaffAction(b, msg, strings.Replace(text, staffCommandToken, spec.Name, 1))
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
	case staffParseBadUsername:
		return reply("staff_act_hint_bad_username")
	case staffParseBareReply:
		return reply("staff_act_hint_no_reply")
	case staffParseNeedDuration:
		return reply("staff_act_hint_need_duration")
	case staffParseBadDuration:
		return reply("staff_act_hint_bad_duration")
	}

	target, targetName, refusal, ok := resolveStaffTarget(tr, req)
	if !ok {
		replyStaffAction(b, msg, refusal)
		return ext.EndGroups
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
		Target:     target.UserID,
		TargetName: targetName,
		Reason:     req.Reason,
		GroupCount: len(links),
		LinksSig:   staffLinksSignature(links),

		DurationSec:    req.DurationSec,
		DurationAmount: req.DurationAmount,
		DurationUnit:   req.DurationUnit,
		OverLimit:      req.OverLimit,
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
	sent, err := msg.Reply(b, staffActionCardText(tr, card), opts)
	if err != nil {
		log.Errorf("[StaffActions] send card: %v", err)
		deleteStaffActionCard(card.Token)
		return ext.EndGroups
	}
	scheduleStaffActionExpiry(b, card.Token, sent.Chat.Id, sent.MessageId)
	return ext.EndGroups
}

// resolveStaffTarget turns the parsed target into a user ID and a display name, or
// into a refusal text to reply with. Nothing is ever guessed (D-04):
//   - a numeric ID is taken as typed and shown with the name the bot has stored;
//   - a text_mention is the entity's user ID with the mention's own name;
//   - an @username is looked up in the users table only, case-insensitively. No
//     match, or more than one, is refused; a lookup that fails is reported as such,
//     never as "never seen". There is no channels-table or live Telegram fallback.
func resolveStaffTarget(tr *i18n.Translator, req staffActionRequest) (target staffTargetRef, name, refusal string, ok bool) {
	target = req.Target
	switch {
	case target.Username != "":
		rows, err := staffUserLookup(target.Username, staffUsernameMatchLimit)
		if err != nil {
			log.Errorf("[StaffActions] username lookup: %v", err)
			text, _ := tr.GetString("staff_act_abort_check_failed")
			return target, "", text, false
		}
		switch len(rows) {
		case 0:
			text, _ := tr.GetString("staff_act_username_unknown", i18n.TranslationParams{"username": staffUsernameToken})
			return target, "", strings.Replace(text, staffUsernameToken, html.EscapeString(target.Username), 1), false
		case 1:
			target.UserID = rows[0].UserId
			return target, staffRowName(rows[0]), "", true
		}
		return target, "", staffAmbiguousUsernameText(tr, target.Username, rows), false
	case target.MentionName != "":
		return target, target.MentionName, "", true
	}
	return target, staffStoredName(target.UserID), "", true
}

// staffAmbiguousUsernameText lists every user that has used the username: one line
// with the escaped name, the numeric ID and when the bot last saw them, newest
// first, then asks for the numeric ID. The list is built here, so stored names never
// go through the translator.
func staffAmbiguousUsernameText(tr *i18n.Translator, username string, rows []models.User) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		line := "• <b>" + staffDisplayTitle(staffRowName(row)) + "</b> <code>" + strconv.FormatInt(row.UserId, 10) + "</code>"
		if seen := staffLastSeen(row); !seen.IsZero() {
			lastSeen, _ := tr.GetString("staff_act_username_last_seen", i18n.TranslationParams{"date": staffDateToken})
			line += " · " + strings.Replace(lastSeen, staffDateToken, seen.UTC().Format("2006-01-02"), 1)
		}
		lines = append(lines, line)
	}
	text, _ := tr.GetString("staff_act_username_ambiguous", i18n.TranslationParams{
		"username": staffUsernameToken,
		"list":     staffMatchListToken,
	})
	text = strings.Replace(text, staffUsernameToken, html.EscapeString(username), 1)
	return strings.Replace(text, staffMatchListToken, strings.Join(lines, "\n"), 1)
}

// staffLastSeen is when the bot last saw a user: the last activity, else the last
// row update, else when the row was created; the zero time when none is known.
func staffLastSeen(row models.User) time.Time {
	for _, seen := range []time.Time{row.LastActivity, row.UpdatedAt, row.CreatedAt} {
		if !seen.IsZero() {
			return seen
		}
	}
	return time.Time{}
}

// staffRowName is the display name of a stored user: the name, else "@username".
func staffRowName(row models.User) string {
	switch {
	case row.Name != "":
		return row.Name
	case row.UserName != "":
		return "@" + row.UserName
	}
	return ""
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
