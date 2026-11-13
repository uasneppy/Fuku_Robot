//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// noteApplyChat returns a fresh chat ID whose note rows, settings row and cache entries are deleted
// before the test and when it ends.
func noteApplyChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Notes{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.NotesSettings{})
		dbcache.DeleteCache(dbcache.CacheKey("notes_list", chatID))
		dbcache.DeleteCache(dbcache.CacheKey("notes_settings", chatID))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	return chatID
}

// noteApplySeams swaps the write seams for the test and restores them when it ends.
func noteApplySeams(t *testing.T, seams noteWriteSeams) {
	t.Helper()
	previous := noteWrites
	noteWrites = seams
	t.Cleanup(func() { noteWrites = previous })
}

// noteApplySeed stores notes straight in the table, without the caches.
func noteApplySeed(t *testing.T, chatID int64, names ...string) {
	t.Helper()
	rows := make([]models.Notes, 0, len(names))
	for _, name := range names {
		rows = append(rows, models.Notes{ChatId: chatID, NoteName: name, NoteContent: "r", MsgType: db.TEXT})
	}
	if err := db.DB.Create(&rows).Error; err != nil {
		t.Fatalf("seed notes: %v", err)
	}
}

func noteApplyInput(name string) noteAddInput {
	return noteAddInput{Name: name, Text: "body", DataType: db.TEXT}
}

// noteApplyRowCount counts the chat's notes_settings rows.
func noteApplyRowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.NotesSettings{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count notes_settings: %v", err)
	}
	return n
}

