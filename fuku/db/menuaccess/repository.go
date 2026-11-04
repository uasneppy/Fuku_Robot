// Package menuaccess holds the owner switch "Admins can change settings" of each group
// (table chat_settings_access).
//
// A chat without a row is owner only. The switch is an authority input, so it is read
// fresh from PostgreSQL on every decision and is never served from the cache layer: there
// is no cache key, no skipLocal entry and no cache.DeleteCache on a write (D-12). A cache
// added later would need a skipLocal entry and a DeleteCache on every write.
package menuaccess

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// errNoDatabase is returned, wrapped, when the database is not initialized.
var errNoDatabase = errors.New("database not initialized")

// AdminsAllowedFresh reports whether admins may change the chat's settings. A chat with no
// row is owner only, (false, nil). Every other failure is returned wrapped and never mapped
// to a value, so a caller refuses on an unreadable switch instead of guessing.
func AdminsAllowedFresh(ctx context.Context, chatID int64) (bool, error) {
	if db.DB == nil {
		return false, fukuerrors.Wrap(errNoDatabase, "read settings access")
	}
	var row models.ChatSettingsAccess
	err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fukuerrors.Wrapf(err, "read settings access of chat %d", chatID)
	}
	return row.AdminsAllowed, nil
}

// settingsAccessColumns are the columns a write updates on conflict: the switch, the last
// writer and the timestamp. A Go false is always written.
var settingsAccessColumns = []string{"admins_allowed", "updated_by", "updated_at"}

// SetAdminsAllowed stores the switch for the chat with one INSERT ... ON CONFLICT (chat_id)
// DO UPDATE, so there is one row per chat and the last writer wins. by is the user who
// pressed the button. No cache is involved: the next AdminsAllowedFresh sees the value.
func SetAdminsAllowed(ctx context.Context, chatID int64, allowed bool, by int64) error {
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "write settings access")
	}
	row := &models.ChatSettingsAccess{ChatID: chatID, AdminsAllowed: allowed, UpdatedBy: by}
	err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns(settingsAccessColumns),
	}).Create(row).Error
	if err != nil {
		return fukuerrors.Wrapf(err, "write settings access of chat %d", chatID)
	}
	return nil
}
