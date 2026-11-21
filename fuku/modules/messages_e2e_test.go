//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/db/timezone"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Custom event messages end to end (plan 21-11; MSG-01 to MSG-09, MSG-11, EVT-01). Everything is real but
// Telegram: the owner works the private Messages page through the deep link and the buttons the bot drew,
// every part is a typed private message through the dispatcher, and an administrator's /ban, /tmute, /kick
// and their silent variants run through the real dispatcher with the Bans and Mutes modules loaded. Only the
// Bot API is the hand-written fake, with one wrapper (messagesE2EClient) that answers a member's getChat as
// a private chat and a media send as a message of the chat it was sent to, so the event placeholders have a
// name to fill and the deletion queue can be read.

// messagesE2EManualDelay is the Bot messages timer of the manual punishment kind every subtest stores.
const messagesE2EManualDelay = 600

// messagesE2EBanText is what the owner types as the ban text: "Out" in bold, then every placeholder the
// ban event fills, {until} last (a permanent ban has no end, so it renders as nothing).
const messagesE2EBanText = "Out {first} ({reason}, by {admin}){until}"

// messagesE2EBanStored is the stored form of messagesE2EBanText with its bold entity.
const messagesE2EBanStored = "<b>Out</b> {first} ({reason}, by {admin}){until}"

// messagesE2EClient wraps a bot's Bot API fake. A getChat of a member the test registered answers that
// member as a private chat (the base fake answers every getChat as a group), and a media send answers a
// message of the chat it was sent to, dated now, with a fresh message ID (the base fake answers every
// sendPhoto with one fixed message of another chat). Every request still reaches the fake, which records it,
// except a send to a chat the test refused (refuse), which answers the refusal and reaches nothing.
type messagesE2EClient struct {
	gotgbot.BotClient

	mu      sync.Mutex
	users   map[int64]gotgbot.User
	nextID  int64
	media   map[int64][]int64
	refused map[int64]error
}

// newMessagesE2EClient wraps inner.
func newMessagesE2EClient(inner gotgbot.BotClient) *messagesE2EClient {
	return &messagesE2EClient{
		BotClient: inner, users: map[int64]gotgbot.User{}, nextID: 7000, media: map[int64][]int64{}, refused: map[int64]error{},
	}
}

// RequestWithContext implements gotgbot.BotClient.
func (c *messagesE2EClient) RequestWithContext(
	ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	chatID := staffParamInt(params, "chat_id")
	c.mu.Lock()
	refusal := c.refused[chatID]
	c.mu.Unlock()
	if refusal != nil && strings.HasPrefix(method, "send") {
		return nil, refusal
	}
	raw, err := c.BotClient.RequestWithContext(ctx, token, method, params, opts)
	if err != nil {
		return raw, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch method {
	case "getChat":
		user, ok := c.users[chatID]
		if !ok {
			return raw, nil
		}
		return json.Marshal(map[string]any{
			"id": user.Id, "type": "private", "first_name": user.FirstName, "last_name": user.LastName, "username": user.Username,
		})
	case "sendPhoto", "sendVideo", "sendAnimation", "sendDocument", "sendSticker":
		c.nextID++
		c.media[chatID] = append(c.media[chatID], c.nextID)
		chatType := "supergroup"
		if chatID > 0 {
			chatType = "private"
		}
		return json.Marshal(map[string]any{
			"message_id": c.nextID, "date": time.Now().Unix(),
			"chat": map[string]any{"id": chatID, "type": chatType, "title": "Test Chat"},
		})
	}
	return raw, nil
}

// lastMedia is the message ID of the newest media message answered for chatID, or 0.
func (c *messagesE2EClient) lastMedia(chatID int64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := c.media[chatID]
	if len(ids) == 0 {
		return 0
	}
	return ids[len(ids)-1]
}

// refuse makes every send to chatID answer err (a 403 for a member who blocked the bot) with no request reaching the
// fake.
func (c *messagesE2EClient) refuse(chatID int64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refused[chatID] = err
}

// register makes getChat of user answer user as a private chat.
func (c *messagesE2EClient) register(user gotgbot.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users[user.Id] = user
}

// messagesE2E is one supergroup with the settings environment (the Settings, Staff and BotUpdates modules on
// the real dispatcher, miniredis), the Bans and Mutes modules, the bot an administrator who may restrict, a
// live creator who works the private menu, an administrator who runs the commands and a 600 s Bot messages
// timer for the manual punishment kind.
type messagesE2E struct {
	*settingsMsgEnv
	owner  gotgbot.User
	client *messagesE2EClient
}

// newMessagesE2E builds one environment for one subtest; real installs the real locale files instead of the
// marker locale, for a subtest that compares a reply with the goldens recorded in Ukrainian.
func newMessagesE2E(t *testing.T, real bool) *messagesE2E {
	t.Helper()
	setCachedBotUsername(t, "FukuTestBot")
	env := &settingsMsgEnv{settingsEnv: newSettingsEnvWith(t, settingsEnvOptions{RealLocale: real})}
	settingsMsgCleanup(t, env.chat.Id)
	staffCleanup(t, env.chat.Id)
	deletionCleanup(t, env.chat.Id)
	timezoneCleanup(t, env.chat.Id)

	env.fake.setBotRole(env.chat.Id, staffRoleAdmin)
	env.fake.sendDate = func() int64 { return time.Now().Unix() }
	LoadBans(env.dispatcher)
	LoadMutes(env.dispatcher)
	client := newMessagesE2EClient(env.bot.BotClient)
	env.bot.BotClient = client

	owner := env.newOwner(env.chat)
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{env.admin.Id, owner.Id})
	if err := deletion.SetRule(context.Background(), env.chat.Id, deletion.KindBotManual, messagesE2EManualDelay); err != nil {
		t.Fatalf("SetRule error = %v", err)
	}
	return &messagesE2E{settingsMsgEnv: env, owner: owner, client: client}
}

