//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
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
	if _, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_welcome", settingsEditControlEnabled); fields["p"] != "w" {
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
	if settingsEditHasNoticeLine(text, settingsNoticeOKIcon) || !settingsEditHasButton(keyboard, "settings_btn_edit_switch_welcome") {
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

// TestSettingsEditStalePromptSamePart: two private menus of one group are open and Set is pressed
// on the same part of both. The first prompt's Cancel (same group, page and part, another message)
// is stale and leaves the second menu's wait alone; the second prompt's Cancel deletes it.
func TestSettingsEditStalePromptSamePart(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	first := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	// A second deep link sends a second private menu; openEditPagePrivate allows one only.
	env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
	sent := env.fake.sentTo(owner.Id)
	last := sent[len(sent)-1]
	pageButton, ok := settingsPageButton(t, staffKeyboardOf(last.Params["reply_markup"]), settingsPageWelcome)
	if !ok {
		t.Fatal("the second private Main page has no Welcome button")
	}
	second := last.MessageID
	env.pressRaw(settingsPrivateChat(owner), owner, second, pageButton.CallbackData)
	if first == second {
		t.Fatalf("both private menus are message %d", first)
	}
	settingsEditPressButton(env, owner, first, "settings_btn_edit_set_text")
	settingsEditPressButton(env, owner, second, "settings_btn_edit_set_text")
	live := settingsEditWaitOf(env, owner)
	if live["m"] != strconv.FormatInt(second, 10) {
		t.Fatalf("the wait names prompt %s, want the second menu %d", live["m"], second)
	}

	answers := env.answerCount()
	settingsEditPressButton(env, owner, first, "settings_btn_edit_cancel")
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the stale Cancel was answered %d times, want once", got)
	}
	if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_edit_prompt_stale")) {
		t.Errorf("the stale Cancel answered %q (alert %v), want the alert settings_edit_prompt_stale", answer, alert)
	}
	if after := settingsEditWaitOf(env, owner); after["n"] != live["n"] || after["m"] != live["m"] {
		t.Errorf("a Cancel on the first menu changed the live wait to %v", after)
	}
	if !env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Error("a Cancel on the first menu deleted the marker of the live wait")
	}

	settingsEditPressButton(env, owner, second, "settings_btn_edit_cancel")
	if env.redis.Exists(settingsInputKey(owner.Id)) || env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Error("Cancel on the second menu left the wait or its marker")
	}
	if text, _ := env.lastEdit(owner.Id, second); !strings.Contains(text, staffMarker("settings_edit_cancelled")) {
		t.Errorf("Cancel on the second menu edited it to %q, want the cancelled notice", text)
	}
}

