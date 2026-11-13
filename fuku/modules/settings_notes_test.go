//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	db_notes "github.com/uasneppy/fuku_robot/fuku/db/notes"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
)

// settingsNotesCountSeams replaces the page's read and count seams with counting wrappers and restores them.
func settingsNotesCountSeams(t *testing.T) (reads, counts *atomic.Int64) {
	t.Helper()
	reads, counts = new(atomic.Int64), new(atomic.Int64)
	read, count := settingsNotesRead, settingsNotesCount
	settingsNotesRead = func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		reads.Add(1)
		return read(ctx, chatID)
	}
	settingsNotesCount = func(ctx context.Context, chatID int64) (int, error) {
		counts.Add(1)
		return count(ctx, chatID)
	}
	t.Cleanup(func() { settingsNotesRead, settingsNotesCount = read, count })
	return reads, counts
}

// settingsNotesStored is the chat's stored note rows by name.
func settingsNotesStored(t *testing.T, chatID int64) map[string]models.Notes {
	t.Helper()
	var rows []models.Notes
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("list the notes: %v", err)
	}
	stored := make(map[string]models.Notes, len(rows))
	for _, row := range rows {
		stored[row.NoteName] = row
	}
	return stored
}

// settingsNotesFlow is a creator of one chat working the Notes page's Add flow. It is the Filters flow (the owner, their
// private menu and the helpers that read the wait and the bot's replies) with its own begin.
type settingsNotesFlow struct {
	*settingsFiltersFlow
}

// newSettingsNotesFlow makes the creator of chat.
func newSettingsNotesFlow(env *settingsListEnv, chat gotgbot.Chat) *settingsNotesFlow {
	env.t.Helper()
	return &settingsNotesFlow{newSettingsFiltersFlow(env, chat)}
}

// begin opens the Notes page in a fresh private menu and presses Add, so the owner has a wait for the name part. It
// returns the menu.
func (f *settingsNotesFlow) begin() *settingsBlacklistMenu {
	f.env.t.Helper()
	m := settingsListOpenMenuPage(f.env, f.chat, f.user, settingsPageNotes)
	add, ok := m.markedButton("settings_btn_notes_add")
	if !ok {
		f.env.t.Fatalf("the Notes page has no Add button")
	}
	m.tap(add)
	return m
}

// settingsNotesContentMessage is the step-2 message of the tests: a text with a bold word, an {admin} directive and a
// markdown button.
func settingsNotesContentMessage() gotgbot.Message {
	return gotgbot.Message{
		Text:     "Read the rules\n{admin} [Docs](buttonurl://https://example.com)",
		Entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 9, Length: 5}},
	}
}

func TestSettingsNotesOpens(t *testing.T) {
	t.Run("a private menu lists the names in order with the intro, the count, the admin-only tag and the Add button", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedNotes(t, env.chat.Id, "gamma", "alpha")
		settingsListSeedAdminNote(t, env.chat.Id, "beta")
		_, text, keyboard := settingsListOpenPrivate(env, env.admin, settingsPageNotes)

		for _, want := range []string{
			staffMarker("settings_header"), staffMarker("settings_page_notes"), staffMarker("settings_notes_intro"),
			staffMarker("settings_notes_count") + " 3",
			"2. <code>beta</code> · " + staffMarker("settings_notes_admin_tag"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the page lacks %q:\n%s", want, text)
			}
		}
		last := -1
		for i, name := range []string{"alpha", "beta", "gamma"} {
			line := fmt.Sprintf("%d. <code>%s</code>", i+1, name)
			at := strings.Index(text, line)
			if at < 0 || at < last {
				t.Errorf("the line %q is missing or out of order:\n%s", line, text)
			}
			last = at
		}
		if strings.Contains(text, "<code>alpha</code> · ") || strings.Contains(text, "<code>gamma</code> · ") {
			t.Errorf("a note anyone can fetch is tagged admin-only:\n%s", text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_btn_notes_add"); !ok {
			t.Errorf("the page has no Add note button: %+v", keyboard)
		}
	})

	t.Run("an empty list draws the empty line and the Add button", func(t *testing.T) {
		env := newSettingsListEnv(t)
		chat := env.addChat()
		settingsListCleanup(t, chat.Id)
		m := settingsListOpenMenuPage(env, chat, env.admin, settingsPageNotes)
		text, keyboard := m.last()
		if !strings.Contains(text, staffMarker("settings_notes_empty")) || strings.Contains(text, staffMarker("settings_notes_count")) {
			t.Errorf("the empty page drew:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{1, 2}) {
			t.Errorf("empty rows = %v, want {1,2}", got)
		}
	})

	t.Run("Main's Notes line counts in private, shows the unknown mark on a failing count and In private in a group", func(t *testing.T) {
		setCachedBotUsername(t, "FukuTestBot")
		env := newSettingsListEnv(t)
		settingsListSeedNotes(t, env.chat.Id, settingsListWords(3)...)
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		line, ok := settingsMainStateLine(text, "settings_page_notes")
		if !ok || !strings.HasSuffix(line, " 3") {
			t.Fatalf("Main's Notes line = %q, want it to end in 3:\n%s", line, text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_page_notes"); !ok {
			t.Errorf("Main has no Notes cell")
		}

		reads, counts := settingsNotesCountSeams(t)
		groupMsg := env.openMenu(env.admin)
		groupMain, _ := env.lastEdit(env.chat.Id, groupMsg)
		groupLine, ok := settingsMainStateLine(groupMain, "settings_page_notes")
		if !ok || !strings.HasSuffix(groupLine, staffMarker("settings_state_private_only")) {
			t.Errorf("a group Main's Notes line = %q, want In private", groupLine)
		}
		if counts.Load() != 0 || reads.Load() != 0 {
			t.Errorf("a group Main read the notes %d times and counted them %d times, want none", reads.Load(), counts.Load())
		}

		settingsNotesCount = func(context.Context, int64) (int, error) { return 0, errors.New("count failed") }
		failedMsg := env.updateID()
		env.pressPrivate(env.admin, failedMsg, env.privateFields(env.chat.Id, "a", settingsActBack))
		failed, _ := env.lastEdit(env.admin.Id, failedMsg)
		unknown, ok := settingsMainStateLine(failed, "settings_page_notes")
		if !ok || !strings.HasSuffix(unknown, settingsUnknownState) {
			t.Errorf("a failing count reads %q, want the unknown mark", unknown)
		}
	})

	t.Run("a failing read is the cannot-open edit", func(t *testing.T) {
		env := newSettingsListEnv(t)
		read := settingsNotesRead
		settingsNotesRead = func(context.Context, int64) ([]settingsListEntry, error) { return nil, errors.New("read failed") }
		t.Cleanup(func() { settingsNotesRead = read })
		msgID := env.updateID()
		env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, "a", settingsActOpen, "p", settingsPageNotes))
		text, keyboard := env.lastEdit(env.admin.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_cannot_open")) || len(keyboard) != 0 {
			t.Errorf("a failing read drew %q with %d rows, want the cannot-open edit with no keyboard", text, len(keyboard))
		}
	})
}

