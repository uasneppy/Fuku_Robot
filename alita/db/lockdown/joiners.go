package lockdown

// Joiner rows follow the same rules as the lockdown rows in repository.go: they are
// read fresh from the database on every replica, never cached, and every state change
// is one conditional update, so a row is claimed and moved by exactly one caller no
// matter how many replicas run a worker.

import (
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// JoinRecord is what the join guard stores about one user who joined a locked group.
type JoinRecord struct {
	LockdownID     uint
	ChatID         int64
	UserID         int64
	FirstName      string
	Username       string
	IsBot          bool
	Path           string
	InviteLink     string
	ViaJoinRequest bool
	PerformerID    int64
	State          string
	BanUntil       int64
	JoinMsgID      int64
}

// RecordJoin stores a joiner row. The insert is refused by the unique
// (lockdown_id, user_id) index when the lockdown already has a row for the user, in
// which case nothing is written and the existing row is returned with recorded
// false: the first delivery of a join wins. recorded true means this call wrote the
// row, which is filled in with its ID.
func RecordJoin(rec JoinRecord) (*models.LockdownJoiner, bool, error) {
	stamp := now()
	row := &models.LockdownJoiner{
		LockdownID:     rec.LockdownID,
		ChatID:         rec.ChatID,
		UserID:         rec.UserID,
		FirstName:      rec.FirstName,
		Username:       rec.Username,
		IsBot:          rec.IsBot,
		JoinPath:       rec.Path,
		InviteLink:     rec.InviteLink,
		ViaJoinRequest: rec.ViaJoinRequest,
		PerformerID:    rec.PerformerID,
		State:          rec.State,
		BanUntil:       rec.BanUntil,
		JoinMsgID:      rec.JoinMsgID,
		CreatedAt:      stamp,
		UpdatedAt:      stamp,
	}
	if row.State == "" {
		row.State = models.JoinerStatePending
	}
	result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		log.Errorf("[Lockdown] RecordJoin: %v", result.Error)
		return nil, false, alitaerrors.Wrapf(result.Error, "record joiner %d of lockdown %d", rec.UserID, rec.LockdownID)
	}
	if result.RowsAffected == 1 {
		return row, true, nil
	}

	var existing models.LockdownJoiner
	err := db.DB.Where("lockdown_id = ? AND user_id = ?", rec.LockdownID, rec.UserID).First(&existing).Error
	if err != nil {
		log.Errorf("[Lockdown] RecordJoin read existing: %v", err)
		return nil, false, alitaerrors.Wrapf(err, "read joiner %d of lockdown %d", rec.UserID, rec.LockdownID)
	}
	return &existing, false, nil
}

// ClaimJoiner moves a joiner row from one state to another and stamps claimed_at, in
// one conditional update that matches only while the row is still in the from state.
// It reports whether this call won the claim.
func ClaimJoiner(id uint, from, to string) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.LockdownJoiner{}).
		Where("id = ? AND state = ?", id, from).
		Updates(map[string]any{"state": to, "claimed_at": stamp, "updated_at": stamp})
	if result.Error != nil {
		log.Errorf("[Lockdown] ClaimJoiner: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "claim joiner row %d", id)
	}
	return result.RowsAffected == 1, nil
}

// MoveJoiner moves a joiner row from one state to another in one conditional update
// that matches only while the row is still in the from state, and releases its claim.
// detail replaces the stored detail. countAttempt adds one to the attempt counter. A
// move from acting to banned, which only a ban that took effect makes, starts the
// unban with the counter back at zero, so a ban that needed retries does not eat
// into the lift's tries. It reports whether this call moved the row.
func MoveJoiner(id uint, from, to, detail string, countAttempt bool) (bool, error) {
	updates := map[string]any{
		"state":      to,
		"detail":     detail,
		"claimed_at": nil,
		"updated_at": now(),
	}
	switch {
	case from == models.JoinerStateActing && to == models.JoinerStateBanned:
		updates["attempts"] = 0
	case countAttempt:
		updates["attempts"] = gorm.Expr("attempts + 1")
	}
	result := db.DB.Model(&models.LockdownJoiner{}).
		Where("id = ? AND state = ?", id, from).
		Updates(updates)
	if result.Error != nil {
		log.Errorf("[Lockdown] MoveJoiner: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "move joiner row %d from %s to %s", id, from, to)
	}
	return result.RowsAffected == 1, nil
}

// ListPendingFresh reads up to limit pending joiner rows, oldest ID first, of
// lockdowns that are active and whose lock Telegram confirmed.
func ListPendingFresh(limit int) ([]models.LockdownJoiner, error) {
	var rows []models.LockdownJoiner
	err := db.DB.
		Joins("JOIN chat_lockdowns ON chat_lockdowns.id = chat_lockdown_joiners.lockdown_id").
		Where("chat_lockdown_joiners.state = ? AND chat_lockdowns.state = ? AND chat_lockdowns.locked_at IS NOT NULL",
			models.JoinerStatePending, models.LockdownStateActive).
		Order("chat_lockdown_joiners.id").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListPendingFresh: %v", err)
		return nil, alitaerrors.Wrap(err, "list pending joiners")
	}
	return rows, nil
}