// TestSettingsEditRefusalCancel: the Cancel button of a refusal reply sits on a message that is
// not the prompt, and it still ends the wait it belongs to.
func TestSettingsEditRefusalCancel(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)

	photo := []gotgbot.PhotoSize{{FileId: "photo-id", FileUniqueId: "photo-unique", Width: 1, Height: 1}}
	sendPrivateMessage(env, owner, gotgbot.Message{Photo: photo})
	sent := env.fake.sentTo(owner.Id)
	refusal := sent[len(sent)-1]
	keyboard := staffKeyboardOf(refusal.Params["reply_markup"])
	button, ok := settingsButtonByMarker(keyboard, "settings_btn_edit_cancel")
	if !ok {
		t.Fatalf("the refusal has no Cancel button: %+v", keyboard)
	}
	if refusal.MessageID == 0 || env.redis.HGet(settingsInputKey(owner.Id), "r"+strconv.FormatInt(refusal.MessageID, 10)) == "" {
		t.Fatalf("the refusal message %d was not noted on the wait: %v", refusal.MessageID, settingsEditWaitOf(env, owner))
	}

	env.pressRaw(settingsPrivateChat(owner), owner, refusal.MessageID, button.CallbackData)
	if env.redis.Exists(settingsInputKey(owner.Id)) || env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Error("Cancel on the refusal reply left the wait or its marker")
	}
	if text, _ := env.lastEdit(owner.Id, refusal.MessageID); !strings.Contains(text, staffMarker("settings_edit_cancelled")) {
		t.Errorf("Cancel on the refusal edited it to %q, want the cancelled notice", text)
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
	if settingsEditHasNoticeLine(text, settingsNoticeOKIcon) || !settingsEditHasButton(keyboard, "settings_btn_edit_set_text") {
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

// TestSettingsEditCancelRedisError: a Cancel that meets a Redis error may never have run its
// script, so the wait can still be live. The prompt and its Cancel button stay (nothing is
// edited), the press is answered once with the alert, and the wait is still there; a retry
// after Redis is back cancels it.
func TestSettingsEditCancelRedisError(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
	edits := len(env.edits(owner.Id, menuID))
	answers := env.answerCount()

	env.redis.SetError("connection reset")
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_cancel")
	env.redis.SetError("")

	answer, alert := env.lastAnswer()
	if got := env.answerCount() - answers; got != 1 || !alert || !strings.Contains(answer, staffMarker("settings_edit_unavailable")) {
		t.Errorf("Cancel with a Redis error answered %q (alert %v, %d answers), want one alert settings_edit_unavailable", answer, alert, got)
	}
	if got := len(env.edits(owner.Id, menuID)); got != edits {
		t.Errorf("the prompt was edited %d times by a Cancel that met a Redis error, want none", got-edits)
	}
	if !env.redis.Exists(settingsInputKey(owner.Id)) || !env.redis.Exists(settingsInputDoneKey(owner.Id)) {
		t.Error("the wait or its marker is gone after a Cancel that never ran")
	}

	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_cancel")
	if env.redis.Exists(settingsInputKey(owner.Id)) {
		t.Error("a Cancel with Redis back did not delete the wait")
	}
	if text, _ := env.lastEdit(owner.Id, menuID); !strings.Contains(text, staffMarker("settings_edit_cancelled")) {
		t.Errorf("the retried Cancel edited the message to %q, want the cancelled notice", text)
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
	if settingsEditHasNoticeLine(text, settingsNoticeOKIcon) || !settingsEditHasButton(keyboard, "settings_btn_edit_set_text") {
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
		{settingsPageWelcome, "ws"}, {settingsPageWelcome, "wr"}, {settingsPageWelcome, "wc"},
		{settingsPageGoodbye, "gs"}, {settingsPageGoodbye, "gr"}, {settingsPageGoodbye, "gc"},
		{settingsPageRules, "rs"}, {settingsPageRules, "rr"}, {settingsPageRules, "rc"},
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
		"rs with t=z":      env.privateFields(group, "a", "rs", "t", "z"),
		"rr with t=z":      env.privateFields(group, "a", "rr", "t", "z"),
		"rc with t=z":      env.privateFields(group, "a", "rc", "t", "z"),
		"rs without t":     env.privateFields(group, "a", "rs"),
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

// TestSettingsEditRulesCallbackParts: Rules Set, Remove and Cancel accept the part codes m and b as they accept
// x, and each press is answered once.
func TestSettingsEditRulesCallbackParts(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageRules)
	for _, code := range []string{"rs", "rr", "rc"} {
		for _, part := range []string{"x", "m", "b"} {
			t.Run(code+" t="+part, func(t *testing.T) {
				answers := env.answerCount()
				env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", code, "t", part))
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press was answered %d times, want once", got)
				}
				if answer, _ := env.lastAnswer(); strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
					t.Errorf("%s with t=%s was refused as an invalid request", code, part)
				}
			})
		}
	}
}

// settingsEditSeedGreetings stores a welcome with a custom text, a photo and two buttons, and a
// goodbye with the same, for chatID.
func settingsEditSeedGreetings(t *testing.T, chatID int64) {
	t.Helper()
	buttons := []models.Button{{Name: "One", Url: "https://one.example"}, {Name: "Two", Url: "https://two.example"}}
	if err := greetings.SetWelcomeText(chatID, "a custom welcome", "welcome-photo", buttons, db.PHOTO); err != nil {
		t.Fatalf("SetWelcomeText: %v", err)
	}
	if err := greetings.SetGoodbyeText(chatID, "a custom goodbye", "goodbye-video", buttons, db.VIDEO); err != nil {
		t.Fatalf("SetGoodbyeText: %v", err)
	}
}

// settingsEditRemoveParts returns the Remove buttons of a drawing as the t values they carry.
func settingsEditRemoveParts(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, code string) []string {
	t.Helper()
	var parts []string
	for _, button := range settingsAllButtons(keyboard) {
		if button.Url != "" {
			continue
		}
		if fields := settingsButtonFields(t, button); fields["a"] == code {
			parts = append(parts, fields["t"])
		}
	}
	return parts
}

func TestSettingsEditRemove(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSeedGreetings(t, env.chat.Id)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)

	_, keyboard := env.lastEdit(owner.Id, menuID)
	if got := strings.Join(settingsEditRemoveParts(t, keyboard, "wr"), ","); got != "x,m,b" {
		t.Fatalf("Remove buttons on a fully set page = %q, want x,m,b", got)
	}

	// Remove text: the default text is back, media and buttons are untouched.
	answers := env.answerCount()
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_remove_text")
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("Remove was answered %d times, want once", got)
	}
	row, _ := settingsEditStored(t, env.chat.Id)
	if w := row.WelcomeSettings; w.WelcomeText != db.DefaultWelcome || w.FileID != "welcome-photo" || len(w.Button) != 2 {
		t.Errorf("after Remove text the welcome is %+v, want the default text with the media and buttons kept", w)
	}
	text, keyboard := env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_text_default")) {
		t.Errorf("the page %q lacks the removed-text notice", text)
	}
	if got := strings.Join(settingsEditRemoveParts(t, keyboard, "wr"), ","); got != "m,b" {
		t.Errorf("Remove buttons after Remove text = %q, want m,b", got)
	}

	// Remove media: no file, type text.
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_remove_media")
	row, _ = settingsEditStored(t, env.chat.Id)
	if w := row.WelcomeSettings; w.FileID != "" || w.WelcomeType != db.TEXT || len(w.Button) != 2 {
		t.Errorf("after Remove media the welcome is %+v, want no file, type text and the buttons kept", w)
	}
	text, _ = env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_media")) {
		t.Errorf("the page %q lacks the removed-media notice", text)
	}

	// Remove buttons: an empty list.
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_remove_buttons")
	row, _ = settingsEditStored(t, env.chat.Id)
	if w := row.WelcomeSettings; len(w.Button) != 0 {
		t.Errorf("after Remove buttons the welcome keeps %+v", w.Button)
	}
	text, keyboard = env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_buttons")) {
		t.Errorf("the page %q lacks the removed-buttons notice", text)
	}
	if got := settingsEditRemoveParts(t, keyboard, "wr"); len(got) != 0 {
		t.Errorf("Remove buttons left on an empty page: %v", got)
	}
	if row.GoodbyeSettings.GoodbyeText != "a custom goodbye" || row.GoodbyeSettings.FileID != "goodbye-video" {
		t.Errorf("a Welcome Remove changed the goodbye: %+v", row.GoodbyeSettings)
	}

	t.Run("goodbye", func(t *testing.T) {
		settingsEditSwitchPage(env, owner, menuID, settingsPageGoodbye)
		settingsEditPressButton(env, owner, menuID, "settings_btn_edit_remove_text")
		row, _ := settingsEditStored(t, env.chat.Id)
		if g := row.GoodbyeSettings; g.GoodbyeText != db.DefaultGoodbye || g.FileID != "goodbye-video" {
			t.Errorf("after Remove text the goodbye is %+v", g)
		}
	})

	t.Run("rules", func(t *testing.T) {
		if err := rules.SetChatRules(env.chat.Id, "the rules"); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		settingsEditSwitchPage(env, owner, menuID, settingsPageRules)
		settingsEditPressButton(env, owner, menuID, "settings_btn_edit_remove_text")
		if row, ok := settingsEditRulesStored(t, env.chat.Id); !ok || row.Rules != "" {
			t.Errorf("after Remove the rules are %+v (found %v), want empty", row, ok)
		}
		text, keyboard := env.lastEdit(owner.Id, menuID)
		if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_text_rules")) {
			t.Errorf("the page %q lacks the removed-rules notice", text)
		}
		if got := settingsEditRemoveParts(t, keyboard, "rr"); len(got) != 0 {
			t.Errorf("Remove left on empty rules: %v", got)
		}
	})
}

