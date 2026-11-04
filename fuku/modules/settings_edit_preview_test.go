//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsEditFakeSends gives the Bot API fake a Message answer for every media send method a
// stored greeting can use. The fake answers only sendMessage, sendPhoto and sendDocument with a
// message by itself, and the other methods with a bare true, which the send path cannot read.
func settingsEditFakeSends(env *settingsEnv) {
	env.t.Helper()
	answer := json.RawMessage(`{"message_id":9100,"date":1,"chat":{"id":1,"type":"private"}}`)
	client := env.fake.moduleBotClient
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, method := range []string{
		"sendSticker", "sendVideo", "sendAudio", "sendVoice", "sendAnimation", "sendVideoNote",
	} {
		client.responses[method] = answer
	}
}

// settingsEditTitleClient wraps the env's Bot API fake so that getChat names the group with the
// title a test chose (the fake answers "Test Chat" for every group).
type settingsEditTitleClient struct {
	gotgbot.BotClient

	title string
}

// RequestWithContext implements gotgbot.BotClient.
func (c *settingsEditTitleClient) RequestWithContext(
	ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	raw, err := c.BotClient.RequestWithContext(ctx, token, method, params, opts)
	if err != nil || method != "getChat" {
		return raw, err
	}
	// The chat ID is far beyond 2^53 in these tests, so every other field stays raw JSON.
	var chat map[string]json.RawMessage
	if json.Unmarshal(raw, &chat) != nil {
		return raw, nil
	}
	title, marshalErr := json.Marshal(c.title)
	if marshalErr != nil {
		return raw, nil
	}
	chat["title"] = title
	out, marshalErr := json.Marshal(chat)
	if marshalErr != nil {
		return raw, nil
	}
	return out, nil
}

// settingsEditNameGroup makes getChat answer title for every group of the env.
func settingsEditNameGroup(env *settingsEnv, title string) {
	env.t.Helper()
	env.bot.BotClient = &settingsEditTitleClient{BotClient: env.bot.BotClient, title: title}
}

// settingsEditPreviewSentTo returns what the bot sent to chatID after the first skip messages,
// as the sendMessage calls it made, in order.
func settingsEditPreviewSentTo(env *settingsEnv, chatID int64, skip int) []staffSentMessage {
	env.t.Helper()
	sent := env.fake.sentTo(chatID)
	if len(sent) < skip {
		env.t.Fatalf("the bot sent %d messages to %d, want at least %d", len(sent), chatID, skip)
	}
	return sent[skip:]
}

func TestSettingsEditFullPreview(t *testing.T) {
	env := newSettingsEditEnv(t)
	settingsEditFakeSends(env)
	settingsEditNameGroup(env, "Fuku & Co")
	owner := env.newOwner(env.chat)

	buttons := []models.Button{{Name: "Rules", Url: "https://example.com"}}
	if err := greetings.SetWelcomeText(env.chat.Id, "Hi {first} in {chatname}", "", buttons, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText: %v", err)
	}
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)

	_, keyboard := env.lastEdit(owner.Id, menuID)
	if !settingsEditHasButton(keyboard, "settings_btn_edit_full_preview") {
		t.Fatalf("the Welcome page has no Full preview button: %+v", keyboard)
	}

	before := len(env.fake.sentTo(owner.Id))
	groupBefore := len(env.fake.sentTo(env.chat.Id))
	answers := env.answerCount()
	fields := settingsEditPressButton(env, owner, menuID, "settings_btn_edit_full_preview")
	if fields["a"] != "wf" || fields["c"] != strconv.FormatInt(env.chat.Id, 10) || len(fields) != 2 {
		t.Fatalf("the Full preview button carries %v, want wf and the group %d in c", fields, env.chat.Id)
	}

	sent := settingsEditPreviewSentTo(env, owner.Id, before)
	if len(sent) != 2 {
		t.Fatalf("the bot sent %d messages to the owner, want the header and the message: %+v", len(sent), sent)
	}
	if got := fmt.Sprint(sent[0].Params["text"]); !containsMarker(got, "settings_edit_preview_header") {
		t.Errorf("the first message is %q, want the preview header", got)
	}
	if got, want := fmt.Sprint(sent[1].Params["text"]), "Hi Owner in Fuku &amp; Co"; got != want {
		t.Errorf("the previewed welcome = %q, want %q", got, want)
	}
	rows := staffKeyboardOf(sent[1].Params["reply_markup"])
	if len(rows) != 1 || len(rows[0]) != 1 || rows[0][0].Text != "Rules" || rows[0][0].Url != "https://example.com" {
		t.Errorf("the previewed keyboard = %+v, want the one Rules button", rows)
	}

	if got := len(env.fake.sentTo(env.chat.Id)) - groupBefore; got != 0 {
		t.Errorf("a preview sent %d messages into the group, want none", got)
	}
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press was answered %d times, want once", got)
	}
	calls := env.fake.callsFor("answerCallbackQuery")
	if text, ok := calls[len(calls)-1].Params["text"]; ok && fmt.Sprint(text) != "" {
		t.Errorf("a private-menu preview answered %q, want an empty answer", text)
	}
}

