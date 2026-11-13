//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
)

// settingsListTwoStepEnv is the settings list environment with the test-only two-step page installed over the
// registries (restored when the test ends) and the in-memory store behind it.
type settingsListTwoStepEnv struct {
	*settingsListEnv
	store *settingsListTestStore
	user  gotgbot.User
}

// newSettingsListTwoStepEnv builds the environment and installs the test page.
func newSettingsListTwoStepEnv(t *testing.T) *settingsListTwoStepEnv {
	t.Helper()
	env := newSettingsListEnv(t)
	store := &settingsListTestStore{}
	withTestSettingsList(t, settingsListTestTwoStepDef(store))
	return &settingsListTwoStepEnv{settingsListEnv: env, store: store, user: env.admin}
}

// begin opens the test page in a fresh private menu of the env's administrator and presses Add, so the user has a wait
// for the keyword part. It returns the menu, whose message is now the step-1 prompt.
func (e *settingsListTwoStepEnv) begin() *settingsBlacklistMenu {
	e.t.Helper()
	m := &settingsBlacklistMenu{env: e.settingsListEnv, chat: e.chat, user: e.user, msgID: e.updateID()}
	m.press("a", settingsActOpen, "p", "i")
	add, ok := m.markedButton("settings_btn_bl_add")
	if !ok {
		e.t.Fatalf("the test page has no Add button")
	}
	m.tap(add)
	return m
}

// field reads one field of the user's wait hash.
func (e *settingsListTwoStepEnv) field(name string) string {
	return e.redis.HGet(settingsInputKey(e.user.Id), name)
}

// say sends text as a private message of the user and returns the message's ID.
func (e *settingsListTwoStepEnv) say(text string) int64 {
	e.t.Helper()
	return settingsListDoorSay(e.settingsListEnv, e.user, text)
}

// sent is how many messages the bot has sent to the user so far.
func (e *settingsListTwoStepEnv) sent() int {
	return settingsEditSentCount(e.settingsEnv, e.user)
}

// reply fails the test unless exactly one message was sent after the first `before` and it carries every key's marker.
func (e *settingsListTwoStepEnv) reply(before int, keys ...string) staffSentMessage {
	e.t.Helper()
	return settingsListDoorReply(e.t, e.settingsListEnv, e.user, before, keys...)
}

// settingsListTestPhoto is the content message of the flow test: a photo with a caption and a bold entity.
func settingsListTestPhoto() gotgbot.Message {
	msg := settingsMediaItemID("photo", "price-photo")
	msg.Caption = "Our prices"
	msg.CaptionEntities = []gotgbot.MessageEntity{{Type: "bold", Offset: 4, Length: 6}}
	return msg
}

