//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

func TestDeletionBotRightClassifier(t *testing.T) {
	member := func(status string, canDelete bool) gotgbot.MergedChatMember {
		return gotgbot.MergedChatMember{Status: status, CanDeleteMessages: canDelete}
	}
	wrapped := func(err error) error { return errors.Join(errors.New("context"), err) }
	cases := []struct {
		name  string
		m     gotgbot.MergedChatMember
		err   error
		trust bool
		want  deletionRight
	}{
		{"creator", member(gotgbot.ChatMemberStatusCreator, false), nil, true, deletionRightCan},
		{"administrator with the right", member(gotgbot.ChatMemberStatusAdministrator, true), nil, true, deletionRightCan},
		{"administrator without it, flag trusted", member(gotgbot.ChatMemberStatusAdministrator, false), nil, true, deletionRightCannot},
		{"administrator without it, flag not trusted", member(gotgbot.ChatMemberStatusAdministrator, false), nil, false, deletionRightCan},
		{"member", member(gotgbot.ChatMemberStatusMember, false), nil, true, deletionRightCannot},
		{"restricted", member(gotgbot.ChatMemberStatusRestricted, false), nil, true, deletionRightCannot},
		{"left", member(gotgbot.ChatMemberStatusLeft, false), nil, true, deletionRightCannot},
		{"kicked", member(gotgbot.ChatMemberStatusKicked, false), nil, true, deletionRightCannot},
		{"a null record", gotgbot.MergedChatMember{}, nil, true, deletionRightUnknown},
		{"an unrecognised status", member("something_new", true), nil, true, deletionRightUnknown},
		{"a 400", gotgbot.MergedChatMember{}, staffFakeError(400, "Bad Request: chat not found"), true, deletionRightCannot},
		{"a 403", gotgbot.MergedChatMember{}, staffFakeError(403, "Forbidden: bot was kicked"), true, deletionRightCannot},
		{"a wrapped 403", gotgbot.MergedChatMember{}, wrapped(staffFakeError(403, "Forbidden")), true, deletionRightCannot},
		{"a 429", gotgbot.MergedChatMember{}, staffFake429(3), true, deletionRightUnknown},
		{"a 500", gotgbot.MergedChatMember{}, staffFakeError(500, "Internal Server Error"), true, deletionRightUnknown},
		{"a transport error", gotgbot.MergedChatMember{}, errors.New("dial tcp: i/o timeout"), true, deletionRightUnknown},
		{"a deadline", gotgbot.MergedChatMember{}, context.DeadlineExceeded, true, deletionRightUnknown},
		{"a shed pacer", gotgbot.MergedChatMember{}, ratelimit.ErrRateLimited, true, deletionRightUnknown},
		{"a shed pacer wrapping a 400", gotgbot.MergedChatMember{}, wrapped(errors.Join(ratelimit.ErrRateLimited, staffFakeError(400, "Bad Request"))), true, deletionRightUnknown},
	}
	for _, tc := range cases {
		if got := deletionClassifyBotRight(tc.m, tc.err, tc.trust); got != tc.want {
			t.Errorf("%s: classified as %v, want %v", tc.name, got, tc.want)
		}
	}
	if deletionRight(0) != deletionRightUnknown {
		t.Error("the zero value of deletionRight is not unknown")
	}
}

func TestDeletionReadBotRight(t *testing.T) {
	env := newDeletionEnv(t)
	chatID := env.chat.Id
	if got := deletionReadBotRight(context.Background(), env.bot, chatID, time.Second, true); got != deletionRightCan {
		t.Fatalf("an administrator with the delete right = %v, want can", got)
	}
	env.fake.setBotMember(chatID, lockdownBotAdminJSON(false, false, false))
	if got := deletionReadBotRight(context.Background(), env.bot, chatID, time.Second, true); got != deletionRightCannot {
		t.Errorf("an administrator without it, trusted = %v, want cannot", got)
	}
	if got := deletionReadBotRight(context.Background(), env.bot, chatID, time.Second, false); got != deletionRightCan {
		t.Errorf("an administrator without it, not trusted = %v, want can", got)
	}
	env.script.set(func(_ context.Context, method string, _ map[string]any) error {
		if method == "getChatMember" {
			return staffFakeError(500, "Internal Server Error")
		}
		return nil
	})
	if got := deletionReadBotRight(context.Background(), env.bot, chatID, time.Second, true); got != deletionRightUnknown {
		t.Errorf("a failing read = %v, want unknown", got)
	}
}

