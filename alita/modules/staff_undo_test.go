//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// The undo callback codes, spelled out so these tests stay written against the
// wire format and not against the constants under test.
const (
	undoAskCode     = "ya"
	undoConfirmCode = "yc"
	undoCancelCode  = "yn"
)

// undoPresser returns a Staff Group member named name who is an administrator with
// the restrict right in every linked group, so an undo they press is allowed
// everywhere.
func (e *staffActionEnv) undoPresser(name string) gotgbot.User {
	e.t.Helper()
	user := gotgbot.User{Id: e.issuer.Id + 1, FirstName: name}
	for _, group := range e.groups {
		e.fake.setMember(group, user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
	}
	return user
}

// lastEditKeyboard is the inline keyboard of the last edit of message msgID of the
// Staff Group.
func (e *staffActionEnv) lastEditKeyboard(msgID int64) [][]gotgbot.InlineKeyboardButton {
	e.t.Helper()
	edits := e.edits(e.staffChat, msgID)
	if len(edits) == 0 {
		e.t.Fatalf("message %d was never edited", msgID)
	}
	return staffKeyboardOf(edits[len(edits)-1].Params["reply_markup"])
}

// undoButtonData returns the callback data of the Undo button on the last edit of
// message msgID. It fails the test unless that edit carries exactly one button,
// labelled as the Undo button, naming action actionID.
func undoButtonData(t *testing.T, env *staffActionEnv, msgID int64, actionID uint) string {
	t.Helper()
	keyboard := env.lastEditKeyboard(msgID)
	var buttons []gotgbot.InlineKeyboardButton
	for _, row := range keyboard {
		buttons = append(buttons, row...)
	}
	if len(buttons) != 1 {
		t.Fatalf("the final summary has %d buttons, want exactly one Undo button: %+v", len(buttons), keyboard)
	}
	if buttons[0].Text != staffMarker("staff_undo_button") {
		t.Fatalf("button label = %q, want the Undo label %q", buttons[0].Text, staffMarker("staff_undo_button"))
	}
	decoded, ok := decodeCallbackData(buttons[0].CallbackData, staffCallbackNamespace)
	if !ok {
		t.Fatalf("the Undo button data %q does not decode", buttons[0].CallbackData)
	}
	if decoded.Fields["a"] != undoAskCode || decoded.Fields["r"] != fmt.Sprint(actionID) || len(decoded.Fields) != 2 {
		t.Fatalf("the Undo button carries %v, want only a=%s and r=%d", decoded.Fields, undoAskCode, actionID)
	}
	return buttons[0].CallbackData
}

// undoCard returns the token and message ID of the last undo confirm card sent to
// the Staff Group: the last message whose keyboard has a Confirm button of the undo
// kind.
func undoCard(t *testing.T, env *staffActionEnv) (token string, msgID int64) {
	t.Helper()
	return undoCardIn(t, env, env.staffChat)
}

// undoCardIn is undoCard for the Staff Group chatID, which is not env.staffChat
// after a chat migration.
func undoCardIn(t *testing.T, env *staffActionEnv, chatID int64) (token string, msgID int64) {
	t.Helper()
	sent := env.fake.sentTo(chatID)
	for i := len(sent) - 1; i >= 0; i-- {
		for _, row := range staffKeyboardOf(sent[i].Params["reply_markup"]) {
			for _, button := range row {
				decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace)
				if ok && decoded.Fields["a"] == undoConfirmCode {
					return decoded.Fields["t"], sent[i].MessageID
				}
			}
		}
	}
	t.Fatalf("no undo confirm card was sent to the Staff Group")
	return "", 0
}

// tapUndoCard presses an undo card button (undoConfirmCode or undoCancelCode).
func (e *staffActionEnv) tapUndoCard(from gotgbot.User, code, token string, msgID int64) {
	e.t.Helper()
	e.tap(from, code, token, msgID)
}

