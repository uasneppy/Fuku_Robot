//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// settingsMsgStored reads one event's group message straight from the database, past every cache layer.
// ok is false when the event has no row.
func settingsMsgStored(t *testing.T, chatID int64, event string) (models.ChatEventMessage, bool) {
	t.Helper()
	var rows []models.ChatEventMessage
	if err := db.DB.Where("chat_id = ? AND event = ? AND dest = ?", chatID, event, eventmsg.DestGroup).Find(&rows).Error; err != nil {
		t.Fatalf("read the %s message of chat %d: %v", event, chatID, err)
	}
	if len(rows) == 0 {
		return models.ChatEventMessage{}, false
	}
	return rows[0], true
}

// settingsMsgSeedFull stores a text, a photo and one button for an event's group message through the
// repository.
func settingsMsgSeedFull(t *testing.T, chatID int64, event string) {
	t.Helper()
	ctx := context.Background()
	if err := eventmsg.SetText(ctx, chatID, event, eventmsg.DestGroup, "Muted {mention}", 1); err != nil {
		t.Fatalf("seed the %s text: %v", event, err)
	}
	if err := eventmsg.SetMedia(ctx, chatID, event, eventmsg.DestGroup, "mute-photo", db.PHOTO, 1); err != nil {
		t.Fatalf("seed the %s media: %v", event, err)
	}
	if err := eventmsg.SetButtons(ctx, chatID, event, eventmsg.DestGroup,
		[]db.Button{{Name: "Rules", Url: "https://example.com/rules"}}, 1); err != nil {
		t.Fatalf("seed the %s buttons: %v", event, err)
	}
}

// settingsMsgPartButton returns the button of an event screen's part row with the action code (hs or hr)
// and the part, failing the test when there is none.
func settingsMsgPartButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, action, part string) gotgbot.InlineKeyboardButton {
	t.Helper()
	button, ok := settingsMsgButtonWith(t, keyboard, "a", action, "t", part)
	if !ok {
		t.Fatalf("the event screen has no %s button for part %s: %+v", action, part, keyboard)
	}
	return button
}

// settingsMsgSendPathText is the text the send path would read for an event of chatID right now (the
// cached read eventMessagesLookup, with no cache reset).
func settingsMsgSendPathText(t *testing.T, chatID int64, event string) string {
	t.Helper()
	msgs, err := eventMessagesLookup(context.Background(), chatID)
	if err != nil {
		t.Fatalf("the send path's read of chat %d failed: %v", chatID, err)
	}
	msg, _ := msgs.Lookup(event, eventmsg.DestGroup)
	return msg.Text
}

// settingsMsgSetWait opens the screen of the event code in a fresh private menu of user and presses Set of
// part; the menu message is then the waiting prompt.
func settingsMsgSetWait(t *testing.T, env *settingsMsgEnv, user gotgbot.User, code, part string) *settingsMsgMenu {
	t.Helper()
	m := settingsMsgPrivateMenu(env, env.chat, user)
	_, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, code)
	m.pressButton(settingsMsgPartButton(t, keyboard, settingsActMsgSet, part))
	if !env.redis.Exists(settingsInputKey(user.Id)) {
		t.Fatalf("Set of %s/%s left user %d with no wait", code, part, user.Id)
	}
	return m
}

// settingsMsgLastSent is the text and keyboard of the last message the bot sent to user's private chat.
func settingsMsgLastSent(t *testing.T, env *settingsMsgEnv, user gotgbot.User) (string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	sent := env.fake.sentTo(user.Id)
	if len(sent) == 0 {
		t.Fatalf("the bot sent nothing to user %d", user.Id)
	}
	last := sent[len(sent)-1]
	return fmt.Sprint(last.Params["text"]), staffKeyboardOf(last.Params["reply_markup"])
}

