package deletion

import (
	"context"
	"errors"

	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// rulesCacheModule is the module name of the rules cache key. The key is listed in
// skipLocal (fuku/db/cache/local.go): the service watcher and every send site read it, and
// a rule switched off must stop acting on every replica at the next message.
const rulesCacheModule = "delete_rules"

var errNoDatabase = errors.New("database not initialized")

// Rules is one chat's deletion rules: the delay in seconds of each kind that has a row.
// A kind that is absent from Delays has no rule (Keep, or Off for a bot kind); a delay of 0
// is present and means "delete now". A chat with no row has an empty, non-nil map.
type Rules struct {
	ChatID int64
	Delays map[string]int
}

// rulesKey returns the cache key of a chat's rules.
func rulesKey(chatID int64) string {
	return cache.CacheKey(rulesCacheModule, chatID)
}

// GetRules returns a chat's rules through the cache. A chat with no row has no rules and
// nothing is created. Any other failure is returned wrapped, never replaced by silent
// defaults, so a caller can tell "unreadable" from "not configured".
func GetRules(ctx context.Context, chatID int64) (*Rules, error) {
	return cache.GetFromCacheOrLoad(ctx, rulesKey(chatID), cache.DefaultCacheTTL,
		func(ctx context.Context) (*Rules, error) {
			return GetRulesFresh(ctx, chatID)
		})
}

// GetRulesFresh is GetRules without the cache, for a caller that must see a write another
// replica just made.
func GetRulesFresh(ctx context.Context, chatID int64) (*Rules, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errNoDatabase, "read deletion rules")
	}
	var rows []models.DeleteRule
	if err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		return nil, fukuerrors.Wrapf(err, "read deletion rules of chat %d", chatID)
	}
	rules := &Rules{ChatID: chatID, Delays: make(map[string]int, len(rows))}
	for _, row := range rows {
		rules.Delays[row.Kind] = row.DelaySecs
	}
	return rules, nil
}

// SetRule stores the delay of one kind: 0 deletes at once, 1..MaxDelaySecs after that many
// seconds. An unknown kind returns ErrUnknownKind and a delay outside the range
// ErrDelayOutOfRange, both before any write. A Go 0 is written, and the cached read is
// dropped.
func SetRule(ctx context.Context, chatID int64, kind string, delay int) error {
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	if err := validateDelay(delay); err != nil {
		return err
	}
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "write deletion rule")
	}
	row := &models.DeleteRule{ChatID: chatID, Kind: kind, DelaySecs: delay}
	err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}, {Name: "kind"}},
		DoUpdates: clause.AssignmentColumns([]string{"delay_secs", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return fukuerrors.Wrapf(err, "write deletion rule of chat %d", chatID)
	}
	cache.DeleteCache(rulesKey(chatID))
	return nil
}

// RemoveRule deletes the row of one kind, so the chat keeps (or, for a bot kind, never
// deletes) it again. Removing an absent row is not an error. An unknown kind returns
// ErrUnknownKind before any write.
func RemoveRule(ctx context.Context, chatID int64, kind string) error {
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "remove deletion rule")
	}
	if err := db.DB.WithContext(ctx).Where("chat_id = ? AND kind = ?", chatID, kind).
		Delete(&models.DeleteRule{}).Error; err != nil {
		return fukuerrors.Wrapf(err, "remove deletion rule of chat %d", chatID)
	}
	cache.DeleteCache(rulesKey(chatID))
	return nil
}
