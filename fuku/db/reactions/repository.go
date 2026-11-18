package reactions

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

func reactionsCacheKey(chatID int64) string {
	return cache.CacheKey("reactions", chatID)
}

func GetReactions(chatID int64) map[string]string {
	return GetReactionsContext(context.Background(), chatID)
}

func GetReactionsContext(ctx context.Context, chatID int64) map[string]string {
	cacheKey := reactionsCacheKey(chatID)
	result, err := cache.GetFromCacheOrLoad(ctx, cacheKey, cache.CacheTTLReactions, func(ctx context.Context) (map[string]string, error) {
		var rows []*models.Reactions
		if err := db.GetRecordsContext(ctx, &rows, models.Reactions{ChatID: chatID}); err != nil {
			log.Errorf("[Database] GetReactions: %v - chat:%d", err, chatID)
			return map[string]string{}, err
		}
		out := make(map[string]string, len(rows))
		for _, r := range rows {
			out[r.Keyword] = r.Emoji
		}
		return out, nil
	})
	if err != nil || result == nil {
		return map[string]string{}
	}
	return result
}

func AddReaction(chatID int64, keyword, emoji string) error {
	r := &models.Reactions{
		ChatID:  chatID,
		Keyword: keyword,
		Emoji:   emoji,
	}
	err := db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}, {Name: "keyword"}},
		DoUpdates: clause.AssignmentColumns([]string{"emoji", "updated_at"}),
	}).Create(r).Error
	if err != nil {
		log.Errorf("[Database] AddReaction: %v - chat:%d keyword:%s", err, chatID, keyword)
		return err
	}
	cache.DeleteCache(reactionsCacheKey(chatID))
	return nil
}

func RemoveReaction(chatID int64, keyword string) error {
	result := db.DB.Where("chat_id = ? AND keyword = ?", chatID, keyword).Delete(&models.Reactions{})
	if result.Error != nil {
		log.Errorf("[Database] RemoveReaction: %v - chat:%d keyword:%s", result.Error, chatID, keyword)
		return result.Error
	}
	cache.DeleteCache(reactionsCacheKey(chatID))
	return nil
}

func ResetReactions(chatID int64) error {
	if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.Reactions{}).Error; err != nil {
		log.Errorf("[Database] ResetReactions: %v - chat:%d", err, chatID)
		return err
	}
	cache.DeleteCache(reactionsCacheKey(chatID))
	return nil
}

func LoadReactionsStats() (reactionsNum, reactionChats int64) {
	reactionsNum = db.CountRows(&models.Reactions{}, "")
	reactionChats = db.CountDistinct(&models.Reactions{}, "chat_id", "")
	return
}

// AddReactionContext stores one reaction. With overwrite it is the upsert /addreaction has always made, and
// added is true. Without it an existing keyword is left as it is (ON CONFLICT DO NOTHING) and added says
// whether the row was inserted. The chat's cache key is dropped after any write that changed a row.
func AddReactionContext(ctx context.Context, chatID int64, keyword, emoji string, overwrite bool) (added bool, err error) {
	r := &models.Reactions{ChatID: chatID, Keyword: keyword, Emoji: emoji}
	conflict := clause.OnConflict{Columns: []clause.Column{{Name: "chat_id"}, {Name: "keyword"}}, DoNothing: true}
	if overwrite {
		conflict = clause.OnConflict{
			Columns:   []clause.Column{{Name: "chat_id"}, {Name: "keyword"}},
			DoUpdates: clause.AssignmentColumns([]string{"emoji", "updated_at"}),
		}
	}
	result := db.DB.WithContext(ctx).Clauses(conflict).Create(r)
	if result.Error != nil {
		log.Errorf("[Database] AddReaction: %v - chat:%d", result.Error, chatID)
		return false, result.Error
	}
	added = overwrite || result.RowsAffected == 1
	if added {
		cache.DeleteCache(reactionsCacheKey(chatID))
	}
	return added, nil
}

// ListReactionEntriesFresh lists the chat's reactions (ID, keyword and emoji) by ID. It is fresh, chat-scoped
// and uncached, returns its read error and creates nothing.
func ListReactionEntriesFresh(ctx context.Context, chatID int64) ([]models.Reactions, error) {
	var rows []models.Reactions
	err := db.DB.WithContext(ctx).Select("id", "keyword", "emoji").Where("chat_id = ?", chatID).Order("id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// GetReactionEntryFresh reads one reaction by its ID, only when it belongs to the chat: a missing ID and the ID
// of another chat's row both return nil and a nil error. It is fresh and creates nothing.
func GetReactionEntryFresh(ctx context.Context, chatID, id int64) (*models.Reactions, error) {
	var row models.Reactions
	err := db.DB.WithContext(ctx).Where("id = ? AND chat_id = ?", id, chatID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// CountReactionsFresh counts the chat's reactions, fresh and with the error returned.
func CountReactionsFresh(ctx context.Context, chatID int64) (int64, error) {
	var n int64
	err := db.DB.WithContext(ctx).Model(&models.Reactions{}).Where("chat_id = ?", chatID).Count(&n).Error
	return n, err
}

// ReactionExistsFresh reports whether the chat has a reaction for keyword, fresh and with the error returned.
func ReactionExistsFresh(ctx context.Context, chatID int64, keyword string) (bool, error) {
	var n int64
	err := db.DB.WithContext(ctx).Model(&models.Reactions{}).Where("chat_id = ? AND keyword = ?", chatID, keyword).Count(&n).Error
	return n > 0, err
}

// RemoveReactionByID deletes one reaction by its ID, only when it belongs to the chat, in one conditional
// statement, and returns the row that went (nil when none did). The cache key is dropped only when a row went.
func RemoveReactionByID(ctx context.Context, chatID, id int64) (*models.Reactions, error) {
	var rows []models.Reactions
	result := db.DB.WithContext(ctx).Clauses(clause.Returning{}).Where("id = ? AND chat_id = ?", id, chatID).Delete(&rows)
	if result.Error != nil {
		log.Errorf("[Database] RemoveReactionByID: %v - chat:%d", result.Error, chatID)
		return nil, result.Error
	}
	if len(rows) != 1 {
		return nil, nil
	}
	cache.DeleteCache(reactionsCacheKey(chatID))
	return &rows[0], nil
}
