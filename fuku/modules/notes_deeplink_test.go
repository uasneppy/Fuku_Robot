//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/connections"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// noteStartParamRe takes the start parameter out of a t.me link in a request.
var noteStartParamRe = regexp.MustCompile(`\?start=([^"'\\\s<>&]+)`)

// telegramStartParamRe is what Telegram accepts as a start parameter.
var telegramStartParamRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// noteDeepLinkChat makes a fresh chat whose notes, settings and cache are removed after the test.
func noteDeepLinkChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	if err := chats.EnsureChatInDb(chatID, "Notes Chat"); err != nil {
		t.Fatalf("EnsureChatInDb() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Notes{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.NotesSettings{}).Error
		_ = db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{}).Error
		cache.DeleteCache(cache.CacheKey("notes_list", chatID))
		cache.DeleteCache(cache.CacheKey("notes_settings", chatID))
	})
	return chatID
}

func addDeepLinkNote(t *testing.T, chatID int64, name, content string, privateOnly, adminOnly bool) *models.Notes {
	t.Helper()
	if err := notes.AddNote(chatID, name, content, "", nil, db.TEXT, privateOnly, false, adminOnly, false, false, false); err != nil {
		t.Fatalf("AddNote(%q) error = %v", name, err)
	}
	row := notes.GetNote(chatID, name)
	if row == nil || row.ID == 0 {
		t.Fatalf("note %q was not stored", name)
	}
	return row
}

// startParams returns every start parameter in the requests of one method.
func startParams(t *testing.T, client *moduleBotClient, method string) []string {
	t.Helper()
	var out []string
	for _, call := range client.callsFor(method) {
		raw, err := json.Marshal(call.Params)
		if err != nil {
			t.Fatalf("marshal %s params: %v", method, err)
		}
		for _, m := range noteStartParamRe.FindAllStringSubmatch(string(raw), -1) {
			out = append(out, m[1])
		}
	}
	return out
}

func wantNoteIDLinks(t *testing.T, where string, got []string, chatID int64, rows ...*models.Notes) {
	t.Helper()
	if len(got) != len(rows) {
		t.Fatalf("%s: %d note links %v, want %d", where, len(got), got, len(rows))
	}
	for i, row := range rows {
		want := fmt.Sprintf("noteid_%d_%d", chatID, row.ID)
		if got[i] != want {
			t.Errorf("%s: link %d is %q, want %q", where, i, got[i], want)
		}
		if !telegramStartParamRe.MatchString(got[i]) {
			t.Errorf("%s: link %q is not a valid start parameter", where, got[i])
		}
		if strings.Contains(got[i], row.NoteName) {
			t.Errorf("%s: link %q carries the note name", where, got[i])
		}
	}
}