func TestSettingsEditRemoveStaleAndFailed(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageWelcome)

	// A stale Remove of a part that is not set writes nothing.
	for _, part := range []string{"x", "m", "b"} {
		env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "wr", "t", part))
		text, _ := env.lastEdit(owner.Id, menuID)
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_edit_nothing_to_remove")) {
			t.Errorf("a stale Remove of %s drew %q, want the nothing-to-remove notice", part, text)
		}
		if _, ok := settingsEditStored(t, env.chat.Id); ok {
			t.Errorf("a stale Remove of %s wrote a greetings row", part)
		}
	}
	settingsEditSwitchPage(env, owner, menuID, settingsPageRules)
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "rr", "t", "x"))
	if text, _ := env.lastEdit(owner.Id, menuID); !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_edit_nothing_to_remove")) {
		t.Errorf("a stale Remove of empty rules drew %q", text)
	}
	if row, ok := settingsEditRulesStored(t, env.chat.Id); ok && row.Rules != "" {
		t.Errorf("a stale Remove wrote rules %q", row.Rules)
	}

	// A failing write redraws the old state with the save-failed notice.
	settingsEditSeedGreetings(t, env.chat.Id)
	if err := rules.SetChatRules(env.chat.Id, "the rules"); err != nil {
		t.Fatalf("SetChatRules: %v", err)
	}
	previousText, previousRules := greetingWrites.WelcomeTextOnly, rulesWrites.SetChatRules
	greetingWrites.WelcomeTextOnly = func(int64, string) error { return errors.New("write refused") }
	rulesWrites.SetChatRules = func(int64, string) error { return errors.New("write refused") }
	defer func() { greetingWrites.WelcomeTextOnly, rulesWrites.SetChatRules = previousText, previousRules }()

	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "rr", "t", "x"))
	text, keyboard := env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
		t.Errorf("a failed Remove of rules drew %q, want the save-failed notice", text)
	}
	if got := settingsEditRemoveParts(t, keyboard, "rr"); len(got) != 1 {
		t.Errorf("the old Rules state lost its Remove button: %v", got)
	}
	settingsEditSwitchPage(env, owner, menuID, settingsPageWelcome)
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "wr", "t", "x"))
	text, keyboard = env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
		t.Errorf("a failed Remove of the text drew %q, want the save-failed notice", text)
	}
	if got := settingsEditRemoveParts(t, keyboard, "wr"); len(got) != 3 {
		t.Errorf("the old Welcome state lost its Remove buttons: %v", got)
	}
	if row, _ := settingsEditStored(t, env.chat.Id); row.WelcomeSettings.WelcomeText != "a custom welcome" {
		t.Errorf("a failed write changed the stored text to %q", row.WelcomeSettings.WelcomeText)
	}
	if row, _ := settingsEditRulesStored(t, env.chat.Id); row.Rules != "the rules" {
		t.Errorf("a failed write changed the stored rules to %q", row.Rules)
	}
}