func TestSettingsMessagesSet(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	group := strconv.FormatInt(env.chat.Id, 10)

	// Nothing is stored, so each part has its Set button alone, carrying k and the private owner field.
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)
	_, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
	settingsMsgWantWidths(t, "the empty ban screen", keyboard, 1, 1, 1, 1, 2)
	for i, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
		fields := settingsButtonFields(t, keyboard[i][0])
		if fields["a"] != settingsActMsgSet || fields["t"] != part || fields[settingsMsgEventField] != "bn" || fields["c"] != group || len(fields) != 4 {
			t.Errorf("part row %d carries %v, want a=hs, t=%s, k=bn and c", i, fields, part)
		}
	}

	// Set text writes the wait for the ban definition and turns the message into the family's text prompt.
	text, keyboard := m.pressButton(keyboard[0][0])
	wait := settingsEditWaitOf(env.settingsEnv, env.admin)
	if wait["c"] != group || wait["p"] != "hbng" || wait["t"] != settingsEditPartText || wait["m"] != strconv.FormatInt(m.msgID, 10) {
		t.Fatalf("wait = %v, want the group, page hbng, part x and the menu message", wait)
	}
	if !containsMarker(text, "settings_msg_wait_text_manual") || !settingsEditPromptHas(text, settingsEditTextMax) {
		t.Errorf("the text prompt %q lacks the manual family's prompt or the limit %d", text, settingsEditTextMax)
	}
	if len(keyboard) != 1 || len(keyboard[0]) != 1 {
		t.Fatalf("the prompt keyboard = %+v, want one Cancel", keyboard)
	}
	cancel := settingsButtonFields(t, keyboard[0][0])
	if cancel["a"] != settingsActMsgCancel || cancel["t"] != settingsEditPartText || cancel[settingsMsgEventField] != "bn" || cancel["c"] != group {
		t.Errorf("the prompt's Cancel carries %v, want a=hc, t=x, k=bn and c", cancel)
	}

	// Cancel ends the wait and draws the ban screen with the cancelled notice.
	text, keyboard = m.pressButton(keyboard[0][0])
	settingsEditWaitGone(t, env.settingsEnv, env.admin)
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_cancelled")) {
		t.Errorf("the screen after Cancel %q lacks the cancelled notice", text)
	}
	settingsMsgWantSilentScreen(t, text, keyboard, "bn", false)

	// Set media and Set buttons show the editor's own prompts.
	for _, tc := range []struct{ part, key string }{
		{settingsEditPartMedia, "settings_edit_wait_media"}, {settingsEditPartButtons, "settings_edit_wait_buttons"},
	} {
		_, keyboard = m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
		text, _ = m.pressButton(settingsMsgPartButton(t, keyboard, settingsActMsgSet, tc.part))
		if !containsMarker(text, tc.key) {
			t.Errorf("the %s prompt %q lacks %s", tc.part, text, tc.key)
		}
		if got := settingsEditWaitOf(env.settingsEnv, env.admin); got["p"] != "hbng" || got["t"] != tc.part {
			t.Errorf("Set %s wrote the wait %v", tc.part, got)
		}
	}

	// Set text on the kick screen writes page hkkg, and a stored media brings the limit down to 1024.
	if err := eventmsg.SetMedia(context.Background(), env.chat.Id, eventmsg.EventKick, eventmsg.DestGroup, "kick-photo", db.PHOTO, 1); err != nil {
		t.Fatalf("seed the kick media: %v", err)
	}
	k := settingsMsgSetWait(t, env, env.admin, "kk", settingsEditPartText)
	if got := settingsEditWaitOf(env.settingsEnv, env.admin); got["p"] != "hkkg" {
		t.Errorf("Set on the kick screen wrote the wait %v, want page hkkg", got)
	}
	if text, _ := env.lastEdit(env.admin.Id, k.msgID); !settingsEditPromptHas(text, settingsEditCaptionMax) {
		t.Errorf("the kick text prompt with a stored media %q does not show the limit %d", text, settingsEditCaptionMax)
	}

	// Without Redis Set answers the alert and edits nothing.
	n := settingsMsgPrivateMenu(env, env.chat, env.admin)
	n.press("a", settingsActMsgEvent, settingsMsgEventField, "mu")
	edits := len(env.edits(env.admin.Id, n.msgID))
	restore := cache.DisableRedisForTest()
	env.pressPrivate(env.admin, n.msgID, env.privateFields(env.chat.Id, "a", settingsActMsgSet, settingsMsgEventField, "mu", "t", settingsEditPartText))
	restore()
	if answer, alert := env.lastAnswer(); !alert || !containsMarker(answer, "settings_edit_unavailable") {
		t.Errorf("Set without Redis answered %q (alert %v), want the alert settings_edit_unavailable", answer, alert)
	}
	if got := len(env.edits(env.admin.Id, n.msgID)); got != edits {
		t.Errorf("Set without Redis edited the menu %d times, want none", got-edits)
	}

	// Set, Remove and Cancel are 42 bytes at a 14-character chat ID.
	for _, code := range []string{settingsActMsgSet, settingsActMsgRemove, settingsActMsgCancel} {
		data := encodeCallbackData(settingsCallbackNamespace, map[string]string{
			"a": code, "c": "-1001234567890", settingsMsgEventField: "bn", "t": settingsEditPartText,
		})
		if len(data) != 42 {
			t.Errorf("the %s button's data %q is %d bytes, want 42", code, data, len(data))
		}
	}
}

