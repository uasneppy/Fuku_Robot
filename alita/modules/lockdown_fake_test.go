//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/ratelimit"
)

// lockdownTestPrePermissions is a group's default permissions as getChat answers
// them: all 16 keys, in this order, with a mix a typed struct would lose. Reactions
// are false while sending is true, which an omitempty bool cannot tell from "unset".
const lockdownTestPrePermissions = `{"can_send_messages":true,"can_send_audios":true,"can_send_documents":false,` +
	`"can_send_photos":true,"can_send_videos":true,"can_send_video_notes":true,"can_send_voice_notes":false,` +
	`"can_send_polls":false,"can_send_other_messages":true,"can_add_web_page_previews":false,` +
	`"can_react_to_messages":false,"can_edit_tag":false,"can_change_info":false,"can_invite_users":true,` +
	`"can_pin_messages":false,"can_manage_topics":false}`

// lockdownFake is the staff fake plus the three things a lockdown needs from
// Telegram: a raw permissions member on getChat, a setChatPermissions that stores
// what it was sent byte for byte, and a scriptable answer for the bot's own
// getChatMember. Every call is recorded exactly once.
type lockdownFake struct {
	*staffActionFake

	lmu       sync.Mutex
	rawPerms  map[int64]string
	chatTypes map[int64]string
	botJSON   map[int64]string
	// onSetPermissions runs at the moment a setChatPermissions request arrives,
	// before it is answered.
	onSetPermissions func(chatID int64)
}

func newLockdownFake() *lockdownFake {
	return &lockdownFake{
		staffActionFake: newStaffActionFake(),
		rawPerms:        make(map[int64]string),
		chatTypes:       make(map[int64]string),
		botJSON:         make(map[int64]string),
	}
}

// setChatPermsRaw stores the permissions member getChat answers for chatID.
func (f *lockdownFake) setChatPermsRaw(chatID int64, raw string) {
	f.lmu.Lock()
	defer f.lmu.Unlock()
	f.rawPerms[chatID] = raw
}

// clearChatPerms makes getChat answer chatID without a permissions member.
func (f *lockdownFake) clearChatPerms(chatID int64) {
	f.lmu.Lock()
	defer f.lmu.Unlock()
	delete(f.rawPerms, chatID)
}

// chatPermsRaw returns the permissions the fake holds for chatID, exactly as stored.
func (f *lockdownFake) chatPermsRaw(chatID int64) string {
	f.lmu.Lock()
	defer f.lmu.Unlock()
	return f.rawPerms[chatID]
}

// setChatType sets the type getChat reports for chatID.
func (f *lockdownFake) setChatType(chatID int64, typ string) {
	f.lmu.Lock()
	defer f.lmu.Unlock()
	f.chatTypes[chatID] = typ
}

// setBotMember sets the raw getChatMember answer for the bot in chatID.
func (f *lockdownFake) setBotMember(chatID int64, rawJSON string) {
	f.lmu.Lock()
	defer f.lmu.Unlock()
	f.botJSON[chatID] = rawJSON
}

// setOnSetPermissions installs the hook that runs when setChatPermissions arrives.
func (f *lockdownFake) setOnSetPermissions(hook func(chatID int64)) {
	f.lmu.Lock()
	defer f.lmu.Unlock()
	f.onSetPermissions = hook
}

// lockdownBotAdminJSON is a getChatMember answer for the bot as an administrator
// holding exactly the named rights.
func lockdownBotAdminJSON(canRestrict, canDelete, canInvite bool) string {
	return fmt.Sprintf(
		`{"status":"administrator","user":{"id":%d,"is_bot":true,"first_name":"Alita"},"can_be_edited":false,`+
			`"is_anonymous":false,"can_manage_chat":true,"can_delete_messages":%t,"can_manage_video_chats":false,`+
			`"can_restrict_members":%t,"can_promote_members":false,"can_change_info":false,"can_invite_users":%t}`,
		staffTestBotID, canDelete, canRestrict, canInvite)
}

