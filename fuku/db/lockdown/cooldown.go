package lockdown

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// RecentlyLiftedFresh reports whether any lockdown of the chat, in any state, started its
// lift after since. It is read straight from the database, never cached, so every replica
// agrees: automatic raid detection uses it to leave a group alone for a while after an
// admin lifted its lockdown. It is one indexed query on (chat_id, lift_started_at). A row
// that was never lifted has no lift_started_at and never matches.
func RecentlyLiftedFresh(chatID int64, since time.Time) (bool, error) {
	var lifted int64
	err := db.DB.Model(&models.ChatLockdown{}).
		Where("chat_id = ? AND lift_started_at > ?", chatID, since.UTC()).
		Limit(1).
		Count(&lifted).Error
	if err != nil {
		log.Errorf("[Lockdown] RecentlyLiftedFresh: %v", err)
		return false, fukuerrors.Wrapf(err, "check recent lift of chat %d", chatID)
	}
	return lifted > 0, nil
}