func TestStaffUndoTracer(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")

	_, msgID := env.startRun("/ban 4242 spamming")
	env.waitRuns()
	action, _ := recordOfCard(t, msgID)
	banCalls := [2]int{len(env.callsTo("banChatMember", env.groups[0])), len(env.callsTo("banChatMember", env.groups[1]))}
	sentBefore := len(env.fake.sentTo(env.staffChat))

	// The final summary offers one Undo button that carries only the record ID.
	env.tapData(presser, env.staffChatObj(), msgID, undoButtonData(t, env, msgID, action.ID))

	// The press posts one confirm card as a reply to the summary, and nothing else
	// happens yet.
	sent := env.fake.sentTo(env.staffChat)
	if len(sent) != sentBefore+1 {
		t.Fatalf("messages to the Staff Group after Undo = %d, want %d (one confirm card)", len(sent)-sentBefore, 1)
	}
	card := sent[len(sent)-1]
	reply, ok := card.Params["reply_parameters"].(*gotgbot.ReplyParameters)
	if !ok || reply == nil || reply.MessageId != msgID || !reply.AllowSendingWithoutReply {
		t.Fatalf("undo card reply_parameters = %+v, want a reply to message %d that survives its deletion", card.Params["reply_parameters"], msgID)
	}
	text := fmt.Sprint(card.Params["text"])
	for _, key := range []string{"staff_undo_label", "staff_undo_card_applies"} {
		if !strings.Contains(text, staffMarker(key)) {
			t.Fatalf("undo card text lacks %s:\n%s", key, text)
		}
	}
	token, cardMsgID := undoCard(t, env)
	var codes []string
	for _, row := range staffKeyboardOf(card.Params["reply_markup"]) {
		for _, button := range row {
			decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace)
			if !ok || decoded.Fields["t"] != token || len(token) != 16 {
				t.Fatalf("undo card button %q does not carry the 16-hex token %q", button.CallbackData, token)
			}
			codes = append(codes, decoded.Fields["a"])
		}
	}
	if strings.Join(codes, ",") != undoConfirmCode+","+undoCancelCode {
		t.Fatalf("undo card buttons = %v, want Confirm (%s) then Cancel (%s)", codes, undoConfirmCode, undoCancelCode)
	}
	for _, group := range env.groups {
		if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
			t.Fatalf("group %d got %d unban calls before Confirm, want none", group, got)
		}
	}

	env.tapUndoCard(presser, undoConfirmCode, token, cardMsgID)
	env.waitRuns()

	for i, group := range env.groups {
		unbans := env.callsTo("unbanChatMember", group)
		if len(unbans) != 1 || !staffParamBool(unbans[0].Params, "only_if_banned") {
			t.Fatalf("group %d unban calls = %+v, want exactly one with only_if_banned", group, unbans)
		}
		if got := len(env.callsTo("banChatMember", group)); got != banCalls[i] {
			t.Fatalf("group %d ban calls = %d, want %d: undo must not ban", group, got, banCalls[i])
		}
		if got := len(env.callsTo("restrictChatMember", group)); got != 0 {
			t.Fatalf("group %d restrict calls = %d, want none", group, got)
		}
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
	}

	summary := env.lastEditText(env.staffChat, cardMsgID)
	for _, title := range []string{"Group A", "Group B"} {
		line := summaryLine(t, summary, title)
		if !strings.HasPrefix(line, "✅ "+title) || !strings.Contains(line, staffMarker("staff_undo_unbanned")) {
			t.Fatalf("undo line for %s = %q, want a done line saying unbanned", title, line)
		}
	}
	if !strings.Contains(summary, "✅ 2 · ⏭ 0 · ❌ 0") || strings.Contains(summary, "⏳") {
		t.Fatalf("undo summary has no final tally or still shows a pending line:\n%s", summary)
	}
	edits := env.edits(env.staffChat, cardMsgID)
	wantNoKeyboard(t, edits[len(edits)-1])

	original := env.lastEditText(env.staffChat, msgID)
	if !strings.Contains(original, staffMarker("staff_undo_marker")) || !strings.Contains(original, "Bob") {
		t.Fatalf("the original summary was not marked as undone by Bob:\n%s", original)
	}
	originalEdits := env.edits(env.staffChat, msgID)
	wantNoKeyboard(t, originalEdits[len(originalEdits)-1])

	// D-08: the original is marked only at the end of the run, after the last write
	// to a linked group.
	env.fake.mu.Lock()
	lastUnban, lastOriginalEdit := -1, -1
	for i, call := range env.fake.calls {
		switch call.Method {
		case "unbanChatMember":
			lastUnban = i
		case "editMessageText":
			if staffParamInt(call.Params, "message_id") == msgID {
				lastOriginalEdit = i
			}
		}
	}
	env.fake.mu.Unlock()
	if lastUnban < 0 || lastOriginalEdit <= lastUnban {
		t.Fatalf("the original's last edit is call %d, the last unban call %d: the original must be marked after the last group write",
			lastOriginalEdit, lastUnban)
	}

	undone, rows := recordOfCard(t, msgID)
	if undone.UndoBy == nil || *undone.UndoBy != presser.Id || undone.UndoByName != "Bob" {
		t.Fatalf("record undo_by = %v name %q, want %d Bob", undone.UndoBy, undone.UndoByName, presser.Id)
	}
	if undone.UndoStartedAt == nil || undone.UndoFinishedAt == nil {
		t.Fatalf("record undo_started_at = %v, undo_finished_at = %v, want both set", undone.UndoStartedAt, undone.UndoFinishedAt)
	}
	for _, row := range rows {
		if row.UndoOutcome != "done" || row.UndoReason != "undone_unbanned" {
			t.Fatalf("group %d undo = %q / %q, want done / undone_unbanned", row.GroupChatID, row.UndoOutcome, row.UndoReason)
		}
	}
}