func TestApplyNote(t *testing.T) {
	ctx := context.Background()
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	const secretName = "zzsecretnotenamezz"
	assertNoNameLogged := func(t *testing.T) {
		t.Helper()
		for _, entry := range hook.AllEntries() {
			if strings.Contains(strings.ToLower(entry.Message), secretName) {
				t.Errorf("log line carries a note name: %q", entry.Message)
			}
		}
	}

	t.Run("refusals before any write", func(t *testing.T) {
		chatID := noteApplyChat(t)
		if got := applyNoteAdd(ctx, chatID, noteApplyInput("  ")).Outcome; got != noteNameBlank {
			t.Errorf("blank = %v, want blank", got)
		}
		for _, bad := range []string{"two words", "#rules", strings.Repeat("x", listKeywordMaxRunes+1)} {
			if got := applyNoteAdd(ctx, chatID, noteApplyInput(bad)).Outcome; got != noteNameBadShape {
				t.Errorf("name %q = %v, want bad shape", bad, got)
			}
		}
		noteApplySeed(t, chatID, "taken")
		if got := applyNoteAdd(ctx, chatID, noteApplyInput("TAKEN")).Outcome; got != noteExists {
			t.Errorf("stored name = %v, want exists", got)
		}
	})

	t.Run("added stores the six flags and drops notes_list once", func(t *testing.T) {
		chatID := noteApplyChat(t)
		noteApplySeed(t, chatID, "old")
		if list := notes.GetNotesList(chatID, true); len(list) != 1 {
			t.Fatalf("primed list = %v", list)
		}
		in := noteAddInput{
			Name: "Fresh", Text: "body", FileID: "file-1", DataType: db.PHOTO,
			Buttons: models.ButtonArray{{Name: "Site", Url: "https://example.com"}},
			PvtOnly: true, AdminOnly: true, WebPrev: true, IsProtected: true, NoNotif: true,
		}
		if got := applyNoteAdd(ctx, chatID, in); got.Outcome != noteAdded || got.Err != nil {
			t.Fatalf("add = %+v", got)
		}
		if list := notes.GetNotesList(chatID, true); len(list) != 2 {
			t.Errorf("notes_list still stale: %v", list)
		}
		var row models.Notes
		if err := db.DB.Where("chat_id = ? AND note_name = ?", chatID, "fresh").Take(&row).Error; err != nil {
			t.Fatalf("stored row: %v", err)
		}
		if row.NoteContent != "body" || row.FileID != "file-1" || row.MsgType != db.PHOTO || len(row.Buttons) != 1 ||
			!row.PrivateOnly || row.GroupOnly || !row.AdminOnly || !row.WebPreview || !row.IsProtected || !row.NoNotif {
			t.Errorf("stored row = %+v", row)
		}
		if n := noteApplyRowCount(t, chatID); n != 0 {
			t.Errorf("notes_settings rows = %d, want none created", n)
		}
	})

	t.Run("raced when the insert lands no row", func(t *testing.T) {
		chatID := noteApplyChat(t)
		seams := noteWrites
		seams.Add = func(context.Context, int64, string, string, string, models.ButtonArray, int, bool, bool, bool, bool, bool, bool) (bool, error) {
			return false, nil
		}
		noteApplySeams(t, seams)
		if got := applyNoteAdd(ctx, chatID, noteApplyInput("hello")).Outcome; got != noteRaced {
			t.Errorf("no insert = %v, want raced", got)
		}
	})

	t.Run("save failed with the error", func(t *testing.T) {
		chatID := noteApplyChat(t)
		boom := errors.New("database is down")
		seams := noteWrites
		seams.Add = func(context.Context, int64, string, string, string, models.ButtonArray, int, bool, bool, bool, bool, bool, bool) (bool, error) {
			return false, boom
		}
		noteApplySeams(t, seams)
		got := applyNoteAdd(ctx, chatID, noteApplyInput(secretName))
		if got.Outcome != noteSaveFailed || !errors.Is(got.Err, boom) {
			t.Errorf("failing add = %+v, want save failed with the error", got)
		}
		assertNoNameLogged(t)
	})

	t.Run("a failed existence lookup stores nothing", func(t *testing.T) {
		chatID := noteApplyChat(t)
		boom := errors.New("lookup failed")
		seams := noteWrites
		seams.Exists = func(context.Context, int64, string) (bool, error) { return false, boom }
		noteApplySeams(t, seams)
		got := applyNoteAdd(ctx, chatID, noteApplyInput(secretName))
		if got.Outcome != noteSaveFailed || !errors.Is(got.Err, boom) {
			t.Errorf("failing lookup = %+v, want save failed with the error", got)
		}
		if notes.DoesNoteExists(chatID, secretName) {
			t.Error("a note was stored although the lookup failed")
		}
		assertNoNameLogged(t)
	})

	t.Run("remove drops notes_list", func(t *testing.T) {
		chatID := noteApplyChat(t)
		noteApplySeed(t, chatID, "hello", "keep")
		if list := notes.GetNotesList(chatID, true); len(list) != 2 {
			t.Fatalf("primed list = %v", list)
		}
		if got := applyNoteRemove(ctx, chatID, "HELLO"); got.Outcome != noteRemoved || got.Err != nil {
			t.Fatalf("remove = %+v", got)
		}
		if list := notes.GetNotesList(chatID, true); len(list) != 1 || list[0] != "keep" {
			t.Errorf("notes_list after remove = %v", list)
		}
		if n := noteApplyRowCount(t, chatID); n != 0 {
			t.Errorf("notes_settings rows = %d, want none created", n)
		}
	})

	t.Run("remove save failed", func(t *testing.T) {
		chatID := noteApplyChat(t)
		boom := errors.New("database is down")
		seams := noteWrites
		seams.Remove = func(context.Context, int64, string) error { return boom }
		noteApplySeams(t, seams)
		got := applyNoteRemove(ctx, chatID, secretName)
		if got.Outcome != noteRemoveSaveFailed || !errors.Is(got.Err, boom) {
			t.Errorf("failing remove = %+v", got)
		}
		assertNoNameLogged(t)
	})
}

// TestNoteCommandsWriteThroughApplyFunctions presses the real /save and /clear handlers with counting
// seams, so a second path that bypassed the apply functions would be seen.
func TestNoteCommandsWriteThroughApplyFunctions(t *testing.T) {
	chatID := noteApplyChat(t)
	var adds, removes int
	seams := noteWrites
	realAdd, realRemove := seams.Add, seams.Remove
	seams.Add = func(ctx context.Context, chatID int64, name, text, fileID string, buttons models.ButtonArray, dataType int, a, b, c, d, e, f bool) (bool, error) {
		adds++
		return realAdd(ctx, chatID, name, text, fileID, buttons, dataType, a, b, c, d, e, f)
	}
	seams.Remove = func(ctx context.Context, chatID int64, name string) error {
		removes++
		return realRemove(ctx, chatID, name)
	}
	noteApplySeams(t, seams)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Notes Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}

	if err := notesModule.addNote(bot, newModuleMessageContext(bot, chat, admin, "/save rules Be kind")); err != ext.EndGroups {
		t.Fatalf("addNote error = %v", err)
	}
	if err := notesModule.rmNote(bot, newModuleMessageContext(bot, chat, admin, "/clear rules")); err != ext.EndGroups {
		t.Fatalf("rmNote error = %v", err)
	}
	if adds != 1 || removes != 1 {
		t.Errorf("seam calls: add %d, remove %d, want 1 each", adds, removes)
	}
}

