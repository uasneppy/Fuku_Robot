//go:build testtools

package modules

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqSchedEnv is the member requirements environment with the deletion clock under the test's
// control: deletionNow reads one settable instant, and every message the fake sends or edits is
// dated by it (or by a fixed offset from it), so no queue row is in 1970.
type mreqSchedEnv struct {
	*memberReqEnv
	clock    *atomic.Int64
	sentBack atomic.Int64
}

// newMreqSchedEnv builds the environment, fixes the deletion clock at a whole second and
// removes the chat's rule and queue rows when the test ends.
func newMreqSchedEnv(t *testing.T) *mreqSchedEnv {
	t.Helper()
	env := &mreqSchedEnv{memberReqEnv: newMemberReqEnv(t), clock: &atomic.Int64{}}
	env.clock.Store(time.Now().UTC().Truncate(time.Second).Unix())
	mreqSwap(t, &deletionNow, func() time.Time { return time.Unix(env.clock.Load(), 0).UTC() })
	// A message is dated that many seconds before the clock (0 means the clock itself).
	env.fake.sendDate = func() int64 { return env.clock.Load() - env.sentBack.Load() }
	deletionCleanup(t, env.chat.Id)
	return env
}

// now is the deletion clock.
func (e *mreqSchedEnv) now() time.Time { return time.Unix(e.clock.Load(), 0).UTC() }

// advance moves the deletion clock forward.
func (e *mreqSchedEnv) advance(d time.Duration) { e.clock.Add(int64(d / time.Second)) }

// setRule stores the delay of a bot kind for the environment's chat.
func (e *mreqSchedEnv) setRule(kind string, delay int) {
	e.t.Helper()
	setBotKindRule(e.t, e.chat.Id, kind, delay)
}

// setBotKindRule stores the delay of a bot kind for a chat through the repository the page uses.
func setBotKindRule(t *testing.T, chatID int64, kind string, delay int) {
	t.Helper()
	if err := deletion.SetRule(t.Context(), chatID, kind, delay); err != nil {
		t.Fatalf("SetRule(%d, %s, %d) error = %v", chatID, kind, delay, err)
	}
}

// queueRows returns the queue rows of the environment's chat by message ID.
func (e *mreqSchedEnv) queueRows() map[int64]models.ScheduledDeletion {
	e.t.Helper()
	var rows []models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Find(&rows).Error; err != nil {
		e.t.Fatalf("read the queue of chat %d: %v", e.chat.Id, err)
	}
	byID := make(map[int64]models.ScheduledDeletion, len(rows))
	for _, row := range rows {
		byID[row.MessageID] = row
	}
	return byID
}

// wantNoRows fails when the chat's queue holds any row.
func (e *mreqSchedEnv) wantNoRows() {
	e.t.Helper()
	if rows := e.queueRows(); len(rows) != 0 {
		e.t.Fatalf("queue rows = %+v, want none", rows)
	}
}

// wantRowFor fails unless the chat's queue holds exactly one row, for messageID, of kind, due
// at due.
func (e *mreqSchedEnv) wantRowFor(messageID int64, kind string, due time.Time) models.ScheduledDeletion {
	e.t.Helper()
	rows := e.queueRows()
	if len(rows) != 1 {
		e.t.Fatalf("queue rows = %+v, want exactly one (message %d)", rows, messageID)
	}
	row, ok := rows[messageID]
	if !ok {
		e.t.Fatalf("queue rows = %+v, want one for message %d", rows, messageID)
	}
	if row.Kind != kind || !row.DeleteAt.Equal(due) {
		e.t.Fatalf("queue row = kind %s delete_at %s, want kind %s delete_at %s", row.Kind, row.DeleteAt, kind, due)
	}
	return row
}

// lastNoticeID is the message ID of the last message the bot sent to the group.
func (e *mreqSchedEnv) lastNoticeID() int64 {
	e.t.Helper()
	notices := e.mreqNotices()
	if len(notices) == 0 {
		e.t.Fatal("the bot sent nothing to the group")
	}
	return notices[len(notices)-1].MessageID
}