func TestSettingsListDoorTwoStepNotes(t *testing.T) {
	env := newSettingsListEnv(t)
	f := newSettingsNotesFlow(env, env.chat)
	// The command's cached list is primed, so the typed add must drop it.
	noteKey := dbcache.CacheKey("notes_list", env.chat.Id)
	settingsListSeedNotes(t, env.chat.Id, "alpha")
	db_notes.GetNotesListContext(context.Background(), env.chat.Id, true)
	if !env.redis.Exists(noteKey) {
		t.Fatalf("the command's cache key %s was not primed", noteKey)
	}

	m := f.begin()
	prompt, keyboard := m.last()
	for _, want := range []string{
		staffMarker("settings_header") + " " + staffMarker("settings_page_notes"),
		staffMarker("settings_notes_wait_name"), staffMarker("settings_notes_wait_name_rules") + " 100",
		staffMarker("settings_entries_step") + " 1 2", staffMarker("settings_edit_wait_footer"),
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the step-1 prompt lacks %q:\n%s", want, prompt)
		}
	}
	if len(keyboard) != 1 || len(keyboard[0]) != 1 {
		t.Fatalf("the step-1 keyboard = %+v, want one row with Cancel", keyboard)
	}
	if fields := settingsButtonFields(t, keyboard[0][0]); fields["a"] != "ec" || fields["t"] != "n" || len(fields) != 3 {
		t.Errorf("Cancel carries %v, want ec, t=n and c", fields)
	}

	before := f.sent()
	nameID := f.say("Rules")
	step2 := f.reply(before, "settings_notes_name_line", "settings_notes_wait_content", "settings_entries_wait_content_limits")
	text := fmt.Sprint(step2.Params["text"])
	for _, want := range []string{staffMarker("settings_notes_name_line") + " rules", staffMarker("settings_entries_step") + " 2 2"} {
		if !strings.Contains(text, want) {
			t.Errorf("the step-2 prompt lacks %q:\n%s", want, text)
		}
	}
	if reply := fmt.Sprint(step2.Params["reply_parameters"]); !strings.Contains(reply, strconv.FormatInt(nameID, 10)) {
		t.Errorf("the step-2 prompt replies to %s, want the name message %d", reply, nameID)
	}
	step2Keyboard := staffKeyboardOf(step2.Params["reply_markup"])
	if len(step2Keyboard) != 1 || len(step2Keyboard[0]) != 1 {
		t.Fatalf("the step-2 keyboard = %+v, want one row with Cancel", step2Keyboard)
	}
	if fields := settingsButtonFields(t, step2Keyboard[0][0]); fields["a"] != "ec" || fields["t"] != "c" || len(fields) != 3 {
		t.Errorf("the step-2 Cancel carries %v, want ec, t=c and c", fields)
	}
	f.waitIs("c", "rules")
	if got := f.field("q"); got != strconv.FormatInt(nameID, 10) {
		t.Errorf("the wait's q = %q, want the name message %d", got, nameID)
	}
	if got := len(settingsNotesStored(t, env.chat.Id)); got != 1 {
		t.Fatalf("step 1 changed the list to %d notes", got)
	}

	before = f.sent()
	msg := settingsNotesContentMessage()
	sendPrivateMessage(env.settingsEnv, f.user, msg)

	want := content.ExtractTypedNoteOrFilter(&msg, "rules", false, "en")
	if want.DataType != db.TEXT || !want.AdminOnly || len(want.Buttons) != 1 || want.Buttons[0].Name != "Docs" {
		t.Fatalf("the extraction of the test message = %+v, want a text with the admin directive and the Docs button", want)
	}
	stored := settingsNotesStored(t, env.chat.Id)
	row, ok := stored["rules"]
	if !ok || len(stored) != 2 {
		t.Fatalf("stored notes = %v, want alpha and rules", stored)
	}
	if row.NoteContent != want.Text || row.MsgType != want.DataType || row.FileID != want.FileID || !row.AdminOnly || row.PrivateOnly || row.GroupOnly ||
		len(row.Buttons) != 1 || row.Buttons[0] != want.Buttons[0] {
		t.Errorf("stored row = %+v, want the fields /save stores for that content: %+v", row, want)
	}
	if env.redis.Exists(noteKey) {
		t.Errorf("the add did not drop %s", noteKey)
	}
	saved := f.reply(before, "settings_page_notes")
	savedText := fmt.Sprint(saved.Params["text"])
	if got := strings.Count(savedText, " rules"); got < 2 {
		t.Errorf("the saved notice names the note %d times, want twice:\n%s", got, savedText)
	}
	for _, wantText := range []string{
		settingsNoticeOKIcon + staffMarker("settings_notes_saved"), "2. <code>rules</code> · " + staffMarker("settings_notes_admin_tag"),
		staffMarker("settings_notes_count") + " 2",
	} {
		if !strings.Contains(savedText, wantText) {
			t.Errorf("the saved page lacks %q:\n%s", wantText, savedText)
		}
	}
	if strings.Contains(savedText, staffMarker("notes_private_conflict_warning")) {
		t.Errorf("a note with no conflicting directives got the conflict warning:\n%s", savedText)
	}
	if keys := settingsEditInputKeys(env.settingsEnv); len(keys) != 0 {
		t.Errorf("the finished flow left %v behind", keys)
	}

	// A note with both {private} and {noprivate} keeps neither flag, as /save stores it, and the notice says so.
	f.begin()
	f.say("both")
	before = f.sent()
	sendPrivateMessage(env.settingsEnv, f.user, gotgbot.Message{Text: "Where? {private} {noprivate}"})
	both := settingsNotesStored(t, env.chat.Id)["both"]
	if both.PrivateOnly || both.GroupOnly {
		t.Errorf("the conflicting directives were stored: private %v, group %v", both.PrivateOnly, both.GroupOnly)
	}
	bothText := fmt.Sprint(f.reply(before, "settings_page_notes").Params["text"])
	if !strings.Contains(bothText, staffMarker("notes_private_conflict_warning")) {
		t.Errorf("the saved notice lacks the conflict warning:\n%s", bothText)
	}

	msgID := env.updateID()
	env.pressPrivate(f.user, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
	mainText, _ := env.lastEdit(f.user.Id, msgID)
	if line, ok := settingsMainStateLine(mainText, "settings_page_notes"); !ok || !strings.HasSuffix(line, " 3") {
		t.Errorf("Main's Notes line = %q, want it to end in 3", line)
	}
}

