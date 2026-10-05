//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// staffFakeMember is the stored state of one member of one fake chat.
type staffFakeMember struct {
	Status             string
	IsMember           bool
	CanSendMessages    bool
	CanRestrictMembers bool
	UntilDate          int64
}

// staffSentMessage is one sendMessage the fake answered.
type staffSentMessage struct {
	ChatID    int64
	MessageID int64
	Params    map[string]any
}

// staffActionFake is a stateful gotgbot.BotClient. Unlike staffBotClient, whose
// getChatMember answers are static, it applies banChatMember, restrictChatMember
// and unbanChatMember to its member records with the Bot API server's semantics:
// restrictChatMember REPLACES the stored status (so it would silently lift a ban),
// and unbanChatMember with only_if_banned=false removes a current member. That is
// what lets tests prove a staff action never lifts a ban by accident.
type staffActionFake struct {
	*staffBotClient

	fmu       sync.Mutex
	records   map[[2]int64]*staffFakeMember
	scripted  map[string][]error
	chatPerms map[int64]gotgbot.ChatPermissions
	nextMsgID int64
	sentLog   []staffSentMessage
}

func newStaffActionFake() *staffActionFake {
	return &staffActionFake{
		staffBotClient: newStaffBotClient(),
		records:        make(map[[2]int64]*staffFakeMember),
		scripted:       make(map[string][]error),
		chatPerms:      make(map[int64]gotgbot.ChatPermissions),
		nextMsgID:      5000,
	}
}

// setMember stores the member record of userID in chatID.
func (f *staffActionFake) setMember(chatID, userID int64, m staffFakeMember) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	f.records[[2]int64{chatID, userID}] = &m
}

// member returns a copy of the record of userID in chatID, or nil.
func (f *staffActionFake) member(chatID, userID int64) *staffFakeMember {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	if m, ok := f.records[[2]int64{chatID, userID}]; ok {
		copied := *m
		return &copied
	}
	return nil
}

// script queues errors for "method" calls to chatID, popped first-in first-out.
// A nil entry lets that one call through.
func (f *staffActionFake) script(method string, chatID int64, errs ...error) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	key := fmt.Sprintf("%s:%d", method, chatID)
	f.scripted[key] = append(f.scripted[key], errs...)
}

// staffFake429 is Telegram's flood-control answer.
func staffFake429(retryAfter int64) error {
	return &gotgbot.TelegramError{
		Code:           429,
		Description:    fmt.Sprintf("Too Many Requests: retry after %d", retryAfter),
		ResponseParams: &gotgbot.ResponseParameters{RetryAfter: retryAfter},
	}
}

func staffFakeError(code int, description string) error {
	return &gotgbot.TelegramError{Code: code, Description: description}
}

func (f *staffActionFake) popScripted(method string, chatID int64) error {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	key := fmt.Sprintf("%s:%d", method, chatID)
	queue := f.scripted[key]
	if len(queue) == 0 {
		return nil
	}
	f.scripted[key] = queue[1:]
	return queue[0]
}

// sentTo returns the sendMessage calls the fake answered for chatID, in order.
func (f *staffActionFake) sentTo(chatID int64) []staffSentMessage {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	var matched []staffSentMessage
	for _, sent := range f.sentLog {
		if sent.ChatID == chatID {
			matched = append(matched, sent)
		}
	}
	return matched
}

func staffParamInt(params map[string]any, key string) int64 {
	var n int64
	if value, ok := params[key]; ok {
		_, _ = fmt.Sscan(fmt.Sprint(value), &n)
	}
	return n
}

func staffParamBool(params map[string]any, key string) bool {
	value, ok := params[key].(bool)
	return ok && value
}

func staffIsAdminStatus(status string) bool {
	return status == gotgbot.ChatMemberStatusCreator || status == gotgbot.ChatMemberStatusAdministrator
}

