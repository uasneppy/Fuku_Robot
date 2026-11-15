//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// Command deletion toolkit (plan 17-03). The environment is the Phase 16 end-to-end one (the
// settings environment's live creator, the real dispatcher with Bans and Rules loaded, the deletion
// clock, the fast deletion pacer, deletionInline.wait registered last) with the commands rule stored
// as 0 and the dispatcher's processor replaced by the wrapper main.go installs. Phase 16's
// environments are unchanged, so no Phase 16 test sees the wrapper.

// commandDeletionEnv is the Phase 16 environment with the Processor wrapper on its dispatcher.
type commandDeletionEnv struct {
	*deletionE2E
}

// useCommandDeletionProcessor installs the wrapper around inner on the dispatcher, as
// newConfiguredDispatcher does for the production processor.
func useCommandDeletionProcessor(d *ext.Dispatcher, inner ext.Processor) {
	d.Processor = CommandDeletionProcessor{Inner: inner}
}

// newCommandDeletionEnv builds the environment with inner under the wrapper and the group's
// member:commands rule stored as 0 through the real apply function.
func newCommandDeletionEnv(t *testing.T, inner ext.Processor) *commandDeletionEnv {
	t.Helper()
	env := &commandDeletionEnv{deletionE2E: newDeletionE2E(t)}
	useCommandDeletionProcessor(env.dispatcher, inner)
	env.setCommandsRule(true)
	return env
}

// setCommandsRule stores member:commands as 0 (on) or removes the row (off) for the env chat.
func (e *commandDeletionEnv) setCommandsRule(on bool) {
	e.t.Helper()
	target := deletionTarget{Remove: true}
	if on {
		target = deletionSet(0)
	}
	res := applyDeleteRule(context.Background(), e.chat.Id, deletion.KindMemberCommands, target)
	if res.Outcome != deletionApplied && res.Outcome != deletionUnchanged {
		e.t.Fatalf("store member:commands (on=%v) outcome = %v, err = %v", on, res.Outcome, res.Err)
	}
}

// callIndexOf returns the position in the fake's ordered call log of the first call of method that
// satisfies match, or -1. It waits for the inline deletes first, so a delete is in the log.
func (e *commandDeletionEnv) callIndexOf(method string, match func(moduleBotCall) bool) int {
	e.t.Helper()
	deletionInline.wait()
	e.fake.callsFor("sendMessage")
	client := e.fake.moduleBotClient
	client.mu.Lock()
	defer client.mu.Unlock()
	for i, call := range client.calls {
		if call.Method == method && match(call) {
			return i
		}
	}
	return -1
}

// replyIndexTo is the position of the first sendMessage to chatID that replies to message id.
func (e *commandDeletionEnv) replyIndexTo(chatID, id int64) int {
	e.t.Helper()
	return e.callIndexOf("sendMessage", func(call moduleBotCall) bool {
		if staffParamInt(call.Params, "chat_id") != chatID {
			return false
		}
		reply, ok := call.Params["reply_parameters"].(*gotgbot.ReplyParameters)
		return ok && reply != nil && reply.MessageId == id
	})
}

// anySendIndex is the position of the first sendMessage to chatID.
func (e *commandDeletionEnv) anySendIndex(chatID int64) int {
	e.t.Helper()
	return e.callIndexOf("sendMessage", func(call moduleBotCall) bool {
		return staffParamInt(call.Params, "chat_id") == chatID
	})
}

// deleteIndexOf is the position of the first deleteMessage of message id in chatID.
func (e *commandDeletionEnv) deleteIndexOf(chatID, id int64) int {
	e.t.Helper()
	return e.callIndexOf("deleteMessage", func(call moduleBotCall) bool {
		return staffParamInt(call.Params, "chat_id") == chatID && staffParamInt(call.Params, "message_id") == id
	})
}

// commandMessage is a command text with its bot_command entity, as Telegram sends it.
func commandMessage(text string) func(*gotgbot.Message) {
	length := len(text)
	if end := strings.IndexAny(text, " \t\n"); end >= 0 {
		length = end
	}
	return func(m *gotgbot.Message) {
		m.Text = text
		m.Entities = []gotgbot.MessageEntity{{Type: gotgbot.MessageEntityTypeBotCommand, Offset: 0, Length: int64(length)}}
	}
}

// wantNoCommandDelete fails when message id of chatID was deleted or any queue row exists.
func (e *commandDeletionEnv) wantNoCommandDelete(chatID, id int64) {
	e.t.Helper()
	if got := e.deleted(chatID, id); got != 0 {
		e.t.Fatalf("deletes of message %d = %d, want none", id, got)
	}
	if rows := e.rows(); len(rows) != 0 {
		e.t.Fatalf("queue rows = %+v, want none", rows)
	}
}
