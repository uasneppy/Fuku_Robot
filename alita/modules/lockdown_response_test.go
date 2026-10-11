//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// responseEnv is a lockdown env that also deletes the burst rows of its chat when the
// test ends.
func responseEnv(t *testing.T) *lockdownEnv {
	t.Helper()
	env := newLockdownEnv(t)
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ChatLockdownBurst{})
	})
	return env
}

// queuedBurst writes the burst row of userID queued and claims it to acting, the way
// the ban run's loop hands an account to lockdownResponseBanOne, and returns it.
func (e *lockdownEnv) queuedBurst(ld *models.ChatLockdown, userID int64, source string, isBot bool) models.ChatLockdownBurst {
	e.t.Helper()
	row := models.ChatLockdownBurst{
		LockdownID: ld.ID,
		ChatID:     e.chat.Id,
		UserID:     userID,
		FirstName:  "Burst",
		IsBot:      isBot,
		Source:     source,
		State:      models.BurstStateQueued,
	}
	// The alert's tracker snapshot may already have saved this user's burst row (a join
	// in the same instant as locked_at counts as pre-lockdown), so reuse that row and
	// reset it to the queued state the test asks for instead of inserting a duplicate.
	var existing models.ChatLockdownBurst
	err := db.DB.Where("lockdown_id = ? AND user_id = ?", ld.ID, userID).Take(&existing).Error
	switch {
	case err == nil:
		row.ID = existing.ID
		if err := db.DB.Model(&existing).Select("first_name", "is_bot", "source", "state").Updates(&row).Error; err != nil {
			e.t.Fatalf("reset burst row of %d: %v", userID, err)
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := db.DB.Create(&row).Error; err != nil {
			e.t.Fatalf("seed burst row of %d: %v", userID, err)
		}
	default:
		e.t.Fatalf("read burst row of %d: %v", userID, err)
	}
	return e.claimBurst(row.ID)
}

// claimBurst moves a queued burst row to acting and returns it as stored.
func (e *lockdownEnv) claimBurst(id uint) models.ChatLockdownBurst {
	e.t.Helper()
	won, err := lockdown.ClaimBurst(id, models.BurstStateQueued, models.BurstStateActing)
	if err != nil || !won {
		e.t.Fatalf("ClaimBurst(%d) = %v, %v, want true", id, won, err)
	}
	var row models.ChatLockdownBurst
	if err := db.DB.First(&row, id).Error; err != nil {
		e.t.Fatalf("read burst row %d: %v", id, err)
	}
	return row
}

// burstRowOf reads the burst row of userID in the env's chat.
func (e *lockdownEnv) burstRowOf(userID int64) models.ChatLockdownBurst {
	e.t.Helper()
	var row models.ChatLockdownBurst
	if err := db.DB.Where("chat_id = ? AND user_id = ?", e.chat.Id, userID).First(&row).Error; err != nil {
		e.t.Fatalf("read burst row of %d: %v", userID, err)
	}
	return row
}

// responseBan runs lockdownResponseBanOne for a claimed burst row.
func (e *lockdownEnv) responseBan(ld *models.ChatLockdown, row models.ChatLockdownBurst, approved map[int64]bool) bool {
	e.t.Helper()
	return lockdownResponseBanOne(context.Background(), e.bot, *ld, row, approved)
}

// memberCallsOf returns the calls of method addressed to userID in the env's chat.
func (e *lockdownEnv) memberCallsOf(method string, userID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range e.calls(method) {
		if staffParamInt(call.Params, "user_id") == userID {
			matched = append(matched, call)
		}
	}
	return matched
}

// wantEmptyBan fails unless the ban call carries no end date and no revoke_messages.
func wantEmptyBan(t *testing.T, call moduleBotCall) {
	t.Helper()
	for _, key := range []string{"until_date", "revoke_messages"} {
		if value, ok := call.Params[key]; ok {
			t.Errorf("banChatMember carries %s = %v, want the parameter absent", key, value)
		}
	}
}

func TestDecideLockdownResponseBan(t *testing.T) {
	const until int64 = 1_900_000_000
	member := func(status string, at int64) gotgbot.MergedChatMember {
		return gotgbot.MergedChatMember{Status: status, UntilDate: at}
	}

	tests := []struct {
		name     string
		member   gotgbot.MergedChatMember
		banUntil int64
		wantBan  bool
		wantSkip string
	}{
		{"creator", member(gotgbot.ChatMemberStatusCreator, 0), until, false, lockdownBanSkipAdmin},
		{"administrator", member(gotgbot.ChatMemberStatusAdministrator, 0), until, false, lockdownBanSkipAdmin},
		{"kicked on the lockdown's own end date", member(gotgbot.ChatMemberStatusKicked, until), until, true, ""},
		{"kicked 2 s off the end date", member(gotgbot.ChatMemberStatusKicked, until+2), until, true, ""},
		{"kicked with no end date", member(gotgbot.ChatMemberStatusKicked, 0), until, false, lockdownBanSkipBanned},
		{"kicked on another end date", member(gotgbot.ChatMemberStatusKicked, until+3600), until, false, lockdownBanSkipBanned},
		{"kicked, account with no joiner row", member(gotgbot.ChatMemberStatusKicked, until), 0, false, lockdownBanSkipBanned},
		{"left", member(gotgbot.ChatMemberStatusLeft, 0), until, true, ""},
		{"member", member(gotgbot.ChatMemberStatusMember, 0), 0, true, ""},
		{"restricted", member(gotgbot.ChatMemberStatusRestricted, 0), 0, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBan, gotSkip := decideLockdownResponseBan(tt.member, tt.banUntil)
			if gotBan != tt.wantBan || gotSkip != tt.wantSkip {
				t.Errorf("decideLockdownResponseBan = %v, %q, want %v, %q", gotBan, gotSkip, tt.wantBan, tt.wantSkip)
			}
		})
	}
}