func TestSettingsListTwoStepFlow(t *testing.T) {
	const page = "i"
	before := settingsListRegistryOf(page, settingsListTestTwoStepCodes)
	t.Run("flow", func(t *testing.T) {
		e := newSettingsListTwoStepEnv(t)
		m := e.begin()

		prompt, keyboard := m.last()
		for _, want := range []string{
			staffMarker("settings_bl_wait"), staffMarker("settings_entries_step") + " 1 2", staffMarker("settings_edit_wait_footer"),
		} {
			if !strings.Contains(prompt, want) {
				t.Errorf("the step-1 prompt lacks %q:\n%s", want, prompt)
			}
		}
		if len(keyboard) != 1 || len(keyboard[0]) != 1 {
			t.Fatalf("the step-1 keyboard = %+v, want one row with Cancel", keyboard)
		}
		if fields := settingsButtonFields(t, keyboard[0][0]); fields["a"] != "zc" || fields["t"] != "k" || len(fields) != 3 {
			t.Errorf("Cancel carries %v, want zc, t=k and c", fields)
		}
		if e.field("p") != page || e.field("t") != "k" || e.field("w") != "" || e.field("q") != "" {
			t.Fatalf("the wait is %s/%s w=%q q=%q, want i/k with neither", e.field("p"), e.field("t"), e.field("w"), e.field("q"))
		}
		oldNonce := e.field("n")

		sentBefore := e.sent()
		keywordID := e.say("Price  List")

		if e.field("t") != "y" || e.field("w") != "price list" || e.field("q") != strconv.FormatInt(keywordID, 10) {
			t.Errorf("after step 1 the wait is t=%q w=%q q=%q, want y, \"price list\" and %d", e.field("t"), e.field("w"), e.field("q"), keywordID)
		}
		if nonce := e.field("n"); nonce == "" || nonce == oldNonce {
			t.Errorf("the nonce after the advance = %q, want a new one (was %q)", nonce, oldNonce)
		}
		step2 := e.reply(sentBefore, "settings_bl_wait", "settings_entries_wait_content_limits", "settings_edit_wait_footer")
		text := fmt.Sprint(step2.Params["text"])
		for _, want := range []string{
			staffMarker("settings_bl_delete_ask") + " price list",
			staffMarker("settings_entries_wait_content_limits") + " 4096 1024",
			staffMarker("settings_entries_step") + " 2 2",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the step-2 prompt lacks %q:\n%s", want, text)
			}
		}
		if reply := fmt.Sprint(step2.Params["reply_parameters"]); !strings.Contains(reply, strconv.FormatInt(keywordID, 10)) {
			t.Errorf("the step-2 prompt replies to %s, want the keyword message %d", reply, keywordID)
		}
		step2Keyboard := staffKeyboardOf(step2.Params["reply_markup"])
		if len(step2Keyboard) != 1 || len(step2Keyboard[0]) != 1 {
			t.Fatalf("the step-2 keyboard = %+v, want one row with Cancel", step2Keyboard)
		}
		if fields := settingsButtonFields(t, step2Keyboard[0][0]); fields["a"] != "zc" || fields["t"] != "y" || len(fields) != 3 {
			t.Errorf("the step-2 Cancel carries %v, want zc, t=y and c", fields)
		}
		if got := e.field("m"); got != strconv.FormatInt(step2.MessageID, 10) {
			t.Errorf("the wait's prompt = %q, want the step-2 prompt %d", got, step2.MessageID)
		}
		closed, closedKeyboard := m.last()
		if !strings.Contains(closed, staffMarker("settings_entries_step_done")) || len(closedKeyboard) != 0 {
			t.Errorf("the step-1 prompt reads %q with keyboard %+v, want settings_entries_step_done and none", closed, closedKeyboard)
		}
		if len(e.store.applies) != 0 {
			t.Fatalf("step 1 stored %d entries", len(e.store.applies))
		}

		sentBefore = e.sent()
		photo := settingsListTestPhoto()
		sendPrivateMessage(e.settingsEnv, e.user, photo)

		if len(e.store.applies) != 1 {
			t.Fatalf("Apply was called %d times, want once", len(e.store.applies))
		}
		want := content.ExtractTypedNoteOrFilter(&photo, "price list", true, "en")
		got := e.store.applies[0]
		if got.Norm != "price list" || !reflect.DeepEqual(got.Ex, want) || want.DataType != db.PHOTO || want.FileID != "price-photo" {
			t.Errorf("Apply got %q and %+v, want price list and the command's reply form %+v", got.Norm, got.Ex, want)
		}
		saved := e.reply(sentBefore, "settings_page_blacklist")
		savedText := fmt.Sprint(saved.Params["text"])
		for _, wantText := range []string{settingsNoticeOKIcon + staffMarker("settings_bl_removed") + " price list", "<code>price list</code>"} {
			if !strings.Contains(savedText, wantText) {
				t.Errorf("the saved page lacks %q:\n%s", wantText, savedText)
			}
		}
		if closedText, closedKeyboard := e.lastEdit(e.user.Id, step2.MessageID); !strings.Contains(closedText, staffMarker("settings_edit_prompt_closed")) || len(closedKeyboard) != 0 {
			t.Errorf("the step-2 prompt reads %q with keyboard %+v, want the closed line and none", closedText, closedKeyboard)
		}
		if keys := settingsEditInputKeys(e.settingsEnv); len(keys) != 0 {
			t.Errorf("the finished flow left %v behind", keys)
		}
	})

	after := settingsListRegistryOf(page, settingsListTestTwoStepCodes)
	if !reflect.DeepEqual(settingsListRegistryKeys(before), settingsListRegistryKeys(after)) {
		t.Errorf("the registries hold %v after the test, want what they held before it: %v",
			settingsListRegistryKeys(after), settingsListRegistryKeys(before))
	}
}

