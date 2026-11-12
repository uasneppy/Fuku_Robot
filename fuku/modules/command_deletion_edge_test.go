//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Command deletion edges (plan 17-04). The anonymous-admin cases run on the lockdown environment of
// the anonymous re-entry tests (the proof prompt, the tap and the re-entered command are the real
// code) with the Processor wrapper on its dispatcher and the commands rule stored as 0.

// anonDeletionEnv is the anonymous-admin lockdown environment with command deletion switched on.
type anonDeletionEnv struct {
	*lockdownEnv
	creator gotgbot.User
	target  gotgbot.User
}

func newAnonDeletionEnv(t *testing.T, anonMode bool) *anonDeletionEnv {
	t.Helper()
	env, creator := newAnonLockdownEnv(t, anonMode)
	withFastDeletionPacer(t)
	deletionCleanup(t, env.chat.Id)
	t.Cleanup(deletionInline.wait)
	LoadBans(env.dispatcher)
	useCommandDeletionProcessor(env.dispatcher, ext.BaseProcessor{})
	res := applyDeleteRule(context.Background(), env.chat.Id, deletion.KindMemberCommands, deletionSet(0))
	if res.Outcome != deletionApplied {
		t.Fatalf("store member:commands outcome = %v, err = %v", res.Outcome, res.Err)
	}
	target := env.newJoiner("Target")
	env.fake.setMember(env.chat.Id, target.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	return &anonDeletionEnv{lockdownEnv: env, creator: creator, target: target}
}

// commandID is the message ID of the last message the test sent (sendAnonymous uses the update ID).
func (e *anonDeletionEnv) commandID() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nextUpdate
}

// callLog is the fake's whole ordered call log as "method" or "method:<message id>" entries for the
// env chat.
func (e *anonDeletionEnv) callLog() []string {
	e.t.Helper()
	deletionInline.wait()
	e.fake.callsFor("sendMessage")
	client := e.fake.moduleBotClient
	client.mu.Lock()
	defer client.mu.Unlock()
	var log []string
	for _, call := range client.calls {
		if call.Params["chat_id"] == nil || staffParamInt(call.Params, "chat_id") != e.chat.Id {
			continue
		}
		if call.Method == "deleteMessage" {
			log = append(log, fmt.Sprintf("deleteMessage:%d", staffParamInt(call.Params, "message_id")))
			continue
		}
		log = append(log, call.Method)
	}
	return log
}

func indexOfEntry(log []string, want string) int {
	for i, entry := range log {
		if entry == want {
			return i
		}
	}
	return -1
}

func (e *anonDeletionEnv) queueRowsOfChat() []models.ScheduledDeletion {
	e.t.Helper()
	var rows []models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Find(&rows).Error; err != nil {
		e.t.Fatalf("read the queue: %v", err)
	}
	return rows
}

