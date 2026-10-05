package staff

// The staff action audit tables (staff_actions, staff_action_groups) are authority
// and accountability reads: they are always read fresh from the database and never
// cached, so no cache key is read here and none needs invalidation. Writes use
// db.DB without a request context, so the cancelled context of a run that a
// shutdown stopped never loses a record write.

import (
	"errors"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

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
