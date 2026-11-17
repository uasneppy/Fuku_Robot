//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/filters"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
)

// storedNoteText is the content of the group's note name, read from the database.
func storedNoteText(t *testing.T, chatID int64, name string) string {
	t.Helper()
	var row models.Notes
	if err := db.DB.Where("chat_id = ? AND note_name = ?", chatID, name).First(&row).Error; err != nil {
		t.Fatalf("read note %q: %v", name, err)
	}
	return row.NoteContent
}

// storedFilterText is the reply of the group's filter keyword, read from the database.
func storedFilterText(t *testing.T, chatID int64, keyword string) string {
	t.Helper()
	var row models.ChatFilters
	if err := db.DB.Where("chat_id = ? AND keyword = ?", chatID, keyword).First(&row).Error; err != nil {
		t.Fatalf("read filter %q: %v", keyword, err)
	}
	return row.FilterReply
}

// overwriteCardKind is what differs between the note and the filter overwrite card.
type overwriteCardKind struct {
	name     string
	ns       string
	no       string
	seed     func(t *testing.T, chatID int64)
	stored   func(t *testing.T, chatID int64) string
	setCard  func(t *testing.T, token string, base overwriteBase)
	cardLeft func(token string) bool
	handler  func(*gotgbot.Bot, *ext.Context) error
}

var overwriteCardKinds = []overwriteCardKind{
	{
		name: "note",
		ns:   "notes.overwrite",
		no:   "no",
		seed: func(t *testing.T, chatID int64) {
			if err := notes.AddNote(chatID, "rules", "old text", "", nil, db.TEXT, false, false, false, false, false, false); err != nil {
				t.Fatalf("AddNote() error = %v", err)
			}
			t.Cleanup(func() { _ = notes.RemoveAllNotes(chatID) })
		},
		stored: func(t *testing.T, chatID int64) string { return storedNoteText(t, chatID, "rules") },
		setCard: func(t *testing.T, token string, base overwriteBase) {
			if err := setNoteOverwriteCache(token, overwriteNote{overwriteBase: base}); err != nil {
				t.Fatalf("setNoteOverwriteCache() error = %v", err)
			}
			t.Cleanup(func() { deleteNoteOverwriteCache(token) })
		},
		cardLeft: func(token string) bool {
			_, err := getOverwriteCache[overwriteNote](noteOverwriteCacheKey(token))
			return err == nil
		},
		handler: notesModule.noteOverWriteHandler,
	},
	{
		name: "filter",
		ns:   "filters_overwrite",
		no:   "cancel",
		seed: func(t *testing.T, chatID int64) {
			if err := filters.AddFilter(chatID, "rules", "old text", "", nil, db.TEXT); err != nil {
				t.Fatalf("AddFilter() error = %v", err)
			}
			t.Cleanup(func() { _ = filters.RemoveAllFilters(chatID) })
		},
		stored: func(t *testing.T, chatID int64) string { return storedFilterText(t, chatID, "rules") },
		setCard: func(t *testing.T, token string, base overwriteBase) {
			if err := setFilterOverwriteCache(token, overwriteFilter{overwriteBase: base}); err != nil {
				t.Fatalf("setFilterOverwriteCache() error = %v", err)
			}
			t.Cleanup(func() { deleteFilterOverwriteCache(token) })
		},
		cardLeft: func(token string) bool {
			_, err := getOverwriteCache[overwriteFilter](filterOverwriteCacheKey(token))
			return err == nil
		},
		handler: filtersModule.filterOverWriteHandler,
	},
}