func staffFakeMemberJSON(userID int64, m *staffFakeMember) json.RawMessage {
	user := fmt.Sprintf(`{"id":%d,"is_bot":false,"first_name":"User%d"}`, userID, userID)
	switch m.Status {
	case gotgbot.ChatMemberStatusCreator:
		return json.RawMessage(fmt.Sprintf(`{"status":"creator","user":%s,"is_anonymous":false}`, user))
	case gotgbot.ChatMemberStatusAdministrator:
		return json.RawMessage(fmt.Sprintf(
			`{"status":"administrator","user":%s,"can_be_edited":false,"is_anonymous":false,"can_manage_chat":true,`+
				`"can_delete_messages":true,"can_manage_video_chats":false,"can_restrict_members":%t,`+
				`"can_promote_members":false,"can_change_info":false,"can_invite_users":true}`,
			user, m.CanRestrictMembers))
	case gotgbot.ChatMemberStatusRestricted:
		return json.RawMessage(fmt.Sprintf(
			`{"status":"restricted","user":%s,"is_member":%t,"can_send_messages":%t,"until_date":%d}`,
			user, m.IsMember, m.CanSendMessages, m.UntilDate))
	case gotgbot.ChatMemberStatusKicked:
		return json.RawMessage(fmt.Sprintf(`{"status":"kicked","user":%s,"until_date":%d}`, user, m.UntilDate))
	default:
		return json.RawMessage(fmt.Sprintf(`{"status":%q,"user":%s}`, m.Status, user))
	}
}

func staffFakeChatJSON(chatID int64) string {
	return fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Test Chat"}`, chatID)
}

// RequestWithContext implements gotgbot.BotClient. Scripted errors win. The
// methods that change or read a member are answered from the member records; any
// other method goes to the embedded staffBotClient, which records it. Every call
// is recorded exactly once.
func (f *staffActionFake) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	chatID := staffParamInt(params, "chat_id")
	if err := f.popScripted(method, chatID); err != nil {
		f.record(method, params)
		return nil, err
	}

	userID := staffParamInt(params, "user_id")
	key := [2]int64{chatID, userID}

	switch method {
	case "getChatMember":
		if userID == staffTestBotID {
			break
		}
		if rec := f.member(chatID, userID); rec != nil {
			f.record(method, params)
			return staffFakeMemberJSON(userID, rec), nil
		}
	case "banChatMember":
		f.record(method, params)
		f.fmu.Lock()
		defer f.fmu.Unlock()
		if cur := f.records[key]; cur != nil && staffIsAdminStatus(cur.Status) {
			return nil, staffFakeError(400, "Bad Request: user is an administrator of the chat")
		}
		f.records[key] = &staffFakeMember{
			Status:    gotgbot.ChatMemberStatusKicked,
			UntilDate: staffParamInt(params, "until_date"),
		}
		return json.RawMessage(`true`), nil
	case "restrictChatMember":
		f.record(method, params)
		f.fmu.Lock()
		defer f.fmu.Unlock()
		cur := f.records[key]
		if cur != nil && staffIsAdminStatus(cur.Status) {
			return nil, staffFakeError(400, "Bad Request: user is an administrator of the chat")
		}
		wasMember := cur == nil ||
			cur.Status == gotgbot.ChatMemberStatusMember ||
			(cur.Status == gotgbot.ChatMemberStatusRestricted && cur.IsMember)
		var perms gotgbot.ChatPermissions
		if p, ok := params["permissions"].(gotgbot.ChatPermissions); ok {
			perms = p
		}
		f.records[key] = &staffFakeMember{
			Status:          gotgbot.ChatMemberStatusRestricted,
			IsMember:        wasMember,
			CanSendMessages: perms.CanSendMessages,
			UntilDate:       staffParamInt(params, "until_date"),
		}
		return json.RawMessage(`true`), nil
	case "unbanChatMember":
		f.record(method, params)
		f.fmu.Lock()
		defer f.fmu.Unlock()
		cur := f.records[key]
		if staffParamBool(params, "only_if_banned") {
			if cur != nil && cur.Status == gotgbot.ChatMemberStatusKicked {
				f.records[key] = &staffFakeMember{Status: gotgbot.ChatMemberStatusLeft}
			}
			return json.RawMessage(`true`), nil
		}
		if cur != nil && staffIsAdminStatus(cur.Status) {
			return json.RawMessage(`true`), nil
		}
		f.records[key] = &staffFakeMember{Status: gotgbot.ChatMemberStatusLeft}
		return json.RawMessage(`true`), nil
	case "getChat":
		f.record(method, params)
		f.fmu.Lock()
		defer f.fmu.Unlock()
		out := map[string]any{"id": chatID, "type": "supergroup", "title": "Test Chat"}
		if perms, ok := f.chatPerms[chatID]; ok {
			out["permissions"] = perms
		}
		raw, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		return raw, nil
	case "sendMessage":
		f.record(method, params)
		f.fmu.Lock()
		defer f.fmu.Unlock()
		f.nextMsgID++
		copied := make(map[string]any, len(params))
		for k, v := range params {
			copied[k] = v
		}
		f.sentLog = append(f.sentLog, staffSentMessage{ChatID: chatID, MessageID: f.nextMsgID, Params: copied})
		return json.RawMessage(fmt.Sprintf(`{"message_id":%d,"date":1,"chat":%s}`, f.nextMsgID, staffFakeChatJSON(chatID))), nil
	case "editMessageText":
		f.record(method, params)
		return json.RawMessage(fmt.Sprintf(
			`{"message_id":%d,"date":1,"chat":%s,"text":"edited"}`,
			staffParamInt(params, "message_id"), staffFakeChatJSON(chatID))), nil
	}
	return f.staffBotClient.RequestWithContext(ctx, token, method, params, opts)
}

