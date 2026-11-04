//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// settingsEditPrivateMenu opens page pageKey of the env's group in a private menu of owner and
// returns the private menu message's ID.
func settingsEditPrivateMenu(env *settingsEnv, owner gotgbot.User, pageKey string) int64 {
	env.t.Helper()
	return openEditPagePrivate(env, owner, pageKey)
}

// settingsEditSwitchPage draws page pageKey in the private menu message msgID (the open page
// action, which works from any page).
func settingsEditSwitchPage(env *settingsEnv, owner gotgbot.User, msgID int64, pageKey string) {
	env.t.Helper()
	env.pressPrivate(owner, msgID, env.privateFields(env.chat.Id, "a", "op", "p", pageKey))
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

// settingsEditPromptHas reports whether the drawn prompt shows limit after "Up to".
func settingsEditPromptHas(text string, limit int) bool {
	return strings.Contains(text, " "+strconv.Itoa(limit)+"\n\n"+staffMarker("settings_edit_wait_footer"))
}

// settingsEditInputKeys lists the keys of the wait family in Redis.
func settingsEditInputKeys(env *settingsEnv) []string {
	var found []string
	for _, key := range env.redis.Keys() {
		if strings.HasPrefix(key, "fuku:settings:input") {
			found = append(found, key)
		}
	}
	return found
}

func TestSettingsEditReplaceWait(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	group := strconv.FormatInt(env.chat.Id, 10)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)

	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	firstNonce := settingsEditWaitOf(env, owner)["n"]

	// Set media on the same page replaces the wait and says so.
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "ws", "t", "m"))
	wait := settingsEditWaitOf(env, owner)
	if wait["t"] != "m" || wait["n"] == firstNonce {
		t.Fatalf("wait after Set media = %v, want part m and a new nonce", wait)
	}
	text, _ := env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, staffMarker("settings_edit_replaced_note")) {
		t.Errorf("the prompt of a replacing Set lacks the replaced note: %q", text)
	}

	// The first prompt's Cancel is stale: it answers so, edits the message into the page and
	// leaves the live wait with its TTL.
	answers := env.answerCount()
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "wc", "t", "x"))
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the stale Cancel was answered %d times, want once", got)
	}
	if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_edit_prompt_stale")) {
		t.Errorf("the stale Cancel answered %q (alert %v), want the alert settings_edit_prompt_stale", answer, alert)
	}
	if after := settingsEditWaitOf(env, owner); after["t"] != "m" || after["n"] != wait["n"] {
		t.Errorf("a stale Cancel changed the live wait to %v", after)
	}
	if ttl := env.redis.TTL(settingsInputKey(owner.Id)); ttl <= 0 || ttl > settingsInputTTL {
		t.Errorf("wait TTL after a stale Cancel = %v", ttl)
	}
	text, keyboard := env.lastEdit(owner.Id, menuID)
	if strings.Contains(text, settingsNoticeOKIcon) || !settingsEditHasButton(keyboard, "settings_btn_edit_welcome_toggle") {
		t.Errorf("the stale Cancel did not redraw the page without a notice: %q", text)
	}

	// Set media again refreshes the ten minutes and carries no replaced note.
	env.redis.FastForward(3 * time.Minute)
	if ttl := env.redis.TTL(settingsInputKey(owner.Id)); ttl >= settingsInputTTL {
		t.Fatalf("the fake clock did not move the TTL: %v", ttl)
	}
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "ws", "t", "m"))
	if ttl := env.redis.TTL(settingsInputKey(owner.Id)); ttl != settingsInputTTL {
		t.Errorf("TTL after Set on the same part = %v, want %v", ttl, settingsInputTTL)
	}
	text, _ = env.lastEdit(owner.Id, menuID)
	if strings.Contains(text, staffMarker("settings_edit_replaced_note")) {
		t.Errorf("a refresh of the same part carries the replaced note: %q", text)
	}

	// Set on another group replaces the wait and says so.
	twin := env.addChat()
	editCleanup(t, twin.Id)
	env.fake.setMember(twin.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	env.fake.setCreator(twin.Id, owner.Id)
	env.pressPrivate(owner, menuID, env.privateFields(twin.Id, "a", "ws", "t", "x"))
	if wait := settingsEditWaitOf(env, owner); wait["c"] != strconv.FormatInt(twin.Id, 10) || wait["t"] != "x" || wait["c"] == group {
		t.Fatalf("wait after Set on another group = %v, want that group", wait)
	}
	text, _ = env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, staffMarker("settings_edit_replaced_note")) {
		t.Errorf("the prompt of a Set on another group lacks the replaced note: %q", text)
	}
}

