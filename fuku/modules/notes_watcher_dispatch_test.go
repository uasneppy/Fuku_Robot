//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/locks"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
)

// noteSentTo counts the messages the bot sent to chat whose text holds want.
func noteSentTo(e *phase18E2E, chatID int64, want string) int {
	count := 0
	for _, call := range callsToChat(e.fake.staffBotClient, "sendMessage", chatID) {
		if text, _ := call.Params["text"].(string); strings.Contains(text, want) {
			count++
		}
	}
	return count
}

// TestNotesWatcherLetsLaterGroupsRun proves that a #note message which the note watcher answers still
// reaches the later handler groups: a member's "#rules <link>" in a group with the url lock gets the
// note and is deleted by the lock.
func TestNotesWatcherLetsLaterGroupsRun(t *testing.T) {
	e := newPhase18E2E(t, func(e *phase18E2E) {
		LoadNotes(e.dispatcher)
		LoadLocks(e.dispatcher)
	})
	lockGoldenCleanup(t, e.chat.Id)
	noteGoldenCleanup(t, e.chat.Id)
	settingsLocksWarmBotRight(e.settingsDelEnv)
	if err := notes.AddNote(e.chat.Id, "rules", "be kind to everyone", "", nil, db.TEXT, false, false, false, false, false, false); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if err := locks.UpdateLock(e.chat.Id, "url", true); err != nil {
		t.Fatalf("UpdateLock: %v", err)
	}
	dbcache.ResetLocalForTest()
	member := e.member("Member")

	const link = "https://x.example/a"
	text := "#rules " + link
	id := e.post(member, text, gotgbot.MessageEntity{Type: "url", Offset: 7, Length: int64(len(link))})
	if got := noteSentTo(e, e.chat.Id, "be kind to everyone"); got != 1 {
		t.Fatalf("the note was sent %d times, want once", got)
	}
	if got := e.deleted(id); got != 1 {
		t.Fatalf("the member's locked link was deleted %d times, want once", got)
	}

	// A #word that names no note is left to the later groups as before.
	if other := e.post(member, "#nothing "+link, gotgbot.MessageEntity{Type: "url", Offset: 9, Length: int64(len(link))}); e.deleted(other) != 1 {
		t.Fatalf("a link after an unknown #word was deleted %d times, want once", e.deleted(other))
	}
}