func TestCommandDeletionAnonymousAdmin(t *testing.T) {
	banText := func(e *anonDeletionEnv) string {
		return fmt.Sprintf("/ban %d spam", e.target.Id)
	}

	t.Run("after the tap", func(t *testing.T) {
		env := newAnonDeletionEnv(t, false)

		env.sendAnonymous(banText(env))
		cmdID := env.commandID()
		_, proofID, _ := env.proofButton()

		log := env.callLog()
		if indexOfEntry(log, fmt.Sprintf("deleteMessage:%d", cmdID)) != -1 {
			t.Fatalf("the command was deleted at the first update, before the proof: %v", log)
		}
		if rows := env.queueRowsOfChat(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v at the first update, want none", rows)
		}

		env.tapProof(env.creator)

		log = env.callLog()
		proof := indexOfEntry(log, fmt.Sprintf("deleteMessage:%d", proofID))
		ban := indexOfEntry(log, "banChatMember")
		cmd := indexOfEntry(log, fmt.Sprintf("deleteMessage:%d", cmdID))
		reply := -1
		for i := ban + 1; i < len(log); i++ {
			if log[i] == "sendMessage" {
				reply = i
				break
			}
		}
		if proof < 0 || ban < 0 || reply < 0 || cmd < 0 {
			t.Fatalf("call log %v lacks the proof delete (%d), the ban (%d), the reply (%d) or the command delete (%d)", log, proof, ban, reply, cmd)
		}
		if !(proof < ban && ban < reply && reply < cmd) {
			t.Fatalf("order proof %d, ban %d, reply %d, command delete %d, want strictly increasing: %v", proof, ban, reply, cmd, log)
		}
		if n := strings.Count(strings.Join(log, ","), fmt.Sprintf("deleteMessage:%d", cmdID)); n != 1 {
			t.Errorf("command delete requests = %d, want 1", n)
		}
	})

	t.Run("AnonAdmin on", func(t *testing.T) {
		// D-34 and owner confirmation A17-2: with the setting on the command runs at its first update
		// (sender_chat set, no proof), the wrapper skips that update and the command stays.
		env := newAnonDeletionEnv(t, true)

		env.sendAnonymous(banText(env))
		cmdID := env.commandID()

		log := env.callLog()
		if indexOfEntry(log, "banChatMember") < 0 {
			t.Fatalf("the ban did not run at the first update with AnonAdmin on: %v", log)
		}
		if indexOfEntry(log, "sendMessage") < 0 {
			t.Fatalf("no reply was sent: %v", log)
		}
		if got := indexOfEntry(log, fmt.Sprintf("deleteMessage:%d", cmdID)); got != -1 {
			t.Fatalf("the command message was deleted (log %v), the stated limit says it stays", log)
		}
		if rows := env.queueRowsOfChat(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v, want none", rows)
		}
	})

	t.Run("no tap", func(t *testing.T) {
		env := newAnonDeletionEnv(t, false)

		env.sendAnonymous(banText(env))
		cmdID := env.commandID()
		env.proofButton()

		log := env.callLog()
		if got := indexOfEntry(log, fmt.Sprintf("deleteMessage:%d", cmdID)); got != -1 {
			t.Fatalf("the command was deleted with no tap: %v", log)
		}
		if indexOfEntry(log, "banChatMember") != -1 {
			t.Fatalf("a ban ran with no tap: %v", log)
		}
		if rows := env.queueRowsOfChat(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v, want none", rows)
		}
		// The cache entry expiring changes nothing: a tap after it is the expired answer and deletes nothing.
		dbcache.ResetLocalForTest()
		env.redis.FastForward(25 * time.Second)
		env.tapProof(env.creator)
		if got := indexOfEntry(env.callLog(), fmt.Sprintf("deleteMessage:%d", cmdID)); got != -1 {
			t.Fatalf("a tap after the cache expired deleted the command: %v", env.callLog())
		}
	})

	t.Run("the cached sender is not read", func(t *testing.T) {
		env := newAnonDeletionEnv(t, false)

		env.sendAnonymous(banText(env))
		cmdID := env.commandID()
		cached, err := getAnonAdminCache(env.chat.Id, cmdID)
		if err != nil || cached == nil {
			t.Fatalf("getAnonAdminCache = %v, %v, want the cached command", cached, err)
		}
		if cached.From == nil || !staffServiceUserIDs[cached.From.Id] {
			t.Fatalf("cached From = %+v, want the Group Anonymous Bot (a service ID)", cached.From)
		}

		env.tapProof(env.creator)

		if got := indexOfEntry(env.callLog(), fmt.Sprintf("deleteMessage:%d", cmdID)); got < 0 {
			t.Fatalf("the command was not deleted after the tap although the tapper is a member: %v", env.callLog())
		}
	})

	t.Run("the first update is skipped", func(t *testing.T) {
		env := newAnonDeletionEnv(t, false)

		env.sendAnonymous(banText(env))

		if rows := env.queueRowsOfChat(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v before the tap, want none", rows)
		}
		for _, entry := range env.callLog() {
			if strings.HasPrefix(entry, "deleteMessage") {
				t.Fatalf("a delete was made before the tap: %v", env.callLog())
			}
		}
	})
}