func TestLockdownResponseBanOne(t *testing.T) {
	t.Run("pre-lockdown member", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		user := env.newJoiner("Early")
		env.fake.setMember(env.chat.Id, user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		row := env.queuedBurst(ld, user.Id, models.BurstSourcePre, false)

		if !env.responseBan(ld, row, nil) {
			t.Error("lockdownResponseBanOne = false, want true: the row reached a final state")
		}

		if got := len(env.memberCallsOf("getChatMember", user.Id)); got != 1 {
			t.Errorf("getChatMember calls = %d, want 1", got)
		}
		bans := env.bansOf(user.Id)
		if len(bans) != 1 {
			t.Fatalf("banChatMember calls = %d, want 1", len(bans))
		}
		wantEmptyBan(t, bans[0])
		if got := env.burstRowOf(user.Id); got.State != models.BurstStateBanned || got.Reason != "" || got.ClaimedAt != nil {
			t.Errorf("burst row = %+v, want banned with no reason and no claim", got)
		}
		live := env.fake.member(env.chat.Id, user.Id)
		if live == nil || live.Status != gotgbot.ChatMemberStatusKicked || live.UntilDate != 0 {
			t.Errorf("member in the chat = %+v, want kicked with no end date", live)
		}
	})

	t.Run("joiner under the lockdown's ban", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		raider := env.newJoiner("Raider")
		env.join(raider, raider, "https://t.me/+abc")
		env.cycle()
		joiner := env.joinerRow(raider.Id)
		if joiner.State != models.JoinerStateBanned || joiner.BanUntil == 0 {
			t.Fatalf("setup: joiner = %+v, want banned with an end date", joiner)
		}
		row := env.queuedBurst(ld, raider.Id, models.BurstSourceJoiner, false)

		if !env.responseBan(ld, row, nil) {
			t.Error("lockdownResponseBanOne = false, want true")
		}

		bans := env.bansOf(raider.Id)
		if len(bans) != 2 {
			t.Fatalf("banChatMember calls = %d, want 2 (the worker's, then the upgrade)", len(bans))
		}
		if _, ok := bans[0].Params["until_date"]; !ok {
			t.Error("the worker's own ban carries no end date, want its marker")
		}
		wantEmptyBan(t, bans[1])
		if got := env.joinerRow(raider.Id); got.State != models.JoinerStateKept || got.Detail != models.JoinerDetailResponseBan {
			t.Errorf("joiner row = %q / %q, want kept / response_ban", got.State, got.Detail)
		}
		if got := env.burstRowOf(raider.Id); got.State != models.BurstStateBanned {
			t.Errorf("burst row state = %q, want banned", got.State)
		}
		live := env.fake.member(env.chat.Id, raider.Id)
		if live == nil || live.Status != gotgbot.ChatMemberStatusKicked || live.UntilDate != 0 {
			t.Errorf("member in the chat = %+v, want kicked with no end date", live)
		}
	})

	t.Run("banned by someone else", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		raider := env.newJoiner("Raider")
		env.join(raider, raider, "https://t.me/+abc")
		// An admin bans the raider for good before the worker or the run reaches them.
		env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
		row := env.queuedBurst(ld, raider.Id, models.BurstSourceJoiner, false)

		env.responseBan(ld, row, nil)

		if bans := env.bansOf(raider.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want none: the account is already banned", len(bans))
		}
		if got := env.burstRowOf(raider.Id); got.State != models.BurstStateSkipped || got.Reason != lockdownBanSkipBanned {
			t.Errorf("burst row = %q / %q, want skipped / already_banned", got.State, got.Reason)
		}
		if got := env.joinerRow(raider.Id); got.State != models.JoinerStatePending || got.Detail != "" || got.ClaimedAt != nil {
			t.Errorf("joiner row = %q / %q, claim %v, want back to pending as it was", got.State, got.Detail, got.ClaimedAt)
		}
	})

	t.Run("admin", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		staffer := env.newJoiner("Mod")
		env.fake.setMember(env.chat.Id, staffer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
		row := env.queuedBurst(ld, staffer.Id, models.BurstSourcePre, false)

		env.responseBan(ld, row, nil)

		if bans := env.bansOf(staffer.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want none for an administrator", len(bans))
		}
		if got := env.burstRowOf(staffer.Id); got.State != models.BurstStateSkipped || got.Reason != lockdownBanSkipAdmin {
			t.Errorf("burst row = %q / %q, want skipped / admin", got.State, got.Reason)
		}
	})

	t.Run("approved", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		friend := env.newJoiner("Friend")
		env.fake.setMember(env.chat.Id, friend.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		row := env.queuedBurst(ld, friend.Id, models.BurstSourcePre, false)

		env.responseBan(ld, row, map[int64]bool{friend.Id: true})

		if got := len(env.memberCallsOf("getChatMember", friend.Id)) + len(env.bansOf(friend.Id)); got != 0 {
			t.Errorf("Telegram calls for an approved user = %d, want none", got)
		}
		if got := env.burstRowOf(friend.Id); got.State != models.BurstStateSkipped || got.Reason != lockdownBanSkipApproved {
			t.Errorf("burst row = %q / %q, want skipped / approved", got.State, got.Reason)
		}
	})

	t.Run("let in by an admin", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		guest := env.newJoiner("Guest")
		// A live administrator adds the guest directly: the guard records them exempt.
		env.join(guest, env.admin, "")
		before := env.joinerRow(guest.Id)
		if before.State != models.JoinerStateExempt {
			t.Fatalf("setup: joiner state = %q, want exempt", before.State)
		}
		row := env.queuedBurst(ld, guest.Id, models.BurstSourceJoiner, false)
		lookups := len(env.memberCallsOf("getChatMember", guest.Id))

		if !env.responseBan(ld, row, nil) {
			t.Error("lockdownResponseBanOne = false, want true")
		}

		if got := len(env.memberCallsOf("getChatMember", guest.Id)); got != lookups {
			t.Errorf("getChatMember calls for the guest grew by %d, want none", got-lookups)
		}
		if bans := env.bansOf(guest.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want none for someone an admin let in", len(bans))
		}
		if got := env.burstRowOf(guest.Id); got.State != models.BurstStateSkipped || got.Reason != lockdownBanSkipLetIn {
			t.Errorf("burst row = %q / %q, want skipped / let_in", got.State, got.Reason)
		}
		after := env.joinerRow(guest.Id)
		if after.State != models.JoinerStateExempt || after.Detail != before.Detail || after.ClaimedAt != nil {
			t.Errorf("joiner row = %q / %q, claim %v, want still exempt and unchanged", after.State, after.Detail, after.ClaimedAt)
		}
	})

	t.Run("bot account", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		other := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "SpamBot", IsBot: true}
		env.join(other, other, "https://t.me/+abc")
		row := env.queuedBurst(ld, other.Id, models.BurstSourceJoiner, true)

		env.responseBan(ld, row, nil)

		if bans := env.bansOf(other.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls for another bot = %d, want 1: bots are banned like anyone else", len(bans))
		}
		if got := env.burstRowOf(other.Id); got.State != models.BurstStateBanned {
			t.Errorf("burst row state = %q, want banned", got.State)
		}

		self := env.queuedBurst(ld, env.bot.Id, models.BurstSourcePre, true)
		// /lockdown itself looked at the bot's own rights, so count from here.
		lookups := len(env.memberCallsOf("getChatMember", env.bot.Id))
		env.responseBan(ld, self, nil)
		if got := len(env.memberCallsOf("getChatMember", env.bot.Id)) - lookups; got != 0 {
			t.Errorf("getChatMember calls for the bot itself = %d, want none", got)
		}
		if got := len(env.bansOf(env.bot.Id)); got != 0 {
			t.Errorf("banChatMember calls for the bot itself = %d, want none", got)
		}
		if got := env.burstRowOf(env.bot.Id); got.State != models.BurstStateSkipped || got.Reason != lockdownBanSkipSelf {
			t.Errorf("burst row of the bot = %q / %q, want skipped / bot_self", got.State, got.Reason)
		}
	})

	t.Run("declined request", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		asker := env.newJoiner("Asker")
		env.joinRequest(asker, "https://t.me/+abc")
		env.cycle()
		if got := env.joinerRow(asker.Id); got.State != models.JoinerStateDeclined || got.JoinPath != models.JoinPathRequest {
			t.Fatalf("setup: joiner = %q / %q, want a declined request", got.State, got.JoinPath)
		}
		env.fake.setMember(env.chat.Id, asker.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
		row := env.queuedBurst(ld, asker.Id, models.BurstSourceJoiner, false)

		env.responseBan(ld, row, nil)

		if bans := env.bansOf(asker.Id); len(bans) != 1 {
			t.Fatalf("banChatMember calls = %d, want 1: a declined request is in the burst", len(bans))
		}
		if got := env.burstRowOf(asker.Id); got.State != models.BurstStateBanned || got.Reason != lockdownBanReasonRequest {
			t.Errorf("burst row = %q / %q, want banned / request", got.State, got.Reason)
		}
		if got := env.joinerRow(asker.Id); got.State != models.JoinerStateKept || got.Detail != models.JoinerDetailResponseBan {
			t.Errorf("joiner row = %q / %q, want kept / response_ban", got.State, got.Detail)
		}
	})

	t.Run("failure", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		raider := env.newJoiner("Raider")
		env.join(raider, raider, "https://t.me/+abc")
		env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		failure := staffFakeError(400, "Bad Request: <chat> not found")
		env.fake.script("banChatMember", env.chat.Id, failure, failure, failure)
		row := env.queuedBurst(ld, raider.Id, models.BurstSourceJoiner, false)

		for attempt := 1; attempt <= 3; attempt++ {
			final := env.responseBan(ld, row, nil)
			got := env.burstRowOf(raider.Id)
			if got.Attempts != attempt {
				t.Fatalf("attempt %d: attempts = %d, want %d", attempt, got.Attempts, attempt)
			}
			if joiner := env.joinerRow(raider.Id); joiner.State != models.JoinerStatePending || joiner.Detail != "" {
				t.Errorf("attempt %d: joiner row = %q / %q, want back to pending as it was", attempt, joiner.State, joiner.Detail)
			}
			if attempt < 3 {
				if final || got.State != models.BurstStateQueued {
					t.Fatalf("attempt %d: final %v, state %q, want not final and queued", attempt, final, got.State)
				}
				row = env.claimBurst(got.ID)
				continue
			}
			if !final || got.State != models.BurstStateFailed {
				t.Fatalf("attempt 3: final %v, state %q, want final and failed", final, got.State)
			}
			if !strings.Contains(got.Detail, "&lt;chat&gt; not found") || strings.Contains(got.Detail, "<chat>") {
				t.Errorf("detail = %q, want Telegram's text HTML-escaped", got.Detail)
			}
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		raider := env.newJoiner("Raider")
		env.join(raider, raider, "https://t.me/+abc")
		env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.fake.script("banChatMember", env.chat.Id,
			staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1))
		row := env.queuedBurst(ld, raider.Id, models.BurstSourceJoiner, false)

		if env.responseBan(ld, row, nil) {
			t.Error("lockdownResponseBanOne = true, want false: the row goes back")
		}

		got := env.burstRowOf(raider.Id)
		if got.State != models.BurstStateQueued || got.Attempts != 0 || got.ClaimedAt != nil {
			t.Errorf("burst row = %q, %d attempt(s), claim %v, want queued with none counted", got.State, got.Attempts, got.ClaimedAt)
		}
		if joiner := env.joinerRow(raider.Id); joiner.State != models.JoinerStatePending {
			t.Errorf("joiner state = %q, want back to pending", joiner.State)
		}
	})
}

