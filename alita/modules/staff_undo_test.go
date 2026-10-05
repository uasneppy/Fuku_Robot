//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
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
	sent := env.fake.sentTo(env.staffChat)
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