func TestSettingsMessagesRemove(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	settingsMsgSeedFull(t, env.chat.Id, eventmsg.EventMute)
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)

	_, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "mu")
	settingsMsgWantWidths(t, "the full mute screen", keyboard, 2, 2, 2, 1, 2)

	// Remove text clears the text alone; the line reads Default.
	text, keyboard := m.pressButton(settingsMsgPartButton(t, keyboard, settingsActMsgRemove, settingsEditPartText))
	row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute)
	if row.Text != "" || row.FileID != "mute-photo" || len(row.Buttons) != 1 || row.UpdatedBy != env.admin.Id {
		t.Errorf("after Remove text the row is %+v, want no text, the photo, the button and the presser as writer", row)
	}
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_text_default")) {
		t.Errorf("the screen %q lacks the removed-text notice", text)
	}
	if want := staffMarker("settings_edit_line_text") + " " + staffMarker("settings_state_default"); !strings.Contains(text, want) {
		t.Errorf("the screen %q does not read Default for the text", text)
	}
	settingsMsgWantWidths(t, "the mute screen after Remove text", keyboard, 1, 2, 2, 1, 2)

	// Remove media and Remove buttons clear theirs.
	text, keyboard = m.pressButton(settingsMsgPartButton(t, keyboard, settingsActMsgRemove, settingsEditPartMedia))
	if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute); row.FileID != "" || row.MediaType != 0 || len(row.Buttons) != 1 {
		t.Errorf("after Remove media the row is %+v", row)
	}
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_media")) {
		t.Errorf("the screen %q lacks the removed-media notice", text)
	}
	text, keyboard = m.pressButton(settingsMsgPartButton(t, keyboard, settingsActMsgRemove, settingsEditPartButtons))
	if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute); len(row.Buttons) != 0 {
		t.Errorf("after Remove buttons the row keeps %+v", row.Buttons)
	}
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_buttons")) {
		t.Errorf("the screen %q lacks the removed-buttons notice", text)
	}
	settingsMsgWantWidths(t, "the emptied mute screen", keyboard, 1, 1, 1, 1, 2)
	hub, _ := m.press("a", settingsActOpen, "p", settingsPageMessages)
	settingsMsgWantStates(t, hub, nil)

	// A stale Remove of a part that is not set writes nothing.
	before, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute)
	text, _ = m.press("a", settingsActMsgRemove, settingsMsgEventField, "mu", "t", settingsEditPartText)
	if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_edit_nothing_to_remove")) {
		t.Errorf("a stale Remove drew %q, want the nothing-to-remove notice", text)
	}
	if after, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute); !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("a stale Remove wrote the row (updated at %v, before %v)", after.UpdatedAt, before.UpdatedAt)
	}

	// A failed write redraws the stored state with the save-failed notice and keeps the text.
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventMute, "Muted again", false)
	mreqSwap(t, &eventMsgWrites.Text, func(context.Context, int64, string, string, string, int64) error {
		return errors.New("the event messages are not writable")
	})
	text, keyboard = m.press("a", settingsActMsgRemove, settingsMsgEventField, "mu", "t", settingsEditPartText)
	if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
		t.Errorf("a failed Remove drew %q, want the save-failed notice", text)
	}
	if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute); row.Text != "Muted again" {
		t.Errorf("a failed Remove changed the text to %q", row.Text)
	}
	if _, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgRemove, "t", settingsEditPartText); !ok {
		t.Errorf("the redraw after a failed Remove lost the text's Remove button: %+v", keyboard)
	}
}