// liftUntilDone sends /unlockdown and runs worker cycles until the lockdown is no
// longer lifting.
func (e *lockdownEnv) liftUntilDone(ld *models.ChatLockdown) {
	e.t.Helper()
	e.send(e.admin, "/unlockdown")
	for range 10 {
		e.cycle()
		row, err := lockdown.GetFresh(ld.ID)
		if err != nil {
			e.t.Fatalf("read lockdown: %v", err)
		}
		if row != nil && row.State == models.LockdownStateLifted {
			return
		}
	}
	e.t.Fatal("the lockdown was not lifted after 10 worker cycles")
}

func TestLockdownBanSurvivesLift(t *testing.T) {
	env := responseEnv(t)
	ld, users := env.lockAndBan("J1", "J2", "J3")
	j1, j2, j3 := users[0], users[1], users[2]

	for _, user := range []gotgbot.User{j1, j2} {
		row := env.queuedBurst(ld, user.Id, models.BurstSourceJoiner, false)
		if !env.responseBan(ld, row, nil) {
			t.Fatalf("lockdownResponseBanOne for %d = false, want true", user.Id)
		}
		if got := env.joinerRow(user.Id); got.State != models.JoinerStateKept || got.Detail != models.JoinerDetailResponseBan {
			t.Fatalf("joiner %d = %q / %q, want kept / response_ban", user.Id, got.State, got.Detail)
		}
	}

	env.liftUntilDone(ld)

	for _, user := range []gotgbot.User{j1, j2} {
		if unbans := env.unbansOf(user.Id); len(unbans) != 0 {
			t.Errorf("unbanChatMember calls for %d = %d, want none: a ban for good survives the lift", user.Id, len(unbans))
		}
		live := env.fake.member(env.chat.Id, user.Id)
		if live == nil || live.Status != gotgbot.ChatMemberStatusKicked || live.UntilDate != 0 {
			t.Errorf("member %d in the chat = %+v, want still kicked with no end date", user.Id, live)
		}
	}
	if unbans := env.unbansOf(j3.Id); len(unbans) != 1 {
		t.Errorf("unbanChatMember calls for the third joiner = %d, want 1: the lockdown's own ban is lifted as before", len(unbans))
	}
	if got := env.messagesWith(staffMarker("lockdown_lift_tally_kept") + " 2"); got != 1 {
		t.Errorf("tally messages naming the 2 bans left alone = %d, want 1", got)
	}
}