// settingsNotesIDOf is the row ID of the chat's stored note.
func settingsNotesIDOf(t *testing.T, chatID int64, name string) int64 {
	t.Helper()
	row, ok := settingsNotesStored(t, chatID)[name]
	if !ok {
		t.Fatalf("note %q is not stored in chat %d", name, chatID)
	}
	return int64(row.ID)
}

// settingsNotesPrimeCache fills the command's notes_list cache key of the chat and returns it.
func settingsNotesPrimeCache(t *testing.T, env *settingsListEnv, chatID int64) string {
	t.Helper()
	db_notes.GetNotesListContext(context.Background(), chatID, true)
	key := dbcache.CacheKey("notes_list", chatID)
	if !env.redis.Exists(key) {
		t.Fatalf("the cache key %s was not primed", key)
	}
	return key
}

func TestSettingsNotesDelete(t *testing.T) {
	t.Run("the ask draws a card naming the note, Yes deletes that row and drops the cache, No returns to the page", func(t *testing.T) {
		env := newSettingsListEnv(t)
		owner := settingsFiltersOwner(env, env.chat)
		settingsListSeedNotes(t, env.chat.Id, "alpha", "beta", "gamma", "delta")
		key := settingsNotesPrimeCache(t, env, env.chat.Id)
		m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageNotes)
		deltaID := settingsNotesIDOf(t, env.chat.Id, "delta")

		m.tap(m.button("🗑 3"))
		text, keyboard := m.last()
		if !strings.Contains(text, staffMarker("settings_notes_delete_ask")+" delta") {
			t.Fatalf("the card does not name the note:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{2, 2}) {
			t.Errorf("card rows = %v, want {2,2}", got)
		}
		yes, _ := m.markedButton("settings_btn_yes")
		yesFields := settingsButtonFields(t, yes)
		if yesFields["a"] != "ey" || yesFields["r"] != strconv.FormatInt(deltaID, 10) || yesFields["e"] == "" || len(yesFields) != 4 {
			t.Errorf("Yes carries %v, want a=ey, the row ID, an expiry and c", yesFields)
		}
		no, _ := m.markedButton("settings_btn_no")
		if f := settingsButtonFields(t, no); f["a"] != "eg" || f["v"] != "1" || len(f) != 3 {
			t.Errorf("No carries %v, want a=eg, v=1 and c", f)
		}
		if got := settingsNotesStored(t, env.chat.Id); len(got) != 4 {
			t.Errorf("the ask changed the list: %v", got)
		}

		answers := env.answerCount()
		m.tap(yes)
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the Yes was answered %d times, want once", got)
		}
		text, _ = m.last()
		if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_notes_removed")+" delta") {
			t.Errorf("the page lacks the removed notice naming the note:\n%s", text)
		}
		for _, line := range []string{"1. <code>alpha</code>", "2. <code>beta</code>", "3. <code>gamma</code>"} {
			if !strings.Contains(text, line) {
				t.Errorf("the redrawn page lacks %q:\n%s", line, text)
			}
		}
		if _, still := settingsNotesStored(t, env.chat.Id)["delta"]; still {
			t.Errorf("the note is still stored")
		}
		if env.redis.Exists(key) {
			t.Errorf("the delete did not drop %s", key)
		}

		// No returns to the page the ask came from, with no notice.
		m.tap(m.button("🗑 1"))
		no, _ = m.markedButton("settings_btn_no")
		m.tap(no)
		text, _ = m.last()
		if !strings.Contains(text, "1. <code>alpha</code>") || strings.Contains(text, staffMarker("settings_notes_delete_ask")) {
			t.Errorf("No did not return to the page:\n%s", text)
		}
	})

	t.Run("the bot not an administrator by the cached list answers the notice and draws no card", func(t *testing.T) {
		env := newSettingsListEnv(t)
		chat := env.addChat()
		env.fake.setBotRole(chat.Id, staffRoleMember)
		settingsListCleanup(t, chat.Id)
		settingsListSeedNotes(t, chat.Id, "alpha")
		owner := settingsFiltersOwner(env, chat)
		m := settingsListOpenMenuPage(env, chat, owner, settingsPageNotes)
		m.tap(m.button("🗑 1"))
		if text, alert := env.lastAnswer(); text != "" || alert {
			t.Errorf("a notice refusal answered %q (alert %v), want an empty answer", text, alert)
		}
		text, _ := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("connections_is_user_connected_bot_not_admin")) || strings.Contains(text, staffMarker("settings_notes_delete_ask")) {
			t.Errorf("the redraw lacks the bot notice or drew a card:\n%s", text)
		}
	})

	t.Run("an administrator without can_change_info is refused at the ask and sees no card", func(t *testing.T) {
		env := newSettingsListEnv(t)
		settingsListSeedNotes(t, env.chat.Id, "alpha")
		m := settingsListOpenMenuPage(env, env.chat, env.admin, settingsPageNotes)
		edits, answers := len(env.edits(env.admin.Id, m.msgID)), env.answerCount()
		m.tap(m.button("🗑 1"))
		if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker("settings_no_change_info")) || !alert {
			t.Errorf("the ask answered %q (alert %v), want the settings_no_change_info alert", text, alert)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the ask was answered %d times, want once", got)
		}
		if got := len(env.edits(env.admin.Id, m.msgID)); got != edits {
			t.Errorf("the refused ask edited the menu")
		}
	})

	t.Run("another chat's row ID answers already removed and deletes nothing", func(t *testing.T) {
		env := newSettingsListEnv(t)
		owner := settingsFiltersOwner(env, env.chat)
		other := env.addChat()
		settingsListCleanup(t, other.Id)
		settingsListSeedNotes(t, env.chat.Id, "alpha")
		settingsListSeedNotes(t, other.Id, "foreign")
		foreignID := settingsNotesIDOf(t, other.Id, "foreign")
		m := settingsListOpenMenuPage(env, env.chat, owner, settingsPageNotes)

		m.press("a", "ex", "r", strconv.FormatInt(foreignID, 10), "v", "1")
		text, _ := m.last()
		if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("settings_entries_already_removed")) || strings.Contains(text, staffMarker("settings_notes_delete_ask")) {
			t.Errorf("a foreign ID's ask drew:\n%s", text)
		}
		m.press("a", "ey", "r", strconv.FormatInt(foreignID, 10), "e", settingsCardExpiry(time.Now()))
		text, _ = m.last()
		if !strings.Contains(text, staffMarker("settings_entries_already_removed")) {
			t.Errorf("a foreign ID's Yes drew:\n%s", text)
		}
		if got := settingsNotesStored(t, other.Id); len(got) != 1 {
			t.Errorf("a forged ID deleted another chat's note: %v", got)
		}
		if got := settingsNotesStored(t, env.chat.Id); len(got) != 1 {
			t.Errorf("a forged ID changed this chat's notes: %v", got)
		}
	})
}

