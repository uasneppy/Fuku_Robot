package approvals

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/errors"
)

func AddApprovedUser(chatID, userID, approvedBy int64, reason string) error {
	approval := &models.ApprovedUsers{
		ChatID:     chatID,
		UserID:     userID,
		ApprovedBy: approvedBy,
		Reason:     reason,
	}

	err := db.CreateRecord(approval)
	if err != nil {
		log.Errorf("[Database] AddApprovedUser: %v - chat:%d user:%d", err, chatID, userID)
		return err
	}

	cache.DeleteCache(cache.CacheKey("approvals", chatID))
	return nil
}

func IsUserApproved(chatID, userID int64) bool {
	for _, u := range GetApprovedUsers(chatID) {
		if u.UserID == userID {
			return true
		}
	}
	return false
}

func GetApprovedUsers(chatID int64) []*models.ApprovedUsers {
	return GetApprovedUsersContext(context.Background(), chatID)
}

func GetApprovedUsersContext(ctx context.Context, chatID int64) []*models.ApprovedUsers {
	cacheKey := cache.CacheKey("approvals", chatID)
	result, err := cache.GetFromCacheOrLoad(ctx, cacheKey, cache.CacheTTLApprovals, func(ctx context.Context) ([]*models.ApprovedUsers, error) {
		var users []*models.ApprovedUsers
		err := db.GetRecordsContext(ctx, &users, models.ApprovedUsers{ChatID: chatID})
		if err != nil {
			log.Errorf("[Database] GetApprovedUsers: %v - chat:%d", err, chatID)
			return nil, err
		}
		return users, nil
	})
	if err != nil {
		return nil
	}
	return result
}

// ListApprovedIDsFresh reads the approved users of one chat straight from the database and
// returns their IDs as a set. Unlike GetApprovedUsers it never reads or fills the cache,
// so no cache key needs invalidation, and it returns its error instead of an empty list:
// a caller that must not act on an unreadable list (a permanent ban) can tell "nobody is
// approved" from "the approvals could not be read".
func ListApprovedIDsFresh(chatID int64) (map[int64]bool, error) {
	var ids []int64
	err := db.DB.Model(&models.ApprovedUsers{}).Where("chat_id = ?", chatID).Pluck("user_id", &ids).Error
	if err != nil {
		log.Errorf("[Database] ListApprovedIDsFresh: %v - chat:%d", err, chatID)
		return nil, errors.Wrapf(err, "list approved users of chat %d", chatID)
	}
	approved := make(map[int64]bool, len(ids))
	for _, id := range ids {
		approved[id] = true
	}
	return approved, nil
}

func RemoveApprovedUser(chatID, userID int64) error {
	result := db.DB.Where("chat_id = ? AND user_id = ?", chatID, userID).Delete(&models.ApprovedUsers{})
	if result.Error != nil {
		log.Errorf("[Database] RemoveApprovedUser: %v - chat:%d user:%d", result.Error, chatID, userID)
		return result.Error
	}

	cache.DeleteCache(cache.CacheKey("approvals", chatID))
	return nil
}

func RemoveAllApprovedUsers(chatID int64) error {
	err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ApprovedUsers{}).Error
	if err != nil {
		log.Errorf("[Database] RemoveAllApprovedUsers: %v - chat:%d", err, chatID)
		return err
	}

	cache.DeleteCache(cache.CacheKey("approvals", chatID))
	return nil
}

func LoadApprovalsStats() (approvedUsers, approvalChats int64) {
	approvedUsers = db.CountRows(&models.ApprovedUsers{}, "")
	approvalChats = db.CountDistinct(&models.ApprovedUsers{}, "chat_id", "")
	return
}