// settingsEditPreviewButtons is the two buttons the preview tests store on a greeting.
var settingsEditPreviewButtons = []models.Button{
	{Name: "One", Url: "https://one.example"},
	{Name: "Two", Url: "https://two.example", SameLine: true},
}

// settingsEditSeedPreview stores a greeting of the page with a text, a media of the given type and
// the two buttons, for chatID.
func settingsEditSeedPreview(t *testing.T, pageKey string, chatID int64, mediaType int, fileID string) {
	t.Helper()
	var err error
	if pageKey == settingsPageWelcome {
		err = greetings.SetWelcomeText(chatID, "Hi {first}", fileID, settingsEditPreviewButtons, mediaType)
	} else {
		err = greetings.SetGoodbyeText(chatID, "Bye {first}", fileID, settingsEditPreviewButtons, mediaType)
	}
	if err != nil {
		t.Fatalf("seed the %s greeting: %v", pageKey, err)
	}
}

// settingsEditPreviewPress presses the preview action code of a private menu with the extra key,
// value pairs and returns nothing: a test reads what the bot sent and answered.
func settingsEditPreviewPress(env *settingsEnv, owner gotgbot.User, msgID int64, code string, kv ...string) {
	env.t.Helper()
	env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, append([]string{"a", code}, kv...)...))
}

