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
