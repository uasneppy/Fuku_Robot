package greetings

import (
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// ClaimGreeting records that the bot is welcoming a member in a chat for the first time
// and reports whether this call is the one that recorded it. It is one INSERT ... ON
// CONFLICT DO NOTHING on greeted_members, whose primary key is (chat_id, user_id), so of
// any number of concurrent claims, on one replica or several, exactly one gets
// claimed true; the others get false and no error.
//
// The greet-once record is read and written fresh in the database. It is never cached,
// so no cache key is deleted, and it is never kept in the operational key store. Rows are
// kept for good. Only the welcome path claims, and only a definite refusal gives the
// claim back through ReleaseGreeting.
func ClaimGreeting(chatID, userID int64) (claimed bool, err error) {
	row := &models.GreetedMember{ChatID: chatID, UserID: userID, CreatedAt: time.Now()}
	result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		log.Errorf("[Database][ClaimGreeting]: chat %d user %d: %v", chatID, userID, result.Error)
		return false, fukuerrors.Wrapf(result.Error, "claim greeting of user %d in chat %d", userID, chatID)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseGreeting gives a member's claim back by deleting that one row, so the member
// can be claimed again. Releasing a row that is not there is not an error. Like the
// claim it is a fresh database write with no cache key to delete.
func ReleaseGreeting(chatID, userID int64) error {
	err := db.DB.Where("chat_id = ? AND user_id = ?", chatID, userID).Delete(&models.GreetedMember{}).Error
	if err != nil {
		log.Errorf("[Database][ReleaseGreeting]: chat %d user %d: %v", chatID, userID, err)
		return fukuerrors.Wrapf(err, "release greeting of user %d in chat %d", userID, chatID)
	}
	return nil
}
