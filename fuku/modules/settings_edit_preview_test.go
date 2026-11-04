//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
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
	var chat map[string]any
	if json.Unmarshal(raw, &chat) != nil {
		return raw, nil
	}
	chat["title"] = c.title
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
		t.Fatalf("the Full preview button carries %v, want wf and the group in c", fields)
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