// settingsNotesForgedPresses are the Notes presses that step 1 must refuse before any I/O (the cases the Blacklist's and
// the Filters' forged tables hold for their own codes).
func settingsNotesForgedPresses(valid string) map[string][]string {
	return map[string][]string{
		"ex with an 18-digit r":    {"a", "ex", "r", "123456789012345678", "v", "1"},
		"ex with a leading zero":   {"a", "ex", "r", "012", "v", "1"},
		"ex with a 5-digit v":      {"a", "ex", "r", "5", "v", "12345"},
		"ex without v":             {"a", "ex", "r", "5"},
		"ey with a non-numeric e":  {"a", "ey", "r", "5", "e", "abc"},
		"ey without e":             {"a", "ey", "r", "5"},
		"ey with a v":              {"a", "ey", "r", "5", "e", valid, "v", "1"},
		"ea with an extra key":     {"a", "ea", "x", "1"},
		"ec with another page's k": {"a", "ec", "t", "k"},
		"ec with another page's w": {"a", "ec", "t", "w"},
		"ec without t":             {"a", "ec"},
		"eg with a letter v":       {"a", "eg", "v", "x"},
		"eg with a 5-digit v":      {"a", "eg", "v", "12345"},
		"st naming page e":         {"a", "st", "p", "e", "t", "x", "v", "1"},
		"tg naming page e":         {"a", "tg", "p", "e", "t", "x", "v", "1"},
	}
}

func TestSettingsListDoorNotesRefusals(t *testing.T) {
	env := newSettingsListEnv(t)
	f := newSettingsNotesFlow(env, env.chat)
	settingsListSeedNotes(t, env.chat.Id, "alpha")

	t.Run("step 1 refusals keep the wait at step 1 and the next text is read as a name", func(t *testing.T) {
		f.begin()
		nonce := f.field("n")
		for _, c := range []struct {
			name, key, words string
			send             func()
		}{
			{"a photo", "settings_edit_refuse_need_text", "", func() {
				sendPrivateMessage(env.settingsEnv, f.user, settingsMediaItemID("photo", "name-photo"))
			}},
			{"two words", "notes_name_invalid", "", func() { f.say("two words") }},
			{"a leading #", "notes_name_invalid", "", func() { f.say("#rules") }},
			{"a name of 101 code points", "notes_name_invalid", "", func() { f.say(strings.Repeat("n", 101)) }},
			{"the ideographic space alone", "settings_entries_refuse_blank", "", func() { f.say("\u3000") }},
			{"an existing name", "settings_notes_refuse_exists", "alpha", func() { f.say("Alpha") }},
		} {
			before := f.sent()
			c.send()
			reply := f.reply(before, c.key, "settings_edit_still_waiting")
			if text := fmt.Sprint(reply.Params["text"]); !strings.Contains(text, c.words) {
				t.Errorf("%s: the reply lacks %q: %s", c.name, c.words, text)
			}
			if fields := settingsButtonFields(t, staffKeyboardOf(reply.Params["reply_markup"])[0][0]); fields["a"] != "ec" || fields["t"] != "n" || len(fields) != 3 {
				t.Errorf("%s: Cancel carries %v, want ec, t=n and c", c.name, fields)
			}
			f.waitIs("n", "")
			if f.field("n") != nonce || f.field("q") != "" {
				t.Errorf("%s: the refusal moved the wait", c.name)
			}
		}
		before := f.sent()
		f.say("Fresh")
		f.reply(before, "settings_entries_wait_content_limits")
		f.waitIs("c", "fresh")
	})

	t.Run("step 2 refusals keep the wait at step 2 with the name", func(t *testing.T) {
		f.endWait()
		f.begin()
		f.say("price")
		before := f.sent()
		sendPrivateMessage(env.settingsEnv, f.user, settingsMediaItemID("poll", ""))
		reply := f.reply(before, "content_need_note_content", "settings_edit_still_waiting")
		if fields := settingsButtonFields(t, staffKeyboardOf(reply.Params["reply_markup"])[0][0]); fields["a"] != "ec" || fields["t"] != "c" {
			t.Errorf("Cancel carries %v, want ec and t=c", fields)
		}
		f.waitIs("c", "price")
		if got := settingsNotesStored(t, env.chat.Id); len(got) != 1 {
			t.Errorf("a refused content stored %v", got)
		}
	})

	t.Run("a name another admin saved between the steps ends the wait and is never overwritten", func(t *testing.T) {
		f.endWait()
		f.begin()
		f.say("taken")
		// /save in the group stores the name between the two steps.
		settingsListSeedNotes(t, env.chat.Id, "taken")
		before := f.sent()
		sendPrivateMessage(env.settingsEnv, f.user, gotgbot.Message{Text: "my own content"})
		f.reply(before, "settings_notes_refuse_exists", "settings_edit_wait_stopped")
		settingsEditWaitGone(t, env.settingsEnv, f.user)
		if row := settingsNotesStored(t, env.chat.Id)["taken"]; row.NoteContent != "content taken" {
			t.Errorf("the page overwrote the note: %+v", row)
		}
	})

	t.Run("a reply after the ten minutes gets one wait-ended note naming the note content", func(t *testing.T) {
		f.endWait()
		f.begin()
		f.say("late")
		env.redis.FastForward(settingsInputTTL + time.Second)
		before := f.sent()
		f.say("too late")
		f.reply(before, "settings_entries_wait_ended_note", "settings_entries_part_content")
		if _, ok := settingsNotesStored(t, env.chat.Id)["late"]; ok {
			t.Errorf("a late reply stored a note")
		}
	})

	for _, c := range []struct {
		name, want string
		kept       bool
		change     func(chatID int64)
	}{
		{"the bot stopped being an administrator, cached and live", "connections_is_user_connected_bot_not_admin", false, func(chatID int64) {
			env.fake.setBotRole(chatID, staffRoleMember)
			settingsListDoorClearAdminCache(chatID)
		}},
		{"the bot's live check answers 429", "settings_check_failed", true, func(chatID int64) {
			env.fake.setBotRole(chatID, staffRoleMember)
			settingsListDoorClearAdminCache(chatID)
			env.fake.script("getChatMember", chatID, nil, staffFake429(1), staffFake429(1), staffFake429(1))
		}},
	} {
		t.Run("a typed step after "+c.name, func(t *testing.T) {
			chat := env.addChat()
			env.fake.setBotRole(chat.Id, staffRoleAdmin)
			settingsListCleanup(t, chat.Id)
			g := newSettingsNotesFlow(env, chat)
			g.begin()
			c.change(chat.Id)
			before := g.sent()
			g.say("anyname")
			reply := g.reply(before, c.want)
			text := fmt.Sprint(reply.Params["text"])
			if stopped := staffMarker("settings_edit_wait_stopped"); strings.Contains(text, stopped) == c.kept {
				t.Errorf("the reply %q has the wait-stopped line = %v, want %v", text, !c.kept, !c.kept)
			}
			if c.kept {
				settingsEditWaitKept(t, env.settingsEnv, g.user)
			} else {
				settingsEditWaitGone(t, env.settingsEnv, g.user)
			}
			if got := settingsNotesStored(t, chat.Id); len(got) != 0 {
				t.Errorf("a refused typed step wrote %v", got)
			}
		})
	}
}