// settingsListRegistryKeys is what a snapshot says is registered, as a comparable value.
func settingsListRegistryKeys(snap settingsListRegistry) map[string]bool {
	keys := map[string]bool{"page": snap.hasPage, "list": snap.hasList}
	for code, has := range snap.hasActions {
		keys["action "+code] = has
	}
	return keys
}

// reset ends whatever wait the previous case left (the wait, its marker) and gives the test page a fresh store, so the
// cases of one environment do not see each other.
func (e *settingsListTwoStepEnv) reset() {
	e.t.Helper()
	e.redis.Del(settingsInputKey(e.user.Id))
	e.redis.Del(settingsInputDoneKey(e.user.Id))
	*e.store = settingsListTestStore{}
}

// advance runs step 1 with keyword and returns the menu, the keyword message's ID and the step-2 prompt the bot sent.
func (e *settingsListTwoStepEnv) advance(keyword string) (*settingsBlacklistMenu, int64, staffSentMessage) {
	e.t.Helper()
	m := e.begin()
	before := e.sent()
	id := e.say(keyword)
	prompt := e.reply(before, "settings_entries_wait_content_limits")
	if e.field("t") != "y" {
		e.t.Fatalf("after step 1 the wait is for part %q, want y", e.field("t"))
	}
	return m, id, prompt
}

// sayAgain delivers text once more as the private message with the given ID, as Telegram can redeliver an update.
func (e *settingsListTwoStepEnv) sayAgain(text string, id int64) {
	e.t.Helper()
	msg := gotgbot.Message{Text: text, MessageId: id, Date: 1, Chat: settingsPrivateChat(e.user), From: &e.user}
	if err := e.dispatcher.ProcessUpdate(e.bot, &gotgbot.Update{UpdateId: e.updateID(), Message: &msg}, nil); err != nil {
		e.t.Fatalf("redelivery of message %d: %v", id, err)
	}
}

// cancelFields is the fields of the one Cancel button of a reply's keyboard, with its fields.
func (e *settingsListTwoStepEnv) cancelFields(sent staffSentMessage) map[string]string {
	e.t.Helper()
	keyboard := staffKeyboardOf(sent.Params["reply_markup"])
	if len(keyboard) != 1 || len(keyboard[0]) != 1 || !containsMarker(keyboard[0][0].Text, "settings_btn_edit_cancel") {
		e.t.Fatalf("the reply keyboard = %+v, want one row with Cancel", keyboard)
	}
	return settingsButtonFields(e.t, keyboard[0][0])
}

// waitStays fails the test unless the wait still exists at part, holding no claim.
func (e *settingsListTwoStepEnv) waitStays(part, keyword string) {
	e.t.Helper()
	settingsEditWaitKept(e.t, e.settingsEnv, e.user)
	if e.field("t") != part || e.field("w") != keyword {
		e.t.Errorf("the wait is t=%q w=%q, want t=%q w=%q", e.field("t"), e.field("w"), part, keyword)
	}
}