// DeletePendingJoin deletes a joiner row that is still pending, and reports whether
// it did. A row in any other state is never touched.
func DeletePendingJoin(id uint) (bool, error) {
	result := db.DB.
		Where("id = ? AND state = ?", id, models.JoinerStatePending).
		Delete(&models.LockdownJoiner{})
	if result.Error != nil {
		log.Errorf("[Lockdown] DeletePendingJoin: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "delete pending joiner row %d", id)
	}
	return result.RowsAffected == 1, nil
}

// CancelPending moves every pending joiner row of a lockdown to cancelled and
// returns how many it moved. It is what a lift does with joiners that were recorded
// but not yet banned: they are never banned.
func CancelPending(lockdownID uint) (int64, error) {
	result := db.DB.Model(&models.LockdownJoiner{}).
		Where("lockdown_id = ? AND state = ?", lockdownID, models.JoinerStatePending).
		Updates(map[string]any{"state": models.JoinerStateCancelled, "claimed_at": nil, "updated_at": now()})
	if result.Error != nil {
		log.Errorf("[Lockdown] CancelPending: %v", result.Error)
		return 0, alitaerrors.Wrapf(result.Error, "cancel pending joiners of lockdown %d", lockdownID)
	}
	return result.RowsAffected, nil
}

// ListJoinersInState reads up to limit joiner rows of one lockdown in one state,
// oldest ID first, which is the order they were recorded in.
func ListJoinersInState(lockdownID uint, state string, limit int) ([]models.LockdownJoiner, error) {
	var rows []models.LockdownJoiner
	err := db.DB.
		Where("lockdown_id = ? AND state = ?", lockdownID, state).
		Order("id").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListJoinersInState: %v", err)
		return nil, alitaerrors.Wrapf(err, "list %s joiners of lockdown %d", state, lockdownID)
	}
	return rows, nil
}

// ReclaimJoin takes a finished joiner row back for a new join of the same user: one
// conditional update that matches only while the row is in one of fromStates and its
// updated_at is older than notAfter. It rewrites the row from rec (state, path, end
// date, performer, link, request flag, join message, names) and clears the attempts,
// the detail and the claim, so the new join starts clean and a fresh dedupe window
// opens. It reports whether this call won, which is true for exactly one of two
// racing callers.
func ReclaimJoin(id uint, fromStates []string, notAfter time.Time, rec JoinRecord) (bool, error) {
	state := rec.State
	if state == "" {
		state = models.JoinerStatePending
	}
	result := db.DB.Model(&models.LockdownJoiner{}).
		Where("id = ? AND state IN ? AND updated_at < ?", id, fromStates, notAfter.UTC()).
		Updates(map[string]any{
			"state":            state,
			"join_path":        rec.Path,
			"ban_until":        rec.BanUntil,
			"performer_id":     rec.PerformerID,
			"invite_link":      rec.InviteLink,
			"via_join_request": rec.ViaJoinRequest,
			"join_msg_id":      rec.JoinMsgID,
			"first_name":       rec.FirstName,
			"username":         rec.Username,
			"attempts":         0,
			"detail":           "",
			"claimed_at":       nil,
			"updated_at":       now(),
		})
	if result.Error != nil {
		log.Errorf("[Lockdown] ReclaimJoin: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "reclaim joiner row %d", id)
	}
	return result.RowsAffected == 1, nil
}

// SetJoinMsg records the ID of the join service message on a row that has none yet.
// The first message wins. updated_at is left alone: a second delivery of a join does
// not open a new dedupe window.
func SetJoinMsg(id uint, msgID int64) error {
	result := db.DB.Model(&models.LockdownJoiner{}).
		Where("id = ? AND join_msg_id = 0", id).
		UpdateColumn("join_msg_id", msgID)
	if result.Error != nil {
		log.Errorf("[Lockdown] SetJoinMsg: %v", result.Error)
		return alitaerrors.Wrapf(result.Error, "set join message of joiner row %d", id)
	}
	return nil
}

// ListJoinMsgsToDeleteFresh reads up to limit banned joiner rows that still carry a
// join service message to delete, of lockdowns that are active or lifting, oldest ID
// first. The message is deleted only after the ban, so only banned rows are listed.
func ListJoinMsgsToDeleteFresh(limit int) ([]models.LockdownJoiner, error) {
	var rows []models.LockdownJoiner
	err := db.DB.
		Joins("JOIN chat_lockdowns ON chat_lockdowns.id = chat_lockdown_joiners.lockdown_id").
		Where("chat_lockdown_joiners.state = ? AND chat_lockdown_joiners.join_msg_id <> 0 AND chat_lockdowns.state IN ?",
			models.JoinerStateBanned, []string{models.LockdownStateActive, models.LockdownStateLifting}).
		Order("chat_lockdown_joiners.id").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListJoinMsgsToDeleteFresh: %v", err)
		return nil, alitaerrors.Wrap(err, "list join messages to delete")
	}
	return rows, nil
}

