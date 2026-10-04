// Package staff stores Staff Groups and the groups linked to them.
package staff

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

const (
	cachePrefixStaffGroup  = "staff_group"
	cachePrefixStaffLinkOf = "staff_link_of"
	maxStaffTitleRunes     = 64
)

// invalidateStaffKeys deletes the cached Staff Group and link lookups of every
// given chat ID. Callers invoke it after the write commits, for both the old
// and the new ID when a chat is re-keyed.
func invalidateStaffKeys(chatIDs ...int64) {
	for _, id := range chatIDs {
		cache.DeleteCache(cache.CacheKey(cachePrefixStaffGroup, id))
		cache.DeleteCache(cache.CacheKey(cachePrefixStaffLinkOf, id))
	}
}

// trimStaffTitle trims surrounding spaces and caps the title at 64 Unicode code
// points without ever splitting a character.
func trimStaffTitle(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxStaffTitleRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxStaffTitleRunes])
}

// CreateStaffGroup stores chatID as a Staff Group owned by ownerUserID. It is
// idempotent: when the chat is already a Staff Group it returns created=false
// with a nil error and leaves the existing row (and its owner) untouched.
func CreateStaffGroup(chatID, ownerUserID int64, title string) (created bool, err error) {
	row := &models.StaffGroup{
		ChatID:      chatID,
		OwnerUserID: ownerUserID,
		Title:       trimStaffTitle(title),
	}
	result := db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoNothing: true,
	}).Create(row)
	if result.Error != nil {
		log.Errorf("[Staff] CreateStaffGroup: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "create staff group %d", chatID)
	}
	invalidateStaffKeys(chatID)
	return result.RowsAffected > 0, nil
}

// GetStaffGroup is the cached gate: it returns the Staff Group for chatID, or
// nil when the chat is not one (or on an error, which is logged). A zero-value
// sentinel is cached for "not found", so CreateStaffGroup must invalidate.
// Never use it as an authority read; use GetStaffGroupFresh for that.
func GetStaffGroup(chatID int64) *models.StaffGroup {
	if chatID == 0 {
		return nil
	}
	result, err := cache.GetFromCacheOrLoad(
		context.Background(),
		cache.CacheKey(cachePrefixStaffGroup, chatID),
		cache.DefaultCacheTTL,
		func(ctx context.Context) (models.StaffGroup, error) {
			var row models.StaffGroup
			err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return models.StaffGroup{}, nil
			}
			if err != nil {
				return models.StaffGroup{}, err
			}
			return row, nil
		},
	)
	if err != nil {
		log.Errorf("[Staff] GetStaffGroup: %v", err)
		return nil
	}
	if result.ChatID == 0 {
		return nil
	}
	return &result
}

// GetStaffGroupFresh reads the Staff Group for chatID straight from the
// database, bypassing every cache. It returns (nil, nil) when there is no row.
// It is the authority read.
func GetStaffGroupFresh(chatID int64) (*models.StaffGroup, error) {
	var row models.StaffGroup
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Staff] GetStaffGroupFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get staff group %d", chatID)
	}
	return &row, nil
}
