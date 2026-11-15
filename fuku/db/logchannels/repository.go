package logchannels

import (
	"context"
	"errors"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

const cachePrefix = "log_channel"

// Categories are the Rose-compatible log channel category names.
const (
	CategorySettings  = "settings"
	CategoryAdmin     = "admin"
	CategoryUser      = "user"
	CategoryAutomated = "automated"
	CategoryReports   = "reports"
	CategoryOther     = "other"
)

var AllCategories = []string{
	CategorySettings,
	CategoryAdmin,
	CategoryUser,
	CategoryAutomated,
	CategoryReports,
	CategoryOther,
}

func invalidate(chatID int64) {
	cache.DeleteCache(cache.CacheKey(cachePrefix, chatID))
}

func Get(chatID int64) *models.LogChannel {
	return GetContext(context.Background(), chatID)
}

func GetContext(ctx context.Context, chatID int64) *models.LogChannel {
	result, err := loadRow(ctx, chatID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Errorf("[LogChannels] Get: %v", err)
		}
		return nil
	}
	return &result
}

// LoadContext returns a chat's log channel row through the same cached log_channel:<chat> read
// the commands make (GetContext), but tells a chat with no linked channel from a failed read: a
// chat with no row is (nil, nil) and any other read error is returned. It creates no row and
// caches only what GetContext's loader caches, so a miss leaves no entry behind. It is for the
// settings menu's Log channel page, which must refuse "no log channel" and show the cannot-open
// edit for "could not read" instead of guessing.
func LoadContext(ctx context.Context, chatID int64) (*models.LogChannel, error) {
	result, err := loadRow(ctx, chatID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

// loadRow is the cached read both GetContext and LoadContext make, under one key, TTL and loader.
func loadRow(ctx context.Context, chatID int64) (models.LogChannel, error) {
	return cache.GetFromCacheOrLoad(ctx, cache.CacheKey(cachePrefix, chatID), cache.CacheTTLLogChannel, func(ctx context.Context) (models.LogChannel, error) {
		var row models.LogChannel
		err := db.GetRecordContext(ctx, &row, models.LogChannel{ChatID: chatID})
		if err != nil {
			return models.LogChannel{}, err
		}
		return row, nil
	})
}

func Set(chatID int64, chatName string, logChannelID int64) error {
	if err := chats.EnsureChatInDb(chatID, chatName); err != nil {
		return err
	}
	row := models.LogChannel{
		ChatID:       chatID,
		LogChannelID: logChannelID,
		CatSettings:  true,
		CatAdmin:     true,
		CatUser:      true,
		CatAutomated: true,
		CatReports:   true,
		CatOther:     true,
	}
	err := db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"log_channel_id", "updated_at",
		}),
	}).Create(&row).Error
	if err != nil {
		log.Errorf("[LogChannels] Set: %v", err)
		return err
	}
	invalidate(chatID)
	return nil
}

func Unset(chatID int64) error {
	result := db.DB.Where("chat_id = ?", chatID).Delete(&models.LogChannel{})
	if result.Error != nil {
		log.Errorf("[LogChannels] Unset: %v", result.Error)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	invalidate(chatID)
	return nil
}

type categoryMeta struct {
	column string
	get    func(*models.LogChannel) bool
}

var logCategories = map[string]categoryMeta{
	CategorySettings:  {column: "cat_settings", get: func(s *models.LogChannel) bool { return s.CatSettings }},
	CategoryAdmin:     {column: "cat_admin", get: func(s *models.LogChannel) bool { return s.CatAdmin }},
	CategoryUser:      {column: "cat_user", get: func(s *models.LogChannel) bool { return s.CatUser }},
	CategoryAutomated: {column: "cat_automated", get: func(s *models.LogChannel) bool { return s.CatAutomated }},
	CategoryReports:   {column: "cat_reports", get: func(s *models.LogChannel) bool { return s.CatReports }},
	CategoryOther:     {column: "cat_other", get: func(s *models.LogChannel) bool { return s.CatOther }},
}

func categoryName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func IsValidCategory(name string) bool {
	_, ok := logCategories[categoryName(name)]
	return ok
}

func categoryColumn(name string) string {
	meta, ok := logCategories[categoryName(name)]
	if !ok {
		return ""
	}
	return meta.column
}

func SetCategory(chatID int64, name string, enabled bool) error {
	col := categoryColumn(name)
	if col == "" {
		return errors.New("unknown log category")
	}
	if Get(chatID) == nil {
		return gorm.ErrRecordNotFound
	}
	err := db.UpdateRecordWithZeroValues(
		&models.LogChannel{},
		models.LogChannel{ChatID: chatID},
		map[string]any{col: enabled},
	)
	if err != nil {
		log.Errorf("[LogChannels] SetCategory: %v", err)
		return err
	}
	invalidate(chatID)
	return nil
}

func CategoryEnabled(settings *models.LogChannel, name string) bool {
	if settings == nil {
		return false
	}
	meta, ok := logCategories[categoryName(name)]
	if !ok {
		return false
	}
	return meta.get(settings)
}

func LoadLogChannelStats() (linkedChats int64) {
	return db.CountRows(&models.LogChannel{}, "")
}
