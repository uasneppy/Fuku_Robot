//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/content"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// settingsEditReservedCheck runs the check of the buttons part of def for text over the stored state.
func settingsEditReservedCheck(def settingsEditPageDef, part, text string, st settingsEditState) settingsEditVerdict {
	st.Kind = def.Kind
	return settingsEditCheck(settingsEditCheckInput{
		Def: def, Part: part, Msg: settingsEditTypedMessage(text), State: st,
		Owner: gotgbot.User{Id: 1, FirstName: "Al"}, GroupTitle: "Group", Language: "en",
	})
}

// TestSettingsEditReservedButtons proves that the buttons the bot adds itself count against Telegram's
// limit of 100 buttons, on the buttons part and on the text part, and that the Welcome page is unchanged.
func TestSettingsEditReservedButtons(t *testing.T) {
	welcome, _ := settingsEditPageByKey(settingsPageWelcome)
	event := settingsEditPageDef{Key: "qtr", Kind: settingsEditEvent, Parts: welcome.Parts, ReservedButtons: 1}
	noReserve := event
	noReserve.ReservedButtons = 0
	refused := func(t *testing.T, got settingsEditVerdict) {
		t.Helper()
		if got.OK || got.RefusalKey != "settings_edit_refuse_buttons_limit" {
			t.Fatalf("OK=%v key=%q, want settings_edit_refuse_buttons_limit", got.OK, got.RefusalKey)
		}
	}
	accepted := func(t *testing.T, got settingsEditVerdict) {
		t.Helper()
		if !got.OK {
			t.Fatalf("refused with %q %q, want accepted", got.RefusalKey, got.RefusalText)
		}
	}

	t.Run("one reserved button leaves room for 99", func(t *testing.T) {
		accepted(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(99, 1000), settingsEditState{}))
		refused(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(100, 1000), settingsEditState{}))
	})
	t.Run("no reserved button leaves room for 100", func(t *testing.T) {
		accepted(t, settingsEditReservedCheck(noReserve, settingsEditPartButtons, settingsEditButtonLines(100, 1000), settingsEditState{}))
	})
	t.Run("the rules button and the reserved row count together", func(t *testing.T) {
		withToken := settingsEditState{Text: "Hi {rules}"}
		refused(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(99, 1000), withToken))
		accepted(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(98, 1000), withToken))
	})
	t.Run("a text that adds the rules button counts the reserved row", func(t *testing.T) {
		stored := settingsEditState{Buttons: settingsEditStoredButtons(98, 1)}
		accepted(t, settingsEditReservedCheck(event, settingsEditPartText, "Hi {rules}", stored))
		stored.Buttons = settingsEditStoredButtons(99, 1)
		refused(t, settingsEditReservedCheck(event, settingsEditPartText, "Hi {rules}", stored))
		accepted(t, settingsEditReservedCheck(noReserve, settingsEditPartText, "Hi {rules}", stored))
	})
	t.Run("the Welcome page's verdicts are unchanged", func(t *testing.T) {
		accepted(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(100, 1000), settingsEditState{}))
		refused(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(101, 1000), settingsEditState{}))
		withToken := settingsEditState{Text: "Hi {rules}"}
		accepted(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(99, 1000), withToken))
		refused(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(100, 1000), withToken))
	})
}

// settingsEditTestCodes are the action codes of the test-only editor page qt: unused by every real page
// and every other test page.
var settingsEditTestCodes = []string{"qs", "qr", "qc", "qv", "qf"}

// settingsEditTestPageKey is the page key of the test-only editor page.
const settingsEditTestPageKey = "qt"

// settingsEditTestCall is one call of a test definition's Write or Remove hook.
type settingsEditTestCall struct {
	Key, Part string
	Verdict   settingsEditVerdict
	By        int64
}

// settingsEditTestStore is the in-memory storage behind the test definitions: one state per definition
// key and the calls of their Write and Remove hooks.
type settingsEditTestStore struct {
	mu      sync.Mutex
	states  map[string]settingsEditState
	writes  []settingsEditTestCall
	removes []settingsEditTestCall
}

func newSettingsEditTestStore() *settingsEditTestStore {
	return &settingsEditTestStore{states: map[string]settingsEditState{}}
}

