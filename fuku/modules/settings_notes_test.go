//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
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
