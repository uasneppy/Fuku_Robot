package lockdown

// The burst of a lockdown (chat_lockdown_burst plus the joiner rows the lockdown
// recorded) is operational state shared by every replica. It is always read fresh from
// the database and never cached, so no cache key is read here and none needs
// invalidation. The pre-lockdown rows are written once, by BeginAlert, in the
// transaction that decides the alert; later changes to a row are conditional updates.

import (
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// BurstAccount is one account of a lockdown's burst, as an alert lists it. Pre is true
// for an account that joined before the lock (from the saved tracker snapshot) and
// false for one the lockdown itself recorded.
type BurstAccount struct {
	UserID     int64
	FirstName  string
	Username   string
	InviteLink string
	IsBot      bool
	Pre        bool
}

// BurstCounts are the numbers behind a lockdown's trigger: Before is how many accounts
// joined in the minutes before the lock, Since how many other accounts the lockdown
// recorded after it.
type BurstCounts struct {
	Before int64
	Since  int64
}

// ClaimBurst moves a burst row from one state to another and stamps claimed_at, in one
// conditional update that matches only while the row is still in the from state. It
// reports whether this call won the claim, so one worker handles an account exactly once.
func ClaimBurst(id uint, from, to string) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdownBurst{}).
		Where("id = ? AND state = ?", id, from).
		Updates(map[string]any{"state": to, "claimed_at": stamp, "updated_at": stamp})
	if result.Error != nil {
		log.Errorf("[Lockdown] ClaimBurst: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "claim burst row %d", id)
	}
	return result.RowsAffected == 1, nil
}

