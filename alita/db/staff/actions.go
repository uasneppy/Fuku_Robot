package staff

// The staff action audit tables (staff_actions, staff_action_groups) are authority
// and accountability reads: they are always read fresh from the database and never
// cached, so no cache key is read here and none needs invalidation. Writes use
// db.DB without a request context, so the cancelled context of a run that a
// shutdown stopped never loses a record write.

import (
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// ActionPrior is the target's state in one group before a staff action, as one
// live getChatMember read reported it. Permissions is the JSON of the target's
// permission set, "" unless the target was restricted.
type ActionPrior struct {
	Status      string
	IsMember    bool
	Until       int64
	Permissions string
}

// ActionGroupResult is how one linked group of a staff action ended: its outcome,
// the staffReason code and Telegram's error text (already HTML-escaped).
type ActionGroupResult struct {
	GroupChatID int64
	Outcome     string
	Reason      string
	Detail      string
}

// CreateAction stores a confirmed staff action and one row per linked group in a
// single transaction. The group rows get the new action's ID. It fills action.ID
// and every group's ID, so the caller can address the rows afterwards.
func CreateAction(action *models.StaffAction, groups []models.StaffActionGroup) error {
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(action).Error; err != nil {
			return err
		}
		for i := range groups {
			groups[i].ActionID = action.ID
		}
		if len(groups) == 0 {
			return nil
		}
		return tx.Create(&groups).Error
	})
	if err != nil {
		log.Errorf("[Staff] CreateAction: %v", err)
		return alitaerrors.Wrapf(err, "create staff action in chat %d", action.StaffChatID)
	}
	return nil
}

// SavePrior writes the target's state before the action into the group's row. It
// goes through a map, so a zero value is never skipped, and it fails with an error
// wrapping gorm.ErrRecordNotFound when no row matched: a prior state that was not
// stored must never be mistaken for a stored one.
func SavePrior(actionID uint, groupChatID int64, prior ActionPrior) error {
	result := db.DB.Model(&models.StaffActionGroup{}).
		Where("action_id = ? AND group_chat_id = ?", actionID, groupChatID).
		Updates(map[string]any{
			"prior_status":      prior.Status,
			"prior_is_member":   prior.IsMember,
			"prior_until":       prior.Until,
			"prior_permissions": prior.Permissions,
			"updated_at":        time.Now(),
		})
	if result.Error != nil {
		log.Errorf("[Staff] SavePrior: %v", result.Error)
		return alitaerrors.Wrapf(result.Error, "save prior state of action %d in group %d", actionID, groupChatID)
	}
	if result.RowsAffected != 1 {
		err := alitaerrors.Wrapf(gorm.ErrRecordNotFound, "save prior state of action %d in group %d", actionID, groupChatID)
		log.Errorf("[Staff] SavePrior: %v", err)
		return err
	}
	return nil
}

// writeGroupResult stores one group's outcome inside tx. applied_at is set when
// the outcome is done and is still NULL, so a repeated write keeps the first time.
func writeGroupResult(tx *gorm.DB, actionID uint, res ActionGroupResult, now time.Time) error {
	result := tx.Model(&models.StaffActionGroup{}).
		Where("action_id = ? AND group_chat_id = ?", actionID, res.GroupChatID).
		Updates(map[string]any{
			"outcome":    res.Outcome,
			"reason":     res.Reason,
			"detail":     res.Detail,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	if res.Outcome == models.StaffActionOutcomeDone {
		return tx.Model(&models.StaffActionGroup{}).
			Where("action_id = ? AND group_chat_id = ? AND applied_at IS NULL", actionID, res.GroupChatID).
			Update("applied_at", now).Error
	}
	return nil
}

// SaveGroupResult stores one group's result as soon as the group finishes. It
// also bumps the parent's updated_at, the heartbeat a crash leaves behind, in the
// same transaction.
func SaveGroupResult(actionID uint, res ActionGroupResult) error {
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := writeGroupResult(tx, actionID, res, now); err != nil {
			return err
		}
		return tx.Model(&models.StaffAction{}).Where("id = ?", actionID).Update("updated_at", now).Error
	})
	if err != nil {
		log.Errorf("[Staff] SaveGroupResult: %v", err)
		return alitaerrors.Wrapf(err, "save result of action %d in group %d", actionID, res.GroupChatID)
	}
	return nil
}