// TestNoteDeepLinkBuildersUseRowID proves every private-note link carries only noteid_<chat>_<row
// id>, which Telegram accepts for a Cyrillic or a 100-character name, and that names shown in the
// HTML lists are escaped.
func TestNoteDeepLinkBuildersUseRowID(t *testing.T) {
	withRealLocales(t)
	chatID := noteDeepLinkChat(t)
	cyrillic := addDeepLinkNote(t, chatID, "правила", "Cyrillic note", true, false)
	long := addDeepLinkNote(t, chatID, strings.Repeat("я", 100), "Long note", true, false)
	hostile := addDeepLinkNote(t, chatID, "a<b&c", "Hostile note", true, false)
	group := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Notes Chat"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	t.Run("group buttons", func(t *testing.T) {
		for _, row := range []*models.Notes{cyrillic, long} {
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if err := notesModule.notesWatcher(bot, newModuleMessageContext(bot, group, member, "#"+row.NoteName)); err != ext.EndGroups {
				t.Fatalf("notesWatcher(%q) = %v, want EndGroups", row.NoteName, err)
			}
			wantNoteIDLinks(t, "watcher", startParams(t, client, "sendMessage"), chatID, row)

			client = newModuleBotClient()
			bot = newModuleTestBot(client)
			if err := notesModule.getNotes(bot, newModuleMessageContext(bot, group, member, "/get "+row.NoteName)); err != ext.EndGroups {
				t.Fatalf("getNotes(%q) = %v, want EndGroups", row.NoteName, err)
			}
			wantNoteIDLinks(t, "/get", startParams(t, client, "sendMessage"), chatID, row)
		}
	})

	t.Run("notes list deep link", func(t *testing.T) {
		client := newModuleBotClient()
		client.responses["getChat"] = []byte(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Notes Chat"}`, chatID))
		bot := newModuleTestBot(client)
		private := gotgbot.Chat{Id: member.Id, Type: "private", FirstName: "Member"}
		arg := fmt.Sprintf("notes_%d", chatID)
		if err := HandleDeepLink(bot, newModuleMessageContext(bot, private, member, "/start "+arg), &member, arg); err != ext.EndGroups {
			t.Fatalf("HandleDeepLink(notes list) = %v, want EndGroups", err)
		}
		links := startParams(t, client, "sendMessage")
		rows := orderLike(links, chatID, cyrillic, long, hostile)
		wantNoteIDLinks(t, "notes list", links, chatID, rows...)
		wantEscapedName(t, client, hostile.NoteName)
	})

	t.Run("private notes list", func(t *testing.T) {
		client := newModuleBotClient()
		client.responses["getChat"] = []byte(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Notes Chat"}`, chatID))
		bot := newModuleTestBot(client)
		reader := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Reader"}
		if err := connections.ConnectId(reader.Id, chatID); err != nil {
			t.Fatalf("ConnectId() error = %v", err)
		}
		t.Cleanup(func() {
			_ = db.DB.Where("user_id = ?", reader.Id).Delete(&models.ConnectionSettings{}).Error
			_ = db.DB.Where("user_id = ?", reader.Id).Delete(&models.User{}).Error
		})
		privateCtx := newModuleMessageContext(bot, gotgbot.Chat{Id: reader.Id, Type: "private", FirstName: "Reader"}, reader, "/notes")
		if err := notesModule.notesList(bot, privateCtx); err != ext.EndGroups {
			t.Fatalf("notesList(private) = %v, want EndGroups", err)
		}
		links := startParams(t, client, "sendMessage")
		rows := orderLike(links, chatID, cyrillic, long, hostile)
		wantNoteIDLinks(t, "private /notes", links, chatID, rows...)
		wantEscapedName(t, client, hostile.NoteName)
	})
}

// orderLike returns the rows in the order their links appear, so a list's own sort order does not
// matter to the test; a link that names no row is left for wantNoteIDLinks to report.
func orderLike(links []string, chatID int64, rows ...*models.Notes) []*models.Notes {
	byLink := make(map[string]*models.Notes, len(rows))
	for _, row := range rows {
		byLink[fmt.Sprintf("noteid_%d_%d", chatID, row.ID)] = row
	}
	out := make([]*models.Notes, 0, len(rows))
	for _, link := range links {
		if row, ok := byLink[link]; ok {
			out = append(out, row)
		}
	}
	if len(out) != len(rows) {
		return rows
	}
	return out
}

func wantEscapedName(t *testing.T, client *moduleBotClient, name string) {
	t.Helper()
	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("sendMessage calls = %d, want the list", len(calls))
	}
	text := fmt.Sprint(calls[0].Params["text"])
	if strings.Contains(text, name) {
		t.Errorf("the list shows the raw name %q: %q", name, text)
	}
	if !strings.Contains(text, "a&lt;b&amp;c") {
		t.Errorf("the list lacks the escaped name: %q", text)
	}
}

