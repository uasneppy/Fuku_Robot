package blacklists

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// AddBlacklist stores a word for the chat with the warn action. It is the wrapper
// of AddBlacklistContext that the older callers use.
func AddBlacklist(chatId int64, trigger string) error {
	return AddBlacklistContext(context.Background(), chatId, trigger, "warn")
}

// AddBlacklistContext stores a lower-cased word for the chat with the given action and
// drops the chat's cached blacklist after a successful insert.
func AddBlacklistContext(ctx context.Context, chatId int64, trigger, action string) error {
	blacklist := &models.BlacklistSettings{
		ChatId: chatId,
		Word:   strings.ToLower(trigger),
		Action: action,
		Reason: "Blacklisted word: '%s'",
	}

	err := db.CreateRecordContext(ctx, blacklist)
	if err != nil {
		log.Errorf("[Database] AddBlacklist: %v - %d", err, chatId)
		return err
	}

	cache.DeleteCache(cache.CacheKey("blacklist", chatId))
	return nil
}

// RemoveBlacklist deletes one word of the chat. It is the wrapper of
// RemoveBlacklistContext that the older callers use.
func RemoveBlacklist(chatId int64, trigger string) error {
	return RemoveBlacklistContext(context.Background(), chatId, trigger)
}

// RemoveBlacklistContext deletes the lower-cased word of the chat and drops the chat's
// cached blacklist only when a row went.
func RemoveBlacklistContext(ctx context.Context, chatId int64, trigger string) error {
	result := db.DB.WithContext(ctx).Where("chat_id = ? AND word = ?", chatId, strings.ToLower(trigger)).Delete(&models.BlacklistSettings{})
	if result.Error != nil {
		log.Errorf("[Database] RemoveBlacklist: %v - %d", result.Error, chatId)
		return result.Error
	}

	if result.RowsAffected > 0 {
		cache.DeleteCache(cache.CacheKey("blacklist", chatId))
	}
	return nil
}

func RemoveAllBlacklist(chatId int64) error {
	err := db.DB.Where("chat_id = ?", chatId).Delete(&models.BlacklistSettings{}).Error
	if err != nil {
		log.Errorf("[Database] RemoveAllBlacklist: %v - %d", err, chatId)
		return err
	}

	cache.DeleteCache(cache.CacheKey("blacklist", chatId))
	return nil
}

// SetBlacklistAction sets the action of every word of the chat. It is the wrapper of
// SetBlacklistActionContext that the older callers use.
func SetBlacklistAction(chatId int64, action string) error {
	_, err := SetBlacklistActionContext(context.Background(), chatId, action)
	return err
}

// SetBlacklistActionContext sets the lower-cased action of every word of the chat, drops
// the chat's cached blacklist after a successful update and returns how many rows the
// update matched (zero for a chat with no words).
func SetBlacklistActionContext(ctx context.Context, chatId int64, action string) (int64, error) {
	result := db.DB.WithContext(ctx).Model(&models.BlacklistSettings{}).Where("chat_id = ?", chatId).Update("action", strings.ToLower(action))
	if result.Error != nil {
		log.Errorf("[Database] SetBlacklistAction: %v - %d", result.Error, chatId)
		return 0, result.Error
	}

	cache.DeleteCache(cache.CacheKey("blacklist", chatId))
	return result.RowsAffected, nil
}

func GetBlacklistSettings(chatId int64) models.BlacklistSettingsSlice {
	return GetBlacklistSettingsContext(context.Background(), chatId)
}

func GetBlacklistSettingsContext(ctx context.Context, chatId int64) models.BlacklistSettingsSlice {
	cacheKey := cache.CacheKey("blacklist", chatId)
	result, err := cache.GetFromCacheOrLoad(ctx, cacheKey, cache.CacheTTLBlacklist, func(ctx context.Context) (models.BlacklistSettingsSlice, error) {
		var blacklists []*models.BlacklistSettings
		err := db.GetRecordsContext(ctx, &blacklists, models.BlacklistSettings{ChatId: chatId})
		if err != nil {
			log.Errorf("[Database] GetBlacklistSettings: %v - %d", err, chatId)
			return models.BlacklistSettingsSlice{}, err
		}
		return models.BlacklistSettingsSlice(blacklists), nil
	})
	if err != nil {
		return models.BlacklistSettingsSlice{}
	}
	return result
}

func LoadBlacklistsStats() (blacklistTriggers, blacklistChats int64) {
	err := db.DB.Model(&models.BlacklistSettings{}).Count(&blacklistTriggers).Error
	if err != nil {
		log.Errorf("[Database] LoadBlacklistsStats (triggers): %v", err)
		return 0, 0
	}

	err = db.DB.Model(&models.BlacklistSettings{}).Distinct("chat_id").Count(&blacklistChats).Error
	if err != nil {
		log.Errorf("[Database] LoadBlacklistsStats (chats): %v", err)
		return blacklistTriggers, 0
	}

	return
}