// TestApplyNoteNameRule proves A19-4: a name that could never be fetched is refused by the check, by the
// apply function and by /save, with nothing stored, and a plain name still stores.
func TestApplyNoteNameRule(t *testing.T) {
	withRealLocales(t)
	ctx := context.Background()
	chatID := noteApplyChat(t)
	for _, bad := range []string{"two words", "a\tb", "a\u3000b", "#rules", strings.Repeat("x", listKeywordMaxRunes+1)} {
		if got := checkNoteName(ctx, chatID, bad).Outcome; got != noteNameBadShape {
			t.Errorf("checkNoteName(%q) = %v, want bad shape", bad, got)
		}
		if got := applyNoteAdd(ctx, chatID, noteApplyInput(bad)).Outcome; got != noteNameBadShape {
			t.Errorf("applyNoteAdd(%q) = %v, want bad shape", bad, got)
		}
	}

	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Notes Chat"}
	run := func(text string, reply *gotgbot.Message) []moduleBotCall {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		msgCtx := newModuleMessageContext(bot, chat, admin, text)
		msgCtx.EffectiveMessage.ReplyToMessage = reply
		if err := notesModule.addNote(bot, msgCtx); err != ext.EndGroups {
			t.Fatalf("addNote(%q) error = %v, want EndGroups", text, err)
		}
		return client.callsFor("sendMessage")
	}
	wantInvalid, _ := i18n.MustNewTranslator("uk").GetString("notes_name_invalid")
	if wantInvalid == "" {
		t.Fatal("notes_name_invalid is missing")
	}
	reply := &gotgbot.Message{MessageId: 55, Date: 1, Chat: chat, From: &member, Text: "reply body"}
	for _, tc := range []struct {
		name  string
		text  string
		reply *gotgbot.Message
	}{
		{"quoted two words in the reply form", `/save "two words"`, reply},
		{"leading hash inline", "/save #x hello", nil},
		{"leading hash in the reply form", "/save #x", reply},
	} {
		calls := run(tc.text, tc.reply)
		if len(calls) != 1 || calls[0].Params["text"] != wantInvalid {
			t.Errorf("%s: replies = %v, want one notes_name_invalid", tc.name, calls)
		}
	}
	var stored int64
	if err := db.DB.Model(&models.Notes{}).Where("chat_id = ?", chatID).Count(&stored).Error; err != nil || stored != 0 {
		t.Errorf("stored notes after the refusals = %d (%v), want 0", stored, err)
	}

	if calls := run("/save rules hello there", nil); len(calls) != 1 {
		t.Fatalf("/save rules replies = %d, want one", len(calls))
	}
	if !notes.DoesNoteExists(chatID, "rules") {
		t.Error("a plain name was not stored")
	}
}

// TestNoteCommandRefusesWhenLookupFails is the command half of the fail-closed lookup: /save answers
// notes_save_failed and stores nothing when its existence read fails (before, it added the note).
func TestNoteCommandRefusesWhenLookupFails(t *testing.T) {
	withRealLocales(t)
	chatID := noteApplyChat(t)
	seams := noteWrites
	seams.Exists = func(context.Context, int64, string) (bool, error) { return false, errors.New("lookup failed") }
	noteApplySeams(t, seams)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Notes Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	if err := notesModule.addNote(bot, newModuleMessageContext(bot, chat, admin, "/save price tag")); err != ext.EndGroups {
		t.Fatalf("addNote error = %v, want EndGroups", err)
	}
	want, _ := i18n.MustNewTranslator("uk").GetString("notes_save_failed")
	calls := client.callsFor("sendMessage")
	if want == "" || len(calls) != 1 || calls[0].Params["text"] != want {
		t.Errorf("replies = %v, want one notes_save_failed", calls)
	}
	if notes.DoesNoteExists(chatID, "price") {
		t.Error("a note was stored although the lookup failed")
	}
}