// MoveBurst moves a burst row from one state to another in one conditional update that
// matches only while the row is in the from state, and releases its claim. reason and
// detail replace the stored ones. countAttempt adds one to the attempt counter. It
// reports whether this call moved the row.
func MoveBurst(id uint, from, to, reason, detail string, countAttempt bool) (bool, error) {
	updates := map[string]any{
		"state":      to,
		"reason":     reason,
		"detail":     detail,
		"claimed_at": nil,
		"updated_at": now(),
	}
	if countAttempt {
		updates["attempts"] = gorm.Expr("attempts + 1")
	}
	result := db.DB.Model(&models.ChatLockdownBurst{}).
		Where("id = ? AND state = ?", id, from).
		Updates(updates)
	if result.Error != nil {
		log.Errorf("[Lockdown] MoveBurst: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "move burst row %d from %s to %s", id, from, to)
	}
	return result.RowsAffected == 1, nil
}

// exemptJoinerUsers is the subquery of the users whose joiner row of the lockdown is
// exempt: a live creator or administrator (or an anonymous admin) let them in, so a ban
// run never acts on them.
func exemptJoinerUsers(tx *gorm.DB, lockdownID uint) *gorm.DB {
	return tx.Model(&models.LockdownJoiner{}).
		Select("user_id").
		Where("lockdown_id = ? AND state = ?", lockdownID, models.JoinerStateExempt)
}

// CountBanCandidatesFresh counts, straight from the database, the accounts a ban run
// started now would act on: the unhandled burst rows (empty state) plus the lockdown's
// joiner rows that have no burst row yet, leaving out the excluded users and every user
// whose joiner row of the lockdown is exempt. Live admins are only known at run time and
// are skipped then, so the number is the accounts the run will look at, not the ones it
// will ban. It is the N on the "Ban N recent joiners" button and card.
func CountBanCandidatesFresh(lockdownID uint, exclude []int64) (int64, error) {
	fail := func(err error) (int64, error) {
		log.Errorf("[Lockdown] CountBanCandidatesFresh: %v", err)
		return 0, alitaerrors.Wrapf(err, "count ban candidates of lockdown %d", lockdownID)
	}

	unhandled := db.DB.Model(&models.ChatLockdownBurst{}).
		Where("lockdown_id = ? AND state = ?", lockdownID, models.BurstStateNone).
		Where("user_id NOT IN (?)", exemptJoinerUsers(db.DB, lockdownID))
	if len(exclude) > 0 {
		unhandled = unhandled.Where("user_id NOT IN ?", exclude)
	}
	var fromBurst int64
	if err := unhandled.Count(&fromBurst).Error; err != nil {
		return fail(err)
	}

	known := db.DB.Model(&models.ChatLockdownBurst{}).
		Select("user_id").
		Where("lockdown_id = ?", lockdownID)
	fresh := db.DB.Model(&models.LockdownJoiner{}).
		Where("lockdown_id = ? AND state <> ?", lockdownID, models.JoinerStateExempt).
		Where("user_id NOT IN (?)", known)
	if len(exclude) > 0 {
		fresh = fresh.Where("user_id NOT IN ?", exclude)
	}
	var fromJoiners int64
	if err := fresh.Count(&fromJoiners).Error; err != nil {
		return fail(err)
	}
	return fromBurst + fromJoiners, nil
}

// StartBanRun starts a ban run in one transaction and reports whether it did. The run
// row is inserted first: the uk_chat_lockdown_ban_runs_running partial unique index
// refuses it while the lockdown has a running run, so exactly one caller ever gets
// started true, on any replica, and a refused call writes nothing (started false). Then:
//
//   - the lockdown's joiner rows are copied into the burst (source joiner, joined_at the
//     joiner's creation time), once per user;
//   - the unhandled rows (empty state) of users whose joiner row is exempt are marked
//     skipped with reason let_in for this run, because an admin let them in and no
//     Telegram call is ever made for them (D-22);
//   - every other unhandled row of a user not in exclude is queued for this run, and
//     queued is how many. Rows an earlier run handled are never touched, so a later run
//     takes only what no earlier run did. exclude carries the approved users and the
//     bot, which are never queued.
//
// A run with nothing to queue is moved to done in the same transaction. On started true
// the run's ID and timestamps are filled in.
func StartBanRun(run *models.ChatLockdownBanRun, exclude []int64) (started bool, queued int64, err error) {
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		stamp := now()
		run.State = models.BanRunStateRunning
		run.CreatedAt = stamp
		run.UpdatedAt = stamp
		inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(run)
		if inserted.Error != nil {
			if isUniqueViolation(inserted.Error) {
				return nil
			}
			return inserted.Error
		}
		if inserted.RowsAffected != 1 {
			return nil
		}

		copied := tx.Exec(`INSERT INTO chat_lockdown_burst
			(lockdown_id, chat_id, user_id, first_name, username, is_bot, invite_link, source, joined_at, state, reason, detail, attempts, created_at, updated_at)
			SELECT lockdown_id, chat_id, user_id, first_name, username, is_bot, invite_link, ?, created_at, ?, '', '', 0, ?, ?
			FROM chat_lockdown_joiners WHERE lockdown_id = ?
			ON CONFLICT (lockdown_id, user_id) DO NOTHING`,
			models.BurstSourceJoiner, models.BurstStateNone, stamp, stamp, run.LockdownID)
		if copied.Error != nil {
			return copied.Error
		}

		letIn := tx.Model(&models.ChatLockdownBurst{}).
			Where("lockdown_id = ? AND state = ?", run.LockdownID, models.BurstStateNone).
			Where("user_id IN (?)", exemptJoinerUsers(tx, run.LockdownID)).
			Updates(map[string]any{
				"state":      models.BurstStateSkipped,
				"reason":     models.BurstReasonLetIn,
				"run_id":     run.ID,
				"updated_at": stamp,
			})
		if letIn.Error != nil {
			return letIn.Error
		}

		toQueue := tx.Model(&models.ChatLockdownBurst{}).
			Where("lockdown_id = ? AND state = ?", run.LockdownID, models.BurstStateNone)
		if len(exclude) > 0 {
			toQueue = toQueue.Where("user_id NOT IN ?", exclude)
		}
		marked := toQueue.Updates(map[string]any{
			"state":      models.BurstStateQueued,
			"run_id":     run.ID,
			"updated_at": stamp,
		})
		if marked.Error != nil {
			return marked.Error
		}
		queued = marked.RowsAffected
		started = true

		if queued == 0 {
			finished := tx.Model(&models.ChatLockdownBanRun{}).
				Where("id = ?", run.ID).
				Updates(map[string]any{"state": models.BanRunStateDone, "finished_at": stamp, "updated_at": stamp})
			if finished.Error != nil {
				return finished.Error
			}
			run.State = models.BanRunStateDone
			run.FinishedAt = &stamp
		}
		return nil
	})
	if err != nil {
		log.Errorf("[Lockdown] StartBanRun: %v", err)
		return false, 0, alitaerrors.Wrapf(err, "start a ban run of lockdown %d", run.LockdownID)
	}
	return started, queued, nil
}

