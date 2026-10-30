package staff

import (
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// RekeyChat moves every Staff Group row that names oldChatID to newChatID. It
// updates staff_groups.chat_id, staff_group_links.staff_chat_id,
// staff_group_links.group_chat_id and staff_actions.staff_chat_id in one
// transaction, so a chat that is migrated from a basic group to a supergroup (or
// otherwise gets a new ID) keeps its Staff Group status, its links and its action
// history.
//
// staff_actions.staff_chat_id follows the Staff Group so its history moves with it.
// staff_actions.summary_chat_id and staff_action_groups.group_chat_id are never
// re-keyed: a message ID only means something in the chat it was sent to, and a
// restriction lives in the group it was made in.
//
// It is idempotent: it returns changed=false with a nil error when oldChatID
// equals newChatID, when either is 0, or when no row matched (for example the
// second of two migrate service messages, or a chat that is not a Staff Group
// at all). Both IDs are removed from the cache after the commit.
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
		return nil
	})
	if err != nil {
		log.Errorf("[Staff] RekeyChat %d -> %d: %v", oldChatID, newChatID, err)
		return false, alitaerrors.Wrapf(err, "rekey staff chat %d to %d", oldChatID, newChatID)
	}

	changed = affected > 0
	if changed {
		invalidateStaffKeys(oldChatID, newChatID)
		log.Infof("[Staff] RekeyChat: %d -> %d", oldChatID, newChatID)
	}
	return changed, nil
}