// settingsNotesCommandTwin makes the command twin of the flow's chat: a second chat of the env whose creator is the
// same owner and whose bot is an administrator, with the note commands loaded on the env's dispatcher.
func settingsNotesCommandTwin(f *settingsNotesFlow) gotgbot.Chat {
	f.env.t.Helper()
	twin := settingsFiltersCommandTwin(f.settingsFiltersFlow)
	f.env.fake.setBotRole(twin.Id, staffRoleAdmin)
	return twin
}

// settingsNotesRowShape is the stored row as one comparable value: everything but its ID, chat and times.
func settingsNotesRowShape(row models.Notes) string {
	return fmt.Sprintf("%q|%q|%d|%q|%+v|admin %v|private %v|group %v|preview %v|protected %v|nonotif %v", row.NoteName, row.NoteContent, row.MsgType, row.FileID,
		[]db.Button(row.Buttons), row.AdminOnly, row.PrivateOnly, row.GroupOnly, row.WebPreview, row.IsProtected, row.NoNotif)
}

func TestSettingsParityNoteAdd(t *testing.T) {
	env := newSettingsListEnv(t)
	LoadNotes(env.dispatcher)
	f := newSettingsNotesFlow(env, env.chat)
	pageChat := env.chat
	commandChat := settingsNotesCommandTwin(f)

	photo := settingsMediaItemID("photo", "parity-photo")
	photo.Caption = "Staff only {admin}"
	photo.CaptionEntities = []gotgbot.MessageEntity{{Type: "bold", Offset: 0, Length: 5}}
	video := settingsMediaItemID("video", "parity-video")
	cases := []struct {
		name    string
		message gotgbot.Message
	}{
		{"a text with entities and newlines", gotgbot.Message{
			Text: "Hello\nbold world", Entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 6, Length: 4}},
		}},
		{"a photo with a caption holding {admin}", photo},
		{"a video", video},
		{"a text with markdown buttons", gotgbot.Message{Text: "Open [Docs](buttonurl://https://example.com)"}},
		{"a text with both {private} and {noprivate}", gotgbot.Message{Text: "Where? {private} {noprivate}"}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := fmt.Sprintf("parity%d", i)
			pageKey := settingsNotesPrimeCache(t, env, pageChat.Id)
			commandKey := settingsNotesPrimeCache(t, env, commandChat.Id)

			f.begin()
			f.say(name)
			sendPrivateMessage(env.settingsEnv, f.user, tc.message)
			settingsFiltersCommand(env, commandChat, f.user, "/save "+name, &tc.message)

			pageRow, ok := settingsNotesStored(t, pageChat.Id)[name]
			if !ok {
				t.Fatalf("the page stored no note %q: %v", name, settingsNotesStored(t, pageChat.Id))
			}
			commandRow, ok := settingsNotesStored(t, commandChat.Id)[name]
			if !ok {
				t.Fatalf("/save stored no note %q: %v", name, settingsNotesStored(t, commandChat.Id))
			}
			if got, want := settingsNotesRowShape(pageRow), settingsNotesRowShape(commandRow); got != want {
				t.Errorf("the page stored %s, /save stored %s", got, want)
			}
			for _, key := range []string{pageKey, commandKey} {
				if env.redis.Exists(key) {
					t.Errorf("the cache key %s survived the add", key)
				}
			}
		})
	}
}

func TestSettingsParityNoteRemove(t *testing.T) {
	env := newSettingsListEnv(t)
	LoadNotes(env.dispatcher)
	f := newSettingsNotesFlow(env, env.chat)
	pageChat := env.chat
	commandChat := settingsNotesCommandTwin(f)
	for _, chat := range []gotgbot.Chat{pageChat, commandChat} {
		settingsListSeedNotes(t, chat.Id, "alpha", "beta", "gamma")
	}
	pageKey := settingsNotesPrimeCache(t, env, pageChat.Id)
	commandKey := settingsNotesPrimeCache(t, env, commandChat.Id)

	m := settingsListOpenMenuPage(env, pageChat, f.user, settingsPageNotes)
	m.tap(m.button("🗑 2"))
	yes, _ := m.markedButton("settings_btn_yes")
	m.tap(yes)
	settingsFiltersCommand(env, commandChat, f.user, "/clear beta", nil)

	shapes := func(chatID int64) []string {
		var out []string
		for _, row := range settingsNotesStored(t, chatID) {
			out = append(out, settingsNotesRowShape(row))
		}
		slices.Sort(out)
		return out
	}
	if page, command := shapes(pageChat.Id), shapes(commandChat.Id); !reflect.DeepEqual(page, command) || len(page) != 2 {
		t.Errorf("the page left %v and /clear left %v, want the same two rows", page, command)
	}
	for _, key := range []string{pageKey, commandKey} {
		if env.redis.Exists(key) {
			t.Errorf("the cache key %s survived the delete", key)
		}
	}
}

