//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
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
}
