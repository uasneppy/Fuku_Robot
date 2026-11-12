package aispam

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// IsAISpamEnabled reports whether the AI spam filter is on for the chat.
// A missing row means disabled.
func IsAISpamEnabled(chatID int64) bool {
	return IsAISpamEnabledContext(context.Background(), chatID)
}

func IsAISpamEnabledContext(ctx context.Context, chatID int64) bool {
	settings, err := getAISpamSettingsCachedContext(ctx, chatID)
	if err != nil {
		log.Errorf("[Database] IsAISpamEnabled: %v - %d", err, chatID)
		return false
	}
	return settings.Enabled
}

// LoadEnabledContext reads whether the AI spam filter is on for the chat through the same cached
// getter, and the same ai_spam_settings:<chat> key, as IsAISpamEnabledContext, but returns the
// read error instead of hiding it as off. A chat with no row reads as off and no row is created.
func LoadEnabledContext(ctx context.Context, chatID int64) (bool, error) {
	settings, err := getAISpamSettingsCachedContext(ctx, chatID)
	if err != nil {
		return false, err
	}
	return settings.Enabled, nil
}

// SetAISpamEnabled turns the filter on or off for the chat.
func SetAISpamEnabled(chatID int64, enabled bool) error {
	err := db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(&models.AISpamSettings{
		ChatID:  chatID,
		Enabled: enabled,
	}).Error
	if err != nil {
		log.Errorf("[Database] SetAISpamEnabled: %v - %d", err, chatID)
		return err
	}

	cache.DeleteCache(cache.CacheKey("ai_spam_settings", chatID))
	return nil
}

func getAISpamSettingsCachedContext(ctx context.Context, chatID int64) (*models.AISpamSettings, error) {
	return cache.GetFromCacheOrLoad(ctx, cache.CacheKey("ai_spam_settings", chatID), cache.CacheTTLAISpam,
		func(ctx context.Context) (*models.AISpamSettings, error) {
			settings := &models.AISpamSettings{}
			err := db.GetRecordContext(ctx, settings, models.AISpamSettings{ChatID: chatID})
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &models.AISpamSettings{ChatID: chatID}, nil
			}
			if err != nil {
				return nil, err
			}
			return settings, nil
		})
}

func LoadAISpamStats() (enabledChats int64) {
	return db.CountRows(&models.AISpamSettings{}, "enabled = ?", true)
}
