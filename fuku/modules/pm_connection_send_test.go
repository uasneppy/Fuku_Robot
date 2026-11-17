//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
)

// sendsWith counts the sendMessage requests to chatID whose text holds want.
func (e *pmConnEnv) sendsWith(chatID int64, want string) int {
	count := 0
	for _, call := range e.client.callsFor("sendMessage") {
		if staffParamInt(call.Params, "chat_id") == chatID && strings.Contains(fmt.Sprint(call.Params["text"]), want) {
			count++
		}
	}
	return count
}

// TestConnectedCommandsAnswerInPrivate proves that /get, /welcome and /goodbye sent in the private chat
// connected to a group answer there and post nothing into the group.
func TestConnectedCommandsAnswerInPrivate(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		env := newPMConnEnv(t)
		if err := notes.AddNote(env.group.Id, "plainnote", "plain note body", "", nil, db.TEXT, false, false, false, false, false, false); err != nil {
			t.Fatalf("AddNote() error = %v", err)
		}
		t.Cleanup(func() { _ = notes.RemoveAllNotes(env.group.Id) })
		env.inPrivate(env.member, "/get plainnote", notesModule.getNotes)
		if got := env.sendsWith(env.member.Id, "plain note body"); got != 1 {
			t.Fatalf("the note reached the private chat %d times, want once", got)
		}
		if got := env.sendsWith(env.group.Id, "plain note body"); got != 0 {
			t.Fatalf("the note was posted into the group %d times", got)
		}
	})

	for _, tc := range []struct {
		name    string
		command string
		text    string
	}{
		{name: "welcome", command: "/welcome", text: "connected welcome body"},
		{name: "goodbye", command: "/goodbye noformat", text: "connected goodbye body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newPMConnEnv(t)
			editCleanup(t, env.group.Id)
			var err error
			if tc.name == "welcome" {
				err = greetings.SetWelcomeText(env.group.Id, tc.text, "", nil, db.TEXT)
			} else {
				err = greetings.SetGoodbyeText(env.group.Id, tc.text, "", nil, db.TEXT)
			}
			if err != nil {
				t.Fatalf("set %s: %v", tc.name, err)
			}
			if tc.name == "welcome" {
				env.inPrivate(env.admin, tc.command, greetingsModule.welcome)
			} else {
				env.inPrivate(env.admin, tc.command, greetingsModule.goodbye)
			}
			if got := env.sendsWith(env.admin.Id, tc.text); got != 1 {
				t.Fatalf("the %s reached the private chat %d times, want once", tc.name, got)
			}
			if got := env.sendsWith(env.group.Id, tc.text); got != 0 {
				t.Fatalf("the %s was posted into the group %d times", tc.name, got)
			}
		})
	}
}