// undoButtons lists every button of a keyboard.
func undoButtons(keyboard [][]gotgbot.InlineKeyboardButton) []gotgbot.InlineKeyboardButton {
	var buttons []gotgbot.InlineKeyboardButton
	for _, row := range keyboard {
		buttons = append(buttons, row...)
	}
	return buttons
}

// askUndo presses the Undo button of record actionID on message msgID as presser
// and returns the confirm card it posted.
func (e *staffActionEnv) askUndo(presser gotgbot.User, msgID int64, actionID uint) (token string, cardMsgID int64) {
	e.t.Helper()
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(actionID)})
	if data == "" {
		e.t.Fatal("the Undo callback data did not encode")
	}
	e.tapData(presser, e.staffChatObj(), msgID, data)
	return undoCard(e.t, e)
}

// runUndo asks for an undo as presser, confirms it and waits for the run. It
// returns the card message the undo summary ends up on.
func (e *staffActionEnv) runUndo(presser gotgbot.User, msgID int64, actionID uint) (cardMsgID int64) {
	e.t.Helper()
	token, cardMsgID := e.askUndo(presser, msgID, actionID)
	e.tapUndoCard(presser, undoConfirmCode, token, cardMsgID)
	e.waitRuns()
	return cardMsgID
}

// setStaffChatMembership sets what getChatMember says about user in the Staff Group.
func (e *staffActionEnv) setStaffChatMembership(user int64, status string) {
	e.fake.smu.Lock()
	defer e.fake.smu.Unlock()
	e.fake.members[[2]int64{e.staffChat, user}] = status
}

// callsAddressedTo counts every Telegram request the fake saw for chatID.
func (e *staffActionEnv) callsAddressedTo(chatID int64) int {
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	count := 0
	for _, call := range e.fake.calls {
		if fmt.Sprint(call.Params["chat_id"]) == fmt.Sprint(chatID) {
			count++
		}
	}
	return count
}

func TestStaffUndoButtonRules(t *testing.T) {
	// finalKeyboard runs command to its end and returns the record and the buttons of
	// the final edit.
	finalKeyboard := func(t *testing.T, env *staffActionEnv, command string) (uint, []gotgbot.InlineKeyboardButton) {
		t.Helper()
		_, msgID := env.startRun(command)
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)
		return action.ID, undoButtons(env.lastEditKeyboard(msgID))
	}
	wantButton := func(t *testing.T, env *staffActionEnv, command string) {
		t.Helper()
		_, msgID := env.startRun(command)
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)
		undoButtonData(t, env, msgID, action.ID)
	}

	t.Run("ban with applied groups", func(t *testing.T) {
		wantButton(t, newStaffActionEnv(t, 2), "/ban 4242")
	})
	t.Run("tban is recorded as a ban", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		wantButton(t, env, "/tban 4242 2d")
		action, _ := recordOfCard(t, env.lastCardMsgID())
		if action.Action != "ban" {
			t.Fatalf("record action = %q, want ban", action.Action)
		}
	})
	t.Run("mute", func(t *testing.T) {
		wantButton(t, newStaffActionEnv(t, 2), "/mute 4242")
	})
	t.Run("unban with one applied group", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		env.fake.setMember(env.groups[0], staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
		wantButton(t, env, "/unban 4242")
	})
	t.Run("kick has none, and a forged press is refused", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		presser := env.undoPresser("Bob")
		id, buttons := finalKeyboard(t, env, "/kick 4242")
		if len(buttons) != 0 {
			t.Fatalf("a kick's final summary has the buttons %+v, want none", buttons)
		}
		before := len(env.fake.sentTo(env.staffChat))
		data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(id)})
		env.tapData(presser, env.staffChatObj(), env.lastCardMsgID(), data)
		if text, alert := env.lastAnswer(); text != staffMarker("staff_undo_not_available") || !alert {
			t.Fatalf("answer to a forged kick undo = %q alert=%v, want the not-available alert", text, alert)
		}
		if got := len(env.fake.sentTo(env.staffChat)); got != before {
			t.Fatalf("a forged kick undo posted %d message(s), want none", got-before)
		}
	})
	t.Run("nothing applied has none", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		for _, group := range env.groups {
			env.fake.setMember(group, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}
		_, buttons := finalKeyboard(t, env, "/ban 4242")
		if len(buttons) != 0 {
			t.Fatalf("a ban applied nowhere has the buttons %+v, want none", buttons)
		}
	})
}