func TestSettingsEditSetUnavailable(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	edits := len(env.edits(owner.Id, menuID))

	restore := cache.DisableRedisForTest()
	answers := env.answerCount()
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "ws", "t", "x"))
	answer, alert := env.lastAnswer()
	gotAnswers := env.answerCount() - answers

	// The rest of the menu works with no Redis: the switch stores.
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "tg", "p", "w", "t", "e", "v", "0"))
	restore()

	if gotAnswers != 1 || !alert || !strings.Contains(answer, staffMarker("settings_edit_unavailable")) {
		t.Errorf("Set without Redis answered %q (alert %v, %d answers), want one alert settings_edit_unavailable", answer, alert, gotAnswers)
	}
	if got := len(env.edits(owner.Id, menuID)); got != edits+1 {
		t.Errorf("the message was edited %d times after Set and the switch, want 1 (the switch only)", got-edits)
	}
	if keys := settingsEditInputKeys(env); len(keys) != 0 {
		t.Errorf("Set without Redis left keys %v", keys)
	}
	if row, ok := settingsEditStored(t, env.chat.Id); !ok || row.WelcomeSettings.ShouldWelcome {
		t.Errorf("the switch without Redis stored %+v (found %v), want welcome off", row, ok)
	}
}

func TestSettingsEditStaleCancel(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")

	// The wait was used: the door deleted the wait and its marker.
	env.redis.Del(settingsInputKey(owner.Id))
	env.redis.Del(settingsInputDoneKey(owner.Id))
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_cancel")
	answer, alert := env.lastAnswer()
	if !alert || !strings.Contains(answer, staffMarker("settings_edit_prompt_stale")) {
		t.Errorf("Cancel after the wait was used answered %q (alert %v), want settings_edit_prompt_stale", answer, alert)
	}
	text, keyboard := env.lastEdit(owner.Id, menuID)
	if strings.Contains(text, settingsNoticeOKIcon) || !settingsEditHasButton(keyboard, "settings_btn_edit_set_text") {
		t.Errorf("the stale Cancel did not edit the message into the page without a notice: %q", text)
	}

	// A claim held (a write in flight) is not cancelled either.
	settingsEditSwitchPage(env, owner, menuID, settingsPageWelcome)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	nonce := settingsEditWaitOf(env, owner)["n"]
	if _, ok, err := settingsInputClaim(context.Background(), owner.Id, nonce); err != nil || !ok {
		t.Fatalf("claim = %v, %v", ok, err)
	}
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_cancel")
	if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_edit_prompt_stale")) {
		t.Errorf("Cancel while a write is in flight answered %q (alert %v)", answer, alert)
	}
	if !env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Error("Cancel deleted a wait whose write is in flight")
	}
}

func TestSettingsEditExpiredPrompt(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")

	env.redis.FastForward(settingsInputTTL + time.Second)
	if env.redis.Exists(settingsInputKey(owner.Id)) || !env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Fatal("after ten minutes the wait should be gone and its marker still there")
	}
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_cancel")
	answer, alert := env.lastAnswer()
	if !alert || !strings.Contains(answer, staffMarker("settings_edit_wait_expired")) {
		t.Errorf("Cancel after the wait expired answered %q (alert %v), want settings_edit_wait_expired", answer, alert)
	}
	text, keyboard := env.lastEdit(owner.Id, menuID)
	if strings.Contains(text, settingsNoticeOKIcon) || !settingsEditHasButton(keyboard, "settings_btn_edit_set_text") {
		t.Errorf("the expired Cancel did not edit the message into the page: %q", text)
	}
	if env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Error("the marker is still there after the person was told")
	}
}

