package channels

import (
	"errors"

	log "github.com/sirupsen/logrus"
	"github.com/uasneppy/Fuku_Robot/fuku/db"
	"github.com/uasneppy/Fuku_Robot/fuku/db/cache"
	"github.com/uasneppy/Fuku_Robot/fuku/db/models"
	"gorm.io/gorm"
)

// getChannelSettingsRaw retrieves channel settings with all relevant columns.
// Returns channel settings for the specified chat or nil if not found.
func getChannelSettingsRaw(chatID int64) (*models.ChannelSettings, error) {
	if db.DB == nil {
		return nil, errors.New("database not initialized")
	}

	var settings models.ChannelSettings
	err := db.DB.Model(&models.ChannelSettings{}).
		Select("id, chat_id, channel_id, channel_name, username").
		Where("chat_id = ?", chatID).
		First(&settings).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		log.Errorf("[OptimizedChannelQueries] getChannelSettingsRaw: %v", err)
		return nil, err
	}

	return &settings, nil
}

// GetChannelSettingsCached retrieves channel settings with caching layer for improved performance.
// Uses 30-minute cache TTL and falls back to direct query if cache fails.
func GetChannelSettingsCached(chatID int64) (*models.ChannelSettings, error) {
	cacheKey := cache.CacheKey("channel", chatID)

	cached, err := cache.GetFromCacheOrLoad(cacheKey, cache.CacheTTLChannels, func() (*models.ChannelSettings, error) {
		return getChannelSettingsRaw(chatID)
	})
	if err != nil {
		return getChannelSettingsRaw(chatID)
	}

	return cached, nil
}