// lastCardMsgID is the message ID of the last staff action card of the Staff Group.
func (e *staffActionEnv) lastCardMsgID() int64 {
	e.t.Helper()
	_, msgID := e.card()
	return msgID
}

func TestStaffUndoButtonOnFallback(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)

	env.send(env.issuer, "/ban 4242")
	token, cardMsgID := env.card()
	// The first edit is the Confirm's pending summary and goes through; the second is
	// the final one, which fails with a 400 so delivery falls back at once.
	env.fake.script("editMessageText", env.staffChat, nil, staffFakeError(400, "Bad Request: message to edit not found"))
	env.tap(env.issuer, staffActRunConfirm, token, cardMsgID)
	env.waitRuns()

	sent := env.fake.sentTo(env.staffChat)
	fallback := sent[len(sent)-1]
	if fallback.MessageID == cardMsgID || len(sent) != 2 {
		t.Fatalf("messages to the Staff Group = %d, want the card and one fallback summary", len(sent))
	}
	if !strings.Contains(fmt.Sprint(fallback.Params["text"]), "Group A") {
		t.Fatalf("the fallback message is not the summary:\n%v", fallback.Params["text"])
	}
	action, _ := recordOfCard(t, fallback.MessageID)
	buttons := undoButtons(staffKeyboardOf(fallback.Params["reply_markup"]))
	if len(buttons) != 1 || buttons[0].Text != staffMarker("staff_undo_button") {
		t.Fatalf("fallback summary buttons = %+v, want the one Undo button", buttons)
	}
	decoded, ok := decodeCallbackData(buttons[0].CallbackData, staffCallbackNamespace)
	if !ok || decoded.Fields["a"] != undoAskCode || decoded.Fields["r"] != fmt.Sprint(action.ID) {
		t.Fatalf("fallback Undo button carries %+v, want a=%s r=%d", decoded, undoAskCode, action.ID)
	}
	if action.SummaryChatID != env.staffChat || action.SummaryMsgID != fallback.MessageID {
		t.Fatalf("record summary = chat %d message %d, want chat %d message %d (the fallback)",
			action.SummaryChatID, action.SummaryMsgID, env.staffChat, fallback.MessageID)
	}

	// The Undo then replies to, and marks, the message staff actually see.
	presser := env.undoPresser("Bob")
	env.tapData(presser, env.staffChatObj(), fallback.MessageID,
		encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(action.ID)}))
	_, card := undoCard(t, env)
	sent = env.fake.sentTo(env.staffChat)
	reply, _ := sent[len(sent)-1].Params["reply_parameters"].(*gotgbot.ReplyParameters)
	if card == 0 || reply == nil || reply.MessageId != fallback.MessageID {
		t.Fatalf("the undo card replies to %+v, want the fallback message %d", reply, fallback.MessageID)
	}
}