func TestSettingsEditPromptMax(t *testing.T) {
	welcome, rulesDef := settingsEditDef(t, settingsPageWelcome), settingsEditDef(t, settingsPageRules)
	for name, tc := range map[string]struct {
		def  settingsEditPageDef
		st   settingsEditState
		want int
	}{
		"welcome without media":  {welcome, settingsEditState{Kind: settingsEditWelcome}, 4096},
		"welcome with a photo":   {welcome, settingsEditState{Kind: settingsEditWelcome, MediaType: db.PHOTO, FileID: "f"}, 1024},
		"welcome with a sticker": {welcome, settingsEditState{Kind: settingsEditWelcome, MediaType: db.STICKER, FileID: "f"}, 1024},
		"rules":                  {rulesDef, settingsEditState{Kind: settingsEditRules, Text: "t"}, 3800},
	} {
		if got := settingsEditPromptMax(tc.def, tc.st); got != tc.want {
			t.Errorf("%s: settingsEditPromptMax = %d, want %d", name, got, tc.want)
		}
	}

	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	if text, _ := env.lastEdit(owner.Id, menuID); !settingsEditPromptHas(text, 4096) {
		t.Errorf("the text prompt of a group without media is not 4096: %q", text)
	}

	if err := greetings.SetWelcomeText(env.chat.Id, "caption", "photo-file", nil, db.PHOTO); err != nil {
		t.Fatalf("SetWelcomeText: %v", err)
	}
	settingsEditSwitchPage(env, owner, menuID, settingsPageWelcome)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	if text, _ := env.lastEdit(owner.Id, menuID); !settingsEditPromptHas(text, 1024) {
		t.Errorf("the text prompt of a group with a photo is not 1024: %q", text)
	}

	settingsEditSwitchPage(env, owner, menuID, settingsPageRules)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	if text, _ := env.lastEdit(owner.Id, menuID); !settingsEditPromptHas(text, 3800) ||
		!strings.Contains(text, staffMarker("settings_edit_wait_text_rules")) {
		t.Errorf("the Rules prompt is not the rules text with 3800: %q", text)
	}
}

func TestSettingsEditGroupMenuRefusal(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	for _, tc := range []struct{ page, code string }{
		{settingsPageWelcome, "ws"}, {settingsPageWelcome, "wc"},
		{settingsPageGoodbye, "gs"}, {settingsPageGoodbye, "gc"},
		{settingsPageRules, "rs"}, {settingsPageRules, "rc"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			msgID := openEditPage(env, owner, tc.page)
			answers := env.answerCount()
			env.press(owner, msgID, env.groupFields(owner, "a", tc.code, "t", "x"))
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("the press was answered %d times, want once", got)
			}
			text, keyboard := env.lastEdit(env.chat.Id, msgID)
			if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_edit_group_note")) {
				t.Errorf("the page %q lacks the group note", text)
			}
			if !settingsEditHasButton(keyboard, "settings_btn_edit_set_text") {
				t.Error("the redrawn page lost its Set button")
			}
			if button, ok := settingsButtonByMarker(keyboard, "settings_btn_open_private"); !ok || button.Url == "" {
				t.Errorf("the redrawn page has no Open in private row: %+v", keyboard)
			}
			if keys := settingsEditInputKeys(env); len(keys) != 0 {
				t.Errorf("a group menu press created %v", keys)
			}
			if _, ok := settingsEditStored(t, env.chat.Id); ok {
				t.Error("a group menu press wrote a greetings row")
			}
			if row, ok := settingsEditRulesStored(t, env.chat.Id); ok && row.Rules != "" {
				t.Errorf("a group menu press wrote rules %q", row.Rules)
			}
		})
	}
}

func TestSettingsEditCallbackFields(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	group := env.chat.Id
	for name, fields := range map[string]map[string]string{
		"ws without t":     env.privateFields(group, "a", "ws"),
		"ws with t=z":      env.privateFields(group, "a", "ws", "t", "z"),
		"rs with t=m":      env.privateFields(group, "a", "rs", "t", "m"),
		"ws with t=é":      env.privateFields(group, "a", "ws", "t", "é"),
		"ws with an extra": env.privateFields(group, "a", "ws", "t", "x", "q", "1"),
		"wc without t":     env.privateFields(group, "a", "wc"),
		"wc with t=z":      env.privateFields(group, "a", "wc", "t", "z"),
		"gs with t and u":  env.privateFields(group, "a", "gs", "t", "x", "u", "1"),
	} {
		t.Run(name, func(t *testing.T) {
			lookups := env.memberLookupsOf(group, owner.Id)
			answers := env.answerCount()
			edits := len(env.edits(owner.Id, menuID))
			env.pressPrivate(owner, menuID, fields)
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("the press was answered %d times, want once", got)
			}
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("answer = %q (alert %v), want the alert common_callback_invalid_request", answer, alert)
			}
			if got := env.memberLookupsOf(group, owner.Id); got != lookups {
				t.Errorf("the refused press made %d getChatMember calls", got-lookups)
			}
			if got := len(env.edits(owner.Id, menuID)); got != edits {
				t.Errorf("the refused press edited the message %d times", got-edits)
			}
			if keys := settingsEditInputKeys(env); len(keys) != 0 {
				t.Errorf("the refused press created %v", keys)
			}
		})
	}
}