// member makes a plain member of the group with a fresh ID, whose getChat answers their name.
func (e *messagesE2E) member(first string) gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: first}
	e.client.register(user)
	e.fake.setProfile(user.Id, first, "", "")
	memberReqCleanup(e.t, nil, []int64{user.Id})
	return user
}

// messagesE2EMenu is one private settings message of the group and the keyboard it shows now.
type messagesE2EMenu struct {
	e        *messagesE2E
	user     gotgbot.User
	msgID    int64
	text     string
	keyboard [][]gotgbot.InlineKeyboardButton
}

// lastPrivate is the newest message the bot sent to user's private chat, as a menu.
func (e *messagesE2E) lastPrivate(user gotgbot.User) *messagesE2EMenu {
	e.t.Helper()
	sent := e.fake.sentTo(user.Id)
	if len(sent) == 0 {
		e.t.Fatalf("the bot sent nothing to user %d", user.Id)
	}
	last := sent[len(sent)-1]
	return &messagesE2EMenu{
		e: e, user: user, msgID: last.MessageID,
		text: fmt.Sprint(last.Params["text"]), keyboard: staffKeyboardOf(last.Params["reply_markup"]),
	}
}

// openMain opens the group's settings in user's private chat through the deep link and returns its Main page.
func (e *messagesE2E) openMain(user gotgbot.User) *messagesE2EMenu {
	e.t.Helper()
	e.sendPrivate(user, fmt.Sprintf("/start settings_%d", e.chat.Id))
	return e.lastPrivate(user)
}

// openEvent walks Main, Messages, Admin actions and the event's cell, each a press of the drawn button.
func (e *messagesE2E) openEvent(user gotgbot.User, code string) *messagesE2EMenu {
	e.t.Helper()
	m := e.openMain(user)
	m.tap("a", settingsActOpen, "p", settingsPageMessages)
	m.tap("a", settingsActMsgFamily, "t", "m")
	m.tap("a", settingsActMsgEvent, settingsMsgEventField, code)
	return m
}

// has reports whether the current keyboard has a button with every field pair.
func (m *messagesE2EMenu) has(kv ...string) bool {
	m.e.t.Helper()
	_, ok := settingsMsgButtonWith(m.e.t, m.keyboard, kv...)
	return ok
}

// tap presses the drawn button whose fields carry every pair and keeps the message as the press left it.
func (m *messagesE2EMenu) tap(kv ...string) {
	m.e.t.Helper()
	button, ok := settingsMsgButtonWith(m.e.t, m.keyboard, kv...)
	if !ok {
		m.e.t.Fatalf("the screen has no button %v:\n%s\n%+v", kv, m.text, m.keyboard)
	}
	edits := len(m.e.edits(m.user.Id, m.msgID))
	m.e.pressRaw(settingsPrivateChat(m.user), m.user, m.msgID, button.CallbackData)
	if len(m.e.edits(m.user.Id, m.msgID)) > edits {
		m.text, m.keyboard = m.e.lastEdit(m.user.Id, m.msgID)
	}
}