// lockdownParamText is a request parameter as text: raw JSON and strings as they
// are, anything else marshalled.
func lockdownParamText(value any) string {
	switch v := value.(type) {
	case json.RawMessage:
		return string(v)
	case []byte:
		return string(v)
	case string:
		return v
	}
	raw, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("lockdownFake: marshal parameter: %v", err))
	}
	return string(raw)
}

// RequestWithContext implements gotgbot.BotClient. It answers setChatPermissions,
// getChat and the bot's getChatMember itself and hands every other method to the
// staff fake.
func (f *lockdownFake) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	chatID := staffParamInt(params, "chat_id")

	switch method {
	case "setChatPermissions":
		if err := f.popScripted(method, chatID); err != nil {
			f.record(method, params)
			return nil, err
		}
		f.record(method, params)
		f.lmu.Lock()
		hook := f.onSetPermissions
		f.lmu.Unlock()
		if hook != nil {
			hook(chatID)
		}
		f.lmu.Lock()
		f.rawPerms[chatID] = lockdownParamText(params["permissions"])
		f.lmu.Unlock()
		return json.RawMessage(`true`), nil

	case "getChat":
		if err := f.popScripted(method, chatID); err != nil {
			f.record(method, params)
			return nil, err
		}
		f.record(method, params)
		f.lmu.Lock()
		defer f.lmu.Unlock()
		typ := "supergroup"
		if custom, ok := f.chatTypes[chatID]; ok {
			typ = custom
		}
		answer := fmt.Sprintf(`{"id":%d,"type":%q,"title":"Test Chat"`, chatID, typ)
		if raw, ok := f.rawPerms[chatID]; ok {
			answer += `,"permissions":` + raw
		}
		return json.RawMessage(answer + `}`), nil

	case "getChatMember":
		if staffParamInt(params, "user_id") == staffTestBotID {
			f.lmu.Lock()
			raw, ok := f.botJSON[chatID]
			f.lmu.Unlock()
			if ok {
				if err := f.popScripted(method, chatID); err != nil {
					f.record(method, params)
					return nil, err
				}
				f.record(method, params)
				return json.RawMessage(raw), nil
			}
		}
	}
	return f.staffActionFake.RequestWithContext(ctx, token, method, params, opts)
}

// lockdownEnv is one supergroup with an administrator who may restrict members, a
// real dispatcher with the Lockdown module loaded, and miniredis behind it.
type lockdownEnv struct {
	t          *testing.T
	fake       *lockdownFake
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	chat       gotgbot.Chat
	admin      gotgbot.User

	mu         sync.Mutex
	nextUpdate int64
}

// lockdownCleanup deletes the lockdown rows of the given chats when the test ends,
// joiners first.
func lockdownCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		if len(chatIDs) == 0 {
			return
		}
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.LockdownJoiner{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatLockdown{})
	})
}

// seedLockdownJoiner writes one joiner row of a lockdown directly, in the given
// state, with the first name "Joiner<userID>". The join guard that writes these rows
// in production arrives in a later plan; tests seed them to show what a status
// command reads.
func seedLockdownJoiner(t *testing.T, lockdownID uint, chatID, userID int64, state string) models.LockdownJoiner {
	t.Helper()
	joiner := models.LockdownJoiner{
		LockdownID: lockdownID,
		ChatID:     chatID,
		UserID:     userID,
		FirstName:  fmt.Sprintf("Joiner%d", userID),
		JoinPath:   models.JoinPathMember,
		State:      state,
	}
	if err := db.DB.Create(&joiner).Error; err != nil {
		t.Fatalf("seed joiner %d in state %s: %v", userID, state, err)
	}
	return joiner
}

// withFastLockdownPacer installs a pacer with a 1 ms interval and a 5 ms retry_after
// unit as lockdownPacer for the test, so a 429 costs milliseconds and the production
// pacer's 100 ms spacing does not slow the suite.
func withFastLockdownPacer(t *testing.T) {
	t.Helper()
	previous := lockdownPacer
	lockdownPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
		NextKey:        "alita:lockdown:pace:next",
		BlockKey:       "alita:lockdown:pace:block",
		Interval:       time.Millisecond,
		MaxRetries:     3,
		MaxWait:        60 * time.Second,
		RetryAfterUnit: 5 * time.Millisecond,
	})
	t.Cleanup(func() { lockdownPacer = previous })
}