func TestMemberReqNoticeScheduled(t *testing.T) {
	punishments := []struct {
		name   string
		action int
		// delay is how long after its send the notice is due: the timer for a notice with no
		// button, the 24 hour cap for a mute notice, which carries the button.
		delay time.Duration
	}{
		{name: "warn", action: models.RequirementActionWarn, delay: 600 * time.Second},
		{name: "kick", action: models.RequirementActionKick, delay: 600 * time.Second},
		{name: "ban", action: models.RequirementActionBan, delay: 600 * time.Second},
		{name: "mute", action: models.RequirementActionMute, delay: 24 * time.Hour},
	}
	for _, tc := range punishments {
		t.Run(tc.name+" notice is queued by the automatic-punishments timer", func(t *testing.T) {
			env := newMreqSchedEnv(t)
			env.setRule(deletion.KindBotAuto, 600)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: tc.action})
			member := env.mreqMember("Alex", "", "alex")

			if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome == memberReqNone {
				t.Fatalf("enforcement = %+v, want a punishment", got)
			}

			env.wantRowFor(env.lastNoticeID(), deletion.KindBotAuto, env.now().Add(tc.delay))
		})

		t.Run(tc.name+" notice writes no row with the kind off", func(t *testing.T) {
			env := newMreqSchedEnv(t)
			env.setRule(deletion.KindBotManual, 600)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: tc.action})
			member := env.mreqMember("Alex", "", "alex")

			if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome == memberReqNone {
				t.Fatalf("enforcement = %+v, want a punishment", got)
			}

			if len(env.mreqNotices()) != 1 {
				t.Fatalf("notices = %d, want the one notice", len(env.mreqNotices()))
			}
			env.wantNoRows()
		})
	}

	t.Run("the cap's pause notice is queued the same way", func(t *testing.T) {
		mreqSwap(t, &memberReqCapPerMinute, 1)
		mreqFixedClock(t)
		env := newMreqSchedEnv(t)
		env.setRule(deletion.KindBotAuto, 600)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick})

		first := env.mreqMember("Alex", "", "alex")
		if got := env.mreqEnforce(env.mreqCase(first)); got.Outcome != memberReqRemoved {
			t.Fatalf("first member = %+v, want Kicked", got)
		}
		second := env.mreqMember("Alex", "", "alex")
		if got := env.mreqEnforce(env.mreqCase(second)); got.Outcome != memberReqNone {
			t.Fatalf("second member = %+v, want None (the cap refuses)", got)
		}

		if n := mreqPausedNotices(env.memberReqEnv); n != 1 {
			t.Fatalf("pause notices = %d, want one", n)
		}
		rows := env.queueRows()
		if len(rows) != 2 {
			t.Fatalf("queue rows = %+v, want one for the kick notice and one for the pause notice", rows)
		}
		for _, notice := range env.mreqNotices() {
			row, ok := rows[notice.MessageID]
			if !ok || row.Kind != deletion.KindBotAuto || !row.DeleteAt.Equal(env.now().Add(600*time.Second)) {
				t.Errorf("notice %d row = %+v (%v), want a bot:punish_auto row due in 600 s", notice.MessageID, row, ok)
			}
		}
	})

	t.Run("the pause notice writes no row with the kind off", func(t *testing.T) {
		mreqSwap(t, &memberReqCapPerMinute, 1)
		mreqFixedClock(t)
		env := newMreqSchedEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick})
		for i := 0; i < 2; i++ {
			env.mreqEnforce(env.mreqCase(env.mreqMember("Alex", "", "alex")))
		}
		if n := mreqPausedNotices(env.memberReqEnv); n != 1 {
			t.Fatalf("pause notices = %d, want one", n)
		}
		env.wantNoRows()
	})
}

func TestMemberReqMuteNoticeAtCap(t *testing.T) {
	for _, timer := range []int{60, 600, 86400} {
		t.Run("timer of "+time.Duration(timer*int(time.Second)).String(), func(t *testing.T) {
			env := newMreqSchedEnv(t)
			env.setRule(deletion.KindBotAuto, timer)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
			member := env.mreqMember("Alex", "", "alex")

			if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
				t.Fatalf("enforcement = %+v, want Muted", got)
			}

			env.wantRowFor(env.lastNoticeID(), deletion.KindBotAuto, env.now().Add(24*time.Hour))
		})
	}

	t.Run("a stored timer of zero is a rule too, so the cap applies", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		env.setRule(deletion.KindBotAuto, 0)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("enforcement = %+v, want Muted", got)
		}

		env.wantRowFor(env.lastNoticeID(), deletion.KindBotAuto, env.now().Add(24*time.Hour))
	})

	t.Run("the kind off writes no row", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("enforcement = %+v, want Muted", got)
		}

		env.wantNoRows()
	})

	t.Run("a send that failed writes no row", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		env.setRule(deletion.KindBotAuto, 600)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		// The notice is tried twice (the retry every notice gets), and both fail.
		env.fake.script("sendMessage", env.chat.Id, errors.New("send failed"), errors.New("send failed"))

		env.mreqEnforce(env.mreqCase(member))

		if n := len(env.mreqNotices()); n != 0 {
			t.Fatalf("notices = %d, want none (both sends failed)", n)
		}
		env.wantNoRows()
	})

	t.Run("a notice that needed a retry is scheduled from the date of the message that went out", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		env.setRule(deletion.KindBotAuto, 600)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		member := env.mreqMember("Alex", "", "alex")
		env.fake.script("sendMessage", env.chat.Id, errors.New("send failed"))
		// The message that finally went out is dated 30 s before the clock.
		env.sentBack.Store(30)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("enforcement = %+v, want Muted", got)
		}

		row := env.wantRowFor(env.lastNoticeID(), deletion.KindBotAuto, env.now().Add(-30*time.Second).Add(24*time.Hour))
		if !row.SentAt.Equal(env.now().Add(-30 * time.Second)) {
			t.Errorf("sent_at = %s, want the date of the message that went out", row.SentAt)
		}
	})
}