// staffKeyboardOf reads the inline keyboard out of a recorded reply_markup
// parameter, which is a struct value, not JSON text.
func staffKeyboardOf(value any) [][]gotgbot.InlineKeyboardButton {
	switch markup := value.(type) {
	case gotgbot.InlineKeyboardMarkup:
		return markup.InlineKeyboard
	case *gotgbot.InlineKeyboardMarkup:
		if markup != nil {
			return markup.InlineKeyboard
		}
	}
	return nil
}

// staffActionEnv is a Staff Group with linked groups, a creator who made them
// all, an issuer who is an administrator with the restrict right in every linked
// group, a real dispatcher with the StaffActions and Staff modules loaded, and
// miniredis behind the card.
type staffActionEnv struct {
	t          *testing.T
	fake       *staffActionFake
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	staffChat  int64
	groups     []int64
	owner      int64
	issuer     gotgbot.User

	mu         sync.Mutex
	nextUpdate int64
}

func newStaffActionEnv(t *testing.T, groupCount int) *staffActionEnv {
	t.Helper()
	withMiniredis(t)
	withStaffLocale(t)

	fake := newStaffActionFake()
	bot := newModuleTestBot(fake.moduleBotClient)
	bot.BotClient = fake

	env := &staffActionEnv{
		t:          t,
		fake:       fake,
		bot:        bot,
		staffChat:  uniqueModuleChatID(),
		owner:      uniqueLinkOwnerID(),
		issuer:     gotgbot.User{Id: uniqueLinkOwnerID() + 7, FirstName: "Iss<i>uer"},
		nextUpdate: 100,
	}
	ids := []int64{env.staffChat}
	for i := 0; i < groupCount; i++ {
		env.groups = append(env.groups, uniqueModuleChatID())
	}
	ids = append(ids, env.groups...)
	staffCleanup(t, ids...)

	for _, id := range ids {
		fake.setCreator(id, env.owner)
	}
	if _, err := staff.CreateStaffGroup(env.staffChat, env.owner, "Staff HQ"); err != nil {
		t.Fatalf("create Staff Group: %v", err)
	}
	for i, group := range env.groups {
		link := &models.StaffGroupLink{
			GroupChatID: group,
			StaffChatID: env.staffChat,
			OwnerUserID: env.owner,
			GroupTitle:  fmt.Sprintf("Group %c", 'A'+i),
		}
		if err := staff.CreateLink(link); err != nil {
			t.Fatalf("create link: %v", err)
		}
		fake.setMember(group, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
	}

	env.dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadStaffActions(env.dispatcher)
	LoadStaff(env.dispatcher)
	return env
}

func (e *staffActionEnv) staffChatObj() gotgbot.Chat {
	return gotgbot.Chat{Id: e.staffChat, Type: "supergroup", Title: "Staff HQ"}
}

func (e *staffActionEnv) updateID() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextUpdate++
	return e.nextUpdate
}

// process runs one update through the real dispatcher.
func (e *staffActionEnv) process(update *gotgbot.Update) {
	e.t.Helper()
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", update.UpdateId, err)
	}
}

// messageUpdate builds a plain text message update in chat from from.
func (e *staffActionEnv) messageUpdate(chat gotgbot.Chat, from *gotgbot.User, text string) *gotgbot.Update {
	id := e.updateID()
	return &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId: id,
			Date:      1,
			Chat:      chat,
			From:      from,
			Text:      text,
		},
	}
}

