//go:build testtools

package modules

import (
	"context"
	"errors"
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

// settingsMsgSeedPM stores one event's private copy through the repository: its text when text is not empty, a photo
// and one Rules button when full is true. A part write that inserts the pm row leaves the copy off.
func settingsMsgSeedPM(t *testing.T, chatID int64, event, text string, full bool) {
	t.Helper()
	ctx := context.Background()
	if text != "" {
		if err := eventmsg.SetText(ctx, chatID, event, eventmsg.DestPM, text, 1); err != nil {
			t.Fatalf("seed the %s private text of chat %d: %v", event, chatID, err)
		}
	}
	if !full {
		return
	}
	if err := eventmsg.SetMedia(ctx, chatID, event, eventmsg.DestPM, "pm-photo", db.PHOTO, 1); err != nil {
		t.Fatalf("seed the %s private media of chat %d: %v", event, chatID, err)
	}
	if err := eventmsg.SetButtons(ctx, chatID, event, eventmsg.DestPM, []db.Button{{Name: "Rules", Url: "https://example.com/rules"}}, 1); err != nil {
		t.Fatalf("seed the %s private buttons of chat %d: %v", event, chatID, err)
	}
}

// settingsMsgSendCount is how many sends of every send method the bot has made to any chat so far.
func settingsMsgSendCount(env *settingsEnv) int {
	n := 0
	for _, method := range settingsEditSendMethods {
		n += len(env.fake.callsFor(method))
	}
	return n
}

// settingsMsgPMPreviewSeen presses a preview of the event code's private copy (hf with no part, hv with the part, both
// with d=p) in a fresh private menu of user and returns the press's answer and what the bot sent user's private chat
// since. It fails the test when the bot sent anything to any other chat.
func settingsMsgPMPreviewSeen(t *testing.T, env *settingsMsgEnv, user gotgbot.User, code, part string) (string, bool, []moduleBotCall) {
	t.Helper()
	mark, total := settingsEditMarkSends(env.settingsEnv, user.Id), settingsMsgSendCount(env.settingsEnv)
	kv := []string{"a", settingsActMsgFull, settingsMsgEventField, code, settingsMsgDestField, settingsMsgDestPMCode}
	if part != "" {
		kv = append(kv, "t", part)
		kv[1] = settingsActMsgPreview
	}
	env.pressPrivate(user, env.updateID(), env.privateFields(env.chat.Id, kv...))
	answer, alert := env.lastAnswer()
	sent := settingsEditNewSends(env.settingsEnv, user.Id, mark, false)
	if others := settingsMsgSendCount(env.settingsEnv) - total - len(sent); others != 0 {
		t.Errorf("the private copy preview of %s/%q sent %d messages to chats other than the presser's", code, part, others)
	}
	return answer, alert, sent
}

// settingsMsgPMPreviewContent fails the test unless the press was answered empty and sent exactly the private copy's
// header line and one content message, and returns the content's method and decoded parameters.
func settingsMsgPMPreviewContent(t *testing.T, env *settingsMsgEnv, user gotgbot.User, code, part string) (string, map[string]any) {
	t.Helper()
	answer, alert, sent := settingsMsgPMPreviewSeen(t, env, user, code, part)
	if answer != "" || alert {
		t.Fatalf("the private copy preview of %s/%q answered %q (alert %t), want an empty answer", code, part, answer, alert)
	}
	if len(sent) != 2 {
		t.Fatalf("the private copy preview of %s/%q sent %d messages, want the header and the content: %+v", code, part, len(sent), sent)
	}
	header := rulesCallParams(t, sent[0])
	if sent[0].Method != "sendMessage" || !containsMarker(fmt.Sprint(header["text"]), "settings_msg_preview_header_pm") {
		t.Errorf("the first message of the private copy preview of %s/%q is %s %v, want its header line", code, part, sent[0].Method, header["text"])
	}
	return sent[1].Method, rulesCallParams(t, sent[1])
}

func TestSettingsMessagesPMPreview(t *testing.T) {
	env := newSettingsMsgEnv(t)
	settingsEditFakeSends(env.settingsEnv)
	// An administrator without can_change_info: the previews need only the admin check every press makes.
	listEditAdmin(env.settingsEnv, env.chat, env.admin, false)
	admin := env.admin
	groupMark := settingsEditMarkSends(env.settingsEnv, env.chat.Id)
	queued := func() int64 {
		var n int64
		if err := db.DB.Model(&models.ScheduledDeletion{}).Where("chat_id IN ?", []int64{env.chat.Id, admin.Id}).Count(&n).Error; err != nil {
			t.Fatalf("count the queued deletions: %v", err)
		}
		return n
	}
	queuedBefore := queued()
	settingsMsgSeedPM(t, env.chat.Id, eventmsg.EventBan, "{first} / {admin} / {reason}", true)
	stored, _ := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan)
	filled := html.EscapeString(admin.FirstName) + " / " + html.EscapeString(formatting.GetFullName(admin.FirstName, admin.LastName)) + " / " +
		staffMarker("settings_msg_sample_reason")

	t.Run("full preview of a custom copy", func(t *testing.T) {
		// The copy is off: a preview shows it all the same.
		method, params := settingsMsgPMPreviewContent(t, env, admin, "bn", "")
		if got := fmt.Sprint(params["caption"]); method != "sendPhoto" || fmt.Sprint(params["photo"]) != "pm-photo" || got != filled {
			t.Errorf("Full preview of the ban's copy = %s %v caption %q, want sendPhoto pm-photo with %q", method, params["photo"], got, filled)
		}
		// The stored button alone: the group's Unban row is a live callback that never reaches a private chat.
		keyboard := settingsMsgSentKeyboard(t, params)
		if len(keyboard) != 1 || len(keyboard[0]) != 1 || keyboard[0][0].Text != "Rules" || keyboard[0][0].Url != "https://example.com/rules" ||
			keyboard[0][0].CallbackData != "" {
			t.Errorf("Full preview of the ban's copy carries %+v, want the stored Rules button alone", keyboard)
		}
	})

	t.Run("part previews", func(t *testing.T) {
		method, params := settingsMsgPMPreviewContent(t, env, admin, "bn", settingsEditPartText)
		if text := fmt.Sprint(params["text"]); method != "sendMessage" || text != filled || len(settingsMsgSentKeyboard(t, params)) != 0 {
			t.Errorf("Preview text of the ban's copy = %s %q with %+v, want %q and no keyboard", method, text, params["reply_markup"], filled)
		}
		method, params = settingsMsgPMPreviewContent(t, env, admin, "bn", settingsEditPartMedia)
		if _, captioned := params["caption"]; method != "sendPhoto" || fmt.Sprint(params["photo"]) != "pm-photo" || captioned {
			t.Errorf("Preview media of the ban's copy = %s %v, want the photo alone", method, params)
		}
		method, params = settingsMsgPMPreviewContent(t, env, admin, "bn", settingsEditPartButtons)
		keyboard := settingsMsgSentKeyboard(t, params)
		if method != "sendMessage" || !containsMarker(fmt.Sprint(params["text"]), "settings_edit_preview_buttons_body") ||
			len(keyboard) != 1 || keyboard[0][0].Text != "Rules" {
			t.Errorf("Preview buttons of the ban's copy = %s %q with %+v, want the buttons line and the Rules button", method, params["text"], keyboard)
		}
	})

	t.Run("timed", func(t *testing.T) {
		settingsMsgSeedPM(t, env.chat.Id, eventmsg.EventTimedBan, "{duration} until {until}", false)
		const layout = "02.01.2006 15:04"
		earliest := formatChatTime(time.Now().Add(settingsMsgSampleSpan), chatClockOf(env.chat.Id), layout)
		_, params := settingsMsgPMPreviewContent(t, env, admin, "tb", "")
		latest := formatChatTime(time.Now().Add(settingsMsgSampleSpan), chatClockOf(env.chat.Id), layout)
		if got := fmt.Sprint(params["text"]); got != "2 hours until "+earliest && got != "2 hours until "+latest {
			t.Errorf("Full preview of the timed ban's copy = %q, want 2 hours until %q", got, earliest)
		}
	})

	t.Run("rows", func(t *testing.T) {
		m := settingsMsgPrivateMenu(env, env.chat, admin)
		_, keyboard := settingsMsgOpenPM(t, m, "bn")
		settingsMsgWantWidths(t, "the full private copy screen", keyboard, 2, 2, 2, 2, 2, 1, 2)
		full, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgFull)
		if fields := settingsButtonFields(t, full); !ok || fields[settingsMsgEventField] != "bn" || fields[settingsMsgDestField] != settingsMsgDestPMCode ||
			fields["c"] == "" || len(fields) != 4 {
			t.Errorf("the private copy's Full preview carries %v, want a=hf, k=bn, d=p and c", fields)
		}
		_, keyboard = settingsMsgOpenPM(t, m, "kk")
		settingsMsgWantWidths(t, "the empty private copy screen", keyboard, 1, 1, 1, 1, 1, 2)
		if _, ok := settingsMsgButtonWith(t, keyboard, "a", settingsActMsgPreview); ok {
			t.Errorf("the empty private copy screen offers a part Preview: %+v", keyboard)
		}
		for code, want := range map[string]int{settingsActMsgPreview: 46, settingsActMsgFull: 42} {
			fields := map[string]string{"a": code, "c": "-1001234567890", settingsMsgEventField: "bn", settingsMsgDestField: settingsMsgDestPMCode}
			if code == settingsActMsgPreview {
				fields["t"] = settingsEditPartText
			}
			if data := encodeCallbackData(settingsCallbackNamespace, fields); len(data) != want {
				t.Errorf("the private copy's %s button data %q is %d bytes, want %d", code, data, len(data), want)
			}
		}
	})

	t.Run("rights", func(t *testing.T) {
		env.fake.script("sendPhoto", admin.Id, staffFakeError(403, "Forbidden: bot can't initiate conversation with a user"))
		answer, alert, sent := settingsMsgPMPreviewSeen(t, env, admin, "bn", "")
		if !alert || !containsMarker(answer, "settings_edit_preview_need_start") || len(sent) != 2 {
			t.Errorf("a 403 on the content answered %q (alert %t) after %d sends, want the start-the-bot alert after the header and the refused photo", answer, alert, len(sent))
		}
	})

	t.Run("full preview of the default", func(t *testing.T) {
		realEnv := &settingsMsgEnv{settingsEnv: newSettingsEnvWith(t, settingsEnvOptions{RealLocale: true})}
		settingsMsgCleanup(t, realEnv.chat.Id)
		listEditAdmin(realEnv.settingsEnv, realEnv.chat, realEnv.admin, false)
		settingsEditSetLanguages(t, realEnv.admin.Id, "en", realEnv.chat.Id, "es")
		if err := eventmsg.SetSilent(context.Background(), realEnv.chat.Id, eventmsg.EventBan, eventmsg.DestPM, false, 1); err != nil {
			t.Fatalf("switch the ban's private copy on: %v", err)
		}
		mark := settingsEditMarkSends(realEnv.settingsEnv, realEnv.admin.Id)
		realEnv.pressPrivate(realEnv.admin, realEnv.updateID(), realEnv.privateFields(realEnv.chat.Id,
			"a", settingsActMsgFull, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode))
		sent := settingsEditNewSends(realEnv.settingsEnv, realEnv.admin.Id, mark, false)
		if len(sent) != 2 {
			t.Fatalf("Full preview of the default private copy sent %d messages, want the header and the copy: %+v", len(sent), sent)
		}
		header, _ := i18n.MustNewTranslator("en").GetString("settings_msg_preview_header_pm")
		if got := fmt.Sprint(rulesCallParams(t, sent[0])["text"]); got != header {
			t.Errorf("the header line = %q, want the presser's English %q", got, header)
		}
		spanish := i18n.MustNewTranslator("es")
		reason, _ := spanish.GetString("settings_msg_sample_reason")
		spec, _ := eventPMSpecFor(eventmsg.EventBan)
		// The press's group is the getChat answer of the fake, whose title is "Test Chat".
		group := gotgbot.Chat{Id: realEnv.chat.Id, Type: "supergroup", Title: "Test Chat"}
		want := eventPMDefaultText(spanish, spec, &group, eventValues{Reason: reason}, "")
		params := rulesCallParams(t, sent[1])
		if got := fmt.Sprint(params["text"]); sent[1].Method != "sendMessage" || got != want {
			t.Errorf("Full preview of the default private copy = %s %q, want the Spanish default private text %q", sent[1].Method, got, want)
		}
		if keyboard := settingsMsgSentKeyboard(t, params); len(keyboard) != 0 {
			t.Errorf("Full preview of the default private copy carries %+v, want no keyboard", keyboard)
		}
	})

	// Nothing reached the group, nothing was written, queued or waited for.
	if calls := settingsEditNewSends(env.settingsEnv, env.chat.Id, groupMark, false); len(calls) != 0 {
		t.Errorf("the previews sent %d messages into the group, want none: %+v", len(calls), calls)
	}
	if after, _ := settingsMsgStoredPM(t, env.chat.Id, eventmsg.EventBan); !after.UpdatedAt.Equal(stored.UpdatedAt) || after.Text != stored.Text || !after.Silent {
		t.Errorf("the previews wrote the ban's pm row: %+v, before %+v", after, stored)
	}
	if n := settingsMsgRows(t, env.chat.Id); n != 2 {
		t.Errorf("the chat has %d event message rows after the previews, want the 2 seeded", n)
	}
	if got := queued() - queuedBefore; got != 0 {
		t.Errorf("the previews queued %d deletions, want none", got)
	}
	if env.redis.Exists(settingsInputKey(admin.Id)) {
		t.Error("the previews left a wait")
	}
}