// mreqSchedMuted mutes a member through the real watcher with the automatic-punishments timer
// stored (or left off when timer is negative) and returns the member and the mute notice. The
// notice is queued at the 24 hour cap when the kind is on.
func mreqSchedMuted(t *testing.T, env *mreqSchedEnv, timer int) (gotgbot.User, staffSentMessage) {
	t.Helper()
	if timer >= 0 {
		env.setRule(deletion.KindBotAuto, timer)
	}
	member, _, notice := mreqMutedMember(t, env.memberReqEnv)
	if timer >= 0 {
		env.wantRowFor(notice.MessageID, deletion.KindBotAuto, env.now().Add(24*time.Hour))
	} else {
		env.wantNoRows()
	}
	return member, notice
}

func TestMemberReqFixedRescheduleMovesUp(t *testing.T) {
	const tap = 20 * time.Minute

	t.Run("the tap moves the 24 hour row up to the timer counted from the tap", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		member, notice := mreqSchedMuted(t, env, 600)
		member = env.mreqFix(member)
		tapped := env.now().Add(tap)
		env.advance(tap)

		env.mreqPressFixed(member, notice)

		if edits := env.mreqNoticeEdits(notice.MessageID); len(edits) != 1 {
			t.Fatalf("edits of the notice = %d, want the one confirmation", len(edits))
		}
		// Counted from the notice's date the row would be due 10 minutes after the notice, 10 minutes
		// before the tap, and the confirmation would vanish at once.
		row := env.wantRowFor(notice.MessageID, deletion.KindBotAuto, tapped.Add(600*time.Second))
		if !row.DeleteAt.After(tapped) {
			t.Errorf("delete_at = %s, want after the tap %s", row.DeleteAt, tapped)
		}
	})

	t.Run("a timer longer than the cap changes nothing", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		member, notice := mreqSchedMuted(t, env, 86400)
		member = env.mreqFix(member)
		env.advance(tap)

		env.mreqPressFixed(member, notice)

		env.wantRowFor(notice.MessageID, deletion.KindBotAuto, env.now().Add(-tap).Add(24*time.Hour))
	})

	t.Run("the kind off schedules nothing", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		member, notice := mreqSchedMuted(t, env, -1)
		member = env.mreqFix(member)

		env.mreqPressFixed(member, notice)

		if edits := env.mreqNoticeEdits(notice.MessageID); len(edits) != 1 {
			t.Fatalf("edits of the notice = %d, want the one confirmation", len(edits))
		}
		env.wantNoRows()
	})

	// WR-03: switching the kind off drops the already queued 24 hour row with the rule (the notice
	// then simply stays, which is what Off means), and a tap after that reschedules nothing.
	t.Run("the kind turned off after the notice drops the 24 hour row and the tap writes none", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		member, notice := mreqSchedMuted(t, env, 600)
		member = env.mreqFix(member)
		env.wantRowFor(notice.MessageID, deletion.KindBotAuto, env.now().Add(24*time.Hour))
		if err := deletion.RemoveRule(t.Context(), env.chat.Id, deletion.KindBotAuto); err != nil {
			t.Fatalf("RemoveRule error = %v", err)
		}
		env.wantNoRows()
		env.advance(tap)

		env.mreqPressFixed(member, notice)

		env.wantNoRows()
	})

	t.Run("a failed edit schedules nothing and the 24 hour row stays", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		member, notice := mreqSchedMuted(t, env, 600)
		member = env.mreqFix(member)
		env.fake.script("editMessageText", env.chat.Id, staffFakeError(502, "Bad Gateway"))
		env.advance(tap)

		env.mreqPressFixed(member, notice)

		env.wantRowFor(notice.MessageID, deletion.KindBotAuto, env.now().Add(-tap).Add(24*time.Hour))
	})

	t.Run("a refused tap schedules nothing", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		_, notice := mreqSchedMuted(t, env, 600)
		other := env.mreqMember("Sam", "Jones", "sam")
		env.advance(tap)

		env.mreqPressFixed(other, notice)

		env.wantRowFor(notice.MessageID, deletion.KindBotAuto, env.now().Add(-tap).Add(24*time.Hour))
	})

	t.Run("a tap that finds the member still broken schedules nothing", func(t *testing.T) {
		env := newMreqSchedEnv(t)
		member, notice := mreqSchedMuted(t, env, 600)
		env.advance(tap)

		env.mreqPressFixed(member, notice)

		env.wantRowFor(notice.MessageID, deletion.KindBotAuto, env.now().Add(-tap).Add(24*time.Hour))
	})
}
