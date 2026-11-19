//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// settingsMsgPreviewSeen presses a preview of the event code (hf with no part, hv with the part) in a fresh
// private menu of user and returns the press's answer and what the bot sent user's private chat since: the
// header line first, then the content.
func settingsMsgPreviewSeen(t *testing.T, env *settingsMsgEnv, user gotgbot.User, code, part string) (string, bool, []moduleBotCall) {
	t.Helper()
	mark := settingsEditMarkSends(env.settingsEnv, user.Id)
	kv := []string{"a", settingsActMsgFull, settingsMsgEventField, code}
	if part != "" {
		kv = []string{"a", settingsActMsgPreview, settingsMsgEventField, code, "t", part}
	}
	env.pressPrivate(user, env.updateID(), env.privateFields(env.chat.Id, kv...))
	answer, alert := env.lastAnswer()
	return answer, alert, settingsEditNewSends(env.settingsEnv, user.Id, mark, false)
}

// settingsMsgPreviewContent fails the test unless the press was answered empty and sent exactly the header
// line and one content message, and returns the content's method and decoded parameters.
func settingsMsgPreviewContent(t *testing.T, env *settingsMsgEnv, user gotgbot.User, code, part string) (string, map[string]any) {
	t.Helper()
	answer, alert, sent := settingsMsgPreviewSeen(t, env, user, code, part)
	if answer != "" || alert {
		t.Fatalf("the preview of %s/%q answered %q (alert %t), want an empty answer", code, part, answer, alert)
	}
	if len(sent) != 2 {
		t.Fatalf("the preview of %s/%q sent %d messages, want the header and the content: %+v", code, part, len(sent), sent)
	}
	header := rulesCallParams(t, sent[0])
	if sent[0].Method != "sendMessage" || !containsMarker(fmt.Sprint(header["text"]), "settings_msg_preview_header") {
		t.Errorf("the first message of the preview of %s/%q is %s %v, want the header line", code, part, sent[0].Method, header["text"])
	}
	return sent[1].Method, rulesCallParams(t, sent[1])
}

// settingsMsgSentKeyboard is the inline keyboard of decoded send parameters (rulesCallParams), nil when there
// is none or it is empty.
func settingsMsgSentKeyboard(t *testing.T, params map[string]any) [][]gotgbot.InlineKeyboardButton {
	t.Helper()
	value, ok := params["reply_markup"]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode the keyboard: %v", err)
	}
	var markup gotgbot.InlineKeyboardMarkup
	if err := json.Unmarshal(raw, &markup); err != nil {
		t.Fatalf("decode the keyboard %s: %v", raw, err)
	}
	return markup.InlineKeyboard
}