// send runs a message in the Staff Group as from.
func (e *staffActionEnv) send(from gotgbot.User, text string) {
	e.t.Helper()
	e.process(e.messageUpdate(e.staffChatObj(), &from, text))
}

// card returns the token and message ID of the last confirm card sent to the
// Staff Group.
func (e *staffActionEnv) card() (token string, msgID int64) {
	e.t.Helper()
	sent := e.fake.sentTo(e.staffChat)
	for i := len(sent) - 1; i >= 0; i-- {
		for _, row := range staffKeyboardOf(sent[i].Params["reply_markup"]) {
			for _, button := range row {
				decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace)
				if ok && decoded.Fields["a"] == staffActRunConfirm {
					return decoded.Fields["t"], sent[i].MessageID
				}
			}
		}
	}
	e.t.Fatalf("no confirm card was sent to the Staff Group")
	return "", 0
}

// tapData runs a callback query with raw data on message msgID of chat.
func (e *staffActionEnv) tapData(from gotgbot.User, chat gotgbot.Chat, msgID int64, data string) {
	e.t.Helper()
	id := e.updateID()
	e.process(&gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("cb-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: chat},
			Data:         data,
			ChatInstance: "staff-action-test",
		},
	})
}

// tap presses a card button in the Staff Group.
func (e *staffActionEnv) tap(from gotgbot.User, action, token string, msgID int64) {
	e.t.Helper()
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": action, "t": token})
	if data == "" {
		e.t.Fatalf("callback data for %s did not encode", action)
	}
	e.tapData(from, e.staffChatObj(), msgID, data)
}

// waitRuns waits for every fan-out started so far.
func (e *staffActionEnv) waitRuns() {
	e.t.Helper()
	done := make(chan struct{})
	go func() {
		staffActionRunsWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		e.t.Fatal("staff action runs did not finish within 10s")
	}
}

// callsTo returns the recorded calls of method addressed to chatID.
func (e *staffActionEnv) callsTo(method string, chatID int64) []moduleBotCall {
	return callsToChat(e.fake.staffBotClient, method, chatID)
}

// writes returns the ban, restrict and unban calls addressed to chatID.
func (e *staffActionEnv) writes(chatID int64) []moduleBotCall {
	var all []moduleBotCall
	for _, method := range []string{"banChatMember", "restrictChatMember", "unbanChatMember"} {
		all = append(all, e.callsTo(method, chatID)...)
	}
	return all
}

// memberLookups counts getChatMember calls about userID in chatID.
func (e *staffActionEnv) memberLookups(chatID, userID int64) int {
	count := 0
	for _, call := range e.callsTo("getChatMember", chatID) {
		if staffParamInt(call.Params, "user_id") == userID {
			count++
		}
	}
	return count
}

// edits returns the editMessageText calls on message msgID of chatID, in order.
func (e *staffActionEnv) edits(chatID, msgID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range e.callsTo("editMessageText", chatID) {
		if staffParamInt(call.Params, "message_id") == msgID {
			matched = append(matched, call)
		}
	}
	return matched
}

// lastEditText is the text of the last edit of message msgID of chatID.
func (e *staffActionEnv) lastEditText(chatID, msgID int64) string {
	e.t.Helper()
	edits := e.edits(chatID, msgID)
	if len(edits) == 0 {
		e.t.Fatalf("message %d of chat %d was never edited", msgID, chatID)
	}
	return fmt.Sprint(edits[len(edits)-1].Params["text"])
}

// wantNoKeyboard fails when a recorded call still carries inline buttons.
func wantNoKeyboard(t *testing.T, call moduleBotCall) {
	t.Helper()
	if keyboard := staffKeyboardOf(call.Params["reply_markup"]); len(keyboard) != 0 {
		t.Fatalf("%s kept the keyboard %+v, want none", call.Method, keyboard)
	}
}

// lastAnswer returns the text and alert flag of the last answerCallbackQuery.
func (e *staffActionEnv) lastAnswer() (text string, alert bool) {
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

// answerCount counts answerCallbackQuery calls.
func (e *staffActionEnv) answerCount() int {
	return len(e.fake.callsFor("answerCallbackQuery"))
}
