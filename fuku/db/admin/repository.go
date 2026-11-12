package admin

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func GetAdminSettings(chatID int64) *models.AdminSettings {
	return checkAdminSetting(chatID)
}

func checkAdminSetting(chatID int64) (adminSrc *models.AdminSettings) {
	adminSrc = &models.AdminSettings{}

	err := db.GetRecord(adminSrc, models.AdminSettings{ChatId: chatID})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		adminSrc = &models.AdminSettings{ChatId: chatID, AnonAdmin: false}
		err := db.CreateRecord(adminSrc)
		if err != nil {
			log.Errorf("[Database][checkAdminSetting]: %v ", err)
		}
	} else if err != nil {
		adminSrc = &models.AdminSettings{ChatId: chatID, AnonAdmin: false}
		log.Errorf("[Database][checkAdminSetting]: %v ", err)
	}
	return adminSrc
}

// LoadAnonAdminContext reads whether anonymous-admin mode is on for the chat with one plain SELECT
// of the admin table, which is not cached. Unlike GetAdminSettings it returns the read error
// instead of hiding it as off and it never creates the row: a chat with no row reads as off.
func LoadAnonAdminContext(ctx context.Context, chatID int64) (bool, error) {
	var row models.AdminSettings
	err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.AnonAdmin, nil
}

func SetAnonAdminMode(chatID int64, val bool) error {
	adminSrc := checkAdminSetting(chatID)
	adminSrc.AnonAdmin = val

	err := db.UpdateRecordWithZeroValues(&models.AdminSettings{}, models.AdminSettings{ChatId: chatID}, map[string]any{"anon_admin": val})
	if err != nil {
		log.Errorf("[Database] SetAnonAdminMode: %v - %d", err, chatID)
		return err
	}
	return nil
}