func TestLockdownBanRaces(t *testing.T) {
	t.Run("worker holds the row", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		j1 := env.newJoiner("J1")
		env.join(j1, j1, "https://t.me/+abc")
		joiner := env.joinerRow(j1.Id)

		// The worker is mid-ban: its claim is fresh.
		won, err := lockdown.ClaimJoiner(joiner.ID, models.JoinerStatePending, models.JoinerStateActing)
		if err != nil || !won {
			t.Fatalf("setup: ClaimJoiner = %v, %v", won, err)
		}
		row := env.queuedBurst(ld, j1.Id, models.BurstSourceJoiner, false)

		if env.responseBan(ld, row, nil) {
			t.Error("lockdownResponseBanOne = true, want false: the worker holds the row")
		}

		calls := len(env.memberCallsOf("getChatMember", j1.Id)) + len(env.bansOf(j1.Id))
		if calls != 0 {
			t.Errorf("Telegram calls for the account = %d, want none while the worker holds it", calls)
		}
		got := env.burstRowOf(j1.Id)
		if got.State != models.BurstStateQueued || got.Attempts != 0 || got.ClaimedAt != nil {
			t.Errorf("burst row = %q, %d attempt(s), claim %v, want back in the queue with no attempt", got.State, got.Attempts, got.ClaimedAt)
		}
		if held := env.joinerRow(j1.Id); held.State != models.JoinerStateActing {
			t.Errorf("joiner row = %q, want still acting: the response must not touch the worker's claim", held.State)
		}

		// The worker finished: the lockdown's own ban is in place.
		if moved, err := lockdown.MoveJoiner(joiner.ID, models.JoinerStateActing, models.JoinerStateBanned, "", false); err != nil || !moved {
			t.Fatalf("setup: MoveJoiner = %v, %v", moved, err)
		}
		env.fake.setMember(env.chat.Id, j1.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: joiner.BanUntil})

		row = env.claimBurst(got.ID)
		if !env.responseBan(ld, row, nil) {
			t.Error("second lockdownResponseBanOne = false, want true")
		}

		bans := env.bansOf(j1.Id)
		if len(bans) != 1 {
			t.Fatalf("banChatMember calls = %d, want 1", len(bans))
		}
		wantEmptyBan(t, bans[0])
		if after := env.joinerRow(j1.Id); after.State != models.JoinerStateKept || after.Detail != models.JoinerDetailResponseBan {
			t.Errorf("joiner row = %q / %q, want kept / response_ban", after.State, after.Detail)
		}
	})

	t.Run("response ban first", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		j4 := env.newJoiner("J4")
		env.join(j4, j4, "https://t.me/+abc")
		if row := env.joinerRow(j4.Id); row.State != models.JoinerStatePending {
			t.Fatalf("setup: joiner = %q, want pending", row.State)
		}
		env.fake.setMember(env.chat.Id, j4.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		row := env.queuedBurst(ld, j4.Id, models.BurstSourceJoiner, false)

		if !env.responseBan(ld, row, nil) {
			t.Fatal("lockdownResponseBanOne = false, want true")
		}
		env.cycle()

		bans := env.bansOf(j4.Id)
		if len(bans) != 1 {
			t.Fatalf("banChatMember calls after a full cycle = %d, want 1: the worker must not ban them again", len(bans))
		}
		wantEmptyBan(t, bans[0])
		if got := env.joinerRow(j4.Id); got.State != models.JoinerStateKept {
			t.Errorf("joiner state after the cycle = %q, want kept", got.State)
		}
		live := env.fake.member(env.chat.Id, j4.Id)
		if live == nil || live.UntilDate != 0 {
			t.Errorf("member in the chat = %+v, want banned with no end date", live)
		}
	})

	t.Run("lift holds the row", func(t *testing.T) {
		env := responseEnv(t)
		ld, users := env.lockAndBan("J")
		joiner := env.joinerRow(users[0].Id)
		if won, err := lockdown.ClaimJoiner(joiner.ID, models.JoinerStateBanned, models.JoinerStateUnbanning); err != nil || !won {
			t.Fatalf("setup: ClaimJoiner = %v, %v", won, err)
		}
		row := env.queuedBurst(ld, users[0].Id, models.BurstSourceJoiner, false)
		bansBefore := len(env.bansOf(users[0].Id))
		lookups := len(env.memberCallsOf("getChatMember", users[0].Id))

		if env.responseBan(ld, row, nil) {
			t.Error("lockdownResponseBanOne = true, want false: the lift holds the row")
		}

		if got := len(env.bansOf(users[0].Id)) - bansBefore; got != 0 {
			t.Errorf("banChatMember calls = %d, want none", got)
		}
		if got := len(env.memberCallsOf("getChatMember", users[0].Id)) - lookups; got != 0 {
			t.Errorf("getChatMember calls = %d, want none", got)
		}
		if got := env.burstRowOf(users[0].Id); got.State != models.BurstStateQueued || got.Attempts != 0 {
			t.Errorf("burst row = %q with %d attempt(s), want queued with none", got.State, got.Attempts)
		}
	})

	t.Run("turned exempt after queueing", func(t *testing.T) {
		env := responseEnv(t)
		ld := env.lockAsAdmin("raid")
		j5 := env.newJoiner("J5")
		env.join(j5, j5, "https://t.me/+abc")
		joiner := env.joinerRow(j5.Id)
		row := env.queuedBurst(ld, j5.Id, models.BurstSourceJoiner, false)
		// An admin approves the user after the run was queued (the chat_member path).
		if moved, err := lockdown.MoveJoiner(joiner.ID, models.JoinerStatePending, models.JoinerStateExempt, "", false); err != nil || !moved {
			t.Fatalf("setup: MoveJoiner = %v, %v", moved, err)
		}

		env.responseBan(ld, row, nil)

		if got := len(env.memberCallsOf("getChatMember", j5.Id)) + len(env.bansOf(j5.Id)); got != 0 {
			t.Errorf("Telegram calls for the account = %d, want none", got)
		}
		if got := env.burstRowOf(j5.Id); got.State != models.BurstStateSkipped || got.Reason != lockdownBanSkipLetIn {
			t.Errorf("burst row = %q / %q, want skipped / let_in", got.State, got.Reason)
		}
		if got := env.joinerRow(j5.Id); got.State != models.JoinerStateExempt {
			t.Errorf("joiner state = %q, want still exempt", got.State)
		}
	})

	// Characterization of the known limit (research Pitfall 3): a kept row is not
	// reclaimed, so someone an admin unbans by hand and who joins again is not banned
	// again and gets no new joiner row.
	t.Run("unbanned by hand, then rejoins", func(t *testing.T) {
		env := responseEnv(t)
		ld, users := env.lockAndBan("J1")
		j1 := users[0]
		row := env.queuedBurst(ld, j1.Id, models.BurstSourceJoiner, false)
		if !env.responseBan(ld, row, nil) {
			t.Fatal("lockdownResponseBanOne = false, want true")
		}
		kept := env.joinerRow(j1.Id)
		bans := len(env.bansOf(j1.Id))

		env.fake.setMember(env.chat.Id, j1.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
		ageJoiner(t, kept.ID, time.Hour)
		env.join(j1, j1, "https://t.me/+abc")
		env.cycle()

		if rows := env.joinerRows(j1.Id); len(rows) != 1 || rows[0].ID != kept.ID || rows[0].State != models.JoinerStateKept {
			t.Errorf("joiner rows after the rejoin = %+v, want the one kept row, not reclaimed", rows)
		}
		if got := len(env.bansOf(j1.Id)); got != bans {
			t.Errorf("banChatMember calls grew by %d after the rejoin, want none (the known limit)", got-bans)
		}
	})
}

func TestLockdownStatusCountsResponseBans(t *testing.T) {
	env := responseEnv(t)
	ld, users := env.lockAndBan("J1", "J2")

	env.askStatus(env.admin)
	env.wantReplyHas(staffMarker("lockdown_status_removed") + " 2")

	for _, user := range users {
		row := env.queuedBurst(ld, user.Id, models.BurstSourceJoiner, false)
		if !env.responseBan(ld, row, nil) {
			t.Fatalf("lockdownResponseBanOne for %d = false, want true", user.Id)
		}
	}

	env.askStatus(env.admin)
	env.wantReplyHas(staffMarker("lockdown_status_removed") + " 2")
}