func TestSettingsListTwoStepBranches(t *testing.T) {
	e := newSettingsListTwoStepEnv(t)

	t.Run("step 1 refusals keep the wait at step 1 and the next text is read as a keyword", func(t *testing.T) {
		e.reset()
		e.begin()
		nonce := e.field("n")
		e.redis.FastForward(2 * time.Second)
		ttl := e.redis.TTL(settingsInputKey(e.user.Id))

		refusals := []struct {
			name  string
			send  func()
			keys  []string
			setup func()
			words string
		}{
			{"a photo", func() { sendPrivateMessage(e.settingsEnv, e.user, settingsMediaItem("photo")) }, []string{"settings_edit_refuse_need_text"}, func() {}, ""},
			{"U+3000 alone", func() { e.say("\u3000") }, []string{"settings_entries_refuse_blank"}, func() {}, ""},
			{"a refusal of Check", func() { e.say("Taken Word") }, nil, func() { e.store.checkRefusal = "check-says-no" }, "check-says-no"},
		}
		for _, c := range refusals {
			c.setup()
			checks, before := e.store.checkCalls, e.sent()
			c.send()
			reply := e.reply(before, append(c.keys, "settings_edit_still_waiting")...)
			if fields := e.cancelFields(reply); fields["a"] != "zc" || fields["t"] != "k" || len(fields) != 3 {
				t.Errorf("%s: Cancel carries %v, want zc, t=k and c", c.name, fields)
			}
			if reply.Params["reply_parameters"] == nil {
				t.Errorf("%s: the refusal is not a reply to the message", c.name)
			}
			if text := fmt.Sprint(reply.Params["text"]); !strings.Contains(text, c.words) {
				t.Errorf("%s: the reply lacks %q: %s", c.name, c.words, text)
			}
			e.waitStays("k", "")
			if e.field("n") != nonce || e.field("q") != "" || e.redis.TTL(settingsInputKey(e.user.Id)) != ttl {
				t.Errorf("%s: the refusal moved or extended the wait", c.name)
			}
			if c.name != "a refusal of Check" && e.store.checkCalls != checks {
				t.Errorf("%s: Check was called for a message the engine refuses first", c.name)
			}
		}
		e.store.checkRefusal = ""
		before := e.sent()
		e.say("Fresh Word")
		e.reply(before, "settings_entries_wait_content_limits")
		e.waitStays("y", "fresh word")
	})

	t.Run("an error of Check keeps the wait", func(t *testing.T) {
		e.reset()
		e.begin()
		e.store.checkErr = errors.New("database is down")
		before := e.sent()
		e.say("word")
		e.reply(before, "settings_check_failed")
		e.waitStays("k", "")
	})

	t.Run("a page's Add gate draws its screen and creates no wait", func(t *testing.T) {
		e.reset()
		e.store.gate = "the-gate-screen"
		m := &settingsBlacklistMenu{env: e.settingsListEnv, chat: e.chat, user: e.user, msgID: e.updateID()}
		m.press("a", settingsActOpen, "p", "i")
		add, _ := m.markedButton("settings_btn_bl_add")
		m.tap(add)
		if text, _ := m.last(); text != "the-gate-screen" {
			t.Errorf("Add drew %q, want the gate's screen", text)
		}
		if keys := settingsEditInputKeys(e.settingsEnv); len(keys) != 0 {
			t.Errorf("a gated Add created %v", keys)
		}
	})

	t.Run("a redelivered keyword message is ignored at step 2", func(t *testing.T) {
		e.reset()
		_, keywordID, _ := e.advance("Price List")
		before := e.sent()
		e.sayAgain("Price List", keywordID)
		if texts := settingsListDoorTexts(e.settingsListEnv, e.user, before); len(texts) != 0 {
			t.Errorf("the redelivery was answered: %q", texts)
		}
		e.waitStays("y", "price list")
		if len(e.store.applies) != 0 {
			t.Errorf("the redelivery was stored: %+v", e.store.applies)
		}
	})

	t.Run("a poll is refused with the extraction's words and keeps part y and w", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		before := e.sent()
		sendPrivateMessage(e.settingsEnv, e.user, settingsMediaItem("poll"))
		reply := e.reply(before, "content_need_filter_content", "settings_edit_still_waiting")
		if fields := e.cancelFields(reply); fields["a"] != "zc" || fields["t"] != "y" {
			t.Errorf("the refusal's Cancel carries %v, want zc and t=y", fields)
		}
		e.waitStays("y", "price list")
		if len(e.store.applies) != 0 {
			t.Errorf("a poll was stored")
		}
	})

	t.Run("a Check that now refuses ends the wait with its text", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		e.store.checkRefusal = "taken-between"
		before := e.sent()
		e.say("the reply")
		reply := e.reply(before, "settings_edit_wait_stopped")
		if text := fmt.Sprint(reply.Params["text"]); !strings.Contains(text, "taken-between") {
			t.Errorf("the end lacks the refusal's text: %s", text)
		}
		settingsEditWaitGone(t, e.settingsEnv, e.user)
		if len(e.store.applies) != 0 {
			t.Errorf("an entry refused at step 2 was stored")
		}
	})

	t.Run("an Apply that answers taken ends the wait with its text", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		e.store.applyOutcome, e.store.applyText = settingsListApplyTaken, "lost-the-race"
		before := e.sent()
		e.say("the reply")
		reply := e.reply(before, "settings_edit_wait_stopped")
		if text := fmt.Sprint(reply.Params["text"]); !strings.Contains(text, "lost-the-race") {
			t.Errorf("the end lacks the taken text: %s", text)
		}
		settingsEditWaitGone(t, e.settingsEnv, e.user)
	})

	t.Run("an Apply that fails releases the claim and the next reply is applied", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		e.store.applyOutcome = settingsListApplyFailed
		before := e.sent()
		e.say("the reply")
		reply := e.reply(before, "common_settings_save_failed", "settings_edit_still_waiting")
		if fields := e.cancelFields(reply); fields["t"] != "y" {
			t.Errorf("the refusal's Cancel carries %v, want t=y", fields)
		}
		e.waitStays("y", "price list")

		e.store.applyOutcome = settingsListApplySaved
		before = e.sent()
		e.say("the reply again")
		e.reply(before, "settings_page_blacklist")
		settingsEditWaitGone(t, e.settingsEnv, e.user)
		if len(e.store.applies) != 2 || len(e.store.entries) != 1 || e.store.entries[0].Text != "price list" {
			t.Errorf("applies %d, entries %+v, want two calls and one stored entry", len(e.store.applies), e.store.entries)
		}
	})

	t.Run("an album is applied once from its first item and the notice says so", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		const group = "9100456"
		first, second := settingsMediaItemID("photo", "album-1"), settingsMediaItemID("photo", "album-2")
		first.Caption = "first caption"
		first.MediaGroupId, second.MediaGroupId = group, group
		e.store.applyNotice = "page-extra-line"
		before := e.sent()
		sendPrivateMessage(e.settingsEnv, e.user, first)
		sendPrivateMessage(e.settingsEnv, e.user, second)
		reply := e.reply(before, "settings_edit_album_cut")
		if text := fmt.Sprint(reply.Params["text"]); !strings.Contains(text, "page-extra-line") {
			t.Errorf("the saved page lacks the page's own notice line: %s", text)
		}
		if len(e.store.applies) != 1 || e.store.applies[0].Ex.FileID != "album-1" {
			t.Errorf("applies %+v, want one call from the first item", e.store.applies)
		}
	})

	t.Run("Add at step 2 restarts at step 1 with the replaced note", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		m := e.begin()
		if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_edit_replaced_note")) {
			t.Errorf("the new prompt lacks the replaced note:\n%s", text)
		}
		e.waitStays("k", "")
		if e.field("q") != "" {
			t.Errorf("the restarted wait still holds q=%q", e.field("q"))
		}
	})

	t.Run("a reply after the ten minutes names the reply part once", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		e.redis.FastForward(settingsInputTTL + time.Second)
		before := e.sent()
		e.say("too late")
		e.reply(before, "settings_entries_wait_ended_note", "settings_entries_part_reply")
		settingsEditWaitGone(t, e.settingsEnv, e.user)
		if len(e.store.applies) != 0 {
			t.Errorf("a late reply was stored")
		}
	})

	t.Run("/cancel at step 2 deletes the wait and its w", func(t *testing.T) {
		e.reset()
		e.advance("Price List")
		before := e.sent()
		sendPrivateMessage(e.settingsEnv, e.user, gotgbot.Message{
			Text: "/cancel", Entities: []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}},
		})
		e.reply(before, "settings_edit_cancelled")
		settingsEditWaitGone(t, e.settingsEnv, e.user)
		if e.redis.Exists(settingsInputKey(e.user.Id)) || e.field("w") != "" {
			t.Errorf("the cancelled wait still holds w")
		}
	})
}