// ClearJoinMsg forgets the join service message of a row once the worker handled it.
// updated_at is left alone, for the reason given on SetJoinMsg.
func ClearJoinMsg(id uint) error {
	result := db.DB.Model(&models.LockdownJoiner{}).
		Where("id = ?", id).
		UpdateColumn("join_msg_id", 0)
	if result.Error != nil {
		log.Errorf("[Lockdown] ClearJoinMsg: %v", result.Error)
		return alitaerrors.Wrapf(result.Error, "clear join message of joiner row %d", id)
	}
	return nil
}

// ReleaseStaleClaims gives back every claim a worker made before the cut-off and never
// finished, because the worker stopped (a restart, a crash, a call that never came
// back). The claim is cleared and no attempt is counted, so the work is simply tried
// again by whichever replica gets to it first:
//
//   - an acting row of an active lockdown goes back to pending (the ban or decline is
//     repeated, which is idempotent);
//   - an acting join request of a lifting lockdown is cancelled, because a lift never
//     declines anyone;
//   - any other acting row of a lifting lockdown goes to banned, and the lift's live
//     check of the member decides whether the ban really landed;
//   - an unbanning row goes back to banned, and the same live check makes a repeated
//     unban safe.
//
// The four updates run in one transaction and match disjoint rows. It returns how many
// rows it released.
func ReleaseStaleClaims(before time.Time) (int64, error) {
	cutoff := before.UTC()
	inLockdownState := "lockdown_id IN (SELECT id FROM chat_lockdowns WHERE state = ?)"
	steps := []struct {
		where string
		args  []any
		to    string
	}{
		{
			"state = ? AND claimed_at < ? AND " + inLockdownState,
			[]any{models.JoinerStateActing, cutoff, models.LockdownStateActive},
			models.JoinerStatePending,
		},
		{
			"state = ? AND claimed_at < ? AND join_path = ? AND " + inLockdownState,
			[]any{models.JoinerStateActing, cutoff, models.JoinPathRequest, models.LockdownStateLifting},
			models.JoinerStateCancelled,
		},
		{
			"state = ? AND claimed_at < ? AND " + inLockdownState,
			[]any{models.JoinerStateActing, cutoff, models.LockdownStateLifting},
			models.JoinerStateBanned,
		},
		{
			"state = ? AND claimed_at < ?",
			[]any{models.JoinerStateUnbanning, cutoff},
			models.JoinerStateBanned,
		},
	}

	var released int64
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		released = 0
		for _, step := range steps {
			result := tx.Model(&models.LockdownJoiner{}).
				Where(step.where, step.args...).
				Updates(map[string]any{"state": step.to, "claimed_at": nil, "updated_at": now()})
			if result.Error != nil {
				return result.Error
			}
			released += result.RowsAffected
		}
		return nil
	})
	if err != nil {
		log.Errorf("[Lockdown] ReleaseStaleClaims: %v", err)
		return 0, alitaerrors.Wrap(err, "release stale joiner claims")
	}
	return released, nil
}

// joinerBanTolerance is how many seconds a live ban's end date may differ from a
// row's ban_until and still match it. It is the repository's copy of the module's
// lockdownBanUntilTolerance, which cannot be imported from here.
const joinerBanTolerance int64 = 2

// HasJoinerBanFresh reports whether the chat has a joiner row for the user whose
// ban_until is within joinerBanTolerance seconds of until, read straight from the
// database. It is how a "user was banned" update is recognised as the lockdown's own
// ban: only the lockdown ends a ban on such a date. An until of 0 (a permanent ban)
// never matches and costs no query.
func HasJoinerBanFresh(chatID, userID, until int64) (bool, error) {
	if until == 0 {
		return false, nil
	}
	var count int64
	err := db.DB.Model(&models.LockdownJoiner{}).
		Where("chat_id = ? AND user_id = ? AND ban_until BETWEEN ? AND ?",
			chatID, userID, until-joinerBanTolerance, until+joinerBanTolerance).
		Count(&count).Error
	if err != nil {
		log.Errorf("[Lockdown] HasJoinerBanFresh: %v", err)
		return false, alitaerrors.Wrapf(err, "look up a lockdown ban of user %d in chat %d", userID, chatID)
	}
	return count > 0, nil
}
