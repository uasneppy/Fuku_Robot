// Package staff stores Staff Groups and the groups linked to them.
package staff

import (
	"context"
	"errors"
	"strings"
	"time"
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

// ErrAlreadyLinked is returned by CreateLink when the group already has a link.
var ErrAlreadyLinked = errors.New("staff: group already linked")

// ErrStaffGroupMissing is returned by CreateLink when the named Staff Group has
// no staff_groups row.
var ErrStaffGroupMissing = errors.New("staff: staff group not found")

// ErrRoleConflict is returned by CreateStaffGroup when the chat is currently a
// linked group of some Staff Group, and by CreateLink when the group being linked
// is itself a Staff Group: the two roles never overlap (D-10).
var ErrRoleConflict = errors.New("staff: chat already holds the other staff role")

// CreateStaffGroup stores chatID as a Staff Group owned by ownerUserID. It is
// idempotent: when the chat is already a Staff Group it returns created=false
// with a nil error and leaves the existing row (and its owner) untouched.
//
// The two staff roles never overlap (D-10): in the same transaction as the
// insert it refuses a chat that is currently a linked group and returns
// ErrRoleConflict without writing. This application-level check works on both
// PostgreSQL and SQLite. It cannot stop two replicas that race past the check at
// the same moment; only a database-level constraint can.
func CreateStaffGroup(chatID, ownerUserID int64, title string) (created bool, err error) {
	row := &models.StaffGroup{
		ChatID:      chatID,
		OwnerUserID: ownerUserID,
		Title:       trimStaffTitle(title),
	}
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		var linked int64
		if err := tx.Model(&models.StaffGroupLink{}).Where("group_chat_id = ?", chatID).Count(&linked).Error; err != nil {
			return err
		}
		if linked > 0 {
			return ErrRoleConflict
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "chat_id"}},
			DoNothing: true,
		}).Create(row)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected > 0
		return nil
	})
	if errors.Is(err, ErrRoleConflict) {
		return false, ErrRoleConflict
	}
	if err != nil {
		log.Errorf("[Staff] CreateStaffGroup: %v", err)
		return false, alitaerrors.Wrapf(err, "create staff group %d", chatID)
	}
	invalidateStaffKeys(chatID)
	return created, nil
}

// GetStaffGroup is the cached gate: it returns the Staff Group for chatID, or
// nil when the chat is not one (or on an error, which is logged). A zero-value
// sentinel is cached for "not found", so CreateStaffGroup must invalidate.
// Never use it as an authority read; use GetStaffGroupFresh for that.
func GetStaffGroup(chatID int64) *models.StaffGroup {
	return getCachedRow(cachePrefixStaffGroup, "chat_id = ?", chatID, func(row *models.StaffGroup) bool {
		return row.ChatID == 0
	})
}

// getCachedRow is the cached gate behind GetStaffGroup and GetLinkOfGroup. query
// is a constant condition with one placeholder for chatID. It caches the zero
// value for "no row", which isMissing recognises, and returns nil for it and for
// a failed load (logged).
func getCachedRow[T any](prefix, query string, chatID int64, isMissing func(*T) bool) *T {
	if chatID == 0 {
		return nil
	}
	result, err := cache.GetFromCacheOrLoad(
		context.Background(),
		cache.CacheKey(prefix, chatID),
		cache.DefaultCacheTTL,
		func(ctx context.Context) (T, error) {
			var row T
			err := db.DB.WithContext(ctx).Where(query, chatID).First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var none T
				return none, nil
			}
			if err != nil {
				var none T
				return none, err
			}
			return row, nil
		},
	)
	if err != nil {
		log.Errorf("[Staff] cached lookup %s for chat %d: %v", prefix, chatID, err)
		return nil
	}
	if isMissing(&result) {
		return nil
	}
	return &result
}

// CountLinksByStaffFresh counts the groups linked to the Staff Group staffChatID
// straight from the database, bypassing every cache.
func CountLinksByStaffFresh(staffChatID int64) (int64, error) {
	var count int64
	err := db.DB.Model(&models.StaffGroupLink{}).Where("staff_chat_id = ?", staffChatID).Count(&count).Error
	if err != nil {
		log.Errorf("[Staff] CountLinksByStaffFresh: %v", err)
		return 0, alitaerrors.Wrapf(err, "count staff links %d", staffChatID)
	}
	return count, nil
}