// setPart opens the event's screen, presses Set of part and sends msg as the typed reply; the saved page
// arrives as a new private message.
func (e *messagesE2E) setPart(user gotgbot.User, code, part string, msg gotgbot.Message) {
	e.t.Helper()
	m := e.openEvent(user, code)
	m.tap("a", settingsActMsgSet, "t", part)
	if !e.redis.Exists(settingsInputKey(user.Id)) {
		e.t.Fatalf("Set of %s/%s left no wait", code, part)
	}
	sendPrivateMessage(e.settingsEnv, user, msg)
	if e.redis.Exists(settingsInputKey(user.Id)) {
		e.t.Fatalf("the typed %s of %s did not finish the wait", part, code)
	}
}

// setText sets the event's text from a typed message whose first word is bold.
func (e *messagesE2E) setText(code, text string) {
	e.t.Helper()
	first := strings.IndexByte(text, ' ')
	if first < 0 {
		first = len(text)
	}
	e.setPart(e.owner, code, settingsEditPartText, gotgbot.Message{
		Text: text, Entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 0, Length: int64(first)}},
	})
}

// setBanMessage sets the ban text, a photo and a Rules button from the page, as the owner.
func (e *messagesE2E) setBanMessage() {
	e.t.Helper()
	e.setText("bn", messagesE2EBanText)
	e.setPart(e.owner, "bn", settingsEditPartMedia, settingsMediaItemID("photo", "ban-photo"))
	e.setPart(e.owner, "bn", settingsEditPartButtons, gotgbot.Message{Text: "[Rules](buttonurl://https://example.com/rules)"})
	row, ok := settingsMsgStored(e.t, e.chat.Id, eventmsg.EventBan)
	if !ok || row.Text != messagesE2EBanStored || row.FileID != "ban-photo" || row.MediaType != db.PHOTO ||
		len(row.Buttons) != 1 || row.Buttons[0].Url != "https://example.com/rules" {
		e.t.Fatalf("the stored ban message is %+v, want the typed text, the photo and the Rules button", row)
	}
}

// replyTarget is a message of from in the group for a command to reply to.
func (e *messagesE2E) replyTarget(from gotgbot.User) *gotgbot.Message {
	return &gotgbot.Message{MessageId: e.updateID(), Date: 1, Chat: e.chat, From: &from, Text: "something bad"}
}

// command runs text as a bot command of from in the group, replying to the member's message, through the
// real dispatcher on bot's dispatcher (the base one, or a replica's), and returns the command's message ID.
func (e *messagesE2E) command(env *settingsEnv, from gotgbot.User, text string, to gotgbot.User) int64 {
	e.t.Helper()
	length := len(text)
	if space := strings.IndexByte(text, ' '); space >= 0 {
		length = space
	}
	id := env.updateID()
	msg := &gotgbot.Message{
		MessageId: id, Date: 1, Chat: e.chat, From: &from, Text: text,
		Entities:       []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: int64(length)}},
		ReplyToMessage: e.replyTarget(to),
	}
	if err := env.dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: id, Message: msg}, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// groupSends is every message send of env's bot to the group since mark (a count per method).
func (e *messagesE2E) groupSends(env *settingsEnv, mark map[string]int) []moduleBotCall {
	return settingsEditNewSends(env, e.chat.Id, mark, false)
}

// queued is the queue row of message msgID of the group, if any.
func (e *messagesE2E) queued(msgID int64) (models.ScheduledDeletion, bool) {
	e.t.Helper()
	var rows []models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ? AND message_id = ?", e.chat.Id, msgID).Find(&rows).Error; err != nil {
		e.t.Fatalf("read the queue: %v", err)
	}
	if len(rows) == 0 {
		return models.ScheduledDeletion{}, false
	}
	return rows[0], true
}

// wantQueued fails unless message msgID of the group is queued under the manual kind at its date plus the timer.
func (e *messagesE2E) wantQueued(msgID int64, what string) {
	e.t.Helper()
	row, ok := e.queued(msgID)
	if !ok {
		e.t.Fatalf("%s (message %d) is not queued for deletion", what, msgID)
	}
	if row.Kind != deletion.KindBotManual || row.DeleteAt.Sub(row.SentAt) != messagesE2EManualDelay*time.Second {
		e.t.Errorf("%s is queued as %s at sent %s + %s, want %s at + %ds", what, row.Kind, row.SentAt, row.DeleteAt.Sub(row.SentAt),
			deletion.KindBotManual, messagesE2EManualDelay)
	}
}

