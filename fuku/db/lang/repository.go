package lang

import (
	"context"
	"errors"

	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/user"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

func checkUserInfo(userId int64) (userc *models.User) {
	userc, err := user.GetUserBasicInfoCached(userId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		log.Errorf("[Database] checkUserInfo: %v - %d", err, userId)
		return &models.User{UserId: userId}
	}
	return userc
}

func GetLanguage(ctx *ext.Context) string {
	if ctx == nil {
		return "uk"
	}

	chat := ctx.EffectiveChat
	if chat == nil {
		log.Warn("[GetLanguage] Unable to determine chat context, using default language")
		return "uk"
	}

	if chat.Type == "private" {
		if ctx.EffectiveSender == nil {
			log.Debug("[GetLanguage] No sender in private chat context, using default language")
			return "uk"
		}
		user := ctx.EffectiveSender.User
		if user == nil {
			return "uk"
		}
		return getUserLanguageContext(tracing.UpdateContext(ctx), user.Id)
	}
	return getGroupLanguageContext(tracing.UpdateContext(ctx), chat.Id)
}

// GetGroupLanguage returns a group's language by its chat ID through the same cached
// chat_lang read that GetLanguage makes in a group, which is what /rules reaches. It
// falls back to uk on a missing row, an empty value or an error, and creates no row. It
// is for callers whose update's chat is not the group, such as a private settings menu.
func GetGroupLanguage(ctx context.Context, groupID int64) string {
	return getGroupLanguageContext(ctx, groupID)
}

func getGroupLanguage(GroupID int64) string {
	return getGroupLanguageContext(context.Background(), GroupID)
}

func getGroupLanguageContext(ctx context.Context, groupID int64) string {
	return getCachedLanguage(ctx, cache.CacheKey("chat_lang", groupID), &models.Chat{}, "chat_id", groupID)
}

func getUserLanguage(userID int64) string {
	return getUserLanguageContext(context.Background(), userID)
}

func getUserLanguageContext(ctx context.Context, userID int64) string {
	return getCachedLanguage(ctx, cache.CacheKey("user_lang", userID), &models.User{}, "user_id", userID)
}

func getCachedLanguage(ctx context.Context, key string, model any, idColumn string, id int64) string {
	language, err := loadCachedLanguage(ctx, key, model, idColumn, id)
	if err != nil {
		return "uk"
	}
	return language
}

// LoadGroupLanguageContext returns a group's language by its chat ID through the same cached
// chat_lang read the bot's own group updates make, but returns the read error instead of
// hiding it behind uk and creates no row. A missing row or an empty value reads uk, the bot's
// default. It is for the settings menu's Language page, which must tell a failed read from a
// stored default.
func LoadGroupLanguageContext(ctx context.Context, groupID int64) (string, error) {
	return loadCachedLanguage(ctx, cache.CacheKey("chat_lang", groupID), &models.Chat{}, "chat_id", groupID)
}

func loadCachedLanguage(ctx context.Context, key string, model any, idColumn string, id int64) (string, error) {
	return cache.GetFromCacheOrLoad(ctx, key, cache.CacheTTLLanguage, func(ctx context.Context) (string, error) {
		if db.DB == nil {
			return "", errors.New("database not initialized")
		}
		var result struct{ Language string }
		err := db.DB.WithContext(ctx).Model(model).Select("language").Where(map[string]any{idColumn: id}).Take(&result).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "uk", nil
		}
		if err != nil {
			return "", err
		}
		if result.Language == "" {
			return "uk", nil
		}
		return result.Language, nil
	})
}

func ChangeUserLanguage(UserID int64, lang string) error {
	userc := checkUserInfo(UserID)
	if userc == nil {
		newUser := &models.User{
			UserId:   UserID,
			Language: lang,
		}
		err := db.DB.Create(newUser).Error
		if err != nil {
			log.Errorf("[Database] ChangeUserLanguage (create): %v - %d", err, UserID)
			return err
		}
		cache.DeleteCache(cache.CacheKey("user_lang", UserID))
		cache.DeleteCache(cache.CacheKey("user", UserID))
		log.Infof("[Database] ChangeUserLanguage: created new user %d with language %s", UserID, lang)
		return nil
	} else if userc.Language == lang {
		return nil
	}

	err := db.UpdateRecord(&models.User{}, models.User{UserId: UserID}, models.User{Language: lang})
	if err != nil {
		log.Errorf("[Database] ChangeUserLanguage: %v - %d", err, UserID)
		return err
	}
	cache.DeleteCache(cache.CacheKey("user_lang", UserID))
	cache.DeleteCache(cache.CacheKey("user", UserID))
	log.Infof("[Database] ChangeUserLanguage: %d", UserID)
	return nil
}

func ChangeGroupLanguage(GroupID int64, lang string) error {
	groupc := chats.GetChatSettings(GroupID)

	if groupc.ChatId == 0 {
		newChat := &models.Chat{
			ChatId:   GroupID,
			Language: lang,
		}
		err := db.DB.Create(newChat).Error
		if err != nil {
			log.Errorf("[Database] ChangeGroupLanguage (create): %v - %d", err, GroupID)
			return err
		}
		cache.DeleteCache(cache.CacheKey("chat_lang", GroupID))
		cache.DeleteCache(cache.CacheKey("chat_settings", GroupID))
		cache.DeleteCache(cache.CacheKey("chat", GroupID))
		log.Infof("[Database] ChangeGroupLanguage: created new chat %d with language %s", GroupID, lang)
		return nil
	} else if groupc.Language == lang {
		return nil
	}

	err := db.UpdateRecord(&models.Chat{}, models.Chat{ChatId: GroupID}, models.Chat{Language: lang})
	if err != nil {
		log.Errorf("[Database] ChangeGroupLanguage: %v - %d", err, GroupID)
		return err
	}
	cache.DeleteCache(cache.CacheKey("chat_lang", GroupID))
	cache.DeleteCache(cache.CacheKey("chat_settings", GroupID))
	cache.DeleteCache(cache.CacheKey("chat", GroupID))
	log.Infof("[Database] ChangeGroupLanguage: %d", GroupID)
	return nil
}
