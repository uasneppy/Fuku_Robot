//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/antiraid"
	"github.com/divkix/Alita_Robot/alita/db/greetings"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// sendIn runs a message in any chat of the env as from through the real dispatcher.
func (e *lockdownEnv) sendIn(chat gotgbot.Chat, from gotgbot.User, text string) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		Message:  &gotgbot.Message{MessageId: id, Date: 1, Chat: chat, From: &from, Text: text},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// joinIn runs a chat_member update in any chat of the env: user joined through an
// invite link by their own doing.
func (e *lockdownEnv) joinIn(chat gotgbot.Chat, user gotgbot.User) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		ChatMember: &gotgbot.ChatMemberUpdated{
			Chat:          chat,
			From:          user,
			Date:          1,
			OldChatMember: gotgbot.ChatMemberLeft{User: user},
			NewChatMember: gotgbot.ChatMemberMember{User: user},
			InviteLink:    &gotgbot.ChatInviteLink{InviteLink: "https://t.me/+abc", Creator: e.admin},
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

// callsIn returns the recorded calls of method addressed to chatID.
func (e *lockdownEnv) callsIn(chatID int64, method string) []moduleBotCall {
	return callsToChat(e.fake.staffBotClient, method, chatID)
}

// userCallsIn counts the calls of method about userID in chatID.
func (e *lockdownEnv) userCallsIn(chatID int64, method string, userID int64) int {
	count := 0
	for _, call := range e.callsIn(chatID, method) {
		if staffParamInt(call.Params, "user_id") == userID {
			count++
		}
	}
	return count
}

// writesTo counts everything the bot did to chatID that changes it or posts in it:
// bans, unbans, restrictions, permission changes, messages, deletes and the answers to
// join requests.
func (e *lockdownEnv) writesTo(chatID int64) int {
	total := 0
	for _, method := range []string{
		"banChatMember", "unbanChatMember", "restrictChatMember", "setChatPermissions",
		"sendMessage", "sendPhoto", "deleteMessage", "approveChatJoinRequest", "declineChatJoinRequest",
	} {
		total += len(e.callsIn(chatID, method))
	}
	return total
}

// joinerRowsIn returns the joiner rows of userID in chatID.
func joinerRowsIn(t *testing.T, chatID, userID int64) []models.LockdownJoiner {
	t.Helper()
	var rows []models.LockdownJoiner
	if err := db.DB.Where("chat_id = ? AND user_id = ?", chatID, userID).Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("read joiner rows: %v", err)
	}
	return rows
}

// wantRowState fails unless chatID holds exactly one row of userID, in state want.
func wantRowState(t *testing.T, chatID, userID int64, want string) {
	t.Helper()
	rows := joinerRowsIn(t, chatID, userID)
	if len(rows) != 1 || rows[0].State != want {
		t.Errorf("joiner rows of user %d in chat %d = %+v, want exactly one in state %s", userID, chatID, rows, want)
	}
}