func TestSettingsEditRemoveParity(t *testing.T) {
	env := newSettingsEditEnv(t)
	for _, page := range []struct {
		name, key, command string
		isWelcome          bool
	}{
		{"welcome", settingsPageWelcome, "/resetwelcome", true},
		{"goodbye", settingsPageGoodbye, "/resetgoodbye", false},
	} {
		t.Run(page.name, func(t *testing.T) {
			menuChat, commandChat := env.addChat(), env.addChat()
			editCleanup(t, menuChat.Id, commandChat.Id)
			menuOwner, commandOwner := env.newOwner(menuChat), env.newOwner(commandChat)
			settingsEditSeedGreetings(t, menuChat.Id)
			settingsEditSeedGreetings(t, commandChat.Id)

			env.sendPrivate(menuOwner, fmt.Sprintf("/start settings_%d", menuChat.Id))
			sent := env.fake.sentTo(menuOwner.Id)
			menuID := sent[len(sent)-1].MessageID
			env.pressPrivate(menuOwner, menuID, map[string]string{"a": "op", "p": page.key, "c": strconv.FormatInt(menuChat.Id, 10)})
			for _, part := range []string{"x", "m", "b"} {
				code := page.key + "r"
				env.pressPrivate(menuOwner, menuID, map[string]string{"a": code, "t": part, "c": strconv.FormatInt(menuChat.Id, 10)})
			}
			env.sendIn(commandChat, commandOwner, page.command)

			requireSameGreetingRow(t, loadGreetingRow(t, commandChat.Id), loadGreetingRow(t, menuChat.Id))
			menuStored, _ := settingsEditStored(t, menuChat.Id)
			if page.isWelcome {
				if menuStored.WelcomeSettings.WelcomeText != db.DefaultWelcome || menuStored.WelcomeSettings.FileID != "" {
					t.Errorf("the three Remove presses left %+v, want the default welcome", menuStored.WelcomeSettings)
				}
				if menuStored.GoodbyeSettings.GoodbyeText != "a custom goodbye" {
					t.Errorf("the Welcome presses changed the goodbye: %+v", menuStored.GoodbyeSettings)
				}
			} else if menuStored.GoodbyeSettings.GoodbyeText != db.DefaultGoodbye || menuStored.GoodbyeSettings.FileID != "" {
				t.Errorf("the three Remove presses left %+v, want the default goodbye", menuStored.GoodbyeSettings)
			}
		})
	}

	t.Run("rules", func(t *testing.T) {
		menuChat, commandChat := env.addChat(), env.addChat()
		editCleanup(t, menuChat.Id, commandChat.Id)
		menuOwner, commandOwner := env.newOwner(menuChat), env.newOwner(commandChat)
		for _, id := range []int64{menuChat.Id, commandChat.Id} {
			settingsRulesSeedAll(t, id)
		}
		for _, part := range []string{"x", "m", "b"} {
			env.pressPrivate(menuOwner, 8181, map[string]string{"a": "rr", "t": part, "c": strconv.FormatInt(menuChat.Id, 10)})
		}
		env.sendIn(commandChat, commandOwner, "/clearrules")

		menuRow, menuOK := settingsEditRulesStored(t, menuChat.Id)
		commandRow, commandOK := settingsEditRulesStored(t, commandChat.Id)
		if !menuOK || !commandOK || menuRow.Rules != commandRow.Rules || menuRow.Rules != "" ||
			menuRow.FileID != commandRow.FileID || menuRow.FileID != "" ||
			menuRow.MediaType != commandRow.MediaType || menuRow.MediaType != 0 ||
			len(menuRow.Buttons) != len(commandRow.Buttons) || len(menuRow.Buttons) != 0 {
			t.Errorf("rules differ: menu %+v (%v), command %+v (%v), want both empty", menuRow, menuOK, commandRow, commandOK)
		}
		menuCached, err := rules.LoadRulesContext(context.Background(), menuChat.Id)
		commandCached, err2 := rules.LoadRulesContext(context.Background(), commandChat.Id)
		if err != nil || err2 != nil || menuCached != commandCached {
			t.Errorf("rules reads differ: menu %q (%v), command %q (%v)", menuCached, err, commandCached, err2)
		}
	})
}

