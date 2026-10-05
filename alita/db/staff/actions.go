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