// DeleteStaffGroupWithLinks removes the Staff Group chatID and every link whose
// staff_chat_id is chatID in one transaction, so either all of them still exist
// or none do. removed lists the deleted links in link-id order.
//
// The staff_groups row is deleted first: that delete claims the row, so when two
// callers race only the one whose delete affected a row gets deleted=true and a
// non-empty removed; the other gets (nil, false, nil) and must post nothing.
func DeleteStaffGroupWithLinks(chatID int64) (removed []models.StaffGroupLink, deleted bool, err error) {
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("chat_id = ?", chatID).Delete(&models.StaffGroup{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if err := tx.Where("staff_chat_id = ?", chatID).Order("id ASC").Find(&removed).Error; err != nil {
			return err
		}
		if err := tx.Where("staff_chat_id = ?", chatID).Delete(&models.StaffGroupLink{}).Error; err != nil {
			return err
		}
		deleted = true
		return nil
	})
	if err != nil {
		log.Errorf("[Staff] DeleteStaffGroupWithLinks: %v", err)
		return nil, false, alitaerrors.Wrapf(err, "delete staff group %d", chatID)
	}
	if !deleted {
		return nil, false, nil
	}
	invalidateStaffKeys(chatID)
	for _, link := range removed {
		invalidateStaffKeys(link.GroupChatID)
	}
	return removed, true, nil
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

// checkLinkRoles verifies, on q, that the Staff Group of link exists and that the
// group being linked is not itself a Staff Group.
func checkLinkRoles(q *gorm.DB, link *models.StaffGroupLink) error {
	var staffRows int64
	if err := q.Model(&models.StaffGroup{}).Where("chat_id = ?", link.StaffChatID).Count(&staffRows).Error; err != nil {
		return err
	}
	if staffRows == 0 {
		return ErrStaffGroupMissing
	}
	var groupIsStaff int64
	if err := q.Model(&models.StaffGroup{}).Where("chat_id = ?", link.GroupChatID).Count(&groupIsStaff).Error; err != nil {
		return err
	}
	if groupIsStaff > 0 {
		return ErrRoleConflict
	}
	return nil
}

// CreateLink stores link. It returns ErrStaffGroupMissing when no staff_groups
// row has chat_id = link.StaffChatID, ErrRoleConflict when the group being linked
// is itself a Staff Group (D-10), and ErrAlreadyLinked when the group already has
// a link. The first two leave nothing behind.
//
// The role checks run once before the transaction, so the caller gets the precise
// reason even on PostgreSQL, where the exclusivity trigger would otherwise reject
// the insert with a generic error, and once more inside it, after the insert, so
// they hold at the moment of the write. The insert comes first inside the
// transaction so it takes its write lock up front; a read-then-write upgrade
// fails under contention on SQLite. It is ON CONFLICT DO NOTHING on the unique
// group_chat_id: when two callers race for the same group exactly one gets a nil
// error and the other gets ErrAlreadyLinked. On success the group's cached
// lookups are invalidated after the commit.
func CreateLink(link *models.StaffGroupLink) error {
	link.GroupTitle = trimStaffTitle(link.GroupTitle)
	err := checkLinkRoles(db.DB, link)
	if err == nil {
		err = db.DB.Transaction(func(tx *gorm.DB) error {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(link)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrAlreadyLinked
			}
			return checkLinkRoles(tx, link)
		})
	}
	switch {
	case errors.Is(err, ErrStaffGroupMissing), errors.Is(err, ErrRoleConflict), errors.Is(err, ErrAlreadyLinked):
		return err
	case err != nil:
		log.Errorf("[Staff] CreateLink: %v", err)
		return alitaerrors.Wrapf(err, "create staff link %d", link.GroupChatID)
	}
	invalidateStaffKeys(link.GroupChatID)
	return nil
}

// ListLinksByStaffFresh lists the groups linked to the Staff Group staffChatID in
// link-id order, straight from the database.
func ListLinksByStaffFresh(staffChatID int64) ([]models.StaffGroupLink, error) {
	var links []models.StaffGroupLink
	if err := db.DB.Where("staff_chat_id = ?", staffChatID).Order("id ASC").Find(&links).Error; err != nil {
		log.Errorf("[Staff] ListLinksByStaffFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "list staff links %d", staffChatID)
	}
	return links, nil
}

// GetLinkOfGroup is the cached gate: it returns the link of groupChatID, or nil
// when the group is not linked (or on an error, which is logged). A zero-value
// sentinel is cached for "not found", so every link write invalidates it. Never
// use it as an authority read; use GetLinkOfGroupFresh for that.
func GetLinkOfGroup(groupChatID int64) *models.StaffGroupLink {
	return getCachedRow(cachePrefixStaffLinkOf, "group_chat_id = ?", groupChatID, func(row *models.StaffGroupLink) bool {
		return row.GroupChatID == 0
	})
}

// ListStaffGroupsByOwner lists the Staff Groups whose recorded owner is
// ownerUserID, in id order, straight from the database. The recorded owner only
// narrows the candidates for /linkstaff without an argument; every candidate is
// still verified live before it is used.
func ListStaffGroupsByOwner(ownerUserID int64) ([]models.StaffGroup, error) {
	var groups []models.StaffGroup
	if err := db.DB.Where("owner_user_id = ?", ownerUserID).Order("id ASC").Find(&groups).Error; err != nil {
		log.Errorf("[Staff] ListStaffGroupsByOwner: %v", err)
		return nil, alitaerrors.Wrapf(err, "list staff groups of owner %d", ownerUserID)
	}
	return groups, nil
}

// GetLinkOfGroupFresh reads the link of groupChatID straight from the database,
// bypassing every cache. It returns (nil, nil) when the group is not linked. It
// is the authority read.
func GetLinkOfGroupFresh(groupChatID int64) (*models.StaffGroupLink, error) {
	var row models.StaffGroupLink
	err := db.DB.Where("group_chat_id = ?", groupChatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Staff] GetLinkOfGroupFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get staff link %d", groupChatID)
	}
	return &row, nil
}