// ListQueuedBurstFresh reads up to limit queued burst rows of one run, in the order they
// were queued.
func ListQueuedBurstFresh(runID uint, limit int) ([]models.ChatLockdownBurst, error) {
	var rows []models.ChatLockdownBurst
	err := db.DB.
		Where("run_id = ? AND state = ?", runID, models.BurstStateQueued).
		Order("id").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListQueuedBurstFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "list queued burst rows of run %d", runID)
	}
	return rows, nil
}

// ListRunningBanRunsFresh reads every running ban run, oldest first, on any replica.
func ListRunningBanRunsFresh() ([]models.ChatLockdownBanRun, error) {
	var runs []models.ChatLockdownBanRun
	if err := db.DB.Where("state = ?", models.BanRunStateRunning).Order("id").Find(&runs).Error; err != nil {
		log.Errorf("[Lockdown] ListRunningBanRunsFresh: %v", err)
		return nil, alitaerrors.Wrap(err, "list running ban runs")
	}
	return runs, nil
}

// SetBanRunCard stores where the run's progress card was posted.
func SetBanRunCard(runID uint, chatID, msgID int64) error {
	result := db.DB.Model(&models.ChatLockdownBanRun{}).
		Where("id = ?", runID).
		Updates(map[string]any{"card_chat_id": chatID, "card_msg_id": msgID, "updated_at": now()})
	if result.Error != nil {
		log.Errorf("[Lockdown] SetBanRunCard: %v", result.Error)
		return alitaerrors.Wrapf(result.Error, "set the card of ban run %d", runID)
	}
	return nil
}

// FinishBanRun moves a running ban run to state (done or stopped) with its finish time,
// in one conditional update that matches only while the run is running, so a run
// finishes exactly once. Done also requires that no burst row of the run is queued or
// acting. It reports whether this call finished the run.
func FinishBanRun(runID uint, state string) (bool, error) {
	stamp := now()
	tx := db.DB.Model(&models.ChatLockdownBanRun{}).
		Where("id = ? AND state = ?", runID, models.BanRunStateRunning)
	if state == models.BanRunStateDone {
		open := db.DB.Model(&models.ChatLockdownBurst{}).
			Select("1").
			Where("run_id = ? AND state IN ?", runID, []string{models.BurstStateQueued, models.BurstStateActing})
		tx = tx.Where("NOT EXISTS (?)", open)
	}
	result := tx.Updates(map[string]any{"state": state, "finished_at": stamp, "updated_at": stamp})
	if result.Error != nil {
		log.Errorf("[Lockdown] FinishBanRun: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "finish ban run %d", runID)
	}
	return result.RowsAffected == 1, nil
}

// ListBurstOfRunFresh reads up to limit burst rows of one run that are in state, in the
// order they were queued. The ban run's report uses it to name the accounts that failed.
func ListBurstOfRunFresh(runID uint, state string, limit int) ([]models.ChatLockdownBurst, error) {
	var rows []models.ChatLockdownBurst
	err := db.DB.
		Where("run_id = ? AND state = ?", runID, state).
		Order("id").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListBurstOfRunFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "list %s burst rows of run %d", state, runID)
	}
	return rows, nil
}

// UnqueueBanRun gives back every account of the run that no worker has taken yet: its
// queued rows go back to unhandled (empty state, no run, no claim), so a later run can
// take them. Rows in a final state (banned, skipped, failed) stay with the run, and so
// does an acting row, which belongs to a call in flight on some replica and is moved by
// that call or by the stale-claim release. It reports how many rows it gave back.
func UnqueueBanRun(runID uint) (int64, error) {
	result := db.DB.Model(&models.ChatLockdownBurst{}).
		Where("run_id = ? AND state = ?", runID, models.BurstStateQueued).
		Updates(map[string]any{
			"state":      models.BurstStateNone,
			"run_id":     nil,
			"claimed_at": nil,
			"updated_at": now(),
		})
	if result.Error != nil {
		log.Errorf("[Lockdown] UnqueueBanRun: %v", result.Error)
		return 0, alitaerrors.Wrapf(result.Error, "unqueue ban run %d", runID)
	}
	return result.RowsAffected, nil
}

