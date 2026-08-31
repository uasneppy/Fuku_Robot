package antiraid

import (
	"errors"

	log "github.com/sirupsen/logrus"
	"github.com/uasneppy/Fuku_Robot/fuku/db"
	"github.com/uasneppy/Fuku_Robot/fuku/db/cache"
	"github.com/uasneppy/Fuku_Robot/fuku/db/models"
	"gorm.io/gorm"
)

// getAntiRaidSettingsRaw retrieves anti-raid settings with minimal column selection.
func getAntiRaidSettingsRaw(chatID int64) (*models.AntiRaidSettings, error) {
	if db.DB == nil {
		return defaultAntiRaidSettings(chatID), errors.New("database not initialized")
	}

	var settings models.AntiRaidSettings
	err := db.DB.Model(&models.AntiRaidSettings{}).
		Select("id, chat_id, raid_time, raid_action_time, auto_antiraid_threshold").
		Where("chat_id = ?", chatID).
		First(&settings).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultAntiRaidSettings(chatID), nil
	}
	if err != nil {
		log.Errorf("[OptimizedAntiRaidQueries] getAntiRaidSettingsRaw: %v", err)
		return nil, err
	}

	return &settings, nil
}

// GetAntiRaidSettingsCached retrieves anti-raid settings with caching layer for improved performance.
// Uses 30-minute cache TTL and falls back to direct query if cache fails.
func GetAntiRaidSettingsCached(chatID int64) (*models.AntiRaidSettings, error) {
	cacheKey := cache.CacheKey("antiraid", chatID)

	cached, err := cache.GetFromCacheOrLoad(cacheKey, cache.CacheTTLAntiRaid, func() (*models.AntiRaidSettings, error) {
		return getAntiRaidSettingsRaw(chatID)
	})
	if err != nil {
		return getAntiRaidSettingsRaw(chatID)
	}

	return cached, nil
}