// GetLinkByIDFresh reads one link by its row ID straight from the database,
// bypassing every cache. It returns (nil, nil) when there is no such row. The
// Unlink buttons carry only this ID, so it is the authority read behind them.
func GetLinkByIDFresh(id uint) (*models.StaffGroupLink, error) {
	var row models.StaffGroupLink
	err := db.DB.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Staff] GetLinkByIDFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get staff link by id %d", id)
	}
	return &row, nil
}

// DeleteLink removes the one link with row ID id and nothing else. The delete
// itself claims the row, so when two callers race only the one whose delete
// affected a row gets deleted=true; the other gets (false, nil) and must post
// nothing. The deleted row is read back by the same statement (RETURNING), so
// the cached lookups of its group are invalidated after the commit without a
// separate read that could go stale.
func DeleteLink(id uint) (deleted bool, err error) {
	var removed []models.StaffGroupLink
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.Returning{}).Where("id = ?", id).Delete(&removed)
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected == 1 && len(removed) == 1
		return nil
	})
	if err != nil {
		log.Errorf("[Staff] DeleteLink: %v", err)
		return false, alitaerrors.Wrapf(err, "delete staff link %d", id)
	}
	if !deleted {
		return false, nil
	}
	invalidateStaffKeys(removed[0].GroupChatID)
	return true, nil
}

// DeleteLinkIfOwner removes the link with row ID id only while its recorded
// maker is still ownerUserID, in one conditional statement. That statement is the
// claim: of any number of callers racing on the same link (service message,
// chat_member update, panel recheck, sweep) exactly one gets deleted=true, and only
// that caller may post the removal notice. A link that is gone, or was re-made by
// someone else, gives (false, nil). Like DeleteLink it uses RETURNING, so the
// cached lookups of the group are invalidated by the exact key after the commit.
func DeleteLinkIfOwner(id uint, ownerUserID int64) (deleted bool, err error) {
	var removed []models.StaffGroupLink
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.Returning{}).
			Where("id = ? AND owner_user_id = ?", id, ownerUserID).
			Delete(&removed)
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected == 1 && len(removed) == 1
		return nil
	})
	if err != nil {
		log.Errorf("[Staff] DeleteLinkIfOwner: %v", err)
		return false, alitaerrors.Wrapf(err, "delete staff link %d if owner", id)
	}
	if !deleted {
		return false, nil
	}
	invalidateStaffKeys(removed[0].GroupChatID)
	return true, nil
}

// UpdateStaffGroupOwner refreshes staff_groups.owner_user_id for chatID to
// ownerUserID. The recorded owner is only a lookup hint (it is never an
// authority), so recheckStaffGroup keeps it equal to the live creator.
//
// It is one conditional UPDATE through a map, so a zero-value column is never
// skipped. changed is true only when a row existed with a different owner; the
// same value, or an unknown chat, gives (false, nil). The cached Staff Group
// lookup is invalidated after the write.
func UpdateStaffGroupOwner(chatID, ownerUserID int64) (changed bool, err error) {
	result := db.DB.Model(&models.StaffGroup{}).
		Where("chat_id = ? AND owner_user_id <> ?", chatID, ownerUserID).
		Updates(map[string]any{"owner_user_id": ownerUserID, "updated_at": time.Now()})
	if result.Error != nil {
		log.Errorf("[Staff] UpdateStaffGroupOwner: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "update staff group %d owner", chatID)
	}
	if result.RowsAffected != 1 {
		return false, nil
	}
	invalidateStaffKeys(chatID)
	return true, nil
}
