//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// lockdownSnapshotPermissions is lockdownTestPrePermissions decoded into the typed
// struct, which is what a restrictChatMember call carries.
func lockdownSnapshotPermissions(t *testing.T) gotgbot.ChatPermissions {
	t.Helper()
	var perms gotgbot.ChatPermissions
	if err := json.Unmarshal([]byte(lockdownTestPrePermissions), &perms); err != nil {
		t.Fatalf("decode the test snapshot: %v", err)
	}
	return perms
}

// lockdownCommander adds a creator of the env's chat who can run /unmute and press its
// buttons, and returns that user.
func lockdownCommander(env *lockdownEnv) gotgbot.User {
	commander := env.newJoiner("Creator")
	env.fake.setCreator(env.chat.Id, commander.Id)
	env.fake.setMember(env.chat.Id, commander.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	return commander
}

// lockdownMutedMember adds a muted member (restricted, still in the chat, unable to
// send) to the env's chat and returns it.
func lockdownMutedMember(env *lockdownEnv) gotgbot.User {
	user := env.newJoiner("Muted")
	muted := MutedPermissions
	env.fake.setMember(env.chat.Id, user.Id, staffFakeMember{
		Status:   gotgbot.ChatMemberStatusRestricted,
		IsMember: true,
		Perms:    &muted,
	})
	return user
}

// restrictCallsFor returns the restrictChatMember calls that name userID in the env's
// chat.
func restrictCallsFor(env *lockdownEnv, userID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range env.calls("restrictChatMember") {
		if fmt.Sprint(call.Params["user_id"]) == fmt.Sprint(userID) {
			matched = append(matched, call)
		}
	}
	return matched
}

// sentPermissions is the permission set the one restrictChatMember call for userID
// carried.
func sentPermissions(t *testing.T, env *lockdownEnv, userID int64) gotgbot.ChatPermissions {
	t.Helper()
	calls := restrictCallsFor(env, userID)
	if len(calls) != 1 {
		t.Fatalf("restrictChatMember calls for user %d = %d, want 1", userID, len(calls))
	}
	perms, ok := calls[0].Params["permissions"].(gotgbot.ChatPermissions)
	if !ok {
		t.Fatalf("restrictChatMember permissions parameter is %T, want gotgbot.ChatPermissions", calls[0].Params["permissions"])
	}
	return perms
}

// wantSnapshotPermissions fails unless perms equals the lockdown's stored pre-lockdown
// permissions, and so is not the locked set.
func wantSnapshotPermissions(t *testing.T, perms gotgbot.ChatPermissions) {
	t.Helper()
	want := lockdownSnapshotPermissions(t)
	if !reflect.DeepEqual(perms, want) {
		t.Errorf("unmute sent permissions %+v, want the pre-lockdown set %+v", perms, want)
	}
	if !perms.CanSendMessages {
		t.Error("the unmuted user's own restriction does not allow sending, they would stay muted after the lift")
	}
	if perms.CanSendDocuments {
		t.Error("CanSendDocuments = true, the snapshot has it off")
	}
	if perms.CanReactToMessages == nil || *perms.CanReactToMessages {
		t.Errorf("CanReactToMessages = %v, want a pointer to false", perms.CanReactToMessages)
	}
	if !perms.CanInviteUsers {
		t.Error("CanInviteUsers = false, the snapshot has it on")
	}
}

// pressUnrestrict presses an unrestrict button (action unmute or unban) on message
// msgID of the env's chat as from, through the real dispatcher.
func pressUnrestrict(env *lockdownEnv, from gotgbot.User, msgID int64, action string, userID int64) {
	env.t.Helper()
	data := encodeCallbackData("unrestrict", map[string]string{"a": action, "u": fmt.Sprint(userID)})
	if data == "" {
		env.t.Fatalf("callback data for unrestrict action %s did not encode", action)
	}
	id := env.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		CallbackQuery: &gotgbot.CallbackQuery{
			Id:           fmt.Sprintf("cb-%d", id),
			From:         from,
			Message:      gotgbot.Message{MessageId: msgID, Date: 1, Chat: env.chat},
			Data:         data,
			ChatInstance: "lockdown-unmute-test",
		},
	}
	if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
		env.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

func TestUnmuteDuringLockdownUsesSnapshot(t *testing.T) {
	t.Run("unmute command", func(t *testing.T) {
		env := newLockdownEnv(t)
		LoadMutes(env.dispatcher)
		commander := lockdownCommander(env)
		user := lockdownMutedMember(env)

		env.send(env.admin, "/lockdown")
		env.send(commander, fmt.Sprintf("/unmute %d", user.Id))

		wantSnapshotPermissions(t, sentPermissions(t, env, user.Id))
		env.wantReplyHas(
			staffMarker("mutes_unmute_message"),
			staffMarker("mutes_unmute_lockdown_note"),
		)

		// After the lift the group default is the snapshot again and the user's own
		// record allows sending, so they can talk.
		env.send(env.admin, "/unlockdown")
		if got := env.fake.chatPermsRaw(env.chat.Id); got != lockdownTestPrePermissions {
			t.Errorf("group permissions after the lift = %q, want the snapshot", got)
		}
		member := env.fake.member(env.chat.Id, user.Id)
		if member == nil || member.Perms == nil || !member.Perms.CanSendMessages {
			t.Errorf("the unmuted user's own record after the lift = %+v, want one that allows sending", member)
		}
	})

	t.Run("no lockdown", func(t *testing.T) {
		env := newLockdownEnv(t)
		LoadMutes(env.dispatcher)
		commander := lockdownCommander(env)
		user := lockdownMutedMember(env)

		env.send(commander, fmt.Sprintf("/unmute %d", user.Id))

		live := lockdownSnapshotPermissions(t)
		if got := sentPermissions(t, env, user.Id); !reflect.DeepEqual(got, live) {
			t.Errorf("unmute sent %+v, want the group's live defaults %+v", got, live)
		}
		reply := env.lastReply()
		if !strings.Contains(reply, staffMarker("mutes_unmute_message")) {
			t.Errorf("reply %q does not contain the unmute message", reply)
		}
		if strings.Contains(reply, staffMarker("mutes_unmute_lockdown_note")) {
			t.Errorf("reply %q carries the lockdown note outside a lockdown", reply)
		}
	})

	t.Run("unrestrict button", func(t *testing.T) {
		env := newLockdownEnv(t)
		LoadBans(env.dispatcher)
		commander := lockdownCommander(env)
		user := lockdownMutedMember(env)

		env.send(env.admin, "/lockdown")
		pressUnrestrict(env, commander, 555, "unmute", user.Id)

		wantSnapshotPermissions(t, sentPermissions(t, env, user.Id))
	})

	t.Run("captcha pass", func(t *testing.T) {
		env := newLockdownEnv(t)
		user := lockdownMutedMember(env)

		env.send(env.admin, "/lockdown")
		if err := unmuteCaptchaUser(env.bot, env.chat.Id, user.Id); err != nil {
			t.Fatalf("unmuteCaptchaUser() error = %v", err)
		}

		wantSnapshotPermissions(t, sentPermissions(t, env, user.Id))
	})

	t.Run("staff unmute", func(t *testing.T) {
		env := newLockdownEnv(t)
		withFastStaffPacer(t)
		user := lockdownMutedMember(env)

		env.send(env.admin, "/lockdown")
		err := executeStaffCall(context.Background(), env.bot, env.chat.Id, user.Id, staffVerdict{Call: staffCallUnmute}, 0)
		if err != nil {
			t.Fatalf("executeStaffCall(unmute) error = %v", err)
		}

		wantSnapshotPermissions(t, sentPermissions(t, env, user.Id))
	})
}

// withLockdownLookupError makes every read of a chat's lockdown for an unmute fail
// until the test ends.
func withLockdownLookupError(t *testing.T) {
	t.Helper()
	previous := lockdownSnapshotLookup
	lockdownSnapshotLookup = func(int64) (*models.ChatLockdown, error) {
		return nil, errors.New("lockdown table unreachable")
	}
	t.Cleanup(func() { lockdownSnapshotLookup = previous })
}

func TestUnmuteLockdownLookupFails(t *testing.T) {
	t.Run("unmute command", func(t *testing.T) {
		env := newLockdownEnv(t)
		LoadMutes(env.dispatcher)
		commander := lockdownCommander(env)
		user := lockdownMutedMember(env)

		env.send(env.admin, "/lockdown")
		withLockdownLookupError(t)
		env.send(commander, fmt.Sprintf("/unmute %d", user.Id))

		if calls := restrictCallsFor(env, user.Id); len(calls) != 0 {
			t.Errorf("restrictChatMember calls = %d, want none when the lockdown lookup failed", len(calls))
		}
		for _, sent := range env.replies() {
			if text := fmt.Sprint(sent.Params["text"]); strings.Contains(text, staffMarker("mutes_unmute_message")) {
				t.Errorf("the bot announced an unmute that did not happen: %q", text)
			}
		}
	})

	t.Run("unrestrict button", func(t *testing.T) {
		env := newLockdownEnv(t)
		LoadBans(env.dispatcher)
		commander := lockdownCommander(env)
		user := lockdownMutedMember(env)

		env.send(env.admin, "/lockdown")
		withLockdownLookupError(t)
		pressUnrestrict(env, commander, 555, "unmute", user.Id)

		if calls := restrictCallsFor(env, user.Id); len(calls) != 0 {
			t.Errorf("restrictChatMember calls = %d, want none when the lockdown lookup failed", len(calls))
		}
		for _, call := range env.fake.callsFor("editMessageText") {
			if text := fmt.Sprint(call.Params["text"]); strings.Contains(text, staffMarker("bans_unrestrict_unmuted")) {
				t.Errorf("the button announced an unmute that did not happen: %q", text)
			}
		}
	})

	t.Run("captcha pass", func(t *testing.T) {
		env := newLockdownEnv(t)
		user := lockdownMutedMember(env)
		t.Cleanup(func() { _ = captcha.DeleteMutedUser(user.Id, env.chat.Id) })

		env.send(env.admin, "/lockdown")
		withLockdownLookupError(t)
		if err := restoreCaptchaPermissions(env.bot, env.chat.Id, user.Id); err == nil {
			t.Fatal("restoreCaptchaPermissions() error = nil, want the lookup error")
		}

		if calls := restrictCallsFor(env, user.Id); len(calls) != 0 {
			t.Errorf("restrictChatMember calls = %d, want none when the lockdown lookup failed", len(calls))
		}
		scheduled, err := captcha.GetMutedUser(user.Id, env.chat.Id)
		if err != nil || scheduled == nil {
			t.Fatalf("GetMutedUser() = %v, %v, want the retry scheduled for the sweeper", scheduled, err)
		}
		if scheduled.UnmuteAt.After(time.Now().Add(time.Minute)) {
			t.Errorf("retry scheduled for %v, want it due now", scheduled.UnmuteAt)
		}
	})

	t.Run("staff unmute", func(t *testing.T) {
		env := newLockdownEnv(t)
		withFastStaffPacer(t)
		user := lockdownMutedMember(env)

		env.send(env.admin, "/lockdown")
		withLockdownLookupError(t)
		err := executeStaffCall(context.Background(), env.bot, env.chat.Id, user.Id, staffVerdict{Call: staffCallUnmute}, 0)
		if err == nil {
			t.Fatal("executeStaffCall(unmute) error = nil, want the group to fail")
		}

		if calls := restrictCallsFor(env, user.Id); len(calls) != 0 {
			t.Errorf("restrictChatMember calls = %d, want none when the lockdown lookup failed", len(calls))
		}
	})
}

func TestResolveUnmutePermissionsLockdown(t *testing.T) {
	// countLookups wraps the real lookup and counts its calls.
	countLookups := func(t *testing.T) *int {
		t.Helper()
		var calls int
		previous := lockdownSnapshotLookup
		lockdownSnapshotLookup = func(chatID int64) (*models.ChatLockdown, error) {
			calls++
			return previous(chatID)
		}
		t.Cleanup(func() { lockdownSnapshotLookup = previous })
		return &calls
	}
	live := gotgbot.ChatPermissions{CanSendMessages: false, CanPinMessages: true}
	startLockdown := func(t *testing.T, chatID int64, pre string) *models.ChatLockdown {
		t.Helper()
		lockdownCleanup(t, chatID)
		row := &models.ChatLockdown{
			ChatID:         chatID,
			TriggerKind:    models.LockdownTriggerManual,
			StartedBy:      1,
			PrePermissions: pre,
		}
		started, err := lockdown.Start(row)
		if err != nil || !started {
			t.Fatalf("Start = %v, %v", started, err)
		}
		return row
	}

	t.Run("active lockdown returns the snapshot", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		startLockdown(t, chatID, lockdownTestPrePermissions)
		calls := countLookups(t)

		got, err := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Id: chatID, Permissions: &live})
		if err != nil {
			t.Fatalf("resolveUnmutePermissions() error = %v", err)
		}
		if want := lockdownSnapshotPermissions(t); !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want the stored snapshot %+v", got, want)
		}
		if *calls != 1 {
			t.Errorf("lockdown lookups = %d, want 1", *calls)
		}
	})

	t.Run("no lockdown returns the live permissions", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		lockdownCleanup(t, chatID)
		countLookups(t)

		got, err := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Id: chatID, Permissions: &live})
		if err != nil {
			t.Fatalf("resolveUnmutePermissions() error = %v", err)
		}
		if !reflect.DeepEqual(got, live) {
			t.Errorf("got %+v, want the live permissions %+v", got, live)
		}
	})

	t.Run("lifting lockdown returns the live permissions", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		row := startLockdown(t, chatID, lockdownTestPrePermissions)
		if begun, err := lockdown.BeginLift(row.ID, 1, "Lifter", false); err != nil || !begun {
			t.Fatalf("BeginLift = %v, %v", begun, err)
		}
		countLookups(t)

		got, err := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Id: chatID, Permissions: &live})
		if err != nil {
			t.Fatalf("resolveUnmutePermissions() error = %v", err)
		}
		if !reflect.DeepEqual(got, live) {
			t.Errorf("got %+v, want the restored live permissions %+v", got, live)
		}
	})

	t.Run("chat without permissions falls back to the built-in set", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		lockdownCleanup(t, chatID)
		countLookups(t)

		got, err := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Id: chatID})
		if err != nil {
			t.Fatalf("resolveUnmutePermissions() error = %v", err)
		}
		if want := defaultUnmutePermissions(); !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want the built-in fallback %+v", got, want)
		}
	})

	t.Run("an unreadable snapshot is an error", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		startLockdown(t, chatID, "not json")
		countLookups(t)

		if _, err := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Id: chatID, Permissions: &live}); err == nil {
			t.Fatal("resolveUnmutePermissions() error = nil, want one for a snapshot that cannot be decoded")
		}
	})

	t.Run("a lookup error is returned", func(t *testing.T) {
		withLockdownLookupError(t)

		if _, err := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Id: uniqueModuleChatID(), Permissions: &live}); err == nil {
			t.Fatal("resolveUnmutePermissions() error = nil, want the lookup error")
		}
	})

	t.Run("chat ID 0 never reaches the lockdown lookup", func(t *testing.T) {
		calls := countLookups(t)

		got, err := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Permissions: &live})
		if err != nil {
			t.Fatalf("resolveUnmutePermissions() error = %v", err)
		}
		if !reflect.DeepEqual(got, live) {
			t.Errorf("got %+v, want %+v", got, live)
		}
		if *calls != 0 {
			t.Errorf("lockdown lookups = %d, want 0 for chat ID 0", *calls)
		}
	})
}
