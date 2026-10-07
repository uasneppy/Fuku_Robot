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

// ReclaimJoin is a compile-only stub; the next commit gives it its behaviour.
func ReclaimJoin(id uint, fromStates []string, notAfter time.Time, rec JoinRecord) (bool, error) {
	return false, nil
}

// SetJoinMsg is a compile-only stub; the next commit gives it its behaviour.
func SetJoinMsg(id uint, msgID int64) error {
	return nil
}

// ListJoinMsgsToDeleteFresh is a compile-only stub; the next commit gives it its behaviour.
func ListJoinMsgsToDeleteFresh(limit int) ([]models.LockdownJoiner, error) {
	return nil, nil
}

// ClearJoinMsg is a compile-only stub; the next commit gives it its behaviour.
func ClearJoinMsg(id uint) error {
	return nil
}