func TestStaffUndoOriginalEditFails(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")
	// No progress edit gets in between the scripted ones.
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()
	action, _ := recordOfCard(t, msgID)

	token, cardMsgID := env.askUndo(presser, msgID, action.ID)
	originalEditsBefore := len(env.edits(env.staffChat, msgID))
	// After Confirm the Staff Group is edited three times: the pending summary, the
	// final summary and, at the end of the run, the original. The third one fails.
	env.fake.script("editMessageText", env.staffChat, nil, nil,
		staffFakeError(400, "Bad Request: message can't be edited"))
	env.tapUndoCard(presser, undoConfirmCode, token, cardMsgID)
	env.waitRuns()

	if got := len(env.edits(env.staffChat, msgID)) - originalEditsBefore; got != 1 {
		t.Fatalf("the original summary got %d edit attempt(s), want exactly one", got)
	}
	summary := env.lastEditText(env.staffChat, cardMsgID)
	if strings.Contains(summary, "⏳") || !strings.Contains(summary, "✅ 2 · ⏭ 0 · ❌ 0") {
		t.Fatalf("the undo did not end on a final summary after the original could not be edited:\n%s", summary)
	}
	for _, group := range env.groups {
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
	}
	done, _ := recordOfCard(t, msgID)
	if done.UndoFinishedAt == nil {
		t.Fatal("the record has no undo_finished_at")
	}
}

func TestStaffUndoAfterRekey(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")
	oldStaff := env.staffChat
	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()
	action, _ := recordOfCard(t, msgID)

	newStaff := uniqueModuleChatID()
	staffCleanup(t, newStaff)
	env.fake.setCreator(newStaff, env.owner)
	if changed, err := staff.RekeyChat(oldStaff, newStaff); err != nil || !changed {
		t.Fatalf("RekeyChat = %v, %v, want a change", changed, err)
	}
	env.staffChat = newStaff
	oldEdits := len(env.edits(oldStaff, msgID))

	// The press comes from a message in the new chat; message IDs of the old chat
	// mean nothing there.
	const buttonMsg int64 = 777
	env.tapData(presser, env.staffChatObj(), buttonMsg,
		encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(action.ID)}))
	sent := env.fake.sentTo(newStaff)
	if len(sent) != 1 {
		t.Fatalf("messages to the migrated Staff Group = %d, want the one undo card", len(sent))
	}
	if sent[0].Params["reply_parameters"] != nil {
		t.Fatalf("the undo card replies to %+v, want no reply: the stored summary belongs to the old chat", sent[0].Params["reply_parameters"])
	}
	token, cardMsgID := undoCardIn(t, env, newStaff)
	env.tapUndoCard(presser, undoConfirmCode, token, cardMsgID)
	env.waitRuns()

	if got := len(env.edits(oldStaff, msgID)); got != oldEdits {
		t.Fatalf("message %d of the old chat was edited %d more time(s) by the undo", msgID, got-oldEdits)
	}
	if got := len(env.edits(newStaff, msgID)); got != 0 {
		t.Fatalf("message %d of the new chat was edited %d time(s): it is the old chat's summary ID", msgID, got)
	}
	// The undo itself still ran.
	for _, group := range env.groups {
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
	}
	done, _ := recordOfCard(t, msgID)
	if done.UndoFinishedAt == nil {
		t.Fatal("the record has no undo_finished_at")
	}
}

func TestStaffUndoAllSkipped(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")
	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()
	action, _ := recordOfCard(t, msgID)
	// Bob is only a plain member of every linked group when he presses Undo.
	for _, group := range env.groups {
		env.fake.setMember(group, presser.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	}

	cardMsgID := env.runUndo(presser, msgID, action.ID)

	summary := env.lastEditText(env.staffChat, cardMsgID)
	for _, title := range []string{"Group A", "Group B"} {
		wantSkippedLine(t, summary, title, "staff_act_skip_issuer_not_admin")
	}
	if !strings.Contains(summary, "✅ 0 · ⏭ 2 · ❌ 0") || strings.Contains(summary, "⏳") {
		t.Fatalf("an all-skipped undo has no final tally:\n%s", summary)
	}
	for _, group := range env.groups {
		if unbans := env.callsTo("unbanChatMember", group); len(unbans) != 0 {
			t.Fatalf("group %d got %d unban calls from a presser who is not its admin", group, len(unbans))
		}
		wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusKicked)
	}
	// Nothing was changed, so the action's one undo was given back.
	wantUndoClaimGivenBack(t, msgID)
}