// state is the stored state of key as a Load hook returns it.
func (s *settingsEditTestStore) state(key string) settingsEditState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.states[key]
	st.Kind = settingsEditEvent
	st.TextIsDefault = st.Text == ""
	return st
}

// setText stores a text for key straight in the store.
func (s *settingsEditTestStore) setText(key, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.states[key]
	st.Text = text
	s.states[key] = st
}

// callsOf returns the calls of key among calls.
func (s *settingsEditTestStore) callsOf(calls []settingsEditTestCall, key string) []settingsEditTestCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []settingsEditTestCall
	for _, call := range calls {
		if call.Key == key {
			out = append(out, call)
		}
	}
	return out
}

// settingsEditTestDraw is the Draw hook of a test definition: its own body line naming the definition, the
// editor's part and preview rows and the navigation row.
func settingsEditTestDraw(v settingsView, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen {
	k := newSettingsKeyboard(v)
	settingsEditPartRows(k, def, st)
	settingsEditPreviewRows(k, def, st)
	k.navRow(false)
	return k.screen(settingsCompose(v, def.TitleKey, []string{"drawn:" + def.Key}, opts.Notice, false))
}

// settingsEditTestDef is a hooked definition of the test page: kind event, the test codes, the Welcome
// parts, the extra field k and hooks backed by store. withPreview gives it a Preview hook that sends one
// line naming the definition.
func settingsEditTestDef(key, k string, store *settingsEditTestStore, withPreview bool) settingsEditPageDef {
	hooks := &settingsEditHooks{
		Load: func(_ context.Context, _ int64) (settingsEditState, error) {
			return store.state(key), nil
		},
		Remove: func(_ context.Context, _ int64, part string, by int64) (greetingApplyOutcome, *settingsEditState) {
			store.mu.Lock()
			st := store.states[key]
			switch part {
			case settingsEditPartText:
				st.Text = ""
			case settingsEditPartMedia:
				st.FileID, st.MediaType = "", 0
			case settingsEditPartButtons:
				st.Buttons = nil
			}
			store.states[key] = st
			store.removes = append(store.removes, settingsEditTestCall{Key: key, Part: part, By: by})
			store.mu.Unlock()
			stored := store.state(key)
			return greetingApplied, &stored
		},
		Write: func(_ context.Context, _ int64, part string, verdict settingsEditVerdict, by int64) bool {
			store.mu.Lock()
			defer store.mu.Unlock()
			st := store.states[key]
			switch part {
			case settingsEditPartText:
				st.Text = verdict.Text
			case settingsEditPartButtons:
				st.Buttons = verdict.Buttons
			}
			store.states[key] = st
			store.writes = append(store.writes, settingsEditTestCall{Key: key, Part: part, Verdict: verdict, By: by})
			return true
		},
		Draw: settingsEditTestDraw,
	}
	if withPreview {
		hooks.Preview = func(p *settingsPress, _ settingsEditPageDef, _ settingsEditState, part string) error {
			_, err := p.b.SendMessage(p.query.From.Id, "preview:"+key+":"+part, &gotgbot.SendMessageOpts{ParseMode: formatting.HTML})
			return err
		}
	}
	welcome, _ := settingsEditPageByKey(settingsPageWelcome)
	return settingsEditPageDef{
		Key: key, Kind: settingsEditEvent, TitleKey: "settings_page_welcome", HintKey: "settings_edit_hint_greeting",
		SetCode: "qs", RemoveCode: "qr", CancelCode: "qc", PreviewCode: "qv", FullCode: "qf",
		EditRight: settingsRightChangeInfo, Parts: welcome.Parts,
		Fields: map[string]string{"k": k}, Hooks: hooks,
	}
}

// withTestSettingsEditPage installs the test page qt, whose actions are settingsEditResolvedActions over
// defs (a press names its definition by k and must carry the private owner field c), and registers defs
// with registerSettingsEditDef. The page draws defs[0]. Every registry entry is restored in t.Cleanup.
// A test that uses it never calls t.Parallel.
func withTestSettingsEditPage(t *testing.T, defs ...settingsEditPageDef) {
	t.Helper()
	byK := map[string]settingsEditPageDef{}
	for _, def := range defs {
		byK[def.Fields["k"]] = def
	}
	resolve := func(fields map[string]string) (settingsEditPageDef, bool) {
		if !settingsListPrivateField(fields) {
			return settingsEditPageDef{}, false
		}
		def, ok := byK[fields["k"]]
		return def, ok
	}
	for _, code := range settingsEditTestCodes {
		if owner, taken := settingsActions[code]; taken {
			t.Fatalf("the test editor page cannot use action code %q: page %q owns it", code, owner.pageKey)
		}
	}
	saved := settingsListRegistryOf(settingsEditTestPageKey, settingsEditTestCodes)
	page := settingsPage{
		Key: settingsEditTestPageKey, Order: 999, Icon: "🧪", TitleKey: "settings_page_welcome",
		Open:    settingsEditOpen(defs[0]),
		Actions: settingsEditResolvedActions(defs[0], []string{"k"}, resolve),
	}
	settingsPages[page.Key] = page
	for code, action := range page.Actions {
		settingsActions[code] = settingsActionEntry{pageKey: page.Key, action: action}
	}
	for _, def := range defs {
		registerSettingsEditDef(def)
	}
	t.Cleanup(func() {
		saved.restore(settingsEditTestPageKey, settingsEditTestCodes)
		for _, def := range defs {
			delete(settingsEditExtraDefs, def.Key)
		}
	})
}

// settingsEditLastAnswer is the text of the last answerCallbackQuery call.
func settingsEditLastAnswer(env *settingsEnv) string {
	calls := env.fake.callsFor("answerCallbackQuery")
	if len(calls) == 0 {
		return ""
	}
	return fmt.Sprint(calls[len(calls)-1].Params["text"])
}

// settingsEditWantK fails the test unless button carries k=want (or no k when want is "").
func settingsEditWantK(t *testing.T, button gotgbot.InlineKeyboardButton, want string) {
	t.Helper()
	fields := settingsButtonFields(t, button)
	if got, has := fields["k"]; got != want || (want == "") == has {
		t.Errorf("button %q carries k=%q (present %v), want %q", button.Text, got, has, want)
	}
}

// TestSettingsEditResolvedDefs drives a test-only page that edits two hooked definitions with one set of
// action codes through the real dispatcher, in the private menu of an administrator with can_change_info.
func TestSettingsEditResolvedDefs(t *testing.T) {
	env := newSettingsEnv(t)
	owner := env.newOwner(env.chat)
	store := newSettingsEditTestStore()
	qta := settingsEditTestDef("qta", "a", store, true)
	qtb := settingsEditTestDef("qtb", "b", store, false)
	withTestSettingsEditPage(t, qta, qtb)
	store.setText("qta", "stored a")
	store.setText("qtb", "stored b")
	t.Cleanup(func() {
		env.redis.Del(settingsInputKey(owner.Id))
		env.redis.Del(settingsInputDoneKey(owner.Id))
	})

	env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
	menuID := settingsPrivateOnlySent(t, env, owner).MessageID
	env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", settingsActOpen, "p", settingsEditTestPageKey))

	t.Run("the definition's own Draw carries k on its action buttons only", func(t *testing.T) {
		text, keyboard := env.lastEdit(owner.Id, menuID)
		if !strings.Contains(text, "drawn:qta") {
			t.Fatalf("the page %q is not qta's Draw output", text)
		}
		for _, key := range []string{"settings_btn_edit_set_text", "settings_btn_edit_remove_text", "settings_btn_edit_preview_text", "settings_btn_edit_full_preview"} {
			button, ok := settingsButtonByMarker(keyboard, key)
			if !ok {
				t.Fatalf("the page has no %s button: %+v", key, keyboard)
			}
			settingsEditWantK(t, button, "a")
		}
		for _, key := range []string{"settings_btn_back", "settings_btn_close"} {
			button, ok := settingsButtonByMarker(keyboard, key)
			if !ok {
				t.Fatalf("the page has no %s button", key)
			}
			settingsEditWantK(t, button, "")
		}
	})

	t.Run("Set writes the definition's wait and the prompt's Cancel carries k", func(t *testing.T) {
		fields := settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
		if fields["a"] != "qs" || fields["k"] != "a" || fields["t"] != "x" {
			t.Fatalf("Set carries %v", fields)
		}
		key := settingsInputKey(owner.Id)
		if p, part := env.redis.HGet(key, "p"), env.redis.HGet(key, "t"); p != "qta" || part != "x" {
			t.Fatalf("the wait has p=%q t=%q, want qta and x", p, part)
		}
		_, keyboard := env.lastEdit(owner.Id, menuID)
		cancel, ok := settingsButtonByMarker(keyboard, "settings_btn_edit_cancel")
		if !ok {
			t.Fatalf("the prompt has no Cancel: %+v", keyboard)
		}
		got := settingsButtonFields(t, cancel)
		if got["a"] != "qc" || got["k"] != "a" || got["t"] != "x" || got["c"] != fmt.Sprint(env.chat.Id) || len(got) != 4 {
			t.Errorf("the prompt's Cancel carries %v, want qc, k=a, t=x and c", got)
		}
	})

	t.Run("a typed text reaches qta's Write and the saved page is qta's Draw", func(t *testing.T) {
		const typed = "Hi there bold"
		entities := []gotgbot.MessageEntity{{Type: "bold", Offset: 9, Length: 4}}
		before := len(env.fake.sentTo(owner.Id))
		sendPrivateMessage(env, owner, gotgbot.Message{Text: typed, Entities: entities})

		want := content.ExtractWelcome(&gotgbot.Message{
			Text:     "/setwelcome " + typed,
			Entities: shiftEntities(entities, "/setwelcome "),
		}, "welcome", "en").Text
		writes := store.callsOf(store.writes, "qta")
		if len(writes) != 1 || writes[0].Part != "x" || writes[0].Verdict.Text != want || writes[0].By != owner.Id {
			t.Fatalf("qta's Write calls = %+v, want one text write of %q by %d", writes, want, owner.Id)
		}
		if other := store.callsOf(store.writes, "qtb"); len(other) != 0 {
			t.Errorf("qtb's Write was called: %+v", other)
		}
		sent := env.fake.sentTo(owner.Id)
		if len(sent) != before+1 {
			t.Fatalf("the bot sent %d messages, want the saved page alone", len(sent)-before)
		}
		page := fmt.Sprint(sent[len(sent)-1].Params["text"])
		if !strings.Contains(page, "drawn:qta") || !containsMarker(page, "settings_edit_saved_text") {
			t.Errorf("the saved page %q is not qta's Draw with the saved notice", page)
		}
		closed, closedKeyboard := env.lastEdit(owner.Id, menuID)
		if !containsMarker(closed, "settings_edit_prompt_closed") || len(closedKeyboard) != 0 {
			t.Errorf("the prompt reads %q with %+v, want the closed line and no keyboard", closed, closedKeyboard)
		}
		settingsEditWaitGone(t, env, owner)
	})

	t.Run("a refused text's Cancel carries k and ends that wait", func(t *testing.T) {
		env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", settingsActOpen, "p", settingsEditTestPageKey))
		settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_text")
		before := len(env.fake.sentTo(owner.Id))
		sendPrivateMessage(env, owner, gotgbot.Message{Text: "Hello\n[Rules](buttonurl://https://e.com)"})
		sent := env.fake.sentTo(owner.Id)
		if len(sent) != before+1 {
			t.Fatalf("the bot sent %d messages, want the refusal alone", len(sent)-before)
		}
		refusal := sent[len(sent)-1]
		if text := fmt.Sprint(refusal.Params["text"]); !containsMarker(text, "settings_edit_refuse_text_buttons") {
			t.Fatalf("the refusal reads %q", text)
		}
		keyboard := staffKeyboardOf(refusal.Params["reply_markup"])
		if len(keyboard) != 1 || len(keyboard[0]) != 1 {
			t.Fatalf("the refusal keyboard = %+v, want Cancel alone", keyboard)
		}
		fields := settingsButtonFields(t, keyboard[0][0])
		if fields["a"] != "qc" || fields["k"] != "a" || fields["t"] != "x" || fields["c"] != fmt.Sprint(env.chat.Id) || len(fields) != 4 {
			t.Fatalf("the refusal's Cancel carries %v, want qc, k=a, t=x and c", fields)
		}
		settingsEditWaitKept(t, env, owner)
		env.pressRaw(settingsPrivateChat(owner), owner, refusal.MessageID, keyboard[0][0].CallbackData)
		settingsEditWaitGone(t, env, owner)
		if text, _ := env.lastEdit(owner.Id, refusal.MessageID); !strings.Contains(text, "drawn:qta") || !containsMarker(text, "settings_edit_cancelled") {
			t.Errorf("the refusal was edited to %q, want qta's page with the cancelled notice", text)
		}
	})

	t.Run("Remove of qtb calls qtb's Remove with the presser", func(t *testing.T) {
		env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "qr", "k", "b", "t", "x"))
		removes := store.callsOf(store.removes, "qtb")
		if len(removes) != 1 || removes[0].Part != "x" || removes[0].By != owner.Id {
			t.Fatalf("qtb's Remove calls = %+v, want one text remove by %d", removes, owner.Id)
		}
		if other := store.callsOf(store.removes, "qta"); len(other) != 0 {
			t.Errorf("qta's Remove was called: %+v", other)
		}
		if text, _ := env.lastEdit(owner.Id, menuID); !strings.Contains(text, "drawn:qtb") {
			t.Errorf("the page after Remove reads %q, want qtb's Draw", text)
		}
	})

	t.Run("an unknown message or part is refused before any lookup", func(t *testing.T) {
		for _, kv := range [][]string{
			{"a", "qs", "k", "z", "t", "x"},
			{"a", "qs", "k", "a", "t", "q"},
			{"a", "qf", "k", "z"},
			{"a", "qs", "t", "x"},
		} {
			lookups := env.memberLookupsOf(env.chat.Id, owner.Id)
			env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, kv...))
			if got := settingsEditLastAnswer(env); !containsMarker(got, "common_callback_invalid_request") {
				t.Errorf("%v answered %q, want common_callback_invalid_request", kv, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, owner.Id); got != lookups {
				t.Errorf("%v made %d getChatMember calls, want none", kv, got-lookups)
			}
		}
		// The same press of a group menu (u instead of c) is refused alike.
		env.press(owner, menuID, env.groupFields(owner, "a", "qs", "k", "a", "t", "x"))
		if got := settingsEditLastAnswer(env); !containsMarker(got, "common_callback_invalid_request") {
			t.Errorf("a group-menu press answered %q, want common_callback_invalid_request", got)
		}
	})

	t.Run("an administrator without can_change_info cannot Set", func(t *testing.T) {
		admin := gotgbot.User{Id: uniqueLinkOwnerID() + 7_100_000, FirstName: "NoInfo"}
		listEditAdmin(env, env.chat, admin, false)
		t.Cleanup(func() { env.redis.Del(settingsInputKey(admin.Id)) })
		env.pressPrivate(admin, menuID, env.privateFields(env.chat.Id, "a", "qs", "k", "a", "t", "x"))
		if got := settingsEditLastAnswer(env); !containsMarker(got, "settings_no_change_info") {
			t.Errorf("Set answered %q, want settings_no_change_info", got)
		}
		if env.redis.Exists(settingsInputKey(admin.Id)) {
			t.Error("a refused Set wrote a wait")
		}
	})

	t.Run("Preview goes through the hook, and a definition without one fails", func(t *testing.T) {
		before := len(env.fake.sentTo(owner.Id))
		env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "qv", "k", "a", "t", "x"))
		sent := env.fake.sentTo(owner.Id)
		if len(sent) != before+2 || fmt.Sprint(sent[len(sent)-1].Params["text"]) != "preview:qta:x" {
			t.Fatalf("qta's Preview sent %+v, want the header and the hook's line", sent[before:])
		}
		store.setText("qtb", "stored b")
		before = len(sent)
		env.pressPrivate(owner, menuID, env.privateFields(env.chat.Id, "a", "qf", "k", "b"))
		if got := settingsEditLastAnswer(env); !containsMarker(got, "settings_edit_preview_failed") {
			t.Errorf("qtb's Full preview answered %q, want settings_edit_preview_failed", got)
		}
		if got := len(env.fake.sentTo(owner.Id)); got != before {
			t.Errorf("qtb's Full preview sent %d messages, want none", got-before)
		}
	})
}