func TestSettingsEditPreview(t *testing.T) {
	pages := []struct {
		key, previewCode, mediaMethod, mediaParam, text string
		mediaType                                       int
	}{
		{settingsPageWelcome, "wv", "sendPhoto", "photo", "Hi Owner", db.PHOTO},
		{settingsPageGoodbye, "gv", "sendVideo", "video", "Bye Owner", db.VIDEO},
	}
	for _, page := range pages {
		t.Run(page.key, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			settingsEditFakeSends(env)
			owner := env.newOwner(env.chat)
			settingsEditSeedPreview(t, page.key, env.chat.Id, page.mediaType, "file-1")
			menuID := settingsEditPrivateMenu(env, owner, page.key)
			groupBefore := len(env.fake.sentTo(env.chat.Id))

			t.Run("text alone", func(t *testing.T) {
				before := len(env.fake.sentTo(owner.Id))
				fields := settingsEditPressButton(env, owner, menuID, "settings_btn_edit_preview_text")
				if fields["a"] != page.previewCode || fields["t"] != "x" || len(fields) != 3 {
					t.Fatalf("Preview text carries %v, want %s, t=x and c", fields, page.previewCode)
				}
				sent := settingsEditPreviewSentTo(env, owner.Id, before)
				if len(sent) != 2 || !containsMarker(fmt.Sprint(sent[0].Params["text"]), "settings_edit_preview_header") {
					t.Fatalf("the bot sent %+v, want the header and the text", sent)
				}
				if got := fmt.Sprint(sent[1].Params["text"]); got != page.text {
					t.Errorf("the previewed text = %q, want %q", got, page.text)
				}
				if rows := staffKeyboardOf(sent[1].Params["reply_markup"]); len(rows) != 0 {
					t.Errorf("a text preview carries the keyboard %+v, want none", rows)
				}
				if got := len(callsToChat(env.fake.staffBotClient, page.mediaMethod, owner.Id)); got != 0 {
					t.Errorf("a text preview made %d %s calls, want none", got, page.mediaMethod)
				}
			})

			t.Run("media alone", func(t *testing.T) {
				before := len(env.fake.sentTo(owner.Id))
				settingsEditPressButton(env, owner, menuID, "settings_btn_edit_preview_media")
				sent := settingsEditPreviewSentTo(env, owner.Id, before)
				if len(sent) != 1 || !containsMarker(fmt.Sprint(sent[0].Params["text"]), "settings_edit_preview_header") {
					t.Fatalf("the bot sent %+v, want only the header as a message", sent)
				}
				calls := callsToChat(env.fake.staffBotClient, page.mediaMethod, owner.Id)
				if len(calls) != 1 {
					t.Fatalf("the bot made %d %s calls to the owner, want 1", len(calls), page.mediaMethod)
				}
				if got := fmt.Sprint(calls[0].Params[page.mediaParam]); got != "file-1" {
					t.Errorf("the media sent is %q, want the stored file", got)
				}
				if caption, ok := calls[0].Params["caption"]; ok && fmt.Sprint(caption) != "" {
					t.Errorf("a media preview carries the caption %q, want none", caption)
				}
				if rows := staffKeyboardOf(calls[0].Params["reply_markup"]); len(rows) != 0 {
					t.Errorf("a media preview carries the keyboard %+v, want none", rows)
				}
			})

			t.Run("buttons alone", func(t *testing.T) {
				before := len(env.fake.sentTo(owner.Id))
				settingsEditPressButton(env, owner, menuID, "settings_btn_edit_preview_buttons")
				sent := settingsEditPreviewSentTo(env, owner.Id, before)
				if len(sent) != 2 || !containsMarker(fmt.Sprint(sent[1].Params["text"]), "settings_edit_preview_buttons_body") {
					t.Fatalf("the bot sent %+v, want the header and the buttons body", sent)
				}
				rows := staffKeyboardOf(sent[1].Params["reply_markup"])
				if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0].Text != "One" || rows[0][1].Url != "https://two.example" {
					t.Errorf("the previewed keyboard = %+v, want One and Two on one row", rows)
				}
			})

			if got := len(env.fake.sentTo(env.chat.Id)) - groupBefore; got != 0 {
				t.Errorf("the previews sent %d messages into the group, want none", got)
			}
		})
	}

	t.Run("every media kind goes out through its own send method", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		settingsEditFakeSends(env)
		owner := env.newOwner(env.chat)
		settingsEditSeedPreview(t, settingsPageWelcome, env.chat.Id, db.PHOTO, "file-1")
		menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
		for _, kind := range []struct {
			mediaType     int
			method, param string
		}{
			{db.PHOTO, "sendPhoto", "photo"},
			{db.VIDEO, "sendVideo", "video"},
			{db.DOCUMENT, "sendDocument", "document"},
			{db.STICKER, "sendSticker", "sticker"},
			{db.AUDIO, "sendAudio", "audio"},
			{db.VOICE, "sendVoice", "voice"},
			{db.VIDEO_NOTE, "sendVideoNote", "video_note"},
		} {
			settingsEditSeedPreview(t, settingsPageWelcome, env.chat.Id, kind.mediaType, "file-"+kind.method)
			before := len(callsToChat(env.fake.staffBotClient, kind.method, owner.Id))
			settingsEditPreviewPress(env, owner, menuID, "wv", "t", "m")
			calls := callsToChat(env.fake.staffBotClient, kind.method, owner.Id)
			if len(calls) != before+1 {
				t.Errorf("%s: the bot made %d calls, want %d", kind.method, len(calls), before+1)
				continue
			}
			last := calls[len(calls)-1]
			if got := fmt.Sprint(last.Params[kind.param]); got != "file-"+kind.method {
				t.Errorf("%s: the media sent is %q", kind.method, got)
			}
			if caption, ok := last.Params["caption"]; ok && fmt.Sprint(caption) != "" {
				t.Errorf("%s: carries the caption %q, want none", kind.method, caption)
			}
		}
	})
}

