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

func TestSettingsMessagesTyped(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	user := env.admin

	// wantRefusal fails the test unless exactly one reply was sent since before, carrying the refusal key and
	// the still-waiting line, with a Cancel of the event, and the wait is kept.
	wantRefusal := func(t *testing.T, before int, code, key string) {
		t.Helper()
		settingsEditWantReply(t, env.settingsEnv, user, before, key, "settings_edit_still_waiting")
		_, keyboard := settingsMsgLastSent(t, env, user)
		if _, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgCancel, settingsMsgEventField, code); !ok {
			t.Errorf("the refusal carries no Cancel with k=%s: %+v", code, keyboard)
		}
		settingsEditWaitKept(t, env.settingsEnv, user)
	}

	t.Run("a typed text is stored and the send path reads it at once", func(t *testing.T) {
		if got := settingsMsgSendPathText(t, env.chat.Id, eventmsg.EventBan); got != "" {
			t.Fatalf("the send path reads a ban text %q before any save", got)
		}
		m := settingsMsgSetWait(t, env, user, "bn", settingsEditPartText)
		before := settingsEditSentCount(env.settingsEnv, user)
		sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{
			Text: "Bye {first}, {reason}", Entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 0, Length: 3}},
		})
		const want = "<b>Bye</b> {first}, {reason}"
		if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); row.Text != want || row.UpdatedBy != user.Id {
			t.Errorf("the ban row is %+v, want the text %q written by %d", row, want, user.Id)
		}
		settingsEditWantReply(t, env.settingsEnv, user, before, "settings_edit_saved_text", "settings_msg_ev_ban")
		page, keyboard := settingsMsgLastSent(t, env, user)
		if line := staffMarker("settings_edit_line_text") + " " + staffMarker("settings_edit_state_text_len"); !strings.Contains(page, line) {
			t.Errorf("the saved page %q does not read the text as Set", page)
		}
		if _, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgRemove, "t", settingsEditPartText, settingsMsgEventField, "bn"); !ok {
			t.Errorf("the saved page does not offer Remove for the text: %+v", keyboard)
		}
		if closed, closedKeyboard := env.lastEdit(user.Id, m.msgID); !containsMarker(closed, "settings_edit_prompt_closed") || len(closedKeyboard) != 0 {
			t.Errorf("the prompt was edited to %q with %+v, want the closed line and no keyboard", closed, closedKeyboard)
		}
		settingsEditWaitGone(t, env.settingsEnv, user)
		if got := settingsMsgSendPathText(t, env.chat.Id, eventmsg.EventBan); got != want {
			t.Errorf("the send path reads %q after the save, want %q", got, want)
		}
	})

	t.Run("a typed photo is stored and its caption ignored", func(t *testing.T) {
		settingsMsgSetWait(t, env, user, "bn", settingsEditPartMedia)
		before := settingsEditSentCount(env.settingsEnv, user)
		photo := settingsMediaItemID("photo", "ban-photo")
		photo.Caption = "a caption"
		sendPrivateMessage(env.settingsEnv, user, photo)
		if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); row.FileID != "ban-photo" || row.MediaType != db.PHOTO {
			t.Errorf("the ban media is %q type %d, want ban-photo as a photo", row.FileID, row.MediaType)
		}
		settingsEditWantReply(t, env.settingsEnv, user, before, "settings_edit_saved_media", "settings_edit_caption_ignored")
		settingsEditWaitGone(t, env.settingsEnv, user)
	})

	t.Run("a typed button line is stored as one button", func(t *testing.T) {
		settingsMsgSetWait(t, env, user, "bn", settingsEditPartButtons)
		before := settingsEditSentCount(env.settingsEnv, user)
		sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{Text: "[Rules](buttonurl://https://example.com/rules)"})
		row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan)
		if len(row.Buttons) != 1 || row.Buttons[0].Name != "Rules" || row.Buttons[0].Url != "https://example.com/rules" {
			t.Errorf("the ban buttons are %+v, want one Rules button", row.Buttons)
		}
		settingsEditWantReply(t, env.settingsEnv, user, before, "settings_edit_saved_buttons")
		settingsEditWaitGone(t, env.settingsEnv, user)
	})

	refusals := []struct {
		name, code, part, key string
		msg                   gotgbot.Message
	}{
		{"a text with a button line", "bn", settingsEditPartText, "settings_edit_refuse_text_buttons",
			gotgbot.Message{Text: "Bye\n[a](buttonurl://https://example.com)"}},
		{"a javascript link", "bn", settingsEditPartButtons, "settings_edit_refuse_button_url",
			gotgbot.Message{Text: "[a](buttonurl://javascript:alert)"}},
		{"100 buttons with the Unban row", "bn", settingsEditPartButtons, "settings_edit_refuse_buttons_limit",
			gotgbot.Message{Text: settingsEditButtonLines(100, 1000)}},
		{"a slash text", "bn", settingsEditPartText, "settings_edit_refuse_slash", gotgbot.Message{Text: "/notacommandzz now"}},
	}
	for _, tc := range refusals {
		t.Run("refused: "+tc.name, func(t *testing.T) {
			stored, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan)
			settingsMsgSetWait(t, env, user, tc.code, tc.part)
			before := settingsEditSentCount(env.settingsEnv, user)
			sendPrivateMessage(env.settingsEnv, user, tc.msg)
			wantRefusal(t, before, tc.code, tc.key)
			if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); !row.UpdatedAt.Equal(stored.UpdatedAt) {
				t.Errorf("the refused message wrote the ban row: %+v", row)
			}
		})
	}

	t.Run("refused: a media while the stored text is over 1024", func(t *testing.T) {
		settingsMsgSeed(t, env.chat.Id, eventmsg.EventTimedBan, strings.Repeat("x", 1100), false)
		settingsMsgSetWait(t, env, user, "tb", settingsEditPartMedia)
		before := settingsEditSentCount(env.settingsEnv, user)
		sendPrivateMedia(env.settingsEnv, user, "photo")
		wantRefusal(t, before, "tb", "settings_edit_refuse_media_text_long")
		if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventTimedBan); row.FileID != "" {
			t.Errorf("the refused photo was stored as %q", row.FileID)
		}
	})

	t.Run("an album saves its first item once", func(t *testing.T) {
		settingsMsgSetWait(t, env, user, "ub", settingsEditPartMedia)
		before := settingsEditSentCount(env.settingsEnv, user)
		sendPrivateAlbum(env.settingsEnv, user, settingsMediaAlbumOf(3), false)
		if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventUnban); row.FileID != "album-1" || row.MediaType != db.PHOTO {
			t.Errorf("the unban media is %q type %d, want the album's first photo", row.FileID, row.MediaType)
		}
		settingsEditWantReply(t, env.settingsEnv, user, before, "settings_edit_saved_media", "settings_edit_album_cut")
		settingsEditWaitGone(t, env.settingsEnv, user)
	})
}

