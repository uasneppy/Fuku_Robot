//go:build testtools

package modules

import (
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// Command deletion (plan 17-03, DEL-03): a handled command is answered and then deleted, through
// the real dispatcher with the wrapper main.go installs.

// commandProcessors are the two Inner processors every ordering test runs under: the bare one and
// the production one (tracing), delivered polling-style (the env's deliver passes nil data).
var commandProcessors = []struct {
	name  string
	inner ext.Processor
}{
	{"base processor", ext.BaseProcessor{}},
	{"tracing processor", tracing.TracingProcessor{}},
}

func TestCommandDeletionAfterAnswer(t *testing.T) {
	for _, proc := range commandProcessors {
		t.Run(proc.name, func(t *testing.T) {
			env := newCommandDeletionEnv(t, proc.inner)
			target := env.mreqMember("Bad", "Actor", "bad")

			id := env.deliver(&env.del.owner, commandMessage(fmt.Sprintf("/ban %d", target.Id)))

			if got := env.deleted(env.chat.Id, id); got != 1 {
				t.Fatalf("deletes of the /ban command = %d, want exactly 1", got)
			}
			reply := env.replyIndexTo(env.chat.Id, id)
			del := env.deleteIndexOf(env.chat.Id, id)
			if reply < 0 || del < 0 {
				t.Fatalf("reply index = %d, delete index = %d, want both in the call log (methods %v)", reply, del, env.methods())
			}
			if reply >= del {
				t.Fatalf("the reply request is at %d and the delete at %d, want the answer first (methods %v)", reply, del, env.methods())
			}
			for _, sent := range env.sentIDs() {
				if got := env.deleted(env.chat.Id, sent); got != 0 {
					t.Errorf("the bot's own message %d was deleted %d times, want it kept", sent, got)
				}
			}
			if rows := env.rows(); len(rows) != 0 {
				t.Fatalf("queue rows after an inline delete = %+v, want none", rows)
			}
			if calls := env.fake.callsFor("banChatMember"); len(calls) != 1 {
				t.Errorf("banChatMember calls = %d, want the command's own effect", len(calls))
			}
		})
	}
}