// TestNoteIDDeepLinkHandler routes noteid_ links through the real deep-link router: the note is
// sent, a row ID of another chat is not found, a malformed payload is refused before any Telegram
// call, an admin-only note is refused to a member, and the old note_ payload still works.
func TestNoteIDDeepLinkHandler(t *testing.T) {
	withRealLocales(t)
	chatID := noteDeepLinkChat(t)
	otherChatID := noteDeepLinkChat(t)
	public := addDeepLinkNote(t, chatID, "правила", "Visible note body", true, false)
	adminOnly := addDeepLinkNote(t, chatID, "adminonly", "Hidden note body", false, true)
	foreign := addDeepLinkNote(t, otherChatID, "foreign", "Foreign note body", false, false)
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	private := gotgbot.Chat{Id: member.Id, Type: "private", FirstName: "Member"}

	run := func(t *testing.T, arg string) (*moduleBotClient, error, string) {
		t.Helper()
		client := newModuleBotClient()
		client.responses["getChat"] = []byte(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Notes Chat"}`, chatID))
		bot := newModuleTestBot(client)
		ctx := newModuleMessageContext(bot, private, member, "/start "+arg)
		err := HandleDeepLink(bot, ctx, &member, arg)
		var text string
		if calls := client.callsFor("sendMessage"); len(calls) == 1 {
			text = fmt.Sprint(calls[0].Params["text"])
		} else {
			t.Fatalf("%s: sendMessage calls = %d, want one reply", arg, len(calls))
		}
		return client, err, text
	}
	trText := func(t *testing.T, key string) string {
		t.Helper()
		ctx := newModuleMessageContext(newModuleTestBot(newModuleBotClient()), private, member, "/start")
		text, _ := i18n.MustNewTranslator(lang.GetLanguage(ctx)).GetString(key)
		return text
	}

	t.Run("sends the note", func(t *testing.T) {
		_, err, text := run(t, fmt.Sprintf("noteid_%d_%d", chatID, public.ID))
		if err != ext.EndGroups || !strings.Contains(text, "Visible note body") {
			t.Fatalf("got %v %q, want the note", err, text)
		}
	})

	t.Run("another chat's row is not found", func(t *testing.T) {
		_, err, text := run(t, fmt.Sprintf("noteid_%d_%d", chatID, foreign.ID))
		if err != ext.EndGroups || text != trText(t, "helpers_note_not_exist") {
			t.Fatalf("got %v %q, want the note-does-not-exist reply", err, text)
		}
	})

	t.Run("admin only", func(t *testing.T) {
		_, err, text := run(t, fmt.Sprintf("noteid_%d_%d", chatID, adminOnly.ID))
		if err != ext.ContinueGroups || text != trText(t, "helpers_note_admin_only") {
			t.Fatalf("got %v %q, want the admin-only refusal", err, text)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		for _, arg := range []string{
			"noteid_",
			fmt.Sprintf("noteid_%d", chatID),
			fmt.Sprintf("noteid_%d_", chatID),
			fmt.Sprintf("noteid_%d_abc", chatID),
			fmt.Sprintf("noteid_%d_0", chatID),
			fmt.Sprintf("noteid_%d_-5", chatID),
			fmt.Sprintf("noteid_%d_%d_1", chatID, public.ID),
			fmt.Sprintf("noteid_%d_12345678901234567890", chatID),
			fmt.Sprintf("noteid_x_%d", public.ID),
			fmt.Sprintf("noteid_%d_%d", -chatID, public.ID),
		} {
			client, err, text := run(t, arg)
			if err != ext.EndGroups || text != trText(t, "helpers_invalid_deep_link") {
				t.Errorf("%s: got %v %q, want the invalid-link reply", arg, err, text)
			}
			if calls := client.callsFor("getChat"); len(calls) != 0 {
				t.Errorf("%s: %d getChat calls, want none", arg, len(calls))
			}
		}
	})

	t.Run("old note_ payload", func(t *testing.T) {
		_, err, text := run(t, fmt.Sprintf("note_%d_правила", chatID))
		if err != ext.EndGroups || !strings.Contains(text, "Visible note body") {
			t.Fatalf("got %v %q, want the note", err, text)
		}
	})
}