func newLockdownEnv(t *testing.T) *lockdownEnv {
	t.Helper()
	withMiniredis(t)
	withStaffLocale(t)
	withFastLockdownPacer(t)

	fake := newLockdownFake()
	bot := newModuleTestBot(fake.moduleBotClient)
	bot.BotClient = fake

	env := &lockdownEnv{
		t:          t,
		fake:       fake,
		bot:        bot,
		chat:       gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Lock Chat"},
		admin:      gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Ad<b>min"},
		nextUpdate: 100,
	}
	lockdownCleanup(t, env.chat.Id)

	fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{
		Status:             gotgbot.ChatMemberStatusAdministrator,
		CanRestrictMembers: true,
	})
	fake.setChatPermsRaw(env.chat.Id, lockdownTestPrePermissions)

	env.dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadLockdown(env.dispatcher)
	return env
}

func (e *lockdownEnv) updateID() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextUpdate++
	return e.nextUpdate
}

// send runs a message in the lockdown chat as from through the real dispatcher.
func (e *lockdownEnv) send(from gotgbot.User, text string) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId: id,
			Date:      1,
			Chat:      e.chat,
			From:      &from,
			Text:      text,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// replies returns the messages the bot sent to the lockdown chat, in order.
func (e *lockdownEnv) replies() []staffSentMessage {
	return e.fake.sentTo(e.chat.Id)
}

// lastReply is the text of the last message the bot sent to the lockdown chat.
func (e *lockdownEnv) lastReply() string {
	e.t.Helper()
	sent := e.replies()
	if len(sent) == 0 {
		e.t.Fatal("the bot sent nothing to the lockdown chat")
	}
	return fmt.Sprint(sent[len(sent)-1].Params["text"])
}

// calls returns the recorded calls of method addressed to the lockdown chat.
func (e *lockdownEnv) calls(method string) []moduleBotCall {
	return callsToChat(e.fake.staffBotClient, method, e.chat.Id)
}

// loadJoinModules loads the greetings and captcha modules on the env's dispatcher, so
// a test can show they are never reached for a joiner of a locked group.
func (e *lockdownEnv) loadJoinModules() {
	e.t.Helper()
	LoadGreetings(e.dispatcher)
	LoadCaptcha(e.dispatcher)
}

