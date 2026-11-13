//go:build testtools

package modules

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

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