func TestSettingsAuthorityMessagesEdit(t *testing.T) {
	// refusedPress presses Set (or Remove) of the ban text as user on a fresh private message and fails the
	// test unless it is answered the alert key with nothing edited, no wait and nothing written.
	refusedPress := func(t *testing.T, env *settingsMsgEnv, user gotgbot.User, action, key string) {
		t.Helper()
		before, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan)
		msgID := env.updateID()
		env.pressPrivate(user, msgID, env.privateFields(env.chat.Id, "a", action, settingsMsgEventField, "bn", "t", settingsEditPartText))
		if answer, alert := env.lastAnswer(); !alert || !containsMarker(answer, key) {
			t.Errorf("%s answered %q (alert %v), want the alert %s", action, answer, alert, key)
		}
		if got := len(env.edits(user.Id, msgID)); got != 0 {
			t.Errorf("the refused %s made %d edits, want none", action, got)
		}
		if env.redis.Exists(settingsInputKey(user.Id)) {
			t.Errorf("the refused %s left a wait", action)
		}
		if after, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); after.Text != before.Text || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Errorf("the refused %s wrote the ban row: %+v", action, after)
		}
	}
	// typedEnds sets a wait for user, moves the world, sends a typed text and fails the test unless the one
	// reply is the key followed by the wait-stopped line, the wait is gone and nothing is stored.
	typedEnds := func(t *testing.T, env *settingsMsgEnv, user gotgbot.User, change func(), key string) {
		t.Helper()
		settingsMsgSetWait(t, env, user, "bn", settingsEditPartText)
		change()
		before := settingsEditSentCount(env.settingsEnv, user)
		sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{Text: "a new ban text"})
		settingsEditWantReply(t, env.settingsEnv, user, before, key)
		if text := settingsEditTextsSince(env.settingsEnv, user, before)[0]; !strings.HasSuffix(text, staffMarker(key)+"\n\n"+staffMarker("settings_edit_wait_stopped")) {
			t.Errorf("the reply %q is not %s, a blank line and the wait-stopped line", text, key)
		}
		settingsEditWaitGone(t, env.settingsEnv, user)
		if row, ok := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); ok && row.Text != "" {
			t.Errorf("the typed text was stored: %+v", row)
		}
	}

	t.Run("an administrator without can_change_info is refused at Set and Remove", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
		refusedPress(t, env, env.admin, settingsActMsgSet, "settings_no_change_info")
		settingsMsgSeed(t, env.chat.Id, eventmsg.EventBan, "Banned", false)
		refusedPress(t, env, env.admin, settingsActMsgRemove, "settings_no_change_info")

		// The screens and Cancel need only the admin check.
		m := settingsMsgPrivateMenu(env, env.chat, env.admin)
		text, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
		settingsMsgWantSilentScreen(t, text, keyboard, "bn", false)
		m.press("a", settingsActMsgCancel, settingsMsgEventField, "bn", "t", settingsEditPartText)
		if answer, _ := env.lastAnswer(); containsMarker(answer, "settings_no_change_info") {
			t.Errorf("Cancel was refused with %q, want only the admin check", answer)
		}
	})

	t.Run("an administrator demoted between Set and the typed text stores nothing", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		typedEnds(t, env, env.admin, func() { listEditAdmin(env.settingsEnv, env.chat, env.admin, false) }, "settings_no_change_info")
	})

	t.Run("an administrator the owner gate refuses is refused at the press and at the typed text", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsMsgEnv(t)
		env.newOwner(env.chat)
		listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
		env.setAdminsAllowed(env.chat.Id, false)
		refusedPress(t, env, env.admin, settingsActMsgSet, "settings_gate_owner_only")
		env.setAdminsAllowed(env.chat.Id, true)
		typedEnds(t, env, env.admin, func() { env.setAdminsAllowed(env.chat.Id, false) }, "settings_gate_owner_only")
	})

	t.Run("a plain member is refused", func(t *testing.T) {
		env := newSettingsMsgEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 9_100_000, FirstName: "Member"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		refusedPress(t, env, member, settingsActMsgSet, "settings_not_admin")
	})

	t.Run("the live creator sets and removes", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsMsgEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		settingsMsgSetWait(t, env, owner, "bn", settingsEditPartText)
		sendPrivateMessage(env.settingsEnv, owner, gotgbot.Message{Text: "Gone"})
		if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); row.Text != "Gone" || row.UpdatedBy != owner.Id {
			t.Fatalf("the creator's typed text left %+v", row)
		}
		settingsMsgPrivateMenu(env, env.chat, owner).press("a", settingsActMsgRemove, settingsMsgEventField, "bn", "t", settingsEditPartText)
		if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); row.Text != "" {
			t.Errorf("the creator's Remove left the text %q", row.Text)
		}
	})
}