func TestSettingsMessagesPreview(t *testing.T) {
	env := newSettingsMsgEnv(t)
	settingsEditFakeSends(env.settingsEnv)
	// An administrator without can_change_info: the previews need only the admin check every press makes.
	listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
	admin := env.admin
	groupMark := settingsEditMarkSends(env.settingsEnv, env.chat.Id)
	deletes := len(env.fake.callsFor("deleteMessage"))
	queued := func() int64 {
		var n int64
		if err := db.DB.Model(&models.ScheduledDeletion{}).Where("chat_id IN ?", []int64{env.chat.Id, admin.Id}).Count(&n).Error; err != nil {
			t.Fatalf("count the queued deletions: %v", err)
		}
		return n
	}
	queuedBefore := queued()

	t.Run("the screen offers Preview of each set part and Full preview, within 64 bytes", func(t *testing.T) {
		m := settingsMsgPrivateMenu(env, env.chat, admin)
		_, keyboard := m.press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
		full, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgFull)
		if fields := settingsButtonFields(t, full); !ok || fields[settingsMsgEventField] != "bn" || fields["c"] == "" || len(fields) != 3 {
			t.Errorf("the empty ban screen's Full preview carries %v, want a=hf, k=bn and c", fields)
		}
		if _, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgPreview); ok {
			t.Errorf("the empty ban screen offers a part Preview: %+v", keyboard)
		}
		for code, want := range map[string]int{settingsActMsgPreview: 42, settingsActMsgFull: 38} {
			fields := map[string]string{"a": code, "c": "-1001234567890", settingsMsgEventField: "bn"}
			if code == settingsActMsgPreview {
				fields["t"] = settingsEditPartText
			}
			if data := encodeCallbackData(settingsCallbackNamespace, fields); len(data) != want {
				t.Errorf("the %s button's data %q is %d bytes, want %d", code, data, len(data), want)
			}
		}
	})

	t.Run("a custom text is filled with the presser and the sample values, with no keyboard", func(t *testing.T) {
		settingsMsgSeed(t, env.chat.Id, eventmsg.EventBan, "{first} / {admin} / {reason} / {duration}", false)
		method, params := settingsMsgPreviewContent(t, env, admin, "bn", "")
		name := html.EscapeString(admin.FirstName)
		want := name + " / " + html.EscapeString(formatting.GetFullName(admin.FirstName, admin.LastName)) + " / " +
			staffMarker("settings_msg_sample_reason") + " / "
		if got := fmt.Sprint(params["text"]); method != "sendMessage" || got != want {
			t.Errorf("Full preview of the ban = %s %q, want sendMessage %q", method, got, want)
		}
		// The ban's own Unban row is a live callback and is never composed into a private chat.
		if keyboard := settingsMsgSentKeyboard(t, params); len(keyboard) != 0 {
			t.Errorf("Full preview of the ban carries a keyboard: %+v", keyboard)
		}
		if _, text := settingsMsgPreviewContent(t, env, admin, "bn", settingsEditPartText); fmt.Sprint(text["text"]) != want {
			t.Errorf("Preview text of the ban = %q, want %q", text["text"], want)
		}
	})

	t.Run("a timed event fills the sample duration and an end two hours ahead", func(t *testing.T) {
		settingsMsgSeed(t, env.chat.Id, eventmsg.EventTimedBan, "{duration} until {until}", false)
		const layout = "02.01.2006 15:04"
		earliest := formatChatTime(time.Now().Add(settingsMsgSampleSpan), chatClockOf(env.chat.Id), layout)
		_, params := settingsMsgPreviewContent(t, env, admin, "tb", "")
		latest := formatChatTime(time.Now().Add(settingsMsgSampleSpan), chatClockOf(env.chat.Id), layout)
		got := fmt.Sprint(params["text"])
		if got != "2 hours until "+earliest && got != "2 hours until "+latest {
			t.Errorf("Full preview of the timed ban = %q, want 2 hours until %q", got, earliest)
		}
	})

	t.Run("a photo and a button are one send with the filled caption and the stored button alone", func(t *testing.T) {
		settingsMsgSeedFull(t, env.chat.Id, eventmsg.EventMute)
		stored, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute)
		defer func() {
			if after, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventMute); !after.UpdatedAt.Equal(stored.UpdatedAt) || after.Text != stored.Text {
				t.Errorf("the previews wrote the mute row: %+v, before %+v", after, stored)
			}
		}()
		method, params := settingsMsgPreviewContent(t, env, admin, "mu", "")
		caption := fmt.Sprint(params["caption"])
		if method != "sendPhoto" || fmt.Sprint(params["photo"]) != "mute-photo" ||
			!strings.HasPrefix(caption, "Muted ") || !strings.Contains(caption, strconv.FormatInt(admin.Id, 10)) {
			t.Errorf("Full preview of the mute = %s %v caption %q, want sendPhoto mute-photo with the filled caption", method, params["photo"], caption)
		}
		keyboard := settingsMsgSentKeyboard(t, params)
		if len(keyboard) != 1 || len(keyboard[0]) != 1 || keyboard[0][0].Text != "Rules" || keyboard[0][0].Url != "https://example.com/rules" {
			t.Errorf("Full preview of the mute carries %+v, want the stored Rules button alone", keyboard)
		}

		method, params = settingsMsgPreviewContent(t, env, admin, "mu", settingsEditPartText)
		if text := fmt.Sprint(params["text"]); method != "sendMessage" || !strings.HasPrefix(text, "Muted ") || len(settingsMsgSentKeyboard(t, params)) != 0 {
			t.Errorf("Preview text of the mute = %s %q with %+v, want the filled text and no keyboard", method, text, params["reply_markup"])
		}
		method, params = settingsMsgPreviewContent(t, env, admin, "mu", settingsEditPartMedia)
		if _, captioned := params["caption"]; method != "sendPhoto" || fmt.Sprint(params["photo"]) != "mute-photo" || captioned {
			t.Errorf("Preview media of the mute = %s %v, want the photo alone", method, params)
		}
		method, params = settingsMsgPreviewContent(t, env, admin, "mu", settingsEditPartButtons)
		keyboard = settingsMsgSentKeyboard(t, params)
		if method != "sendMessage" || !containsMarker(fmt.Sprint(params["text"]), "settings_edit_preview_buttons_body") ||
			len(keyboard) != 1 || keyboard[0][0].Text != "Rules" {
			t.Errorf("Preview buttons of the mute = %s %q with %+v, want the buttons line and the Rules button", method, params["text"], keyboard)
		}
	})

	t.Run("Silent on still previews the message the event would post", func(t *testing.T) {
		settingsMsgSeed(t, env.chat.Id, eventmsg.EventKick, "Kicked {first}", true)
		if _, params := settingsMsgPreviewContent(t, env, admin, "kk", ""); fmt.Sprint(params["text"]) != "Kicked "+html.EscapeString(admin.FirstName) {
			t.Errorf("Full preview of the silent kick = %q, want the stored text filled", params["text"])
		}
	})

	t.Run("a stale Preview of an unset part sends nothing", func(t *testing.T) {
		answer, alert, sent := settingsMsgPreviewSeen(t, env, admin, "um", settingsEditPartMedia)
		if !alert || !containsMarker(answer, "settings_edit_nothing_to_preview") || len(sent) != 0 {
			t.Errorf("a stale Preview answered %q (alert %t) and sent %d messages, want the nothing-to-preview alert and none", answer, alert, len(sent))
		}
	})

	t.Run("a presser who never started the bot is asked to", func(t *testing.T) {
		env.fake.script("sendMessage", admin.Id, staffFakeError(403, "Forbidden: bot can't initiate conversation with a user"))
		answer, alert, sent := settingsMsgPreviewSeen(t, env, admin, "ub", "")
		if !alert || !containsMarker(answer, "settings_edit_preview_need_start") || len(sent) != 1 {
			t.Errorf("a 403 answered %q (alert %t) after %d sends, want the start-the-bot alert after the refused header", answer, alert, len(sent))
		}
	})

	t.Run("the default text is the send site's own, in the group's language, with the header in the presser's", func(t *testing.T) {
		realEnv := &settingsMsgEnv{settingsEnv: newSettingsEnvWith(t, settingsEnvOptions{RealLocale: true})}
		settingsMsgCleanup(t, realEnv.chat.Id)
		listEditAdmin(realEnv.settingsEnv, realEnv.chat, realEnv.admin, false)
		settingsEditSetLanguages(t, realEnv.admin.Id, "en", realEnv.chat.Id, "es")
		mark := settingsEditMarkSends(realEnv.settingsEnv, realEnv.admin.Id)
		realEnv.pressPrivate(realEnv.admin, realEnv.updateID(), realEnv.privateFields(realEnv.chat.Id, "a", settingsActMsgFull, settingsMsgEventField, "bn"))
		sent := settingsEditNewSends(realEnv.settingsEnv, realEnv.admin.Id, mark, false)
		if len(sent) != 2 {
			t.Fatalf("Full preview of the default ban sent %d messages, want the header and the message: %+v", len(sent), sent)
		}
		header, _ := i18n.MustNewTranslator("en").GetString("settings_msg_preview_header")
		if got := fmt.Sprint(rulesCallParams(t, sent[0])["text"]); got != header {
			t.Errorf("the header line = %q, want the presser's English %q", got, header)
		}
		spanish := i18n.MustNewTranslator("es")
		reason, _ := spanish.GetString("settings_msg_sample_reason")
		want, _ := banDefaultReply(spanish, eventDefaultInput{Target: realEnv.admin, Reason: reason})
		params := rulesCallParams(t, sent[1])
		got := fmt.Sprint(params["text"])
		if got != want || !strings.Contains(got, formatting.MentionHtml(realEnv.admin.Id, realEnv.admin.FirstName)) || !strings.Contains(got, reason) {
			t.Errorf("Full preview of the default ban = %q, want the Spanish default %q with the mention and the sample reason", got, want)
		}
		if keyboard := settingsMsgSentKeyboard(t, params); len(keyboard) != 0 {
			t.Errorf("Full preview of the default ban carries %+v, want no keyboard", keyboard)
		}
	})

	// Nothing reached the group, nothing was written, queued or deleted, and no wait was made.
	if calls := settingsEditNewSends(env.settingsEnv, env.chat.Id, groupMark, false); len(calls) != 0 {
		t.Errorf("the previews sent %d messages into the group, want none: %+v", len(calls), calls)
	}
	if n := settingsMsgRows(t, env.chat.Id); n != 4 {
		t.Errorf("the chat has %d event message rows after the previews, want the 4 seeded", n)
	}
	if got := queued() - queuedBefore; got != 0 {
		t.Errorf("the previews queued %d deletions, want none", got)
	}
	if got := len(env.fake.callsFor("deleteMessage")) - deletes; got != 0 {
		t.Errorf("the previews deleted %d messages, want none", got)
	}
	if env.redis.Exists(settingsInputKey(admin.Id)) {
		t.Error("the previews left a wait")
	}
}