// punished counts the calls of method about user in the group on env's bot.
func (e *messagesE2E) punished(env *settingsEnv, method string, user gotgbot.User) int {
	n := 0
	for _, call := range callsToChat(env.fake.staffBotClient, method, e.chat.Id) {
		if staffParamInt(call.Params, "user_id") == user.Id {
			n++
		}
	}
	return n
}

// methodsSince is the Bot API methods env's fake was asked for after the first skip, in order.
func methodsSince(env *settingsEnv, skip int) []string {
	methods := eventMsgMethods(env.fake.moduleBotClient)
	if skip > len(methods) {
		return nil
	}
	return methods[skip:]
}

// messagesE2EGolden is a moderation golden for the reply of command msgID to member: the goldens punish
// member 42 and reply to message 101, so both are put in. The chat stays <chat>.
func messagesE2EGolden(golden string, member gotgbot.User, msgID int64) string {
	out := strings.ReplaceAll(golden, "?id=42", "?id="+strconv.FormatInt(member.Id, 10))
	out = strings.ReplaceAll(out, "u=42", "u="+strconv.FormatInt(member.Id, 10))
	return strings.ReplaceAll(out, `"message_id":101`, `"message_id":`+strconv.FormatInt(msgID, 10))
}

// wantGolden fails unless call is the golden request for the reply of command msgID to member.
func (e *messagesE2E) wantGolden(call moduleBotCall, golden string, member gotgbot.User, msgID int64) {
	e.t.Helper()
	got := strings.ReplaceAll(legacyRequestJSON(e.t, call), strconv.FormatInt(e.chat.Id, 10), "<chat>")
	if want := messagesE2EGolden(golden, member, msgID); call.Method != "sendMessage" || got != want {
		e.t.Errorf("the reply is %s\n got %q\nwant %q", call.Method, got, want)
	}
}