func TestOverwriteCardThroughPrivateConnection(t *testing.T) {
	for _, kind := range overwriteCardKinds {
		t.Run(kind.name, func(t *testing.T) {
			env := newPMConnEnv(t)
			kind.seed(t, env.group.Id)
			private := func(u gotgbot.User) gotgbot.Chat {
				return gotgbot.Chat{Id: u.Id, Type: "private", FirstName: u.FirstName}
			}
			card := func(userID int64) overwriteBase {
				return overwriteBase{ChatID: env.group.Id, UserID: userID, ItemName: "rules", Text: "new text", DataType: db.TEXT}
			}
			n := 0
			press := func(from gotgbot.User, chat gotgbot.Chat, action, token string) {
				t.Helper()
				ctx := newModuleCallbackContext(env.bot, chat, from, encodeCallbackData(kind.ns, map[string]string{"a": action, "t": token}))
				if err := kind.handler(env.bot, ctx); err != nil && err != ext.EndGroups {
					t.Fatalf("press %s error = %v", action, err)
				}
			}
			token := func() string {
				n++
				return fmt.Sprintf("pm-%s-%d-%d", kind.name, env.group.Id, n)
			}
			wantRefused := func(tok string) {
				t.Helper()
				if got := kind.stored(t, env.group.Id); got != "old text" {
					t.Fatalf("stored %s = %q after a refused press, want the old text", kind.name, got)
				}
				if !kind.cardLeft(tok) {
					t.Fatal("a refused press consumed the card")
				}
			}

			// Refused, nothing consumed: another user, a card with no creator, a creator who is no
			// longer an administrator, a missing token.
			tok := token()
			kind.setCard(t, tok, card(env.admin.Id))
			press(env.member, private(env.member), "yes", tok)
			wantRefused(tok)

			zero := token()
			kind.setCard(t, zero, card(0))
			press(env.admin, private(env.admin), "yes", zero)
			wantRefused(zero)

			demoted := token()
			kind.setCard(t, demoted, card(env.member.Id))
			answersBefore := len(env.client.callsFor("answerCallbackQuery"))
			press(env.member, private(env.member), "yes", demoted)
			wantRefused(demoted)
			answers := env.client.callsFor("answerCallbackQuery")
			if len(answers) != answersBefore+1 || !strings.Contains(fmt.Sprint(answers[len(answers)-1].Params["text"]), staffMarker("chat_status_user_admin_button_error")) {
				t.Fatalf("answers to the non-admin press = %+v, want the one admin refusal", answers[answersBefore:])
			}

			press(env.admin, private(env.admin), "yes", "no-such-token")
			if got := kind.stored(t, env.group.Id); got != "old text" {
				t.Fatalf("stored %s = %q after a missing token, want the old text", kind.name, got)
			}

			// No by the creator consumes the card and changes nothing.
			cancel := token()
			kind.setCard(t, cancel, card(env.admin.Id))
			press(env.admin, private(env.admin), kind.no, cancel)
			if kind.cardLeft(cancel) {
				t.Fatal("No left the card in the cache")
			}
			if got := kind.stored(t, env.group.Id); got != "old text" {
				t.Fatalf("stored %s = %q after No, want the old text", kind.name, got)
			}

			// A group callback in another chat than the card's is still cancelled.
			foreign := token()
			kind.setCard(t, foreign, overwriteBase{ChatID: uniqueModuleChatID(), UserID: env.admin.Id, ItemName: "rules", Text: "new text", DataType: db.TEXT})
			press(env.admin, env.group, "yes", foreign)
			if got := kind.stored(t, env.group.Id); got != "old text" {
				t.Fatalf("stored %s = %q after a foreign card in the group, want the old text", kind.name, got)
			}

			// Yes by the creator, a live admin of the card's group, replaces it once.
			edits, answersBefore := len(env.client.callsFor("editMessageText")), len(env.client.callsFor("answerCallbackQuery"))
			yes := token()
			kind.setCard(t, yes, card(env.admin.Id))
			press(env.admin, private(env.admin), "yes", yes)
			if got := kind.stored(t, env.group.Id); got != "new text" {
				t.Fatalf("stored %s = %q after Yes, want the new text", kind.name, got)
			}
			if kind.cardLeft(yes) {
				t.Fatal("Yes left the card in the cache")
			}
			if got := len(env.client.callsFor("editMessageText")) - edits; got != 1 {
				t.Fatalf("edits for Yes = %d, want 1", got)
			}
			if got := len(env.client.callsFor("answerCallbackQuery")) - answersBefore; got != 1 {
				t.Fatalf("answers for Yes = %d, want 1", got)
			}
		})
	}
}

// overwriteCardData returns the callback data of the newest button in namespace ns whose action
// is action, from the messages the bot sent.
func overwriteCardData(t *testing.T, client *pmConnClient, ns, action string) string {
	t.Helper()
	sent := client.callsFor("sendMessage")
	for i := len(sent) - 1; i >= 0; i-- {
		for _, row := range staffKeyboardOf(sent[i].Params["reply_markup"]) {
			for _, button := range row {
				if decoded, ok := decodeCallbackData(button.CallbackData, ns); ok {
					if a, _ := decoded.Field("a"); a == action {
						return button.CallbackData
					}
				}
			}
		}
	}
	t.Fatalf("the bot sent no %s button with action %q", ns, action)
	return ""
}

func TestOverwriteCardThroughPrivateConnectionEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		kind    overwriteCardKind
		command string
	}{
		{overwriteCardKinds[0], "/save rules new text"},
		{overwriteCardKinds[1], "/filter rules new text"},
	} {
		t.Run(tc.kind.name, func(t *testing.T) {
			env := newPMConnEnv(t)
			tc.kind.seed(t, env.group.Id)
			dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
			LoadNotes(dispatcher)
			LoadFilters(dispatcher)
			private := gotgbot.Chat{Id: env.admin.Id, Type: "private", FirstName: env.admin.FirstName}

			if err := dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: 1, Message: &gotgbot.Message{
				MessageId: 11, Date: 1, Chat: private, From: &env.admin, Text: tc.command,
			}}, nil); err != nil {
				t.Fatalf("ProcessUpdate(command) error = %v", err)
			}
			data := overwriteCardData(t, env.client, tc.kind.ns, "yes")

			if err := dispatcher.ProcessUpdate(env.bot, &gotgbot.Update{UpdateId: 2, CallbackQuery: &gotgbot.CallbackQuery{
				Id:           "pm-overwrite",
				From:         env.admin,
				Message:      gotgbot.Message{MessageId: 12, Date: 1, Chat: private},
				Data:         data,
				ChatInstance: "pm-overwrite",
			}}, nil); err != nil {
				t.Fatalf("ProcessUpdate(press) error = %v", err)
			}
			if got := tc.kind.stored(t, env.group.Id); got != "new text" {
				t.Fatalf("stored %s = %q after the press in private, want the new text", tc.kind.name, got)
			}
		})
	}
}