func TestSettingsDriftMessagesEdit(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	m := settingsMsgPrivateMenu(env, env.chat, env.admin)

	// A text stored straight through the repository is drawn with its length and counted Custom.
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventKick, "Kicked {mention}", false)
	text, _ := m.press("a", settingsActMsgEvent, settingsMsgEventField, "kk")
	if want := staffMarker("settings_edit_state_text_len") + " " + fmt.Sprint(len("Kicked {mention}")); !strings.Contains(text, want) {
		t.Errorf("the kick screen %q lacks the stored text's length", text)
	}
	hub, _ := m.press("a", settingsActOpen, "p", settingsPageMessages)
	settingsMsgWantStates(t, hub, map[string]string{eventmsg.EventKick: "settings_msg_state_custom"})

	// A page Remove is what the send path reads at once, after it had cached the old text.
	if got := settingsMsgSendPathText(t, env.chat.Id, eventmsg.EventKick); got != "Kicked {mention}" {
		t.Fatalf("the send path reads %q before the Remove", got)
	}
	m.press("a", settingsActMsgRemove, settingsMsgEventField, "kk", "t", settingsEditPartText)
	if got := settingsMsgSendPathText(t, env.chat.Id, eventmsg.EventKick); got != "" {
		t.Errorf("after the page Remove the send path still reads %q", got)
	}

	// A typed save is what the send path reads at once.
	settingsMsgSetWait(t, env, env.admin, "kk", settingsEditPartText)
	sendPrivateMessage(env.settingsEnv, env.admin, gotgbot.Message{Text: "Out you go"})
	if got := settingsMsgSendPathText(t, env.chat.Id, eventmsg.EventKick); got != "Out you go" {
		t.Errorf("after the typed save the send path reads %q, want the new text", got)
	}

	// Opening every screen of the page writes nothing.
	other := env.addChat()
	o := settingsMsgPrivateMenu(env, other, env.admin)
	o.press("a", settingsActOpen, "p", settingsPageMessages)
	o.press("a", settingsActMsgFamily, "t", "m")
	for _, ev := range settingsMsgManualEvents {
		o.press("a", settingsActMsgEvent, settingsMsgEventField, ev.Code)
	}
	if n := settingsMsgRows(t, other.Id); n != 0 {
		t.Errorf("opening every screen wrote %d event message rows, want none", n)
	}
}
