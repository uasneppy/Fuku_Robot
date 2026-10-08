//go:build testtools

package modules

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// lockdownWriteCalls counts the Telegram calls that change the group or one of its
// members, in any chat.
func (e *lockdownEnv) lockdownWriteCalls() int {
	total := 0
	for _, method := range []string{"setChatPermissions", "banChatMember", "unbanChatMember"} {
		total += len(e.fake.callsFor(method))
	}
	return total
}

// lockdownUpdatedAts returns updated_at of every lockdown row of the chat by row ID.
func (e *lockdownEnv) lockdownUpdatedAts() map[uint]time.Time {
	e.t.Helper()
	var rows []models.ChatLockdown
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Find(&rows).Error; err != nil {
		e.t.Fatalf("read lockdown rows: %v", err)
	}
	stamps := make(map[uint]time.Time, len(rows))
	for _, row := range rows {
		stamps[row.ID] = row.UpdatedAt
	}
	return stamps
}

// askStatus sends /lockdownstatus as from and proves the command wrote nothing: no
// setChatPermissions, banChatMember or unbanChatMember call, and an unchanged
// updated_at on every lockdown row of the chat.
func (e *lockdownEnv) askStatus(from gotgbot.User) {
	e.t.Helper()
	writesBefore := e.lockdownWriteCalls()
	stampsBefore := e.lockdownUpdatedAts()

	e.send(from, "/lockdownstatus")

	if got := e.lockdownWriteCalls(); got != writesBefore {
		e.t.Errorf("/lockdownstatus made %d Telegram write call(s), want none", got-writesBefore)
	}
	stampsAfter := e.lockdownUpdatedAts()
	if len(stampsAfter) != len(stampsBefore) {
		e.t.Errorf("lockdown rows of the chat = %d after /lockdownstatus, were %d", len(stampsAfter), len(stampsBefore))
	}
	for id, before := range stampsBefore {
		if after, ok := stampsAfter[id]; !ok || !after.Equal(before) {
			e.t.Errorf("lockdown %d updated_at changed from %v to %v: /lockdownstatus must write nothing", id, before, after)
		}
	}
}

// wantReplyLacks fails when the last reply contains any of the unwanted texts.
func (e *lockdownEnv) wantReplyLacks(unwanted ...string) {
	e.t.Helper()
	reply := e.lastReply()
	for _, text := range unwanted {
		if strings.Contains(reply, text) {
			e.t.Errorf("last reply %q contains %q, want it absent", reply, text)
		}
	}
}

// lockAsAdmin runs /lockdown with a reason and returns the active row.
func (e *lockdownEnv) lockAsAdmin(reason string) *models.ChatLockdown {
	e.t.Helper()
	e.send(e.admin, "/lockdown "+reason)
	row, err := lockdown.GetActiveFresh(e.chat.Id)
	if err != nil || row == nil || row.LockedAt == nil {
		e.t.Fatalf("setup: GetActiveFresh = %v, %v, want a confirmed lockdown", row, err)
	}
	return row
}