// TestDeletionServiceLatchNeedsLiveCheck: a refused service message latches the chat for service
// kinds only after one live read of the bot shows it cannot delete, never for bot kinds, and the
// decision is made at most once per chat per cycle.
func TestDeletionServiceLatchNeedsLiveCheck(t *testing.T) {
	const plainMember = `{"status":"member","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`
	botReads := func(env *deletionEnv) int {
		n := 0
		for _, call := range env.fake.callsFor("getChatMember") {
			if staffParamInt(call.Params, "chat_id") == env.chat.Id && staffParamInt(call.Params, "user_id") == staffTestBotID {
				n++
			}
		}
		return n
	}
	// refuseService answers the batch and every service message with a refusal, and the live
	// read of the bot with memberErr (nil lets it through to the fake's record).
	refuseService := func(env *deletionEnv, service []int64, memberErr error) {
		perID := make(map[int64]error, len(service))
		for _, id := range service {
			perID[id] = deletionRefusal()
		}
		env.script.set(func(_ context.Context, method string, params map[string]any) error {
			switch method {
			case "deleteMessages":
				return deletionRefusal()
			case "deleteMessage":
				return perID[staffParamInt(params, "message_id")]
			case "getChatMember":
				return memberErr
			}
			return nil
		})
	}

	t.Run("a plain member bot latches service kinds and keeps deleting its own messages", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, plainMember)
		service := env.seedDue(env.chat.Id, 50000, 3, deletion.KindPinned)
		bots := env.seedDue(env.chat.Id, 50100, 2, deletion.KindBotWelcome)
		seedDeletionRow(t, env.chat.Id, 50900, env.now.Add(time.Hour), env.now.Add(-time.Minute), deletion.KindTitle)
		refuseService(env, service, nil)

		env.pass()

		if got := deletionLatchValue(t, env, env.chat.Id); got != deletionScopeService {
			t.Fatalf("latch = %q, want %q", got, deletionScopeService)
		}
		if n := botReads(env); n != 1 {
			t.Fatalf("live reads of the bot = %d, want exactly 1", n)
		}
		singles := env.singleCalls(env.chat.Id)
		if len(singles) != 3 || singles[0] != service[0] || singles[1] != bots[0] || singles[2] != bots[1] {
			t.Fatalf("single calls = %v, want the first service message then both bot messages (the other service rows are dropped with no call)", singles)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want the claimed and the pending service rows gone and the bot rows deleted", left)
		}
	})

	// SEC-02: the worker trusts the administrator flag for a supergroup ID, so an administrator
	// without the delete right is latched there (the basic-group cases are in
	// TestDeletionWorkerTrustsAdminFlagInSupergroups).
	t.Run("an administrator without the right is latched in a supergroup, the flag is trusted", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, lockdownBotAdminJSON(false, false, false))
		service := env.seedDue(env.chat.Id, 51000, 3, deletion.KindPinned)
		refuseService(env, service, nil)

		env.pass()

		if got := deletionLatchValue(t, env, env.chat.Id); got != deletionScopeService {
			t.Fatalf("latch = %q, want %q", got, deletionScopeService)
		}
		if n := botReads(env); n != 1 {
			t.Fatalf("live reads of the bot = %d, want 1 per chat per cycle", n)
		}
		if singles := env.singleCalls(env.chat.Id); len(singles) != 1 {
			t.Fatalf("single calls = %v, want one (the other service rows are dropped with no call)", singles)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want none", left)
		}
	})

	t.Run("an unknown answer sets no latch", func(t *testing.T) {
		env := newDeletionEnv(t)
		service := env.seedDue(env.chat.Id, 52000, 3, deletion.KindPinned)
		refuseService(env, service, staffFakeError(500, "Internal Server Error"))

		env.pass()

		if got := deletionLatchValue(t, env, env.chat.Id); got != "" {
			t.Fatalf("latch = %q, want none", got)
		}
		if n := botReads(env); n != 1 {
			t.Fatalf("live reads of the bot = %d, want 1", n)
		}
		if left := env.queuedNoWait(env.chat.Id); len(left) != 0 {
			t.Fatalf("rows left = %+v, want the refused rows dropped", left)
		}
	})

	t.Run("a bot kind refused the same way sets no latch and reads nothing", func(t *testing.T) {
		env := newDeletionEnv(t)
		env.fake.setBotMember(env.chat.Id, plainMember)
		bots := env.seedDue(env.chat.Id, 53000, 3, deletion.KindBotWelcome)
		perID := map[int64]error{}
		for _, id := range bots {
			perID[id] = deletionRefusal()
		}
		env.scriptDelete(deletionRefusal(), perID)

		env.pass()

		if got := deletionLatchValue(t, env, env.chat.Id); got != "" {
			t.Fatalf("latch = %q, want none for bot kinds", got)
		}
		if n := botReads(env); n != 0 {
			t.Fatalf("live reads of the bot = %d, want 0: a bot kind never asks about the right", n)
		}
	})
}