func TestSettingsEditRulesPreview(t *testing.T) {
	stored := "<b>Be kind</b>"

	newRulesEnv := func(t *testing.T) (*settingsEnv, gotgbot.User) {
		env := newSettingsEditEnv(t)
		settingsEditNameGroup(env, env.chat.Title)
		return env, env.newOwner(env.chat)
	}

	t.Run("rules that are set", func(t *testing.T) {
		env, owner := newRulesEnv(t)
		if err := rules.SetChatRules(env.chat.Id, stored); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		menuID := settingsEditPrivateMenu(env, owner, settingsPageRules)

		before := len(env.fake.sentTo(owner.Id))
		fields := settingsEditPressButton(env, owner, menuID, "settings_btn_edit_preview_text")
		if fields["a"] != "rv" || fields["t"] != "x" {
			t.Fatalf("Preview text carries %v, want rv and t=x", fields)
		}
		sent := settingsEditPreviewSentTo(env, owner.Id, before)
		if len(sent) != 2 || !containsMarker(fmt.Sprint(sent[0].Params["text"]), "settings_edit_preview_header_rules") {
			t.Fatalf("the bot sent %+v, want the rules header and the text", sent)
		}
		if got := fmt.Sprint(sent[1].Params["text"]); got != stored {
			t.Errorf("the previewed rules = %q, want the stored text %q", got, stored)
		}

		// Full preview is the /rules message itself, also while private rules is on.
		tr := i18n.MustNewTranslator("en")
		want := buildRulesText(tr, env.chat.Title, stored)
		for _, private := range []bool{false, true} {
			if err := rules.SetPrivateRules(env.chat.Id, private); err != nil {
				t.Fatalf("SetPrivateRules(%t): %v", private, err)
			}
			before = len(env.fake.sentTo(owner.Id))
			fullFields := settingsEditPressButton(env, owner, menuID, "settings_btn_edit_full_preview")
			if fullFields["a"] != "rf" || len(fullFields) != 2 {
				t.Fatalf("Full preview carries %v, want rf and c", fullFields)
			}
			sent = settingsEditPreviewSentTo(env, owner.Id, before)
			if len(sent) != 2 || !containsMarker(fmt.Sprint(sent[0].Params["text"]), "settings_edit_preview_header_rules") {
				t.Fatalf("private=%t: the bot sent %+v, want the rules header and the message", private, sent)
			}
			if got := fmt.Sprint(sent[1].Params["text"]); got != want {
				t.Errorf("private=%t: Full preview = %q, want buildRulesText's %q", private, got, want)
			}
		}

		// The same chat's /rules answer is that very text.
		if err := rules.SetPrivateRules(env.chat.Id, false); err != nil {
			t.Fatalf("SetPrivateRules: %v", err)
		}
		groupBefore := len(env.fake.sentTo(env.chat.Id))
		env.send(env.admin, "/rules")
		reply := settingsEditPreviewSentTo(env, env.chat.Id, groupBefore)
		if len(reply) != 1 || fmt.Sprint(reply[0].Params["text"]) != want {
			t.Errorf("/rules sent %+v, want the text Full preview sends: %q", reply, want)
		}
	})

	t.Run("no rules", func(t *testing.T) {
		env, owner := newRulesEnv(t)
		menuID := settingsEditPrivateMenu(env, owner, settingsPageRules)
		_, keyboard := env.lastEdit(owner.Id, menuID)
		if settingsEditHasButton(keyboard, "settings_btn_edit_preview_text") {
			t.Error("the Rules page draws Preview text with no rules set")
		}
		before := len(env.fake.sentTo(owner.Id))
		settingsEditPressButton(env, owner, menuID, "settings_btn_edit_full_preview")
		sent := settingsEditPreviewSentTo(env, owner.Id, before)
		if len(sent) != 2 || !containsMarker(fmt.Sprint(sent[1].Params["text"]), "rules_no_rules_set") {
			t.Fatalf("the bot sent %+v, want the rules header and the no-rules line", sent)
		}
	})
}

