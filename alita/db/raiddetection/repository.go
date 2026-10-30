// Package raiddetection holds the per-chat settings of automatic raid detection: whether
// the join surge, message flood and AI burst triggers are on and how many it takes to fire
// each.
// A chat without a row uses Defaults, so every group is protected until an admin changes
// it. Reads go through the cache under cache.CacheKey("raid_detection", chatID), and every
// write must cache.DeleteCache that key.
package raiddetection

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	// DefaultAIThreshold is how many AI spam verdicts within 120 seconds fire the AI burst
	// trigger for a chat with no settings row (D-05).
	DefaultAIThreshold = 5
	// MinThreshold is the lowest threshold a chat can store (D-18).
	MinThreshold = 3
	// MaxThreshold is the highest threshold a chat can store.
	MaxThreshold = 500
)

// cacheModule is the module name of the settings cache key.
const cacheModule = "raid_detection"

// Defaults returns the settings of a chat with no row: all three triggers on at the
// default thresholds. The returned value is a fresh copy. The AI fields satisfy the
// table's CHECK, so a chat's first write of any trigger inserts a valid row.
func Defaults(chatID int64) *models.ChatRaidDetection {
	return &models.ChatRaidDetection{
		ChatID:         chatID,
		JoinEnabled:    true,
		JoinThreshold:  DefaultJoinThreshold,
		FloodEnabled:   true,
		FloodThreshold: DefaultFloodThreshold,
		AIEnabled:      true,
		AIThreshold:    DefaultAIThreshold,
	}
}

// GetSettings returns a chat's raid detection settings through the cache. A chat with no
// row gets Defaults (D-11); any other failure is returned wrapped, never replaced by
// silent defaults, so a caller can tell "unreadable" from "not configured".
//
// A value the previous build cached has no AI fields, so it decodes with AIEnabled false
// and AIThreshold 0. The stored rows always satisfy the CHECK (threshold at least 3), so a
// threshold of 0 means "not populated" and reads as the AI defaults, never as an AI
// trigger that is off or fires on nothing (research Pitfall 1). The cache hands out a
// freshly decoded value on every read, so changing the returned one is safe.
func GetSettings(ctx context.Context, chatID int64) (*models.ChatRaidDetection, error) {
	settings, err := loadSettings(ctx, chatID)
	if err != nil || settings == nil {
		return settings, err
	}
	if settings.AIThreshold == 0 {
		settings.AIEnabled = true
		settings.AIThreshold = DefaultAIThreshold
	}
	return settings, nil
}

// loadSettings is the cache-or-load read behind GetSettings.
func loadSettings(ctx context.Context, chatID int64) (*models.ChatRaidDetection, error) {
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

// Column sets of the six writes. Each write touches only its own columns plus updated_at,
// so two admins changing two triggers at once never overwrite each other, and a Go false
// is always written (an upsert of named columns, not an UpdateRecord that skips zero
// values).
var (
	joinEnabledColumns    = []string{"join_enabled", "updated_at"}
	joinThresholdColumns  = []string{"join_enabled", "join_threshold", "updated_at"}
	floodEnabledColumns   = []string{"flood_enabled", "updated_at"}
	floodThresholdColumns = []string{"flood_enabled", "flood_threshold", "updated_at"}
	aiEnabledColumns      = []string{"ai_enabled", "updated_at"}
	aiThresholdColumns    = []string{"ai_enabled", "ai_threshold", "updated_at"}
)

// thresholdInRange reports whether threshold is a value a chat may store.
func thresholdInRange(threshold int) bool {
	return threshold >= MinThreshold && threshold <= MaxThreshold
}

// upsertSettings writes row with one INSERT ... ON CONFLICT (chat_id) DO UPDATE that
// updates only columns, then drops the cached read of the chat's settings. A chat's first
// write inserts the other trigger's defaults (row is Defaults with its own columns set).
// A failed write is returned wrapped and leaves the cache alone.
func upsertSettings(row *models.ChatRaidDetection, columns []string) error {
	if db.DB == nil {
		return alitaerrors.Wrap(errors.New("database not initialized"), "write raid detection settings")
	}
	err := db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(row).Error
	if err != nil {
		return alitaerrors.Wrapf(err, "write raid detection settings of chat %d", row.ChatID)
	}
	cache.DeleteCache(cache.CacheKey(cacheModule, row.ChatID))
	return nil
}

// SetJoinEnabled switches the join surge trigger on or off and keeps its threshold.
func SetJoinEnabled(chatID int64, enabled bool) error {
	row := Defaults(chatID)
	row.JoinEnabled = enabled
	return upsertSettings(row, joinEnabledColumns)
}

// SetJoinThreshold sets the join surge threshold and turns the trigger on. A threshold
// outside MinThreshold..MaxThreshold returns ErrThresholdOutOfRange before any write.
func SetJoinThreshold(chatID int64, threshold int) error {
	if !thresholdInRange(threshold) {
		return ErrThresholdOutOfRange
	}
	row := Defaults(chatID)
	row.JoinEnabled = true
	row.JoinThreshold = threshold
	return upsertSettings(row, joinThresholdColumns)
}

// SetFloodEnabled switches the flood trigger on or off and keeps its threshold.
func SetFloodEnabled(chatID int64, enabled bool) error {
	row := Defaults(chatID)
	row.FloodEnabled = enabled
	return upsertSettings(row, floodEnabledColumns)
}

// SetFloodThreshold sets the flood threshold and turns the trigger on. A threshold
// outside MinThreshold..MaxThreshold returns ErrThresholdOutOfRange before any write.
func SetFloodThreshold(chatID int64, threshold int) error {
	if !thresholdInRange(threshold) {
		return ErrThresholdOutOfRange
	}
	row := Defaults(chatID)
	row.FloodEnabled = true
	row.FloodThreshold = threshold
	return upsertSettings(row, floodThresholdColumns)
}

// SetAIEnabled switches the AI burst trigger on or off and keeps its threshold.
func SetAIEnabled(chatID int64, enabled bool) error {
	row := Defaults(chatID)
	row.AIEnabled = enabled
	return upsertSettings(row, aiEnabledColumns)
}

// SetAIThreshold sets the AI burst threshold and turns the trigger on. A threshold
// outside MinThreshold..MaxThreshold returns ErrThresholdOutOfRange before any write.
func SetAIThreshold(chatID int64, threshold int) error {
	if !thresholdInRange(threshold) {
		return ErrThresholdOutOfRange
	}
	row := Defaults(chatID)
	row.AIEnabled = true
	row.AIThreshold = threshold
	return upsertSettings(row, aiThresholdColumns)
}
