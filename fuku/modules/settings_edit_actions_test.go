//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// settingsEditPrivateMenu opens page pageKey of the env's group in a private menu of owner and
// returns the private menu message's ID.
func settingsEditPrivateMenu(env *settingsEnv, owner gotgbot.User, pageKey string) int64 {
	env.t.Helper()
	return openEditPagePrivate(env, owner, pageKey)
}

// settingsEditPressButton presses the button labelled labelKey of the last drawing of message
// msgID in owner's private chat and returns the decoded fields of the button.
func settingsEditPressButton(env *settingsEnv, owner gotgbot.User, msgID int64, labelKey string) map[string]string {
	env.t.Helper()
	_, keyboard := env.lastEdit(owner.Id, msgID)
	button, ok := settingsButtonByMarker(keyboard, labelKey)
	if !ok {
		env.t.Fatalf("the page has no button labelled %s: %+v", labelKey, keyboard)
	}
	env.pressRaw(settingsPrivateChat(owner), owner, msgID, button.CallbackData)
	return settingsButtonFields(env.t, button)
}

// settingsEditWaitOf reads owner's wait hash straight from Redis. It fails the test when there
// is none.
func settingsEditWaitOf(env *settingsEnv, owner gotgbot.User) map[string]string {
	env.t.Helper()
	key := settingsInputKey(owner.Id)
	if !env.redis.Exists(key) {
		env.t.Fatalf("user %d has no wait", owner.Id)
	}
	keys, err := env.redis.HKeys(key)
	if err != nil {
		env.t.Fatalf("HKeys(%s) error = %v", key, err)
	}
	fields := make(map[string]string, len(keys))
	for _, field := range keys {
		fields[field] = env.redis.HGet(key, field)
	}
	return fields
}

func TestSettingsEditSetWritesWait(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)

	// Nothing is set, so each part has its Set button alone.
	_, keyboard := env.lastEdit(owner.Id, menuID)
	for _, label := range []string{"settings_btn_edit_set_text", "settings_btn_edit_set_media", "settings_btn_edit_set_buttons"} {
		if !settingsEditHasButton(keyboard, label) {
			t.Fatalf("the Welcome page has no %s button", label)
		}
	}

	before := time.Now()
	answers := env.answerCount()
	fields := settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	if fields["a"] != "ws" || fields["t"] != "x" || fields["c"] != strconv.FormatInt(env.chat.Id, 10) {
		t.Fatalf("the Set button carries %v, want ws, t=x and the group in c", fields)
	}
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press was answered %d times, want once", got)
	}

	wait := settingsEditWaitOf(env, owner)
	if wait["c"] != strconv.FormatInt(env.chat.Id, 10) || wait["p"] != "w" || wait["t"] != "x" ||
		wait["m"] != strconv.FormatInt(menuID, 10) || !settingsInputNonceValid(wait["n"]) {
		t.Fatalf("wait = %v, want the group, page w, part x, the menu message and a 16-hex nonce", wait)
	}
	expiry, err := strconv.ParseInt(wait["x"], 10, 64)
	if err != nil {
		t.Fatalf("expiry %q is not a number", wait["x"])
	}
	until := time.UnixMilli(expiry)
	if until.Before(before.Add(settingsInputTTL-time.Second)) || until.After(time.Now().Add(settingsInputTTL+time.Second)) {
		t.Errorf("the wait ends at %v, want about ten minutes from now", until)
	}
	if ttl := env.redis.TTL(settingsInputKey(owner.Id)); ttl != settingsInputTTL {
		t.Errorf("wait TTL = %v, want %v", ttl, settingsInputTTL)
	}
	marker, err := env.redis.Get(settingsInputDoneKey(owner.Id))
	if want := fmt.Sprintf("%d|w|x", env.chat.Id); err != nil || marker != want {
		t.Errorf("marker = %q, %v, want %q", marker, err, want)
	}

	// The message became the prompt: text with the limit and the end time, one Cancel button.
	text, keyboard := env.lastEdit(owner.Id, menuID)
	for _, want := range []string{
		staffMarker("settings_edit_wait_text"),
		" " + strconv.Itoa(settingsEditTextMax) + "\n\n" + staffMarker("settings_edit_wait_footer") + " " + until.UTC().Format("15:04"),
		strconv.FormatInt(env.chat.Id, 10),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the prompt %q lacks %q", text, want)
		}
	}
	if strings.Contains(text, staffMarker("settings_edit_replaced_note")) {
		t.Errorf("a first wait carries the replaced note: %q", text)
	}
	if len(keyboard) != 1 || len(keyboard[0]) != 1 || !containsMarker(keyboard[0][0].Text, "settings_btn_edit_cancel") {
		t.Fatalf("the prompt keyboard = %+v, want one row with Cancel", keyboard)
	}
	cancel := settingsButtonFields(t, keyboard[0][0])
	if cancel["a"] != "wc" || cancel["t"] != "x" || cancel["c"] != strconv.FormatInt(env.chat.Id, 10) || len(cancel) != 3 {
		t.Errorf("Cancel carries %v, want wc, t=x and c", cancel)
	}
	for _, absent := range []string{"settings_btn_back", "settings_btn_close"} {
		if settingsEditHasButton(keyboard, absent) {
			t.Errorf("the prompt has the %s button", absent)
		}
	}
	if _, ok := settingsEditStored(t, env.chat.Id); ok {
		t.Error("Set wrote a greetings row")
	}
}

func TestSettingsEditCancelButton(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	if !env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Fatal("Set wrote no wait")
	}

	answers := env.answerCount()
	fields := settingsEditPressButton(env, owner, menuID, "settings_btn_edit_cancel")
	if fields["a"] != "wc" {
		t.Fatalf("the Cancel button carries %v", fields)
	}
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("Cancel was answered %d times, want once", got)
	}
	if env.redis.Exists(settingsInputKey(owner.Id)) || env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Error("Cancel left the wait or its marker")
	}

	text, keyboard := env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_cancelled")) {
		t.Errorf("the page after Cancel %q lacks the cancelled notice", text)
	}
	if _, fields := settingsEditControl(t, keyboard, "settings_btn_edit_welcome_toggle", settingsEditControlEnabled); fields["p"] != "w" {
		t.Errorf("the page after Cancel is not Welcome: %v", fields)
	}
	if _, ok := settingsEditStored(t, env.chat.Id); ok {
		t.Error("Cancel wrote a greetings row")
	}
}