// TallyBanRun counts the burst rows of one run by state and then by reason.
func TallyBanRun(runID uint) (map[string]map[string]int64, error) {
	var rows []struct {
		State  string
		Reason string
		Count  int64
	}
	err := db.DB.Model(&models.ChatLockdownBurst{}).
		Select("state, reason, COUNT(*) AS count").
		Where("run_id = ?", runID).
		Group("state, reason").
		Scan(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] TallyBanRun: %v", err)
		return nil, alitaerrors.Wrapf(err, "tally ban run %d", runID)
	}
	tally := make(map[string]map[string]int64)
	for _, row := range rows {
		if tally[row.State] == nil {
			tally[row.State] = make(map[string]int64)
		}
		tally[row.State][row.Reason] = row.Count
	}
	return tally, nil
}

// ReleaseStaleBurstClaims gives back every burst claim made before the cut-off and never
// finished, because the worker stopped: the row goes back to queued with the claim
// cleared and no attempt counted, so the account is tried again by whichever replica
// gets to it first. The joiner row the claim may have held is released by
// ReleaseStaleClaims. It returns how many rows it released.
func ReleaseStaleBurstClaims(before time.Time) (int64, error) {
	result := db.DB.Model(&models.ChatLockdownBurst{}).
		Where("state = ? AND claimed_at < ?", models.BurstStateActing, before.UTC()).
		Updates(map[string]any{"state": models.BurstStateQueued, "claimed_at": nil, "updated_at": now()})
	if result.Error != nil {
		log.Errorf("[Lockdown] ReleaseStaleBurstClaims: %v", result.Error)
		return 0, alitaerrors.Wrap(result.Error, "release stale burst claims")
	}
	return result.RowsAffected, nil
}

// BurstSampleFresh reads the burst of a lockdown straight from the database: the first
// limit accounts and the counts of the whole burst. The pre-lockdown accounts come
// first, by join time and then by ID, followed by the joiner rows of the lockdown whose
// user has no pre-lockdown row (so someone who joined, left and joined again is listed
// once, as a pre-lockdown account), in the order they were recorded. Before is the
// number of pre-lockdown rows and Since the number of those joiner rows, whatever their
// state and whatever limit is. A limit of zero or less returns the counts alone.
func BurstSampleFresh(lockdownID uint, limit int) ([]BurstAccount, BurstCounts, error) {
	var counts BurstCounts
	fail := func(err error) ([]BurstAccount, BurstCounts, error) {
		log.Errorf("[Lockdown] BurstSampleFresh: %v", err)
		return nil, BurstCounts{}, alitaerrors.Wrapf(err, "read the burst of lockdown %d", lockdownID)
	}

	preUsers := db.DB.Model(&models.ChatLockdownBurst{}).
		Select("user_id").
		Where("lockdown_id = ? AND source = ?", lockdownID, models.BurstSourcePre)

	if err := db.DB.Model(&models.ChatLockdownBurst{}).
		Where("lockdown_id = ? AND source = ?", lockdownID, models.BurstSourcePre).
		Count(&counts.Before).Error; err != nil {
		return fail(err)
	}
	if err := db.DB.Model(&models.LockdownJoiner{}).
		Where("lockdown_id = ? AND user_id NOT IN (?)", lockdownID, preUsers).
		Count(&counts.Since).Error; err != nil {
		return fail(err)
	}
	if limit <= 0 {
		return nil, counts, nil
	}

	var pre []models.ChatLockdownBurst
	if err := db.DB.
		Where("lockdown_id = ? AND source = ?", lockdownID, models.BurstSourcePre).
		Order("joined_at, id").
		Limit(limit).
		Find(&pre).Error; err != nil {
		return fail(err)
	}
	accounts := make([]BurstAccount, 0, len(pre))
	for _, row := range pre {
		accounts = append(accounts, BurstAccount{
			UserID:     row.UserID,
			FirstName:  row.FirstName,
			Username:   row.Username,
			InviteLink: row.InviteLink,
			IsBot:      row.IsBot,
			Pre:        true,
		})
	}

	if rest := limit - len(accounts); rest > 0 && counts.Since > 0 {
		var joiners []models.LockdownJoiner
		if err := db.DB.
			Where("lockdown_id = ? AND user_id NOT IN (?)", lockdownID, preUsers).
			Order("id").
			Limit(rest).
			Find(&joiners).Error; err != nil {
			return fail(err)
		}
		for _, row := range joiners {
			accounts = append(accounts, BurstAccount{
				UserID:     row.UserID,
				FirstName:  row.FirstName,
				Username:   row.Username,
				InviteLink: row.InviteLink,
				IsBot:      row.IsBot,
			})
		}
	}
	return accounts, counts, nil
}