// FinalizeAction is the authoritative end of a run: in one transaction it writes
// every group's result and sets finished_at, only while it is still NULL, so a
// second call keeps the first finish time.
func FinalizeAction(actionID uint, results []ActionGroupResult) error {
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, res := range results {
			if err := writeGroupResult(tx, actionID, res, now); err != nil {
				return err
			}
		}
		if err := tx.Model(&models.StaffAction{}).
			Where("id = ? AND finished_at IS NULL", actionID).
			Update("finished_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&models.StaffAction{}).Where("id = ?", actionID).Update("updated_at", now).Error
	})
	if err != nil {
		log.Errorf("[Staff] FinalizeAction: %v", err)
		return alitaerrors.Wrapf(err, "finalize staff action %d", actionID)
	}
	return nil
}

// GetActionFresh reads one staff action by its row ID straight from the database.
// It returns (nil, nil) when there is no such row.
func GetActionFresh(id uint) (*models.StaffAction, error) {
	var row models.StaffAction
	err := db.DB.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Staff] GetActionFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get staff action %d", id)
	}
	return &row, nil
}

// ActionTally counts a staff action's group rows by outcome.
type ActionTally struct {
	Done, Skipped, Failed, Pending int
}

// ListActionsFresh lists one Staff Group's staff actions, newest first, straight
// from the database. The order is the record ID descending, so two actions created
// in the same second keep a stable order. Only rows of staffChatID are returned.
func ListActionsFresh(staffChatID int64, offset, limit int) ([]models.StaffAction, error) {
	var rows []models.StaffAction
	err := db.DB.Where("staff_chat_id = ?", staffChatID).
		Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error
	if err != nil {
		log.Errorf("[Staff] ListActionsFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "list staff actions of chat %d", staffChatID)
	}
	return rows, nil
}

// TallyActionGroups counts the group rows of each given action by outcome in one
// query. An action with no group rows is absent from the map, and an empty ID list
// returns an empty map without a query.
func TallyActionGroups(actionIDs []uint) (map[uint]ActionTally, error) {
	tallies := make(map[uint]ActionTally, len(actionIDs))
	if len(actionIDs) == 0 {
		return tallies, nil
	}
	var counts []struct {
		ActionID uint
		Outcome  string
		N        int
	}
	err := db.DB.Model(&models.StaffActionGroup{}).
		Select("action_id, outcome, COUNT(*) AS n").
		Where("action_id IN ?", actionIDs).
		Group("action_id, outcome").
		Scan(&counts).Error
	if err != nil {
		log.Errorf("[Staff] TallyActionGroups: %v", err)
		return nil, alitaerrors.Wrapf(err, "tally groups of %d staff actions", len(actionIDs))
	}
	for _, count := range counts {
		tally := tallies[count.ActionID]
		switch count.Outcome {
		case models.StaffActionOutcomeDone:
			tally.Done += count.N
		case models.StaffActionOutcomeSkipped:
			tally.Skipped += count.N
		case models.StaffActionOutcomeFailed:
			tally.Failed += count.N
		default:
			tally.Pending += count.N
		}
		tallies[count.ActionID] = tally
	}
	return tallies, nil
}

// ListActionGroupsFresh lists the per-group rows of one staff action in the order
// the run visited the groups (seq), straight from the database.
func ListActionGroupsFresh(actionID uint) ([]models.StaffActionGroup, error) {
	var rows []models.StaffActionGroup
	if err := db.DB.Where("action_id = ?", actionID).Order("seq ASC").Find(&rows).Error; err != nil {
		log.Errorf("[Staff] ListActionGroupsFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "list staff action %d groups", actionID)
	}
	return rows, nil
}

// SetSummaryMessage points the record at the message that now holds the action's
// final summary. The run calls it when the final edit fell back to a new message,
// so a later "Undone" line lands on the message staff actually see. The chat and
// the message ID are always written together: a message ID only means something in
// the chat it was sent to.
func SetSummaryMessage(actionID uint, chatID, msgID int64) error {
	result := db.DB.Model(&models.StaffAction{}).Where("id = ?", actionID).
		Updates(map[string]any{
			"summary_chat_id": chatID,
			"summary_msg_id":  msgID,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		log.Errorf("[Staff] SetSummaryMessage: %v", result.Error)
		return alitaerrors.Wrapf(result.Error, "set summary message of staff action %d", actionID)
	}
	if result.RowsAffected != 1 {
		err := alitaerrors.Wrapf(gorm.ErrRecordNotFound, "set summary message of staff action %d", actionID)
		log.Errorf("[Staff] SetSummaryMessage: %v", err)
		return err
	}
	return nil
}

// ClaimUndo is the one-undo-per-action guarantee (D-09). It records who undoes the
// action and when with a single conditional update that matches only while no undo
// was claimed, and reports whether this call won: exactly one caller ever sees
// true for a record, on any replica. It outlives the Redis confirm card, which
// expires after an hour, so an old card can never start a second undo.
func ClaimUndo(actionID uint, by int64, byName string) (claimed bool, err error) {
	now := time.Now()
	result := db.DB.Model(&models.StaffAction{}).
		Where("id = ? AND undo_started_at IS NULL", actionID).
		Updates(map[string]any{
			"undo_by":         by,
			"undo_by_name":    byName,
			"undo_started_at": now,
			"updated_at":      now,
		})
	if result.Error != nil {
		log.Errorf("[Staff] ClaimUndo: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "claim undo of staff action %d", actionID)
	}
	return result.RowsAffected == 1, nil
}

// writeUndoResult stores one group's undo outcome inside tx. It fails with
// gorm.ErrRecordNotFound when no row matched.
func writeUndoResult(tx *gorm.DB, actionID uint, res ActionGroupResult, now time.Time) error {
	result := tx.Model(&models.StaffActionGroup{}).
		Where("action_id = ? AND group_chat_id = ?", actionID, res.GroupChatID).
		Updates(map[string]any{
			"undo_outcome": res.Outcome,
			"undo_reason":  res.Reason,
			"undo_detail":  res.Detail,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SaveUndoResult stores one group's undo result as soon as the group finishes, and
// bumps the parent's updated_at in the same transaction.
func SaveUndoResult(actionID uint, res ActionGroupResult) error {
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := writeUndoResult(tx, actionID, res, now); err != nil {
			return err
		}
		return tx.Model(&models.StaffAction{}).Where("id = ?", actionID).Update("updated_at", now).Error
	})
	if err != nil {
		log.Errorf("[Staff] SaveUndoResult: %v", err)
		return alitaerrors.Wrapf(err, "save undo result of action %d in group %d", actionID, res.GroupChatID)
	}
	return nil
}

// FinalizeUndo is the authoritative end of an undo: in one transaction it writes
// every given group's undo result, marks each group the original did not apply to
// as skipped with skip_not_applied (its undo outcome is still empty), and sets
// undo_finished_at, only while it is still NULL, so a second call keeps the first
// finish time.
func FinalizeUndo(actionID uint, results []ActionGroupResult) error {
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, res := range results {
			if err := writeUndoResult(tx, actionID, res, now); err != nil {
				return err
			}
		}
		if err := tx.Model(&models.StaffActionGroup{}).
			Where("action_id = ? AND outcome <> ? AND undo_outcome = ''", actionID, models.StaffActionOutcomeDone).
			Updates(map[string]any{
				"undo_outcome": models.StaffActionOutcomeSkipped,
				"undo_reason":  "skip_not_applied",
				"updated_at":   now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.StaffAction{}).
			Where("id = ? AND undo_finished_at IS NULL", actionID).
			Update("undo_finished_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&models.StaffAction{}).Where("id = ?", actionID).Update("updated_at", now).Error
	})
	if err != nil {
		log.Errorf("[Staff] FinalizeUndo: %v", err)
		return alitaerrors.Wrapf(err, "finalize undo of staff action %d", actionID)
	}
	return nil
}