func TestSettingsEditPreviewWritesNothing(t *testing.T) {
	env := newSettingsEditEnv(t)
	settingsEditFakeSends(env)
	owner := env.newOwner(env.chat)
	settingsEditSeedPreview(t, settingsPageWelcome, env.chat.Id, db.PHOTO, "file-1")
	settingsEditSeedPreview(t, settingsPageGoodbye, env.chat.Id, db.VIDEO, "file-2")
	for _, step := range []func() error{
		func() error { return greetings.SetCleanWelcomeSetting(env.chat.Id, true) },
		func() error { return greetings.SetCleanWelcomeMsgId(env.chat.Id, 4242) },
		func() error { return greetings.SetCleanGoodbyeSetting(env.chat.Id, true) },
		func() error { return greetings.SetCleanGoodbyeMsgId(env.chat.Id, 4343) },
	} {
		if err := step(); err != nil {
			t.Fatalf("seed the clean-old bookkeeping: %v", err)
		}
	}
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)

	row, ok := settingsEditStored(t, env.chat.Id)
	if !ok || row.WelcomeSettings.LastMsgId != 4242 || row.GoodbyeSettings.LastMsgId != 4343 {
		t.Fatalf("the seeded row = %+v, %t, want both last message IDs stored", row, ok)
	}
	groupBefore := len(env.fake.sentTo(env.chat.Id))
	deletes := len(env.fake.callsFor("deleteMessage"))

	for _, press := range [][]string{
		{"wf"}, {"wv", "t", "x"}, {"wv", "t", "m"}, {"wv", "t", "b"},
		{"gf"}, {"gv", "t", "x"}, {"gv", "t", "m"}, {"gv", "t", "b"},
	} {
		settingsEditPreviewPress(env, owner, menuID, press[0], press[1:]...)
		if text, alert := env.lastAnswer(); text != "" || alert {
			t.Fatalf("press %v answered %q (alert %t), want an empty answer", press, text, alert)
		}
	}

	after, ok := settingsEditStored(t, env.chat.Id)
	if !ok || !reflect.DeepEqual(row, after) {
		t.Errorf("the greetings row changed:\nbefore %+v\nafter  %+v", row, after)
	}
	if got := len(env.fake.callsFor("deleteMessage")) - deletes; got != 0 {
		t.Errorf("the previews deleted %d messages, want none", got)
	}
	if got := len(env.fake.sentTo(env.chat.Id)) - groupBefore; got != 0 {
		t.Errorf("the previews sent %d messages into the group, want none", got)
	}
}