func TestSettingsDriftNotes(t *testing.T) {
	env := newSettingsListEnv(t)
	LoadNotes(env.dispatcher)
	f := newSettingsNotesFlow(env, env.chat)
	m := settingsListOpenMenuPage(env, env.chat, f.user, settingsPageNotes)
	draw := func() string {
		t.Helper()
		m.press("a", settingsActOpen, "p", settingsPageNotes)
		text, _ := m.last()
		return text
	}
	mainLine := func() string {
		t.Helper()
		msgID := env.updateID()
		env.pressPrivate(f.user, msgID, env.privateFields(env.chat.Id, "a", settingsActBack))
		text, _ := env.lastEdit(f.user.Id, msgID)
		line, _ := settingsMainStateLine(text, "settings_page_notes")
		return line
	}

	settingsFiltersCommand(env, env.chat, f.user, "/save spam", &gotgbot.Message{Text: "the content"})
	settingsFiltersCommand(env, env.chat, f.user, "/save secret", &gotgbot.Message{Text: "{admin} hidden"})
	out := draw()
	for _, want := range []string{"1. <code>secret</code> · " + staffMarker("settings_notes_admin_tag"), "2. <code>spam</code>", staffMarker("settings_notes_count") + " 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("a note added by /save is not drawn (%q):\n%s", want, out)
		}
	}
	if strings.Contains(out, "<code>spam</code> · ") {
		t.Errorf("a note anyone can fetch is tagged admin-only:\n%s", out)
	}
	if line := mainLine(); !strings.HasSuffix(line, " 2") {
		t.Errorf("Main's Notes line = %q, want it to end in 2", line)
	}
	settingsFiltersCommand(env, env.chat, f.user, "/clear spam", nil)
	if out = draw(); strings.Contains(out, "<code>spam</code>") || !strings.Contains(out, staffMarker("settings_notes_count")+" 1") {
		t.Errorf("a note removed by /clear is still drawn:\n%s", out)
	}
	if line := mainLine(); !strings.HasSuffix(line, " 1") {
		t.Errorf("Main's Notes line = %q after /clear, want it to end in 1", line)
	}
}

func TestSettingsAuthorityParityNotes(t *testing.T) {
	type scenario struct {
		name string
		// botRole is the bot's role in the chat; setup returns the presser.
		botRole string
		setup   func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User
		// wantKey is the refusal the menu answers; wantAlert says it is an alert (else a notice over a redraw). Empty
		// wantKey means the menu writes.
		wantKey   string
		wantAlert bool
		// typedKey is what the door answers a typed step with; empty means it writes.
		typedKey string
		// commandWrites is whether /save writes for the same records.
		commandWrites bool
	}
	scenarios := []scenario{
		{name: "the bot is a member", botRole: staffRoleMember, wantKey: "connections_is_user_connected_bot_not_admin",
			typedKey: "connections_is_user_connected_bot_not_admin",
			setup:    func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User { return settingsFiltersOwner(env, chat) }},
		{name: "an administrator without can_change_info", botRole: staffRoleAdmin, wantKey: "settings_no_change_info", wantAlert: true, typedKey: "settings_no_change_info",
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
				listEditAdmin(env.settingsEnv, chat, env.admin, false)
				cache.InvalidateAdminCache(chat.Id)
				return env.admin
			}},
		{name: "a plain member", botRole: staffRoleAdmin, wantKey: "settings_not_admin", wantAlert: true, typedKey: "settings_edit_refuse_access_lost",
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
				user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Presser"}
				env.fake.setMember(chat.Id, user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
				return user
			}},
		{name: "the owner gate refuses a non-owner administrator who holds can_change_info", botRole: staffRoleAdmin,
			wantKey: "settings_gate_owner_only", wantAlert: true, typedKey: "settings_gate_owner_only", commandWrites: true,
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User {
				user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Presser"}
				listEditAdmin(env.settingsEnv, chat, user, true)
				env.newOwner(chat)
				env.setAdminsAllowed(chat.Id, false)
				return user
			}},
		{name: "every check held", botRole: staffRoleAdmin, commandWrites: true,
			setup: func(env *settingsListEnv, chat gotgbot.Chat) gotgbot.User { return settingsFiltersOwner(env, chat) }},
	}

	env := newSettingsListEnv(t)
	LoadNotes(env.dispatcher)
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			chat := env.addChat()
			env.fake.setBotRole(chat.Id, sc.botRole)
			settingsListCleanup(t, chat.Id)
			settingsListSeedNotes(t, chat.Id, "alpha", "beta")
			user := sc.setup(env, chat)
			t.Cleanup(func() {
				db.DB.Where("user_id = ?", user.Id).Delete(&models.User{})
				dbcache.DeleteCache(dbcache.CacheKey("user", user.Id))
				dbcache.ResetLocalForTest()
			})
			alphaID := settingsNotesIDOf(t, chat.Id, "alpha")

			// The command first: it refuses for the same member and bot records, or writes (the owner gate is the documented
			// carve-out, which gates the menu and not the command).
			settingsFiltersCommand(env, chat, user, "/save viacommand", &gotgbot.Message{Text: "the content"})
			if _, wrote := settingsNotesStored(t, chat.Id)["viacommand"]; wrote != sc.commandWrites {
				t.Errorf("/save wrote = %v, want %v (stored %v)", wrote, sc.commandWrites, settingsNotesStored(t, chat.Id))
			}

			// A typed step is judged like a press. A wait is written directly (the Add press itself is refused when the
			// records refuse); the door then reads the same records live.
			for _, step := range []struct{ name, part, text, key string }{
				{"the name step", "n", "typedname", "typedname"},
				{"the content step", "c", "the content", "typedstep"},
			} {
				wait := settingsInputWait{GroupID: chat.Id, Page: settingsPageNotes, Part: "n", PromptMsgID: 1, Nonce: "0123456789abcdef"}
				if _, err := settingsInputSet(context.Background(), user.Id, wait); err != nil {
					t.Fatalf("%s: write the wait: %v", step.name, err)
				}
				if step.part == "c" {
					wait.Part, wait.Nonce, wait.Pending, wait.StepMsgID = "c", "fedcba9876543210", "typedstep", 1
					wait.ExpiresAt = time.Now().Add(settingsInputTTL)
					if advanced, err := settingsInputAdvance(context.Background(), user.Id, "0123456789abcdef", wait); err != nil || !advanced {
						t.Fatalf("%s: move the wait to step 2: %v, %v", step.name, advanced, err)
					}
				}
				before := len(env.fake.sentTo(user.Id))
				if step.part == "n" {
					settingsListDoorSay(env, user, step.text)
				} else {
					sendPrivateMessage(env.settingsEnv, user, gotgbot.Message{Text: step.text})
				}
				_, stored := settingsNotesStored(t, chat.Id)[step.key]
				if sc.typedKey != "" {
					got := fmt.Sprint(env.fake.sentTo(user.Id)[before].Params["text"])
					if !strings.Contains(got, staffMarker(sc.typedKey)) {
						t.Errorf("%s: answered %q, want %s", step.name, got, sc.typedKey)
					}
					if stored || env.redis.Exists(settingsInputKey(user.Id)) {
						t.Errorf("%s: a refused typed step wrote or left the wait (stored %v)", step.name, stored)
					}
					if sc.typedKey != "settings_edit_refuse_access_lost" && !strings.Contains(got, staffMarker("settings_edit_wait_stopped")) {
						t.Errorf("%s: the refusal does not say the wait stopped", step.name)
					}
				} else if step.part == "c" && !stored {
					t.Errorf("%s: the typed content was not stored where the commands write", step.name)
				}
				if sc.typedKey == "" && step.part == "n" {
					if got := env.redis.HGet(settingsInputKey(user.Id), "t"); got != "c" {
						t.Errorf("%s: the wait is at %q after an accepted name, want c", step.name, got)
					}
					continue
				}
				settingsEditWaitGone(t, env.settingsEnv, user)
			}
			if sc.typedKey != "" && sc.typedKey != "settings_gate_owner_only" && sc.commandWrites {
				t.Errorf("a typed step was refused where the commands write (%s), and that is not the owner-gate carve-out", sc.typedKey)
			}
			before := settingsNotesStored(t, chat.Id)

			m := settingsListOpenMenuPage(env, chat, user, settingsPageNotes)
			presses := map[string][]string{
				"the delete ask": {"a", "ex", "r", strconv.FormatInt(alphaID, 10), "v", "1"},
				"the delete Yes": {"a", "ey", "r", strconv.FormatInt(alphaID, 10), "e", settingsCardExpiry(time.Now())},
				"Add":            {"a", "ea"},
			}
			for _, name := range []string{"the delete ask", "the delete Yes", "Add"} {
				answers, edits := env.answerCount(), len(env.edits(user.Id, m.msgID))
				m.press(presses[name]...)
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("%s: answered %d times, want once", name, got)
				}
				switch {
				case sc.wantKey == "":
				case sc.wantAlert:
					if text, alert := env.lastAnswer(); !strings.Contains(text, staffMarker(sc.wantKey)) || !alert {
						t.Errorf("%s: answered %q (alert %v), want the alert %s", name, text, alert, sc.wantKey)
					}
					if got := len(env.edits(user.Id, m.msgID)); got != edits {
						t.Errorf("%s: an alert refusal edited the menu", name)
					}
				default:
					if text, alert := env.lastAnswer(); text != "" || alert {
						t.Errorf("%s: a notice refusal answered %q (alert %v), want an empty answer", name, text, alert)
					}
					if text, _ := m.last(); !strings.Contains(text, settingsNoticeWarnIcon+staffMarker(sc.wantKey)) {
						t.Errorf("%s: the redraw lacks the notice %s:\n%s", name, sc.wantKey, text)
					}
				}
			}
			after := settingsNotesStored(t, chat.Id)
			if sc.wantKey != "" {
				if !reflect.DeepEqual(before, after) {
					t.Errorf("a refused press wrote: %v -> %v", before, after)
				}
				if env.redis.Exists(settingsInputKey(user.Id)) {
					t.Errorf("a refused Add created a wait")
				}
				return
			}
			if _, still := after["alpha"]; still {
				t.Errorf("the permitted Yes did not delete alpha: %v", after)
			}
			if !env.redis.Exists(settingsInputKey(user.Id)) {
				t.Errorf("the permitted Add created no wait")
			}
		})
	}
}

