package staff

import (
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// RekeyChat moves every Staff Group row that names oldChatID to newChatID. It
// updates staff_groups.chat_id, staff_group_links.staff_chat_id,
// staff_group_links.group_chat_id, staff_actions.staff_chat_id,
// staff_forward_blocks.staff_chat_id and staff_forward_block_settings.staff_chat_id
// in one transaction, so a chat that is migrated from a basic group to a supergroup
// (or otherwise gets a new ID) keeps its Staff Group status, its links, its action
// history and its forward blocklist.
//
// staff_actions.staff_chat_id follows the Staff Group so its history moves with it.
// staff_actions.summary_chat_id and staff_action_groups.group_chat_id are never
// re-keyed: a message ID only means something in the chat it was sent to, and a
// restriction lives in the group it was made in.
//
// It is idempotent: it returns changed=false with a nil error when oldChatID
// equals newChatID, when either is 0, or when no row matched (for example the
// second of two migrate service messages, or a chat that is not a Staff Group
// at all). Both IDs are removed from the cache after the commit, the forward block
// rules key included.
//
// A UNIQUE violation means newChatID is already registered; the error is
// logged and returned and nothing is changed.
func RekeyChat(oldChatID, newChatID int64) (changed bool, err error) {
	if oldChatID == newChatID || oldChatID == 0 || newChatID == 0 {
		return false, nil
	}

	var affected int64
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		updates := []struct {
			model  any
			column string
		}{
			{&models.StaffGroup{}, "chat_id"},
			{&models.StaffGroupLink{}, "staff_chat_id"},
			{&models.StaffGroupLink{}, "group_chat_id"},
			{&models.StaffAction{}, "staff_chat_id"},
		}
		for _, u := range updates {
			result := tx.Model(u.model).
				Where(u.column+" = ?", oldChatID).
				Updates(map[string]any{u.column: newChatID, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			affected += result.RowsAffected
		}
		n, err := rekeyForwardBlocksTx(tx, oldChatID, newChatID)
		affected += n
		return err
	})
	if err != nil {
		log.Errorf("[Staff] RekeyChat %d -> %d: %v", oldChatID, newChatID, err)
		return false, fukuerrors.Wrapf(err, "rekey staff chat %d to %d", oldChatID, newChatID)
	}

	changed = affected > 0
	if changed {
		invalidateStaffKeys(oldChatID, newChatID)
		invalidateForwardBlockKeys(oldChatID, newChatID)
		log.Infof("[Staff] RekeyChat: %d -> %d", oldChatID, newChatID)
	}
	return changed, nil
}

// rekeyForwardBlocksTx moves both forward block tables' staff_chat_id from oldChatID to newChatID
// inside tx and returns the rows touched. Neither table has an updated_at that moves with the key
// (the settings row's updated_at records the last action change), so only the key is written.
func rekeyForwardBlocksTx(tx *gorm.DB, oldChatID, newChatID int64) (int64, error) {
	var affected int64
	for _, model := range []any{&models.StaffForwardBlock{}, &models.StaffForwardBlockSettings{}} {
		result := tx.Model(model).Where("staff_chat_id = ?", oldChatID).Update("staff_chat_id", newChatID)
		if result.Error != nil {
			return affected, result.Error
		}
		affected += result.RowsAffected
	}
	return affected, nil
}