// wantLockedPermissions fails unless the permissions the fake holds for chatID are the
// locked set.
func (e *lockdownEnv) wantLockedPermissions(chatID int64) {
	e.t.Helper()
	var got, want map[string]bool
	if err := json.Unmarshal([]byte(e.fake.chatPermsRaw(chatID)), &got); err != nil {
		e.t.Fatalf("permissions of chat %d are not a bool map: %v", chatID, err)
	}
	if err := json.Unmarshal([]byte(lockdownLockedPermissions), &want); err != nil {
		e.t.Fatalf("locked set is not a bool map: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		e.t.Errorf("permissions of chat %d = %v, want the locked set %v", chatID, got, want)
	}
}

// lockIn locks chat as the env's administrator and returns the confirmed row.
func (e *lockdownEnv) lockIn(chat gotgbot.Chat) *models.ChatLockdown {
	e.t.Helper()
	e.sendIn(chat, e.admin, "/lockdown raid")
	row, err := lockdown.GetActiveFresh(chat.Id)
	if err != nil || row == nil || row.LockedAt == nil {
		e.t.Fatalf("setup: GetActiveFresh(%d) = %v, %v, want a confirmed lockdown", chat.Id, row, err)
	}
	return row
}

func TestLockdownAffectsOnlyItsChat(t *testing.T) {
	for _, order := range []string{"lift A first", "lift B first"} {
		t.Run(order, func(t *testing.T) {
			env := newLockdownEnv(t)
			env.loadJoinModules()
			chatA := env.chat
			chatB := env.addChat()
			env.enableJoinWelcome(false)
			if err := greetings.SetWelcomeToggle(chatB.Id, true); err != nil {
				t.Fatalf("enable welcome in B: %v", err)
			}

			// Only A is locked. J joins both: A holds J, B is a normal group.
			env.lockIn(chatA)
			j := env.newJoiner("J")
			welcomesB := len(env.fake.sentTo(chatB.Id)) + len(env.callsIn(chatB.Id, "sendPhoto"))
			env.joinIn(chatA, j)
			env.joinIn(chatB, j)
			env.cycle()

			wantRowState(t, chatA.Id, j.Id, models.JoinerStateBanned)
			if rows := joinerRowsIn(t, chatB.Id, j.Id); len(rows) != 0 {
				t.Errorf("rows of J in B = %+v, want none: B is not locked", rows)
			}
			if got := env.userCallsIn(chatA.Id, "banChatMember", j.Id); got != 1 {
				t.Errorf("banChatMember calls for J in A = %d, want 1", got)
			}
			if got := env.userCallsIn(chatB.Id, "banChatMember", j.Id); got != 0 {
				t.Errorf("banChatMember calls for J in B = %d, want none", got)
			}
			if got := len(env.callsIn(chatB.Id, "setChatPermissions")); got != 0 {
				t.Errorf("setChatPermissions calls to B = %d, want none: only A was locked", got)
			}
			if got := len(env.callsIn(chatA.Id, "setChatPermissions")); got != 1 {
				t.Errorf("setChatPermissions calls to A = %d, want 1", got)
			}
			if after := len(env.fake.sentTo(chatB.Id)) + len(env.callsIn(chatB.Id, "sendPhoto")); after == welcomesB {
				t.Error("no welcome was sent in B: greetings should run in a group that is not locked")
			}
			env.sendIn(chatB, env.admin, "/lockdownstatus")
			sentB := env.fake.sentTo(chatB.Id)
			if last := fmt.Sprint(sentB[len(sentB)-1].Params["text"]); last != staffMarker("lockdown_not_active") {
				t.Errorf("/lockdownstatus in B replied %q, want %q", last, staffMarker("lockdown_not_active"))
			}

			// B is locked as well. K joins both and is banned in both, by separate rows.
			env.lockIn(chatB)
			k := env.newJoiner("K")
			env.joinIn(chatA, k)
			env.joinIn(chatB, k)
			env.cycle()

			wantRowState(t, chatA.Id, k.Id, models.JoinerStateBanned)
			wantRowState(t, chatB.Id, k.Id, models.JoinerStateBanned)
			rowA, rowB := joinerRowsIn(t, chatA.Id, k.Id), joinerRowsIn(t, chatB.Id, k.Id)
			if len(rowA) == 1 && len(rowB) == 1 && rowA[0].LockdownID == rowB[0].LockdownID {
				t.Errorf("K's rows both belong to lockdown %d, want one lockdown each", rowA[0].LockdownID)
			}

			liftFirst, other := chatA, chatB
			firstBanned, otherBanned := []gotgbot.User{j, k}, []gotgbot.User{k}
			if order == "lift B first" {
				liftFirst, other = chatB, chatA
				firstBanned, otherBanned = []gotgbot.User{k}, []gotgbot.User{j, k}
			}

			otherBefore := env.writesTo(other.Id)
			env.sendIn(liftFirst, env.admin, "/unlockdown")
			env.cycle()

			if active, err := lockdown.GetActiveFresh(liftFirst.Id); err != nil || active != nil {
				t.Errorf("lockdown of the lifted chat after the lift = %v, %v, want none", active, err)
			}
			for _, user := range firstBanned {
				if got := env.userCallsIn(liftFirst.Id, "unbanChatMember", user.Id); got != 1 {
					t.Errorf("unbanChatMember calls for %d in the lifted chat = %d, want 1", user.Id, got)
				}
			}
			if active, err := lockdown.GetActiveFresh(other.Id); err != nil || active == nil || active.LockedAt == nil {
				t.Errorf("lockdown of the other chat = %v, %v, want it still active", active, err)
			}
			env.wantLockedPermissions(other.Id)
			for _, user := range otherBanned {
				wantRowState(t, other.Id, user.Id, models.JoinerStateBanned)
				if got := env.userCallsIn(other.Id, "unbanChatMember", user.Id); got != 0 {
					t.Errorf("unbanChatMember calls for %d in the other chat = %d, want none", user.Id, got)
				}
			}
			if after := env.writesTo(other.Id); after != otherBefore {
				t.Errorf("the lift changed or posted in the other chat %d time(s), want none", after-otherBefore)
			}

			env.sendIn(other, env.admin, "/unlockdown")
			env.cycle()
			if active, err := lockdown.GetActiveFresh(other.Id); err != nil || active != nil {
				t.Errorf("lockdown of the other chat after its own lift = %v, %v, want none", active, err)
			}
			for _, user := range otherBanned {
				if got := env.userCallsIn(other.Id, "unbanChatMember", user.Id); got != 1 {
					t.Errorf("unbanChatMember calls for %d in the other chat after its lift = %d, want 1", user.Id, got)
				}
			}
			if got := env.fake.chatPermsRaw(other.Id); got != lockdownTestPrePermissions {
				t.Errorf("permissions of the other chat after its lift = %q, want its own snapshot", got)
			}
		})
	}
}

func TestAntiRaidStepsAsideDuringLockdown(t *testing.T) {
	t.Run("raid mode does not ban an admin-added user", func(t *testing.T) {
		env := newLockdownEnv(t)
		LoadAntiRaid(env.dispatcher)
		if enabled, err := antiRaidModule.enableRaid(env.chat.Id, 3600); err != nil || !enabled {
			t.Fatalf("enableRaid() = (%v, %v), want true, nil", enabled, err)
		}
		t.Cleanup(func() { _, _ = antiRaidModule.disableRaid(env.chat.Id) })
		env.lockAsAdmin("raid")

		human := env.newJoiner("Human")
		env.serviceJoin(env.admin, nil, human)

		if row := env.joinerRow(human.Id); row.State != models.JoinerStateExempt {
			t.Errorf("row = %+v, want exempt: a live administrator added the user", row)
		}
		env.cycle()
		if bans := env.bansOf(human.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls for the admin-added user = %d, want none: antiraid steps aside during a lockdown", len(bans))
		}

		// After the lift antiraid works as before.
		env.send(env.admin, "/unlockdown")
		env.cycle()
		member := env.newJoiner("Member")
		env.serviceJoin(member, nil, member)
		if bans := env.bansOf(member.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls for a self-join after the lift = %d, want 1: raid mode bans as before", len(bans))
		}
	})

	t.Run("joins are not counted toward the auto trigger", func(t *testing.T) {
		env := newLockdownEnv(t)
		LoadAntiRaid(env.dispatcher)
		if err := antiraid.SetAutoAntiRaidThreshold(env.chat.Id, 50); err != nil {
			t.Fatalf("set the auto threshold: %v", err)
		}
		joins := func() int64 {
			count, err := cache.GetRedisClient().ZCard(cache.Context, joinsKey(env.chat.Id)).Result()
			if err != nil {
				t.Fatalf("read the antiraid joins set: %v", err)
			}
			return count
		}
		env.lockAsAdmin("raid")

		human := env.newJoiner("Human")
		env.serviceJoin(env.admin, nil, human)
		if got := joins(); got != 0 {
			t.Errorf("antiraid joins counted during the lockdown = %d, want 0", got)
		}

		env.send(env.admin, "/unlockdown")
		env.cycle()
		member := env.newJoiner("Member")
		env.serviceJoin(member, nil, member)
		if got := joins(); got != 1 {
			t.Errorf("antiraid joins counted after the lift = %d, want 1: counting works as before", got)
		}
	})
}