func TestSettingsAuthorityParityEdit(t *testing.T) {
	nextAdmin := int64(7_000_000)
	newAdmin := func(env *settingsEnv, chat gotgbot.Chat, canChangeInfo bool) gotgbot.User {
		nextAdmin++
		admin := gotgbot.User{Id: uniqueLinkOwnerID() + nextAdmin, FirstName: "Plain"}
		listEditAdmin(env, chat, admin, canChangeInfo)
		return admin
	}
	press := func(env *settingsEnv, chat gotgbot.Chat, user gotgbot.User, kv ...string) {
		env.pressPrivate(user, 9191, env.privateFields(chat.Id, kv...))
	}

	t.Run("an administrator without can_change_info", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		menuChat, commandChat := env.addChat(), env.addChat()
		editCleanup(t, menuChat.Id, commandChat.Id)
		menuAdmin, commandAdmin := newAdmin(env, menuChat, false), newAdmin(env, commandChat, false)
		settingsEditSeedGreetings(t, menuChat.Id)
		settingsEditSeedGreetings(t, commandChat.Id)

		for _, code := range []struct{ set, remove string }{{"ws", "wr"}, {"gs", "gr"}} {
			for _, action := range []string{code.set, code.remove} {
				answers := env.answerCount()
				press(env, menuChat, menuAdmin, "a", action, "t", "x")
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("%s was answered %d times, want once", action, got)
				}
				answer, alert := env.lastAnswer()
				if !alert || !strings.Contains(answer, staffMarker("settings_no_change_info")) {
					t.Errorf("%s answered %q (alert %v), want the alert settings_no_change_info", action, answer, alert)
				}
			}
		}
		if keys := settingsEditInputKeys(env); len(keys) != 0 {
			t.Errorf("a refused Set created %v", keys)
		}
		if row, _ := settingsEditStored(t, menuChat.Id); row.WelcomeSettings.WelcomeText != "a custom welcome" || row.GoodbyeSettings.GoodbyeText != "a custom goodbye" {
			t.Errorf("a refused Remove changed the row: %+v", row)
		}

		env.sendIn(commandChat, commandAdmin, "/setwelcome something new")
		env.sendIn(commandChat, commandAdmin, "/resetwelcome")
		if !settingsAntifloodSentHas(env, commandChat.Id, staffMarker("chat_status_change_info_cmd_error")) {
			t.Error("/setwelcome and /resetwelcome were not refused for an administrator without can_change_info")
		}
		if row, _ := settingsEditStored(t, commandChat.Id); row.WelcomeSettings.WelcomeText != "a custom welcome" {
			t.Errorf("a refused command changed the welcome to %q", row.WelcomeSettings.WelcomeText)
		}

		// Cancel needs only the admin check: the answer is the stale prompt, never a rights refusal.
		press(env, menuChat, menuAdmin, "a", "wc", "t", "x")
		if answer, alert := env.lastAnswer(); !alert || strings.Contains(answer, staffMarker("settings_no_change_info")) {
			t.Errorf("Cancel answered %q (alert %v), want it allowed for any administrator", answer, alert)
		}
	})

	t.Run("an administrator with can_change_info", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		menuChat, commandChat := env.addChat(), env.addChat()
		editCleanup(t, menuChat.Id, commandChat.Id)
		menuAdmin, commandAdmin := newAdmin(env, menuChat, true), newAdmin(env, commandChat, true)
		settingsEditSeedGreetings(t, menuChat.Id)
		settingsEditSeedGreetings(t, commandChat.Id)

		press(env, menuChat, menuAdmin, "a", "ws", "t", "x")
		if !env.redis.Exists(settingsInputKey(menuAdmin.Id)) {
			t.Error("Set was refused for an administrator with can_change_info")
		}
		press(env, menuChat, menuAdmin, "a", "wr", "t", "x")
		env.sendIn(commandChat, commandAdmin, "/setwelcome something new")
		if row, _ := settingsEditStored(t, commandChat.Id); row.WelcomeSettings.WelcomeText == "a custom welcome" {
			t.Error("/setwelcome was refused for an administrator with can_change_info")
		}
		if row, _ := settingsEditStored(t, menuChat.Id); row.WelcomeSettings.WelcomeText != db.DefaultWelcome {
			t.Errorf("Remove text was refused: the welcome is %q", row.WelcomeSettings.WelcomeText)
		}
	})

	t.Run("rules need the bot as an administrator", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		owner := env.newOwner(env.chat)
		if err := rules.SetChatRules(env.chat.Id, "the rules"); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		for _, action := range []string{"rs", "rr"} {
			answers, edits := env.answerCount(), len(env.edits(owner.Id, 9191))
			press(env, env.chat, owner, "a", action, "t", "x")
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("%s was answered %d times, want once", action, got)
			}
			if got := len(env.edits(owner.Id, 9191)) - edits; got != 1 {
				t.Errorf("%s edited the message %d times, want the one redraw with the notice", action, got)
			}
			text, _ := env.lastEdit(owner.Id, 9191)
			if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("chat_status_bot_not_admin")) {
				t.Errorf("%s with the bot not an administrator drew %q, want the notice chat_status_bot_not_admin", action, text)
			}
		}
		if keys := settingsEditInputKeys(env); len(keys) != 0 {
			t.Errorf("a refused Rules Set created %v", keys)
		}
		if row, _ := settingsEditRulesStored(t, env.chat.Id); row.Rules != "the rules" {
			t.Errorf("a refused Rules Remove changed the rules to %q", row.Rules)
		}
		env.sendIn(env.chat, owner, "/setrules new rules")
		if !settingsAntifloodSentHas(env, env.chat.Id, staffMarker("connections_is_user_connected_bot_not_admin")) &&
			!settingsAntifloodSentHas(env, env.chat.Id, staffMarker("chat_status_bot_not_admin")) {
			t.Error("/setrules was not refused with the bot not an administrator")
		}
		if row, _ := settingsEditRulesStored(t, env.chat.Id); row.Rules != "the rules" {
			t.Errorf("a refused /setrules changed the rules to %q", row.Rules)
		}
		// Cancel needs no bot right.
		press(env, env.chat, owner, "a", "rc", "t", "x")
		if answer, _ := env.lastAnswer(); strings.Contains(answer, staffMarker("chat_status_bot_not_admin")) {
			t.Errorf("Cancel was refused for the bot's role: %q", answer)
		}
	})

	t.Run("rules with the bot as an administrator", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		if err := rules.SetChatRules(env.chat.Id, "the rules"); err != nil {
			t.Fatalf("SetChatRules: %v", err)
		}
		press(env, env.chat, owner, "a", "rs", "t", "x")
		if !env.redis.Exists(settingsInputKey(owner.Id)) {
			t.Error("Rules Set was refused with the bot an administrator")
		}
		press(env, env.chat, owner, "a", "rr", "t", "x")
		if row, _ := settingsEditRulesStored(t, env.chat.Id); row.Rules != "" {
			t.Errorf("Rules Remove left %q", row.Rules)
		}
	})
}