// settingsNotesWorstEntries makes n entries with 17-digit row IDs, the worst case the delete buttons carry.
func settingsNotesWorstEntries(n int) []settingsListEntry {
	entries := make([]settingsListEntry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, settingsListEntry{ID: 12345678901234500 + int64(i), Text: fmt.Sprintf("n%06d", i), AdminOnly: true})
	}
	return entries
}

func TestSettingsListNotesCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	views := settingsBlacklistWorstViews(tr)
	private := views["private"]
	def := settingsNotesDef()
	// 79,992 entries are 9,999 pages: the pager cursor v reaches its four digits.
	worst := settingsNotesWorstEntries(settingsListPageSize * 9999)

	sizes := make(map[string]int)
	measure := func(name string, screen settingsScreen) {
		t.Helper()
		if screen.Omitted != 0 {
			t.Errorf("%s: %d buttons were omitted for their size, want none", name, screen.Omitted)
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if button.CallbackData == "" {
				continue
			}
			if size := len(button.CallbackData); size > 64 {
				t.Errorf("%s: button %q carries %d bytes, over the 64-byte cap", name, button.Text, size)
			}
			code := settingsButtonFields(t, button)["a"]
			sizes[code] = max(sizes[code], len(button.CallbackData))
		}
	}
	for name, page := range map[string]int{"first": 1, "middle": 5000, "page 9998": 9998, "last": 9999} {
		measure("list "+name, renderSettingsList(private, def, worst, page, ""))
	}
	measure("a one-entry list", renderSettingsList(private, def, worst[:1], 1, ""))
	measure("an empty list", renderSettingsList(private, def, nil, 1, ""))
	measure("the card", renderSettingsListCard(private, def, worst[0], 9999))
	until := time.Now()
	measure("the name prompt", renderSettingsListPrompt(private, def, "n", until, true, ""))
	measure("the content prompt", renderSettingsListPrompt(private, def, "c", until, true, "name"))
	measure("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"))

	for code, want := range map[string]int{"eg": 40, "ex": 60, "ey": 64, "ea": 33, "ec": 37} {
		if got := sizes[code]; got != want {
			t.Errorf("the largest %s button is %d bytes, the UI-SPEC table says %d", code, got, want)
		}
	}
	cell := func(v settingsView) int {
		screen := renderSettingsMain(v, []settingsMainEntry{{Key: settingsPageNotes, Icon: def.Icon, Title: "Notes", State: "1", Known: true}}, "")
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if f := settingsButtonFields(t, button); f["a"] == settingsActOpen {
				return len(button.CallbackData)
			}
		}
		return 0
	}
	if got := cell(private); got != 37 {
		t.Errorf("the Main cell is %d bytes in a private menu, want 37", got)
	}
	if got := cell(views["group"]); got != 33 {
		t.Errorf("the Main cell is %d bytes in a group menu, want 33", got)
	}

	t.Run("a row whose ID has 18 digits has no delete button and is counted", func(t *testing.T) {
		entries := settingsNotesWorstEntries(3)
		entries[1].ID = 123456789012345678
		screen := renderSettingsList(private, def, entries, 1, "")
		if screen.Omitted != 1 {
			t.Errorf("Omitted = %d, want 1", screen.Omitted)
		}
		if got := len(screen.Keyboard.InlineKeyboard[0]); got != 2 {
			t.Errorf("the delete row has %d buttons, want 2", got)
		}
	})
}

func TestSettingsListNotesPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	title := strings.Repeat("Ж", 64)
	ampersands := strings.Repeat("&", 100)
	def := settingsNotesDef()
	// The entries a middle page of the worst entries shows: 32 ampersands each, all admin-only, so each line carries the tag.
	entries := make([]settingsListEntry, 0, 24)
	for i := 0; i < 24; i++ {
		entries = append(entries, settingsListEntry{ID: int64(10 + i), Text: strings.Repeat("&", 32) + string(rune('a'+i)), AdminOnly: true})
	}
	widths := func(screen settingsScreen) string {
		return fmt.Sprint(settingsListRowWidths(screen.Keyboard.InlineKeyboard))
	}

	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			views := settingsBlacklistWorstViews(tr)
			for name, v := range views {
				v.GroupTitle = title
				views[name] = v
			}
			private := views["private"]
			check := func(name string, screen settingsScreen, wantWidths string) {
				t.Helper()
				if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
					t.Errorf("%s is %d UTF-16 units, want 1..3800", name, units)
				}
				if screen.Omitted != 0 {
					t.Errorf("%s omitted %d buttons", name, screen.Omitted)
				}
				if wantWidths != "" {
					if got := widths(screen); got != wantWidths {
						t.Errorf("%s has row widths %s, want %s", name, got, wantWidths)
					}
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if label := []rune(button.Text); len(label) > 20 && !strings.HasPrefix(button.Text, "🗑") && button.Url == "" {
						t.Errorf("%s: button %q has %d code points, want at most 20", name, button.Text, len(label))
					}
				}
			}

			// A notice only adds its own length, so the two longest of the notices a page of this table can show hold the
			// worst case of every combination: the removed notice, the expiry, the save failure and the bot notice.
			notices := settingsBlacklistLongest([]string{
				settingsNoticeOK(settingsListEntryText(tr, "settings_notes_removed", "name", ampersands)),
				settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
				settingsNoticeWarn(settingsText(tr, "connections_is_user_connected_bot_not_admin")),
			}, 2)
			for i, notice := range notices {
				check(fmt.Sprintf("the list page with notice %d", i), renderSettingsList(private, def, entries, 2, notice), "[4 4 2 1 2]")
			}
			check("the empty list", renderSettingsList(private, def, nil, 1, ""), "[1 2]")
			card := renderSettingsListCard(private, def, settingsListEntry{ID: 12345678901234567, Text: ampersands}, 9999)
			check("the card", card, "[2 2]")
			if !strings.Contains(card.Text, strings.Repeat("&amp;", 100)) {
				t.Errorf("the card does not name the 100-code-point name")
			}
			until := time.Now()
			check("the name prompt with the replaced note", renderSettingsListPrompt(private, def, "n", until, true, ""), "[1]")
			step2 := renderSettingsListPrompt(private, def, "c", until, true, ampersands)
			check("the content prompt with a 100-code-point name and the replaced note", step2, "[1]")
			if !strings.Contains(step2.Text, strings.Repeat("&amp;", 100)) {
				t.Errorf("the content prompt does not name the 100-code-point name")
			}

			// Every refusal reply, each with the Cancel button and the still-waiting line, and the replies that end a wait.
			refusals := []string{
				settingsText(tr, "settings_entries_refuse_blank"),
				settingsText(tr, "notes_name_invalid"),
				settingsNotesExistsText(tr, ampersands),
				settingsText(tr, "settings_edit_refuse_need_text"),
				settingsText(tr, "content_need_note_content"),
				content.TextLengthRefusal(strings.Repeat("x", 5000), db.TEXT, code),
				content.TextLengthRefusal(strings.Repeat("x", 2000), db.PHOTO, code),
				settingsText(tr, "common_settings_save_failed"),
				settingsText(tr, "connections_is_user_connected_bot_not_admin"),
				settingsText(tr, "settings_no_change_info"),
			}
			for i, reason := range refusals {
				check(fmt.Sprintf("refusal %d", i), renderSettingsInputRefusalCode(private, def.CancelCode, "c", reason), "[1]")
				if ended := reason + "\n\n" + settingsText(tr, "settings_edit_wait_stopped"); panelUTF16Len(ended) > 3800 {
					t.Errorf("the wait-ended reply %d is %d UTF-16 units", i, panelUTF16Len(ended))
				}
			}
			warning := strings.TrimLeft(settingsText(tr, "notes_private_conflict_warning"), "\n")
			saved := settingsNoticeOK(settingsListEntryText(tr, def.TwoStep.SavedKey, def.EntryParam, ampersands)) + "\n" + warning + "\n" + settingsText(tr, "settings_edit_album_cut")
			check("the saved page with the conflict warning", renderSettingsList(private, def, entries, 2, saved), "[4 4 2 1 2]")
			check("the group note", renderSettingsPrivateOnlyNote(views["group"], def.TitleKey, "FukuTestBot"), "[1 2]")
		})
	}
}

func TestSettingsNotesReadsCreateNothing(t *testing.T) {
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	chat := env.addChat()
	env.fake.setBotRole(chat.Id, staffRoleAdmin)
	settingsListCleanup(t, chat.Id)
	count := func() (notes, settings, chats int64) {
		db.DB.Model(&models.Notes{}).Where("chat_id = ?", chat.Id).Count(&notes)
		db.DB.Model(&models.NotesSettings{}).Where("chat_id = ?", chat.Id).Count(&settings)
		db.DB.Model(&models.Chat{}).Where("chat_id = ?", chat.Id).Count(&chats)
		return notes, settings, chats
	}
	key := dbcache.CacheKey("notes_list", chat.Id)

	// A private menu: Main, the page and the card of a forged ask on a chat with no notes.
	notes, settings, chats := count()
	if notes != 0 || settings != 0 {
		t.Fatalf("the fresh chat already has %d notes and %d notes_settings rows", notes, settings)
	}
	owner := settingsFiltersOwner(env, chat)
	m := settingsListOpenMenuPage(env, chat, owner, settingsPageNotes)
	m.press("a", settingsActBack)
	m.press("a", settingsActOpen, "p", settingsPageNotes)
	m.press("a", "ex", "r", "42", "v", "1")
	if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_entries_already_removed")) {
		t.Fatalf("the screens did not draw as expected:\n%s", text)
	}
	if gotNotes, gotSettings, gotChats := count(); gotNotes != 0 || gotSettings != 0 || gotChats != chats {
		t.Errorf("a private menu left %d notes and %d notes_settings rows and changed the chat rows from %d to %d", gotNotes, gotSettings, chats, gotChats)
	}

	// A group menu: Main and the page's note read nothing and create nothing.
	groupMsg := env.openMenu(env.admin)
	env.press(env.admin, groupMsg, env.groupFields(env.admin, "a", settingsActOpen, "p", settingsPageNotes))
	if gotNotes, gotSettings, _ := count(); gotNotes != 0 || gotSettings != 0 {
		t.Errorf("a group menu left %d notes and %d notes_settings rows", gotNotes, gotSettings)
	}
	if env.redis.Exists(key) {
		t.Errorf("opening filled the command's cache key %s", key)
	}
}
