// Package timezone stores the timezone each chat chose for the times the bot shows.
package timezone

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// cacheTTL is how long a chat's stored zone stays in the cache.
const cacheTTL = cache.DefaultCacheTTL

var errNoDatabase = errors.New("database not initialized")

// CacheKey is the cache key of a chat's stored zone.
func CacheKey(chatID int64) string {
	return cache.CacheKey("chat_tz", chatID)
}

// Get returns the zone the chat stored, or "" for a chat with no row (the bot's default applies).
// It reads through the cache, returns its error and creates nothing.
func Get(ctx context.Context, chatID int64) (string, error) {
	return cache.GetFromCacheOrLoad(ctx, CacheKey(chatID), cacheTTL, func(ctx context.Context) (string, error) {
		return GetFresh(ctx, chatID)
	})
}

// GetFresh is Get without the cache.
func GetFresh(ctx context.Context, chatID int64) (string, error) {
	if db.DB == nil {
		return "", fukuerrors.Wrap(errNoDatabase, "read chat timezone")
	}
	var row models.ChatTimezone
	err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fukuerrors.Wrapf(err, "read the timezone of chat %d", chatID)
	}
	return row.TZ, nil
}

// Set stores the chat's zone with one column-targeted upsert and drops the cache key.
func Set(ctx context.Context, chatID int64, tz string, by int64) error {
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "write chat timezone")
	}
	row := &models.ChatTimezone{ChatID: chatID, TZ: tz, UpdatedBy: by, UpdatedAt: time.Now()}
	err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"tz", "updated_by", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return fukuerrors.Wrapf(err, "write the timezone of chat %d", chatID)
	}
	cache.DeleteCache(CacheKey(chatID))
	return nil
}

// Remove deletes the chat's row (back to the bot's default) and drops the cache key.
func Remove(ctx context.Context, chatID int64) error {
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "remove chat timezone")
	}
	if err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).Delete(&models.ChatTimezone{}).Error; err != nil {
		return fukuerrors.Wrapf(err, "remove the timezone of chat %d", chatID)
	}
	cache.DeleteCache(CacheKey(chatID))
	return nil
}
