//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// A joiner who is banned on purpose between the guard recording them and the worker
// reaching them must stay banned: the worker's ban would replace the end date of the
// existing ban with the lockdown marker, and the lift would then remove it (D-05).
func TestLockdownWorkerKeepsDeliberateBanPlacedBeforeIt(t *testing.T) {
	cases := []struct {
		name  string
		until func(row models.LockdownJoiner) int64
	}{
		{"a permanent ban", func(models.LockdownJoiner) int64 { return 0 }},
		{"a timed ban of another length", func(row models.LockdownJoiner) int64 { return row.BanUntil + 3600 }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			env.lockAsAdmin("raid")
			raider := env.newJoiner("Raider")
			other := env.newJoiner("Other")
			env.join(raider, raider, "https://t.me/+abc")
			env.join(other, other, "https://t.me/+abc")
			row := env.joinerRow(raider.Id)
			if row.State != models.JoinerStatePending {
				t.Fatalf("setup: joiner is %q, want pending", row.State)
			}

			// An admin (or a staff fan-out) bans the raider before the worker runs.
			env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{
				Status:    gotgbot.ChatMemberStatusKicked,
				UntilDate: tt.until(row),
			})
			env.cycle()

			if bans := env.bansOf(raider.Id); len(bans) != 0 {
				t.Fatalf("banChatMember calls = %v, want none: the user was already banned", bans)
			}
			if got := env.joinerRow(raider.Id); got.State != models.JoinerStateKept {
				t.Errorf("joiner state = %q, want kept", got.State)
			}

			env.send(env.admin, "/unlockdown")
			env.cycle()

			if unbans := env.unbansOf(raider.Id); len(unbans) != 0 {
				t.Errorf("unbanChatMember calls = %v, want none: the lift never lifts a deliberate ban", unbans)
			}
			member := env.fake.member(env.chat.Id, raider.Id)
			if member == nil || member.Status != gotgbot.ChatMemberStatusKicked || member.UntilDate != tt.until(row) {
				t.Errorf("raider in the chat = %+v, want still banned on the date it was banned with", member)
			}
			if got := env.joinerRow(other.Id); got.State != models.JoinerStateUnbanned {
				t.Errorf("the other joiner's state = %q, want unbanned: only the deliberate ban is kept", got.State)
			}
			if got := env.messagesWith(staffMarker("lockdown_lift_tally_kept") + " 1"); got != 1 {
				t.Errorf("tally messages naming the one kept ban = %d, want 1", got)
			}
		})
	}
}

// A joiner who is already kicked on the row's own end date is the lockdown's ban whose
// answer was lost (a timeout after the call took effect): it is recorded as banned and
// no second ban is sent, so the lift still removes it.
func TestLockdownWorkerRecognisesItsOwnBanAlreadyInPlace(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	row := env.joinerRow(raider.Id)
	env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{
		Status:    gotgbot.ChatMemberStatusKicked,
		UntilDate: row.BanUntil,
	})

	env.cycle()

	if bans := env.bansOf(raider.Id); len(bans) != 0 {
		t.Errorf("banChatMember calls = %v, want none: the lockdown's ban was already in place", bans)
	}
	if got := env.joinerRow(raider.Id); got.State != models.JoinerStateBanned {
		t.Errorf("joiner state = %q, want banned", got.State)
	}

	env.send(env.admin, "/unlockdown")
	env.cycle()
	if got := env.joinerRow(raider.Id); got.State != models.JoinerStateUnbanned {
		t.Errorf("joiner state after the lift = %q, want unbanned", got.State)
	}
}

// A live status the worker cannot read never turns into a ban: the row goes back with
// one attempt counted and is banned on the next cycle.
func TestLockdownWorkerDoesNotBanWhenStatusUnreadable(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))

	env.cycle()

	if bans := env.bansOf(raider.Id); len(bans) != 0 {
		t.Fatalf("banChatMember calls = %v, want none while the status could not be read", bans)
	}
	got := env.joinerRow(raider.Id)
	if got.State != models.JoinerStatePending || got.Attempts != 1 {
		t.Fatalf("joiner = %q with %d attempt(s), want pending with 1", got.State, got.Attempts)
	}

	env.cycle()
	if got := env.joinerRow(raider.Id); got.State != models.JoinerStateBanned {
		t.Errorf("joiner state after the retry = %q, want banned", got.State)
	}
}