func TestMessagesEndToEnd(t *testing.T) {
	t.Run("owner sets a ban message", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		member := e.member("Ann & <Bo>")
		e.setBanMessage()

		// The hub's ban line reads Custom, every other event Default.
		m := e.openMain(e.owner)
		m.tap("a", settingsActOpen, "p", settingsPageMessages)
		settingsMsgWantStates(t, m.text, map[string]string{eventmsg.EventBan: "settings_msg_state_custom"})

		mark, calls := settingsEditMarkSends(e.settingsEnv, e.chat.Id), len(eventMsgMethods(e.fake.moduleBotClient))
		id := e.command(e.settingsEnv, e.admin, "/ban spam & co", member)
		methods := methodsSince(e.settingsEnv, calls)
		if ban, photo := eventMsgIndex(methods, "banChatMember"), eventMsgIndex(methods, "sendPhoto"); ban < 0 || photo < ban {
			t.Fatalf("methods = %v, want banChatMember before the custom sendPhoto", methods)
		}
		sends := e.groupSends(e.settingsEnv, mark)
		if len(sends) != 1 || sends[0].Method != "sendPhoto" {
			t.Fatalf("the group got %+v, want one sendPhoto", sends)
		}
		params := rulesCallParams(t, sends[0])
		const caption = "<b>Out</b> Ann &amp; &lt;Bo&gt; (spam &amp; co, by Ad&lt;b&gt;min)"
		if params["caption"] != caption || params["photo"] != "ban-photo" || params["parse_mode"] != "HTML" {
			t.Errorf("sendPhoto caption %q photo %v parse mode %v, want %q, ban-photo and HTML", params["caption"], params["photo"], params["parse_mode"], caption)
		}
		if reply, _ := params["reply_parameters"].(map[string]any); reply["message_id"] != float64(id) {
			t.Errorf("reply_parameters = %v, want the command %d", params["reply_parameters"], id)
		}
		rows := settingsMsgSentKeyboard(t, params)
		if len(rows) != 2 || len(rows[0]) != 1 || len(rows[1]) != 1 ||
			rows[0][0].CallbackData != encodeCallbackData("unrestrict", map[string]string{"a": "unban", "u": fmt.Sprint(member.Id)}) ||
			rows[1][0].Text != "Rules" || rows[1][0].Url != "https://example.com/rules" {
			t.Errorf("the keyboard = %+v, want the Unban row, then the Rules button", rows)
		}
		e.wantQueued(e.client.lastMedia(e.chat.Id), "the custom ban message")
	})

	t.Run("previews", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		e.setBanMessage()
		group := settingsEditMarkSends(e.settingsEnv, e.chat.Id)

		m := e.openEvent(e.owner, "bn")
		mark := settingsEditMarkSends(e.settingsEnv, e.owner.Id)
		m.tap("a", settingsActMsgFull, settingsMsgEventField, "bn")
		seen := settingsEditNewSends(e.settingsEnv, e.owner.Id, mark, true)
		if len(seen) != 1 || seen[0].Method != "sendPhoto" {
			t.Fatalf("Full preview sent %+v to the owner, want the header and one sendPhoto", seen)
		}
		params := rulesCallParams(t, seen[0])
		want := "<b>Out</b> Owner (" + staffMarker("settings_msg_sample_reason") + ", by Owner)"
		if params["caption"] != want || params["photo"] != "ban-photo" {
			t.Errorf("the preview caption %q photo %v, want %q and ban-photo", params["caption"], params["photo"], want)
		}
		if rows := settingsMsgSentKeyboard(t, params); len(rows) != 1 || rows[0][0].Url != "https://example.com/rules" {
			t.Errorf("the preview keyboard = %+v, want the Rules button alone, with no Unban row", rows)
		}
		if sends := e.groupSends(e.settingsEnv, group); len(sends) != 0 {
			t.Errorf("Full preview sent %d messages to the group, want none", len(sends))
		}
	})

	t.Run("timed", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		member := e.member("Tim")
		if err := timezone.Set(context.Background(), e.chat.Id, "Asia/Tokyo", e.owner.Id); err != nil {
			t.Fatalf("timezone.Set error = %v", err)
		}
		e.setText("tm", "Quiet {first} for {duration} until {until}")

		mark := settingsEditMarkSends(e.settingsEnv, e.chat.Id)
		e.command(e.settingsEnv, e.admin, "/tmute 30m", member)
		restricts := callsToChat(e.fake.staffBotClient, "restrictChatMember", e.chat.Id)
		if len(restricts) != 1 {
			t.Fatalf("restrictChatMember calls = %d, want 1", len(restricts))
		}
		tokyo, err := time.LoadLocation("Asia/Tokyo")
		if err != nil {
			t.Fatalf("load Asia/Tokyo: %v", err)
		}
		until := time.Unix(staffParamInt(restricts[0].Params, "until_date"), 0)
		want := "<b>Quiet</b> Tim for 30 minutes until " + until.In(tokyo).Format("02.01.2006 15:04") + " (UTC+9)"
		sends := e.groupSends(e.settingsEnv, mark)
		if len(sends) != 1 || sends[0].Method != "sendMessage" || fmt.Sprint(sends[0].Params["text"]) != want {
			t.Fatalf("the group got %+v, want one message %q", sends, want)
		}
		sent := e.fake.sentTo(e.chat.Id)
		e.wantQueued(sent[len(sent)-1].MessageID, "the custom tmute message")
	})

	t.Run("refused", func(t *testing.T) {
		e := newMessagesE2E(t, true)
		member := e.member("Ann & <Bo>")
		e.setBanMessage()
		e.fake.script("sendPhoto", e.chat.Id, staffFakeError(400, "Bad Request: wrong file identifier/HTTP URL specified"))

		mark, calls := settingsEditMarkSends(e.settingsEnv, e.chat.Id), len(eventMsgMethods(e.fake.moduleBotClient))
		id := e.command(e.settingsEnv, e.admin, "/ban spam <b>", member)
		methods := methodsSince(e.settingsEnv, calls)
		ban, photo, text := eventMsgIndex(methods, "banChatMember"), eventMsgIndex(methods, "sendPhoto"), eventMsgIndex(methods, "sendMessage")
		if ban < 0 || photo < ban || text < photo {
			t.Fatalf("methods = %v, want the ban, then the refused photo, then the default", methods)
		}
		sends := e.groupSends(e.settingsEnv, mark)
		if len(sends) != 2 {
			t.Fatalf("the group got %+v, want the refused photo and the default", sends)
		}
		for _, call := range sends {
			if call.Method == "sendMessage" {
				e.wantGolden(call, goldenModerationBanReason, member, id)
			}
		}
		sent := e.fake.sentTo(e.chat.Id)
		e.wantQueued(sent[len(sent)-1].MessageID, "the default ban reply after the refusal")
	})

	t.Run("silent", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		kicked, sbanned, skicked := e.member("Kim"), e.member("Sab"), e.member("Ska")

		m := e.openEvent(e.owner, "kk")
		m.tap("a", settingsActMsgSilent, settingsMsgEventField, "kk", "v", "1")
		if row, ok := settingsMsgStored(t, e.chat.Id, eventmsg.EventKick); !ok || !row.Silent || row.UpdatedBy != e.owner.Id {
			t.Fatalf("after the Silent press the kick row is %+v, want Silent on, written by the owner", row)
		}
		if !strings.Contains(m.text, staffMarker("settings_msg_line_silent")) || !m.has("a", settingsActMsgSilent, "v", "0") {
			t.Errorf("the kick screen after the press does not offer Silent off:\n%s", m.text)
		}

		mark := settingsEditMarkSends(e.settingsEnv, e.chat.Id)
		e.command(e.settingsEnv, e.admin, "/kick spam", kicked)
		if got := e.punished(e.settingsEnv, "unbanChatMember", kicked); got != 1 {
			t.Errorf("/kick removed the member %d times, want once", got)
		}
		e.command(e.settingsEnv, e.admin, "/sban", sbanned)
		e.command(e.settingsEnv, e.admin, "/skick", skicked)
		if e.punished(e.settingsEnv, "banChatMember", sbanned) != 1 || e.punished(e.settingsEnv, "unbanChatMember", skicked) != 1 {
			t.Errorf("/sban or /skick did not act")
		}
		if sends := e.groupSends(e.settingsEnv, mark); len(sends) != 0 {
			t.Errorf("a silent kick, /sban and /skick posted %+v, want nothing", sends)
		}
	})

	t.Run("remove", func(t *testing.T) {
		e := newMessagesE2E(t, true)
		member := e.member("Ann & <Bo>")
		e.setBanMessage()

		m := e.openEvent(e.owner, "bn")
		for _, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
			m.tap("a", settingsActMsgRemove, "t", part)
		}
		if row, _ := settingsMsgStored(t, e.chat.Id, eventmsg.EventBan); row.Text != "" || row.FileID != "" || row.MediaType != 0 || len(row.Buttons) != 0 {
			t.Fatalf("after the three Removes the ban row is %+v, want every part empty", row)
		}
		if m.has("a", settingsActMsgRemove) {
			t.Errorf("the ban screen still offers a Remove after every part was removed")
		}

		mark := settingsEditMarkSends(e.settingsEnv, e.chat.Id)
		id := e.command(e.settingsEnv, e.admin, "/ban spam <b>", member)
		sends := e.groupSends(e.settingsEnv, mark)
		if len(sends) != 1 {
			t.Fatalf("the group got %+v, want the one default reply", sends)
		}
		e.wantGolden(sends[0], goldenModerationBanReason, member, id)
		sent := e.fake.sentTo(e.chat.Id)
		e.wantQueued(sent[len(sent)-1].MessageID, "the default ban reply")
	})

	t.Run("anonymous", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		member := e.member("Ann")
		e.fake.setMember(e.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		e.setText("bn", messagesE2EBanText)

		e.sendAnonymousMessage("/ban spam", e.replyTarget(member))
		if got := e.punished(e.settingsEnv, "banChatMember", member); got != 0 {
			t.Fatalf("the anonymous /ban banned before the proof (%d calls)", got)
		}
		e.tapProof(e.owner)
		if got := e.punished(e.settingsEnv, "banChatMember", member); got != 1 {
			t.Fatalf("the proved /ban banned %d times, want once", got)
		}
		want := "<b>Out</b> Ann (spam, by " + staffMarker("eventmsg_admin_anon") + ")"
		found := 0
		for _, sent := range e.fake.sentTo(e.chat.Id) {
			text := fmt.Sprint(sent.Params["text"])
			if text == want {
				found++
			}
			if strings.Contains(text, "Owner") {
				t.Errorf("a group message names the tapper: %q", text)
			}
		}
		if found != 1 {
			t.Errorf("the custom ban text %q was posted %d times, want once", want, found)
		}
	})

	t.Run("rights", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		listEditAdmin(e.settingsEnv, e.chat, e.admin, false)
		settingsMsgSeed(t, e.chat.Id, eventmsg.EventBan, "Seeded", false)
		before, _ := settingsMsgStored(t, e.chat.Id, eventmsg.EventBan)

		// Opening and previewing need only the admin check and the owner gate.
		m := e.openEvent(e.admin, "bn")
		answers := e.answerCount()
		mark := settingsEditMarkSends(e.settingsEnv, e.admin.Id)
		m.tap("a", settingsActMsgFull, settingsMsgEventField, "bn")
		if answer, alert := e.lastAnswer(); e.answerCount() != answers+1 || alert || answer != "" {
			t.Errorf("Full preview by an administrator without can_change_info answered %q (alert %v)", answer, alert)
		}
		if seen := settingsEditNewSends(e.settingsEnv, e.admin.Id, mark, true); len(seen) != 1 {
			t.Errorf("Full preview sent %d messages after the header, want 1", len(seen))
		}

		// Every change is refused with the alert and stores nothing.
		for _, kv := range [][]string{
			{"a", settingsActMsgSet, "t", settingsEditPartText},
			{"a", settingsActMsgRemove, "t", settingsEditPartText},
			{"a", settingsActMsgSilent, "v", "1"},
		} {
			edits := len(e.edits(e.admin.Id, m.msgID))
			m.tap(kv...)
			if answer, alert := e.lastAnswer(); !alert || !containsMarker(answer, "settings_no_change_info") {
				t.Errorf("%v answered %q (alert %v), want the alert settings_no_change_info", kv, answer, alert)
			}
			if got := len(e.edits(e.admin.Id, m.msgID)); got != edits {
				t.Errorf("the refused %v edited the screen", kv)
			}
		}
		if e.redis.Exists(settingsInputKey(e.admin.Id)) {
			t.Errorf("a refused Set left a wait")
		}

		// A wait whose sender is demoted before the typed message stores nothing.
		listEditAdmin(e.settingsEnv, e.chat, e.admin, true)
		m = e.openEvent(e.admin, "bn")
		m.tap("a", settingsActMsgSet, "t", settingsEditPartText)
		listEditAdmin(e.settingsEnv, e.chat, e.admin, false)
		sent := settingsEditSentCount(e.settingsEnv, e.admin)
		sendPrivateMessage(e.settingsEnv, e.admin, gotgbot.Message{Text: "Typed after the demotion"})
		settingsEditWantReply(t, e.settingsEnv, e.admin, sent, "settings_no_change_info")
		settingsEditWaitGone(t, e.settingsEnv, e.admin)
		if after, _ := settingsMsgStored(t, e.chat.Id, eventmsg.EventBan); after.Text != before.Text || after.Silent || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Errorf("the ban row changed to %+v, want it as seeded", after)
		}
	})

	t.Run("group menu", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		settingsMsgSeed(t, e.chat.Id, eventmsg.EventBan, "zzsentinelmessage", false)
		msgID := e.openMenu(e.admin)
		text, keyboard := e.lastEdit(e.chat.Id, msgID)
		if line, ok := settingsMainStateLine(text, "settings_page_msgs"); !ok || !containsMarker(line, "settings_state_private_only") {
			t.Errorf("the group Main's Messages line = %q, want In private", line)
		}
		button, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActOpen, "p", settingsPageMessages)
		if !ok {
			t.Fatalf("the group Main has no Messages cell: %+v", keyboard)
		}
		e.pressRaw(e.chat, e.admin, msgID, button.CallbackData)
		text, keyboard = e.lastEdit(e.chat.Id, msgID)
		if !containsMarker(text, "settings_msg_group_note") || containsMarker(text, "settings_msg_intro") || strings.Contains(text, "zzsentinel") {
			t.Errorf("the group Messages cell drew %q, want the group note alone", text)
		}
		if urls := settingsURLButtons(keyboard); len(urls) != 1 || urls[0] != settingsPrivateURL("FukuTestBot", e.chat.Id) {
			t.Errorf("the group note's URL buttons = %v, want the Open in private link", urls)
		}
	})

	t.Run("staff group", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		if _, err := staff.CreateStaffGroup(e.chat.Id, e.owner.Id, "OpsCrewZeta"); err != nil {
			t.Fatalf("CreateStaffGroup error = %v", err)
		}
		m := e.openMain(e.owner)
		if m.has("a", settingsActOpen, "p", settingsPageMessages) || containsMarker(m.text, "settings_page_msgs") {
			t.Errorf("a Staff Group's Main shows the Messages page:\n%s", m.text)
		}
		// A forged press of the page (open, an event, a Silent switch) is the invalid request and writes nothing.
		for _, kv := range [][]string{
			{"a", settingsActOpen, "p", settingsPageMessages},
			{"a", settingsActMsgEvent, settingsMsgEventField, "bn"},
			{"a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1"},
		} {
			e.pressPrivate(e.owner, m.msgID, e.privateFields(e.chat.Id, kv...))
			if answer, alert := e.lastAnswer(); !alert || !containsMarker(answer, "common_callback_invalid_request") {
				t.Errorf("a forged %v in a Staff Group answered %q (alert %v), want the invalid request", kv, answer, alert)
			}
		}
		if n := settingsMsgRows(t, e.chat.Id); n != 0 {
			t.Errorf("the forged presses wrote %d event message rows", n)
		}
	})

	t.Run("no redis", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		first, second := e.member("Una"), e.member("Duo")
		e.setText("bn", messagesE2EBanText)

		restore := cache.DisableRedisForTest()
		defer restore()

		// Set needs the wait store: the alert, and nothing edited.
		m := e.openEvent(e.owner, "bn")
		edits := len(e.edits(e.owner.Id, m.msgID))
		m.tap("a", settingsActMsgSet, "t", settingsEditPartMedia)
		if answer, alert := e.lastAnswer(); !alert || !containsMarker(answer, "settings_edit_unavailable") {
			t.Errorf("Set without Redis answered %q (alert %v), want the alert settings_edit_unavailable", answer, alert)
		}
		if got := len(e.edits(e.owner.Id, m.msgID)); got != edits {
			t.Errorf("Set without Redis edited the screen %d times", got-edits)
		}

		// The send path reads the database: the custom text is posted.
		mark := settingsEditMarkSends(e.settingsEnv, e.chat.Id)
		e.command(e.settingsEnv, e.admin, "/ban spam", first)
		want := "<b>Out</b> Una (spam, by Ad&lt;b&gt;min)"
		if sends := e.groupSends(e.settingsEnv, mark); len(sends) != 1 || fmt.Sprint(sends[0].Params["text"]) != want {
			t.Fatalf("the /ban without Redis posted %+v, want %q", sends, want)
		}

		// Silent works without Redis and the next /ban posts nothing.
		m.tap("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1")
		mark = settingsEditMarkSends(e.settingsEnv, e.chat.Id)
		e.command(e.settingsEnv, e.admin, "/ban spam", second)
		if e.punished(e.settingsEnv, "banChatMember", second) != 1 {
			t.Errorf("the silent /ban did not ban")
		}
		if sends := e.groupSends(e.settingsEnv, mark); len(sends) != 0 {
			t.Errorf("the silent /ban without Redis posted %+v", sends)
		}
	})

	t.Run("two replicas", func(t *testing.T) {
		e := newMessagesE2E(t, false)
		first, second := e.member("Una"), e.member("Duo")
		e.setText("bn", messagesE2EBanText)

		replica := e.otherReplica()
		LoadBans(replica.dispatcher)
		replica.fake.setBotRole(e.chat.Id, staffRoleAdmin)
		replica.fake.sendDate = func() int64 { return time.Now().Unix() }
		replicaClient := newMessagesE2EClient(replica.bot.BotClient)
		replica.bot.BotClient = replicaClient
		for _, user := range []gotgbot.User{first, second} {
			replicaClient.register(user)
		}

		// Replica B posts the custom text and holds it in the shared cache.
		mark := settingsEditMarkSends(replica, e.chat.Id)
		e.command(replica, e.admin, "/ban spam", first)
		want := "<b>Out</b> Una (spam, by Ad&lt;b&gt;min)"
		if sends := e.groupSends(replica, mark); len(sends) != 1 || fmt.Sprint(sends[0].Params["text"]) != want {
			t.Fatalf("replica B posted %+v, want %q", sends, want)
		}

		// The owner turns Silent on through replica A; B's next /ban posts nothing.
		m := e.openEvent(e.owner, "bn")
		m.tap("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1")
		mark = settingsEditMarkSends(replica, e.chat.Id)
		e.command(replica, e.admin, "/ban spam", second)
		if e.punished(replica, "banChatMember", second) != 1 {
			t.Errorf("replica B did not ban after the Silent press")
		}
		if sends := e.groupSends(replica, mark); len(sends) != 0 {
			t.Errorf("replica B posted %+v after the Silent press on A, want nothing", sends)
		}
	})
}