func TestStaffUndoPresserNeedsTheRestrictRight(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")
	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()
	action, _ := recordOfCard(t, msgID)
	// An administrator without can_restrict_members in group B, the original issuer
	// being a restricting admin there: only the presser's own rights count.
	env.fake.setMember(env.groups[1], presser.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})

	cardMsgID := env.runUndo(presser, msgID, action.ID)

	summary := env.lastEditText(env.staffChat, cardMsgID)
	wantDoneLine(t, summary, "Group A")
	wantSkippedLine(t, summary, "Group B", "staff_act_skip_issuer_no_right")
	if got := len(env.callsTo("unbanChatMember", env.groups[1])); got != 0 {
		t.Fatalf("group B got %d unban calls from a presser without the restrict right", got)
	}
	wantMember(t, env.fake, env.groups[0], staffTestTarget, gotgbot.ChatMemberStatusLeft)
	wantMember(t, env.fake, env.groups[1], staffTestTarget, gotgbot.ChatMemberStatusKicked)
}

func TestStaffUndoOrder(t *testing.T) {
	env := newStaffActionEnv(t, 4)
	presser := env.undoPresser("Bob")
	// The issuer is a plain member of group B, so the ban is applied in A, C and D only.
	env.fake.setMember(env.groups[1], env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()
	action, _ := recordOfCard(t, msgID)
	skippedBefore := env.callsAddressedTo(env.groups[1])

	cardMsgID := env.runUndo(presser, msgID, action.ID)

	summary := env.lastEditText(env.staffChat, cardMsgID)
	a, c, d := strings.Index(summary, "Group A"), strings.Index(summary, "Group C"), strings.Index(summary, "Group D")
	if a < 0 || c < a || d < c {
		t.Fatalf("undo summary does not list Group A, Group C, Group D in the original's order:\n%s", summary)
	}
	if strings.Contains(summary, "Group B") {
		t.Fatalf("undo summary lists Group B, where the action was not applied:\n%s", summary)
	}
	if !strings.Contains(summary, staffMarker("staff_undo_not_applied_note")) || !strings.Contains(summary, "✅ 3 · ⏭ 0 · ❌ 0") {
		t.Fatalf("undo summary lacks the not-applied note or its tally:\n%s", summary)
	}
	if got := env.callsAddressedTo(env.groups[1]); got != skippedBefore {
		t.Fatalf("group B got %d Telegram request(s) during the undo, want none", got-skippedBefore)
	}
	_, rows := recordOfCard(t, msgID)
	for i, row := range rows {
		wantOutcome, wantReason := "done", "undone_unbanned"
		if i == 1 {
			wantOutcome, wantReason = "skipped", "skip_not_applied"
		}
		if row.UndoOutcome != wantOutcome || row.UndoReason != wantReason {
			t.Fatalf("group %d (%s) undo = %q / %q, want %q / %q", i, row.GroupTitle, row.UndoOutcome, row.UndoReason, wantOutcome, wantReason)
		}
	}
}

func TestStaffUndoCardsDoNotCross(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	presser := env.undoPresser("Bob")
	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()
	action, _ := recordOfCard(t, msgID)

	// An undo card at the staff action Confirm starts nothing.
	undoToken, undoMsg := env.askUndo(presser, msgID, action.ID)
	env.tap(presser, staffActRunConfirm, undoToken, undoMsg)
	if text, _ := env.lastAnswer(); text != staffMarker("staff_cb_expired") {
		t.Fatalf("an undo card at the action Confirm was answered %q, want the expired answer", text)
	}
	if state := cardState(t, undoToken); state != staffCardPending {
		t.Fatalf("undo card state = %q, want it untouched (pending)", state)
	}
	for _, group := range env.groups {
		if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
			t.Fatalf("group %d got %d unban call(s) from an undo card at the action Confirm", group, got)
		}
	}

	// A staff action card at the undo Confirm starts nothing either.
	env.send(env.issuer, "/ban 4343")
	token, cardMsg := env.card()
	env.tap(env.issuer, undoConfirmCode, token, cardMsg)
	if text, _ := env.lastAnswer(); text != staffMarker("staff_cb_expired") {
		t.Fatalf("an action card at the undo Confirm was answered %q, want the expired answer", text)
	}
	if state := cardState(t, token); state != staffCardPending {
		t.Fatalf("action card state = %q, want it untouched (pending)", state)
	}
	for _, group := range env.groups {
		if got := len(env.callsTo("banChatMember", group)); got != 1 {
			t.Fatalf("group %d ban calls = %d, want only the first ban's", group, got)
		}
	}
}