func TestLockdownStatusCommand(t *testing.T) {
	t.Run("never locked", func(t *testing.T) {
		env := newLockdownEnv(t)

		env.askStatus(env.admin)

		env.wantReplyHas(staffMarker("lockdown_not_active"))
	})

	t.Run("locked", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("spam wave")
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9001, models.JoinerStateBanned)
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9002, models.JoinerStateBanned)

		env.askStatus(env.admin)

		env.wantReplyHas(
			staffMarker("lockdown_status_active"),
			lockdownTime(*row.LockedAt),
			"Ad&lt;b&gt;min",
			staffMarker("lockdown_reason_line"),
			"spam wave",
			staffMarker("lockdown_status_removed")+" 2",
		)
		env.wantReplyLacks(
			staffMarker("lockdown_status_ban_failed"),
			staffMarker("lockdown_status_declined"),
			staffMarker("lockdown_status_manual_change"),
			staffMarker("lockdown_status_unconfirmed"),
		)
	})

	t.Run("no reason", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("")

		env.askStatus(env.admin)

		env.wantReplyHas(staffMarker("lockdown_status_active"))
		env.wantReplyLacks(staffMarker("lockdown_reason_line"))
	})

	t.Run("failed bans and declined requests are listed", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9101, models.JoinerStateBanned)
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9102, models.JoinerStateBanFailed)
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9103, models.JoinerStateDeclined)

		env.askStatus(env.admin)

		env.wantReplyHas(
			staffMarker("lockdown_status_removed")+" 1",
			staffMarker("lockdown_status_ban_failed")+" 1",
			staffMarker("lockdown_status_declined")+" 1",
		)
	})

	t.Run("joiners of an older lockdown are not counted", func(t *testing.T) {
		env := newLockdownEnv(t)
		first := env.lockAsAdmin("first")
		seedLockdownJoiner(t, first.ID, env.chat.Id, 9201, models.JoinerStateBanned)
		env.send(env.admin, "/unlockdown")
		second := env.lockAsAdmin("second")
		seedLockdownJoiner(t, second.ID, env.chat.Id, 9202, models.JoinerStateBanned)
		seedLockdownJoiner(t, second.ID, env.chat.Id, 9203, models.JoinerStateBanned)

		env.askStatus(env.admin)

		env.wantReplyHas(staffMarker("lockdown_status_removed")+" 2", "second")
	})

	t.Run("reopened by hand", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")

		env.askStatus(env.admin)
		env.wantReplyLacks(staffMarker("lockdown_status_manual_change"))

		env.fake.setChatPermsRaw(env.chat.Id, lockdownTestPrePermissions)
		env.askStatus(env.admin)
		env.wantReplyHas(staffMarker("lockdown_status_active"), staffMarker("lockdown_status_manual_change"))
	})

	t.Run("permissions unreadable", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		env.askStatus(env.admin)

		env.wantReplyHas(
			staffMarker("lockdown_status_active"),
			staffMarker("lockdown_status_permissions_unknown"),
		)
		env.wantReplyLacks(staffMarker("lockdown_status_manual_change"))
	})

	t.Run("unconfirmed", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := &models.ChatLockdown{
			ChatID:            env.chat.Id,
			TriggerKind:       models.LockdownTriggerManual,
			StartedBy:         env.admin.Id,
			StartedByName:     "Ad<b>min",
			PrePermissions:    lockdownTestPrePermissions,
			LockedPermissions: lockdownLockedPermissions,
		}
		if started, err := lockdown.Start(row); err != nil || !started {
			t.Fatalf("setup: Start = %v, %v", started, err)
		}

		env.askStatus(env.admin)

		env.wantReplyHas(
			staffMarker("lockdown_status_active"),
			lockdownTime(row.CreatedAt),
			staffMarker("lockdown_status_unconfirmed"),
		)
	})

	t.Run("lifting", func(t *testing.T) {
		env := newLockdownEnv(t)
		row := env.lockAsAdmin("raid")
		if began, err := lockdown.BeginLift(row.ID, env.admin.Id, "Ad<b>min", false); err != nil || !began {
			t.Fatalf("setup: BeginLift = %v, %v", began, err)
		}
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9301, models.JoinerStateUnbanned)
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9302, models.JoinerStateBanned)
		seedLockdownJoiner(t, row.ID, env.chat.Id, 9303, models.JoinerStateKept)

		env.askStatus(env.admin)

		env.wantReplyHas("Ad&lt;b&gt;min")
		if reply := env.lastReply(); !regexp.MustCompile(`@@lockdown-status-lifting@@ .* 2 3`).MatchString(reply) {
			t.Errorf("last reply %q does not show the lifting marker with 2 of 3 done", reply)
		}
		env.wantReplyLacks(staffMarker("lockdown_status_active"))
	})

	t.Run("a lifted lockdown is not locked", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		env.send(env.admin, "/unlockdown")

		env.askStatus(env.admin)

		env.wantReplyHas(staffMarker("lockdown_not_active"))
	})

	t.Run("plain member", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		member := env.newOutsider()
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.askStatus(member)

		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		env.wantReplyLacks(staffMarker("lockdown_status_active"), "spam", "raid")
	})

	t.Run("administrator without the restrict right", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})

		env.askStatus(helper)

		env.wantReplyHas(staffMarker("lockdown_status_active"), "raid")
	})

	t.Run("cache says admin, live says member", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		impostor := env.newOutsider()
		env.fake.setCreator(env.chat.Id, impostor.Id)
		env.fake.setMember(env.chat.Id, impostor.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.askStatus(impostor)

		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		if calls := env.calls("getChatAdministrators"); len(calls) != 0 {
			t.Errorf("getChatAdministrators calls = %d, want 0: the admin list is never authority", len(calls))
		}
	})
}
