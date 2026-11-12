//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/disabling"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
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

// deliverIn runs a message of from in chat through the env's real dispatcher, dated by the deletion
// clock, and returns its ID.
func (e *commandDeletionEnv) deliverIn(chat gotgbot.Chat, from *gotgbot.User, apply func(*gotgbot.Message)) int64 {
	e.t.Helper()
	id := e.updateID()
	msg := &gotgbot.Message{MessageId: id, Date: e.now().Unix(), Chat: chat, From: from}
	if apply != nil {
		apply(msg)
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, &gotgbot.Update{UpdateId: id, Message: msg}, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
	return id
}

// wantAnsweredThenDeleted fails unless message id of chatID was deleted exactly once and, when
// wantAnswer is set, the bot sent something to the chat before that delete.
func (e *commandDeletionEnv) wantAnsweredThenDeleted(chatID, id int64, wantAnswer bool) {
	e.t.Helper()
	if got := e.deleted(chatID, id); got != 1 {
		e.t.Fatalf("deletes of the command = %d, want exactly 1 (methods %v)", got, e.methods())
	}
	if wantAnswer {
		send, del := e.anySendIndex(chatID), e.deleteIndexOf(chatID, id)
		if send < 0 || send >= del {
			e.t.Fatalf("first send at %d, delete at %d, want the answer before the delete (methods %v)", send, del, e.methods())
		}
	}
	if rows := e.rows(); len(rows) != 0 {
		e.t.Fatalf("queue rows = %+v, want none after an inline delete", rows)
	}
}

func TestCommandDeletionSenders(t *testing.T) {
	cases := []struct {
		name string
		from func(env *commandDeletionEnv) gotgbot.User
		// mutate changes the delivered message; nil leaves it as sent.
		mutate func(env *commandDeletionEnv, m *gotgbot.Message)
		want   bool
	}{
		{name: "a member", want: true, from: func(env *commandDeletionEnv) gotgbot.User { return env.mreqMember("Mia", "", "mia") }},
		{name: "an approved user", want: true, from: func(env *commandDeletionEnv) gotgbot.User {
			member := env.mreqMember("Ann", "", "ann")
			if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.del.owner.Id, "trusted"); err != nil {
				t.Fatalf("AddApprovedUser error = %v", err)
			}
			return member
		}},
		{name: "the creator", want: true, from: func(env *commandDeletionEnv) gotgbot.User { return env.del.owner }},
		{name: "a Telegram service account", from: func(*commandDeletionEnv) gotgbot.User {
			return gotgbot.User{Id: 777000, FirstName: "Telegram"}
		}},
		{name: "a bot user", from: func(*commandDeletionEnv) gotgbot.User {
			return gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Robot"}
		}},
		{name: "a channel", from: func(env *commandDeletionEnv) gotgbot.User { return env.mreqMember("Cha", "", "cha") },
			mutate: func(_ *commandDeletionEnv, m *gotgbot.Message) {
				m.SenderChat = &gotgbot.Chat{Id: -1002000000002, Type: gotgbot.ChatTypeChannel, Title: "News"}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCommandDeletionEnv(t, ext.BaseProcessor{})
			from := tc.from(env)

			id := env.deliver(&from, func(m *gotgbot.Message) {
				commandMessage("/rules")(m)
				if tc.mutate != nil {
					tc.mutate(env, m)
				}
			})

			if tc.want {
				env.wantAnsweredThenDeleted(env.chat.Id, id, true)
				return
			}
			env.wantNoCommandDelete(env.chat.Id, id)
		})
	}
}

func TestCommandDeletionRefusedAndFailed(t *testing.T) {
	t.Run("a check refuses a member", func(t *testing.T) {
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		member := env.mreqMember("Mia", "", "mia")
		target := env.mreqMember("Tom", "", "tom")

		id := env.deliver(&member, commandMessage(fmt.Sprintf("/ban %d", target.Id)))

		env.wantAnsweredThenDeleted(env.chat.Id, id, true)
		if calls := env.fake.callsFor("banChatMember"); len(calls) != 0 {
			t.Errorf("banChatMember calls = %d, want the refused command to act on nobody", len(calls))
		}
	})

	t.Run("a disabled command", func(t *testing.T) {
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		member := env.mreqMember("Mia", "", "mia")
		if err := disabling.DisableCMD(env.chat.Id, "banme"); err != nil {
			t.Fatalf("DisableCMD error = %v", err)
		}
		t.Cleanup(func() { _ = disabling.EnableCMD(env.chat.Id, "banme") })

		id := env.deliver(&member, commandMessage("/banme"))

		env.wantAnsweredThenDeleted(env.chat.Id, id, false)
		if calls := env.fake.callsFor("banChatMember"); len(calls) != 0 {
			t.Errorf("banChatMember calls = %d, want a disabled command to do nothing", len(calls))
		}
	})

	t.Run("a command refused in a basic group", func(t *testing.T) {
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		basic := gotgbot.Chat{Id: uniqueModuleChatID(), Type: gotgbot.ChatTypeGroup, Title: "Basic"}
		deletionCleanup(t, basic.Id)
		res := applyDeleteRule(context.Background(), basic.Id, deletion.KindMemberCommands, deletionSet(0))
		if res.Outcome != deletionApplied {
			t.Fatalf("store member:commands outcome = %v, err = %v", res.Outcome, res.Err)
		}
		member := env.mreqMember("Mia", "", "mia")

		id := env.deliverIn(basic, &member, commandMessage("/lockdown"))

		env.wantAnsweredThenDeleted(basic.Id, id, true)
	})

	t.Run("a handler that returns an error", func(t *testing.T) {
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		env.dispatcher.AddHandler(helpers.NewBotCommand("failcmd17", func(*gotgbot.Bot, *ext.Context) error {
			return errors.New("handler failed")
		}))
		member := env.mreqMember("Mia", "", "mia")

		id := env.deliver(&member, commandMessage("/failcmd17"))

		env.wantAnsweredThenDeleted(env.chat.Id, id, false)
	})

	t.Run("a cancel in the group", func(t *testing.T) {
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		member := env.mreqMember("Mia", "", "mia")

		id := env.deliver(&member, commandMessage("/cancel"))

		env.wantAnsweredThenDeleted(env.chat.Id, id, false)
	})
}