// join runs a chat_member update in the lockdown chat: user went from left to member,
// performed by performer, through inviteLink when it is not empty.
func (e *lockdownEnv) join(user, performer gotgbot.User, inviteLink string) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		ChatMember: &gotgbot.ChatMemberUpdated{
			Chat:          e.chat,
			From:          performer,
			Date:          1,
			OldChatMember: gotgbot.ChatMemberLeft{User: user},
			NewChatMember: gotgbot.ChatMemberMember{User: user},
		},
	}
	if inviteLink != "" {
		update.ChatMember.InviteLink = &gotgbot.ChatInviteLink{InviteLink: inviteLink, Creator: performer}
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// memberUpdate runs a chat_member update in the lockdown chat through the real
// dispatcher: oldMember became newMember, performed by from.
func (e *lockdownEnv) memberUpdate(from gotgbot.User, oldMember, newMember gotgbot.ChatMember, viaJoinRequest bool) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		ChatMember: &gotgbot.ChatMemberUpdated{
			Chat:           e.chat,
			From:           from,
			Date:           1,
			OldChatMember:  oldMember,
			NewChatMember:  newMember,
			ViaJoinRequest: viaJoinRequest,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// serviceJoin runs a new_chat_members service message in the lockdown chat through
// the real dispatcher: from sent it (senderChat is set for an anonymous admin) and
// members are the users it names. It returns the message's ID.
func (e *lockdownEnv) serviceJoin(from gotgbot.User, senderChat *gotgbot.Chat, members ...gotgbot.User) int64 {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId:      id,
			Date:           1,
			Chat:           e.chat,
			From:           &from,
			SenderChat:     senderChat,
			NewChatMembers: members,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// addChat adds a second supergroup to the env: it has its own raw permissions, and
// the env's administrator is a live administrator there who may restrict members. Its
// lockdown rows are deleted when the test ends. Everything the env's helpers do still
// addresses the first chat; tests drive the second one through the per-chat helpers of
// the isolation test.
func (e *lockdownEnv) addChat() gotgbot.Chat {
	e.t.Helper()
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Second Lock Chat"}
	lockdownCleanup(e.t, chat.Id)
	e.fake.setMember(chat.Id, e.admin.Id, staffFakeMember{
		Status:             gotgbot.ChatMemberStatusAdministrator,
		CanRestrictMembers: true,
	})
	e.fake.setChatPermsRaw(chat.Id, lockdownTestPrePermissions)
	return chat
}

// joinRequest runs a chat_join_request update in the lockdown chat through the real
// dispatcher: from asked to join, through inviteLink when it is not empty.
func (e *lockdownEnv) joinRequest(from gotgbot.User, inviteLink string) {
	e.t.Helper()
	id := e.updateID()
	request := &gotgbot.ChatJoinRequest{
		Chat:       e.chat,
		From:       from,
		UserChatId: from.Id,
		Date:       1,
	}
	if inviteLink != "" {
		request.InviteLink = &gotgbot.ChatInviteLink{InviteLink: inviteLink, Creator: e.admin}
	}
	update := &gotgbot.Update{UpdateId: id, ChatJoinRequest: request}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// pressJoinRequest presses a button of a join-request card (action is accept,
// decline or ban) on message msgID of the lockdown chat, through the real dispatcher.
func (e *lockdownEnv) pressJoinRequest(from gotgbot.User, msgID int64, action string, userID int64) {
	e.t.Helper()
	data := encodeCallbackData("join_request", map[string]string{"a": action, "u": fmt.Sprint(userID)})
	if data == "" {
		e.t.Fatalf("callback data for join request action %s did not encode", action)
	}
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("cb-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: e.chat},
			Data:         data,
			ChatInstance: "lockdown-test",
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// lastAnswer returns the text and alert flag of the last answerCallbackQuery.
func (e *lockdownEnv) lastAnswer() (text string, alert bool) {
	e.t.Helper()
	calls := e.fake.callsFor("answerCallbackQuery")
	if len(calls) == 0 {
		e.t.Fatal("no callback query was answered")
	}
	last := calls[len(calls)-1]
	if value, ok := last.Params["text"]; ok {
		text = fmt.Sprint(value)
	}
	return text, staffParamBool(last.Params, "show_alert")
}

// ageJoiner moves the updated_at of a joiner row back by the given time with a
// direct update, so a test can show what happens to a delivery that comes later than
// the dedupe window without sleeping through it.
func ageJoiner(t *testing.T, rowID uint, by time.Duration) {
	t.Helper()
	err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", rowID).
		UpdateColumn("updated_at", time.Now().UTC().Add(-by)).Error
	if err != nil {
		t.Fatalf("age joiner row %d: %v", rowID, err)
	}
}

// cycle runs one worker cycle and reports whether it made progress.
func (e *lockdownEnv) cycle() bool {
	e.t.Helper()
	return runLockdownCycle(context.Background(), e.bot)
}

// newJoiner returns a user with a fresh ID and the given first name.
func (e *lockdownEnv) newJoiner(firstName string) gotgbot.User {
	return gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: firstName}
}

// wantReplyHas fails unless the last reply contains every want.
func (e *lockdownEnv) wantReplyHas(wants ...string) {
	e.t.Helper()
	reply := e.lastReply()
	for _, want := range wants {
		if !strings.Contains(reply, want) {
			e.t.Errorf("last reply %q does not contain %q", reply, want)
		}
	}
}
