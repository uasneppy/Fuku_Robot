// Package raiddetection holds the per-chat settings of automatic raid detection: whether
// the join surge and message flood triggers are on and how many it takes to fire each.
// A chat without a row uses Defaults, so every group is protected until an admin changes
// it. Reads go through the cache under cache.CacheKey("raid_detection", chatID), and every
// write must cache.DeleteCache that key.
package raiddetection

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

const (
	// DefaultJoinThreshold is how many different accounts joining within 60 seconds fire
	// the join surge trigger for a chat with no settings row (D-01).
	DefaultJoinThreshold = 15
	// DefaultFloodThreshold is how many messages within 30 seconds fire the flood trigger
	// for a chat with no settings row (D-01).
	DefaultFloodThreshold = 30
	// MinThreshold is the lowest threshold a chat can store (D-18).
	MinThreshold = 3
	// MaxThreshold is the highest threshold a chat can store.
	MaxThreshold = 500
)

// cacheModule is the module name of the settings cache key.
const cacheModule = "raid_detection"

// Defaults returns the settings of a chat with no row: both triggers on at the default
// thresholds. The returned value is a fresh copy.
func Defaults(chatID int64) *models.ChatRaidDetection {
	return &models.ChatRaidDetection{
		ChatID:         chatID,
		JoinEnabled:    true,
		JoinThreshold:  DefaultJoinThreshold,
		FloodEnabled:   true,
		FloodThreshold: DefaultFloodThreshold,
	}
}

// GetSettings returns a chat's raid detection settings through the cache. A chat with no
// row gets Defaults (D-11); any other failure is returned wrapped, never replaced by
// silent defaults, so a caller can tell "unreadable" from "not configured".
func GetSettings(ctx context.Context, chatID int64) (*models.ChatRaidDetection, error) {
	return cache.GetFromCacheOrLoad(ctx, cache.CacheKey(cacheModule, chatID), cache.DefaultCacheTTL,
		func(ctx context.Context) (*models.ChatRaidDetection, error) {
			if db.DB == nil {
				return nil, alitaerrors.Wrap(errors.New("database not initialized"), "raid detection settings")
			}
			var row models.ChatRaidDetection
			err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Defaults(chatID), nil
			}
			if err != nil {
				return nil, alitaerrors.Wrapf(err, "read raid detection settings of chat %d", chatID)
			}
			return &row, nil
		})
}

// ErrThresholdOutOfRange is returned by the threshold setters for a value outside
// MinThreshold..MaxThreshold. Nothing is written.
var ErrThresholdOutOfRange = errors.New("raid detection threshold out of range")

// SetJoinEnabled is scaffolding for the RED commit: it writes nothing yet.
func SetJoinEnabled(chatID int64, enabled bool) error { return nil }

// SetJoinThreshold is scaffolding for the RED commit: it writes nothing yet.
func SetJoinThreshold(chatID int64, threshold int) error { return nil }

// SetFloodEnabled is scaffolding for the RED commit: it writes nothing yet.
func SetFloodEnabled(chatID int64, enabled bool) error { return nil }

// SetFloodThreshold is scaffolding for the RED commit: it writes nothing yet.
func SetFloodThreshold(chatID int64, threshold int) error { return nil }