func TestStaffUndoAccess(t *testing.T) {
	t.Run("an outsider cannot start an undo", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		_, msgID := env.startRun("/ban 4242")
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)
		outsider := gotgbot.User{Id: env.issuer.Id + 50, FirstName: "Eve"}
		env.setStaffChatMembership(outsider.Id, "left")
		before := len(env.fake.sentTo(env.staffChat))

		env.tapData(outsider, env.staffChatObj(), msgID,
			encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(action.ID)}))

		if text, alert := env.lastAnswer(); text != staffMarker("staff_cb_members_only") || !alert {
			t.Fatalf("answer to an outsider = %q alert=%v, want the members-only alert", text, alert)
		}
		if got := len(env.fake.sentTo(env.staffChat)); got != before {
			t.Fatalf("an outsider's press posted %d message(s), want none", got-before)
		}
	})

	t.Run("a record of another Staff Group is refused", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		presser := env.undoPresser("Bob")
		otherStaff := uniqueModuleChatID()
		staffCleanup(t, otherStaff)
		now := time.Now()
		foreign := &models.StaffAction{
			StaffChatID: otherStaff, IssuerUserID: 1, TargetUserID: staffTestTarget, Action: "ban",
			GroupCount: 1, FinishedAt: &now,
		}
		if err := staff.CreateAction(foreign, []models.StaffActionGroup{
			{Seq: 0, GroupChatID: env.groups[0], Outcome: models.StaffActionOutcomeDone},
		}); err != nil {
			t.Fatalf("create foreign record: %v", err)
		}
		before := len(env.fake.sentTo(env.staffChat))

		env.tapData(presser, env.staffChatObj(), 5, encodeCallbackData(staffCallbackNamespace,
			map[string]string{"a": undoAskCode, "r": fmt.Sprint(foreign.ID)}))

		if text, _ := env.lastAnswer(); text != staffMarker("staff_cb_denied") {
			t.Fatalf("answer for another Staff Group's record = %q, want the denied answer", text)
		}
		if got := len(env.fake.sentTo(env.staffChat)); got != before {
			t.Fatalf("a foreign record's press posted %d message(s), want none", got-before)
		}
	})

	t.Run("only the member who pressed Undo can confirm", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		presser := env.undoPresser("Bob")
		_, msgID := env.startRun("/ban 4242")
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)
		token, cardMsgID := env.askUndo(presser, msgID, action.ID)

		env.tapUndoCard(env.issuer, undoConfirmCode, token, cardMsgID)

		if text, alert := env.lastAnswer(); text != staffMarker("staff_undo_card_presser_only") || !alert {
			t.Fatalf("answer to another member's Confirm = %q alert=%v, want the only-the-presser alert", text, alert)
		}
		if state := cardState(t, token); state != staffCardPending {
			t.Fatalf("undo card state = %q, want it still pending", state)
		}
		done, _ := recordOfCard(t, msgID)
		if done.UndoStartedAt != nil {
			t.Fatal("another member's Confirm claimed the undo")
		}
		for _, group := range env.groups {
			if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
				t.Fatalf("group %d got %d unban call(s) from another member's Confirm", group, got)
			}
		}
	})

	t.Run("a presser who left the Staff Group cannot confirm", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		presser := env.undoPresser("Bob")
		_, msgID := env.startRun("/ban 4242")
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)
		token, cardMsgID := env.askUndo(presser, msgID, action.ID)
		env.setStaffChatMembership(presser.Id, "left")

		env.tapUndoCard(presser, undoConfirmCode, token, cardMsgID)
		env.waitRuns()

		if text := env.lastEditText(env.staffChat, cardMsgID); !strings.Contains(text, staffMarker("staff_act_abort_issuer_left")) {
			t.Fatalf("undo card after the presser left:\n%s", text)
		}
		done, _ := recordOfCard(t, msgID)
		if done.UndoStartedAt != nil {
			t.Fatal("an undo was claimed by a presser who had left the Staff Group")
		}
		for _, group := range env.groups {
			if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
				t.Fatalf("group %d got %d unban call(s)", group, got)
			}
		}
	})

	t.Run("only the winner of the claim runs", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		presser := env.undoPresser("Bob")
		_, msgID := env.startRun("/ban 4242")
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)
		token, cardMsgID := env.askUndo(presser, msgID, action.ID)

		// Another replica's Confirm claims the record after this one's last read and
		// before its claim: this one loses.
		previous := staffClaimUndo
		staffClaimUndo = func(id uint, by int64, name string) (time.Time, bool, error) {
			if _, _, err := staff.ClaimUndo(id, 4040, "Winner"); err != nil {
				return time.Time{}, false, err
			}
			return previous(id, by, name)
		}
		t.Cleanup(func() { staffClaimUndo = previous })

		env.tapUndoCard(presser, undoConfirmCode, token, cardMsgID)
		env.waitRuns()

		text := env.lastEditText(env.staffChat, cardMsgID)
		if !strings.Contains(text, staffMarker("staff_undo_already")) || !strings.Contains(text, "Winner") {
			t.Fatalf("the losing card did not name the winner:\n%s", text)
		}
		for _, group := range env.groups {
			if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
				t.Fatalf("group %d got %d unban call(s) from the loser of the claim", group, got)
			}
		}
		done, _ := recordOfCard(t, msgID)
		if done.UndoBy == nil || *done.UndoBy != 4040 {
			t.Fatalf("record undo_by = %v, want the winner 4040", done.UndoBy)
		}
	})

	t.Run("an action is undone once", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		presser := env.undoPresser("Bob")
		_, msgID := env.startRun("/ban 4242")
		env.waitRuns()
		action, _ := recordOfCard(t, msgID)
		first, firstMsg := env.askUndo(presser, msgID, action.ID)
		second, secondMsg := env.askUndo(presser, msgID, action.ID)
		if first == second {
			t.Fatal("two presses shared one card token")
		}

		env.tapUndoCard(presser, undoConfirmCode, first, firstMsg)
		env.waitRuns()
		env.tapUndoCard(presser, undoConfirmCode, second, secondMsg)
		env.waitRuns()

		text := env.lastEditText(env.staffChat, secondMsg)
		if !strings.Contains(text, staffMarker("staff_undo_already")) || !strings.Contains(text, "Bob") {
			t.Fatalf("the second card did not say the action was already undone by Bob:\n%s", text)
		}
		for _, group := range env.groups {
			if got := len(env.callsTo("unbanChatMember", group)); got != 1 {
				t.Fatalf("group %d unban calls = %d, want exactly the first undo's", group, got)
			}
		}

		// The button on the old summary is dead too.
		before := len(env.fake.sentTo(env.staffChat))
		env.tapData(presser, env.staffChatObj(), msgID,
			encodeCallbackData(staffCallbackNamespace, map[string]string{"a": undoAskCode, "r": fmt.Sprint(action.ID)}))
		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("staff_undo_already")) {
			t.Fatalf("answer to Undo on an undone action = %q, want the already-undone text", text)
		}
		if got := len(env.fake.sentTo(env.staffChat)); got != before {
			t.Fatalf("Undo on an undone action posted %d message(s), want none", got-before)
		}
	})
}