func TestSettingsMessagesPMStates(t *testing.T) {
	env := newSettingsMsgEnv(t)
	ctx := context.Background()
	// lines returns the hub's and the family list's line of the event named by nameKey, and Main's Messages line.
	lines := func(t *testing.T, nameKey string) (string, string, string) {
		t.Helper()
		m := settingsMsgPrivateMenu(env, env.chat, env.admin)
		mainText, _ := m.press("a", settingsActBack)
		mainLine, ok := settingsMainStateLine(mainText, "settings_page_msgs")
		if !ok {
			t.Fatalf("Main has no Messages line: %q", mainText)
		}
		hubText, _ := m.press("a", settingsActOpen, "p", settingsPageMessages)
		hubLine, ok := settingsMainStateLine(hubText, nameKey)
		if !ok {
			t.Fatalf("the hub has no %s line: %q", nameKey, hubText)
		}
		familyText, _ := m.press("a", settingsActMsgFamily, "t", "m")
		familyLine, ok := settingsMainStateLine(familyText, nameKey)
		if !ok {
			t.Fatalf("the family list has no %s line: %q", nameKey, familyText)
		}
		return hubLine, familyLine, mainLine
	}
	wantLine := func(t *testing.T, what, line, state string, marked bool) {
		t.Helper()
		if !containsMarker(line, state) || strings.HasSuffix(line, settingsMsgPMSuffix) != marked {
			t.Errorf("%s = %q, want %s with the private copy mark %t", what, line, state, marked)
		}
	}
	wantCount := func(t *testing.T, line string, n int) {
		t.Helper()
		if want := staffMarker("settings_state_msg_n") + " " + strconv.Itoa(n); !strings.Contains(line, want) {
			t.Errorf("Main's Messages line = %q, want %q", line, want)
		}
	}

	t.Run("a copy that is on marks its line and counts", func(t *testing.T) {
		if err := eventmsg.SetSilent(ctx, env.chat.Id, eventmsg.EventBan, eventmsg.DestPM, false, 1); err != nil {
			t.Fatalf("switch the ban's private copy on: %v", err)
		}
		hub, family, main := lines(t, "settings_msg_ev_ban")
		wantLine(t, "the hub's ban line", hub, "settings_state_default", true)
		wantLine(t, "the family list's ban line", family, "settings_state_default", true)
		wantCount(t, main, 1)
		if !strings.HasSuffix(hub, " · 📩") {
			t.Errorf("the hub's ban line %q does not end with the private copy mark", hub)
		}
	})

	t.Run("a stored copy that is off neither marks nor counts", func(t *testing.T) {
		settingsMsgSeedPM(t, env.chat.Id, eventmsg.EventKick, "You were kicked", false)
		hub, family, main := lines(t, "settings_msg_ev_kick")
		wantLine(t, "the hub's kick line", hub, "settings_state_default", false)
		wantLine(t, "the family list's kick line", family, "settings_state_default", false)
		wantCount(t, main, 1)
	})

	t.Run("a custom group message with its copy on counts once", func(t *testing.T) {
		settingsMsgSeed(t, env.chat.Id, eventmsg.EventBan, "Banned {mention}", false)
		hub, family, main := lines(t, "settings_msg_ev_ban")
		wantLine(t, "the hub's ban line", hub, "settings_msg_state_custom", true)
		wantLine(t, "the family list's ban line", family, "settings_msg_state_custom", true)
		wantCount(t, main, 1)
	})

	t.Run("a failed read is the unknown mark on Main", func(t *testing.T) {
		mreqSwap(t, &settingsMsgRead, func(context.Context, int64) (*eventmsg.Messages, error) {
			return nil, errors.New("event messages are unreadable")
		})
		m := settingsMsgPrivateMenu(env, env.chat, env.admin)
		text, _ := m.press("a", settingsActBack)
		if line, ok := settingsMainStateLine(text, "settings_page_msgs"); !ok || !strings.HasSuffix(line, settingsUnknownState) {
			t.Errorf("Main's Messages line with a failing read = %q, want the unknown mark", line)
		}
	})
}