func TestSettingsEditPreviewErrors(t *testing.T) {
	t.Run("a group menu answers the toast and the preview arrives in private", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		settingsEditFakeSends(env)
		menuID := openEditPage(env, env.admin, settingsPageWelcome)
		edits := len(env.edits(env.chat.Id, menuID))
		groupBefore := len(env.fake.sentTo(env.chat.Id))
		adminBefore := len(env.fake.sentTo(env.admin.Id))

		env.press(env.admin, menuID, env.groupFields(env.admin, "a", "wf"))

		text, alert := env.lastAnswer()
		if !containsMarker(text, "settings_edit_preview_sent") || alert {
			t.Errorf("answer = %q (alert %t), want the sent toast and no alert", text, alert)
		}
		sent := settingsEditPreviewSentTo(env, env.admin.Id, adminBefore)
		if len(sent) != 2 || !containsMarker(fmt.Sprint(sent[0].Params["text"]), "settings_edit_preview_header") {
			t.Errorf("the presser's private chat got %+v, want the header and the message", sent)
		}
		if got := len(env.fake.sentTo(env.chat.Id)) - groupBefore; got != 0 {
			t.Errorf("a preview from a group menu sent %d messages into the group, want none", got)
		}
		if got := len(env.edits(env.chat.Id, menuID)); got != edits {
			t.Errorf("the menu was edited %d times by a preview, want none", got-edits)
		}
	})

	failures := []struct {
		name       string
		scripted   []error
		wantAnswer string
		wantSent   int
	}{
		{"a 403 on the header asks the presser to start the bot",
			[]error{staffFakeError(403, "Forbidden: bot can't initiate conversation with a user")},
			"settings_edit_preview_need_start", 0},
		{"a 400 on the content answers the generic failure",
			[]error{nil, staffFakeError(400, "Bad Request: can't parse entities")},
			"settings_edit_preview_failed", 1},
		{"a 429 on the content answers the generic failure",
			[]error{nil, staffFake429(1)},
			"settings_edit_preview_failed", 1},
	}
	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			settingsEditFakeSends(env)
			owner := env.newOwner(env.chat)
			menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
			before := len(env.fake.sentTo(owner.Id))
			env.fake.script("sendMessage", owner.Id, failure.scripted...)

			settingsEditPreviewPress(env, owner, menuID, "wf")
			text, alert := env.lastAnswer()
			if !containsMarker(text, failure.wantAnswer) || !alert {
				t.Errorf("answer = %q (alert %t), want the %s alert", text, alert, failure.wantAnswer)
			}
			if got := len(env.fake.sentTo(owner.Id)) - before; got != failure.wantSent {
				t.Errorf("the bot sent %d messages after the failure, want %d", got, failure.wantSent)
			}
			if _, ok := settingsEditStored(t, env.chat.Id); ok {
				t.Error("a failed preview wrote a greetings row")
			}

			// Nothing is retried, and the next press works.
			settingsEditPreviewPress(env, owner, menuID, "wf")
			if text, alert := env.lastAnswer(); text != "" || alert {
				t.Errorf("the next press answered %q (alert %t), want an empty answer", text, alert)
			}
			if got := len(env.fake.sentTo(owner.Id)) - before; got != failure.wantSent+2 {
				t.Errorf("the bot sent %d messages after both presses, want %d", got, failure.wantSent+2)
			}
		})
	}

	t.Run("a part that is not set has nothing to preview", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		settingsEditFakeSends(env)
		owner := env.newOwner(env.chat)
		if err := greetings.SetWelcomeText(env.chat.Id, "text only", "", nil, db.TEXT); err != nil {
			t.Fatalf("SetWelcomeText: %v", err)
		}
		menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
		before := len(env.fake.sentTo(owner.Id))
		for _, part := range []string{"m", "b"} {
			settingsEditPreviewPress(env, owner, menuID, "wv", "t", part)
			if text, alert := env.lastAnswer(); !containsMarker(text, "settings_edit_nothing_to_preview") || !alert {
				t.Errorf("part %s answered %q (alert %t), want the nothing-to-preview alert", part, text, alert)
			}
		}
		if got := len(env.fake.sentTo(owner.Id)) - before; got != 0 {
			t.Errorf("a stale preview sent %d messages, want none", got)
		}

		settingsEditSwitchPage(env, owner, menuID, settingsPageRules)
		settingsEditPreviewPress(env, owner, menuID, "rv", "t", "x")
		if text, alert := env.lastAnswer(); !containsMarker(text, "settings_edit_nothing_to_preview") || !alert {
			t.Errorf("Rules answered %q (alert %t), want the nothing-to-preview alert", text, alert)
		}
	})

	t.Run("a missing or unknown part is refused before any lookup", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
		lookups := env.memberLookupsOf(env.chat.Id, owner.Id)
		before := len(env.fake.sentTo(owner.Id))
		for _, kv := range [][]string{nil, {"t", "z"}, {"t", ""}} {
			settingsEditPreviewPress(env, owner, menuID, "wv", kv...)
			if text, alert := env.lastAnswer(); !containsMarker(text, "common_callback_invalid_request") || !alert {
				t.Errorf("fields %v answered %q (alert %t), want the invalid request alert", kv, text, alert)
			}
		}
		if got := env.memberLookupsOf(env.chat.Id, owner.Id) - lookups; got != 0 {
			t.Errorf("a refused preview made %d getChatMember calls, want none", got)
		}
		if got := len(env.fake.sentTo(owner.Id)) - before; got != 0 {
			t.Errorf("a refused preview sent %d messages, want none", got)
		}
	})
}
