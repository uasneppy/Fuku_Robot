package filters

import (
	"context"
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

func filterListCacheKey(chatID int64) string {
	return cache.CacheKey("filter_list", chatID)
}

func optimizedFilterCacheKey(chatID int64) string {
	return cache.CacheKey("filters_optimized", chatID)
}

func invalidateFilterCaches(chatID int64) {
	cache.DeleteCache(filterListCacheKey(chatID))
	cache.DeleteCache(optimizedFilterCacheKey(chatID))
}

func GetFiltersList(chatID int64) (allFilterWords []string) {
	return GetFiltersListContext(context.Background(), chatID)
}

func GetFiltersListContext(ctx context.Context, chatID int64) (allFilterWords []string) {
	cacheKey := filterListCacheKey(chatID)
	result, err := cache.GetFromCacheOrLoad(ctx, cacheKey, cache.CacheTTLFilterList, func(ctx context.Context) ([]string, error) {
		var results []*models.ChatFilters
		err := db.GetRecordsContext(ctx, &results, map[string]any{"chat_id": chatID})
		if err != nil {
			log.Errorf("[Database] GetFiltersList: %v - %d", err, chatID)
			return []string{}, err
		}

		filterWords := make([]string, 0, len(results))
		for _, j := range results {
			filterWords = append(filterWords, j.KeyWord)
		}
		return filterWords, nil
	})
	if err != nil {
		return []string{}
	}
	return result
}

// DoesFilterExists reports whether the chat has a filter with the keyword (compared without regard to
// case). A failed read is logged and reads as false, which is the behaviour the callers that cannot
// refuse rely on; use FilterExistsContext where the error matters.
func DoesFilterExists(chatId int64, keyword string) bool {
	exists, err := FilterExistsContext(context.Background(), chatId, keyword)
	if err != nil {
		return false
	}
	return exists
}

// FilterExistsContext reports whether the chat has a filter with the keyword, compared without regard
// to case, and returns the database error instead of hiding it.
func FilterExistsContext(ctx context.Context, chatID int64, keyword string) (bool, error) {
	var filter models.ChatFilters
	err := db.DB.WithContext(ctx).Where("chat_id = ? AND LOWER(keyword) = LOWER(?)", chatID, keyword).Take(&filter).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		log.Errorf("[Database] FilterExistsContext: %v - %d", err, chatID)
		return false, err
	}
	return true, nil
}

// AddFilter stores a filter and reports only the error; a keyword the chat already has is left as it is.
func AddFilter(chatID int64, keyWord, replyText, fileID string, buttons []models.Button, filtType int) error {
	_, err := AddFilterContext(context.Background(), chatID, keyWord, replyText, fileID, buttons, filtType)
	return err
}

// AddFilterContext stores a filter and reports whether a row was inserted: a keyword the chat already
// has (the unique key is chat and keyword) inserts nothing and is not an error, so a caller can tell a
// stored filter from one that lost a race. The chat's filter list and optimized caches are dropped only
// when a row was inserted.
func AddFilterContext(ctx context.Context, chatID int64, keyWord, replyText, fileID string, buttons []models.Button, filtType int) (inserted bool, err error) {
	now := time.Now().UTC()
	newFilter := map[string]any{
		"chat_id":        chatID,
		"keyword":        keyWord,
		"filter_reply":   replyText,
		"msgtype":        filtType,
		"fileid":         fileID,
		"nonotif":        false,
		"filter_buttons": models.ButtonArray(buttons),
		"created_at":     now,
		"updated_at":     now,
	}

	result := db.DB.WithContext(ctx).Model(&models.ChatFilters{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}, {Name: "keyword"}},
		DoNothing: true,
	}).Create(newFilter)
	if result.Error != nil {
		log.Errorf("[Database][AddFilter]: %d - %v", chatID, result.Error)
		return false, result.Error
	}

	if result.RowsAffected > 0 {
		invalidateFilterCaches(chatID)
		return true, nil
	}
	return false, nil
}

func UpdateFilter(chatID int64, keyWord, replyText, fileID string, buttons []models.Button, filtType int) (bool, error) {
	result := db.DB.Model(&models.ChatFilters{}).
		Where("chat_id = ? AND keyword = ?", chatID, keyWord).
		Updates(map[string]any{
			"filter_reply":   replyText,
			"msgtype":        filtType,
			"fileid":         fileID,
			"filter_buttons": models.ButtonArray(buttons),
			"updated_at":     time.Now().UTC(),
		})
	if result.Error != nil {
		log.Errorf("[Database][UpdateFilter]: %d - %v", chatID, result.Error)
		return false, result.Error
	}
	if result.RowsAffected > 0 {
		invalidateFilterCaches(chatID)
	}
	return result.RowsAffected > 0, nil
}

// RemoveFilter deletes the chat's filter with exactly this keyword.
func RemoveFilter(chatID int64, keyWord string) error {
	return RemoveFilterContext(context.Background(), chatID, keyWord)
}

// RemoveFilterContext deletes the chat's filter with exactly this keyword and drops the chat's filter
// caches when a row went. A keyword that is not stored is not an error.
func RemoveFilterContext(ctx context.Context, chatID int64, keyWord string) error {
	result := db.DB.WithContext(ctx).Where("chat_id = ? AND keyword = ?", chatID, keyWord).Delete(&models.ChatFilters{})
	if result.Error != nil {
		log.Errorf("[Database][RemoveFilter]: %d - %v", chatID, result.Error)
		return result.Error
	}

	if result.RowsAffected > 0 {
		invalidateFilterCaches(chatID)
	}
	return nil
}

func RemoveAllFilters(chatID int64) error {
	err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatFilters{}).Error
	if err != nil {
		log.Errorf("[Database][RemoveAllFilters]: %d - %v", chatID, err)
		return err
	}

	invalidateFilterCaches(chatID)
	return nil
}

// CountFilters returns how many filters the chat has. A failed read is logged and reads as 0; use
// CountFiltersContext where the error matters.
func CountFilters(chatID int64) (filtersNum int64) {
	filtersNum, err := CountFiltersContext(context.Background(), chatID)
	if err != nil {
		return 0
	}
	return filtersNum
}

// CountFiltersContext returns how many filters the chat has and the database error instead of hiding
// it. It creates nothing and uses no cache.
func CountFiltersContext(ctx context.Context, chatID int64) (int64, error) {
	var filtersNum int64
	err := db.DB.WithContext(ctx).Model(&models.ChatFilters{}).Where("chat_id = ?", chatID).Count(&filtersNum).Error
	if err != nil {
		log.Errorf("[Database][CountFilters]: %d - %v", chatID, err)
		return 0, err
	}
	return filtersNum, nil
}

func LoadFilterStats() (filtersNum, filtersUsingChats int64) {
	err := db.DB.Model(&models.ChatFilters{}).Count(&filtersNum).Error
	if err != nil {
		log.Errorf("[Database][LoadFilterStats] counting filters: %v", err)
		return
	}

	err = db.DB.Model(&models.ChatFilters{}).Select("COUNT(DISTINCT chat_id)").Scan(&filtersUsingChats).Error
	if err != nil {
		log.Errorf("[Database][LoadFilterStats] counting chats: %v", err)
		return
	}

	return
}