func TestCommandDeletionHeldAndRepeated(t *testing.T) {
	notFound := staffFakeError(400, "Bad Request: message to delete not found")

	t.Run("a member with a pending captcha", func(t *testing.T) {
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		member := env.mreqMember("Mia", "", "mia")
		if err := captcha.SetCaptchaEnabled(env.chat.Id, true); err != nil {
			t.Fatalf("SetCaptchaEnabled error = %v", err)
		}
		if _, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, env.chat.Id, "7", 2); err != nil {
			t.Fatalf("CreateCaptchaAttemptPreMessage error = %v", err)
		}
		// Group -10 deletes the held message first; the wrapper's own delete then meets "not found".
		env.fake.script("deleteMessage", env.chat.Id, nil, notFound)
		hook := mreqShedHook(t)

		id := env.deliver(&member, commandMessage("/rules"))

		if got := env.deleted(env.chat.Id, id); got != 2 {
			t.Fatalf("delete requests of the held command = %d, want the captcha's and one refused call (methods %v)", got, env.methods())
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v, want none: a refused delete is dropped quietly", rows)
		}
		deletionWantNoMessageIDs(t, hook, []int64{id})
	})

	t.Run("a command the handler already deleted", func(t *testing.T) {
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		LoadPurges(env.dispatcher)
		target := env.deliver(&env.del.owner, func(m *gotgbot.Message) { m.Text = "to remove" })
		env.fake.script("deleteMessage", env.chat.Id, nil, nil, notFound)
		hook := mreqShedHook(t)

		id := env.deliver(&env.del.owner, func(m *gotgbot.Message) {
			commandMessage("/del")(m)
			m.ReplyToMessage = &gotgbot.Message{MessageId: target, Chat: env.chat}
		})

		if got := env.deleted(env.chat.Id, id); got < 2 {
			t.Fatalf("delete requests of the command = %d, want the handler's and the wrapper's (methods %v)", got, env.methods())
		}
		if got := env.deleted(env.chat.Id, target); got != 1 {
			t.Errorf("delete requests of the replied-to message = %d, want 1", got)
		}
		if rows := env.rows(); len(rows) != 0 {
			t.Fatalf("queue rows = %+v, want none", rows)
		}
		deletionWantNoMessageIDs(t, hook, []int64{id, target})
	})
}

func TestCommandDeletionStaffGroupNeverTouched(t *testing.T) {
	setup := func(t *testing.T) (*commandDeletionEnv, gotgbot.User) {
		t.Helper()
		env := newCommandDeletionEnv(t, ext.BaseProcessor{})
		staffCleanup(t, env.chat.Id)
		if _, err := staff.CreateStaffGroup(env.chat.Id, env.del.owner.Id, "Staff"); err != nil {
			t.Fatalf("CreateStaffGroup error = %v", err)
		}
		return env, env.mreqMember("Mia", "", "mia")
	}

	t.Run("a Staff Group", func(t *testing.T) {
		env, member := setup(t)

		id := env.deliver(&member, commandMessage("/rules"))

		env.wantNoCommandDelete(env.chat.Id, id)
		if n := env.deleteCalls(env.chat.Id); n != 0 {
			t.Fatalf("delete requests to the Staff Group = %d, want none", n)
		}
	})

	t.Run("the Staff Group read fails", func(t *testing.T) {
		env, member := setup(t)
		mreqSwap(t, &deletionStaffLookup, func(int64) (*models.StaffGroup, error) { return nil, errors.New("lookup failed") })

		id := env.deliver(&member, commandMessage("/rules"))

		env.wantNoCommandDelete(env.chat.Id, id)
		if n := env.deleteCalls(env.chat.Id); n != 0 {
			t.Fatalf("delete requests with the Staff Group read failing = %d, want none (fail closed)", n)
		}
	})
}