func TestStaffUndoRestoresPriorRestriction(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	presser := env.undoPresser("Bob")
	group := env.groups[0]
	until := time.Now().Add(2 * time.Hour).Unix()
	// The target was restricted, with a partial permission set, before the ban.
	env.fake.setMember(group, staffTestTarget, staffFakeMember{
		Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: until,
		Perms: &gotgbot.ChatPermissions{CanSendPolls: true, CanInviteUsers: true},
	})
	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()
	action, rows := recordOfCard(t, msgID)
	recorded := permsJSON(t, priorPermissionsOf(t, rows[0]))

	cardMsgID := env.runUndo(presser, msgID, action.ID)

	restricts := env.callsTo("restrictChatMember", group)
	if len(restricts) != 1 {
		t.Fatalf("restrictChatMember calls = %d, want one restore", len(restricts))
	}
	call := restricts[0]
	if got := permsJSON(t, call.Params["permissions"]); got != recorded {
		t.Fatalf("restored permissions = %s, want the recorded %s", got, recorded)
	}
	if !staffParamBool(call.Params, "use_independent_chat_permissions") || staffParamInt(call.Params, "until_date") != until {
		t.Fatalf("restore call = %+v, want use_independent_chat_permissions and until_date %d", call.Params, until)
	}
	if got := len(env.callsTo("unbanChatMember", group)); got != 0 {
		t.Fatalf("a restore sent %d unban call(s) too", got)
	}
	m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusRestricted)
	if m.UntilDate != until {
		t.Fatalf("restored restriction ends at %d, want %d", m.UntilDate, until)
	}
	line := summaryLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A")
	if !strings.HasPrefix(line, "✅") || !strings.Contains(line, staffMarker("staff_undo_restriction_restored")) {
		t.Fatalf("undo line = %q, want a done line saying the restriction is back", line)
	}
}
