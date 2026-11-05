// Package memberreq holds the member requirement data: each chat's choice of punishment
// per rule (chat_requirements) and the per-member hold that is both the one-punishment
// latch and the record of a mute (chat_requirement_holds).
//
// Settings are read through the cache under cache.CacheKey("member_requirements", chatID)
// and every write must cache.DeleteCache that key. A chat without a row has every rule off.
// Holds are read and written fresh in the database; see holds.go.
package memberreq

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// Rule is the ID of one member requirement rule. Its action column is "<id>_action".
type Rule string

// The rules of chat_requirements.
const (
	RuleLastName Rule = "last_name"
	RuleUsername Rule = "username"
	RuleRussian  Rule = "russian"
	RuleArabic   Rule = "arabic"
	RuleChinese  Rule = "chinese"
)

// Rules lists every rule in the order the settings page shows them.
var Rules = []Rule{RuleLastName, RuleUsername, RuleRussian, RuleArabic, RuleChinese}

var (
	// ErrActionOutOfRange is returned for an action outside 0..4. Nothing is written.
	ErrActionOutOfRange = errors.New("member requirement action out of range")
	// ErrUnknownRule is returned for a rule that is not in Rules. Nothing is written.
	ErrUnknownRule = errors.New("unknown member requirement rule")
)

// cacheModule is the module name of the settings cache key.
const cacheModule = "member_requirements"

// ruleColumns maps each rule to its action column.
var ruleColumns = map[Rule]string{
	RuleLastName: "last_name_action",
	RuleUsername: "username_action",
	RuleRussian:  "russian_action",
	RuleArabic:   "arabic_action",
	RuleChinese:  "chinese_action",
}

// actionField returns a pointer to the action column of rule in row, or nil for a rule it
// does not know.
func actionField(row *models.ChatRequirements, rule Rule) *int {
	switch rule {
	case RuleLastName:
		return &row.LastNameAction
	case RuleUsername:
		return &row.UsernameAction
	case RuleRussian:
		return &row.RussianAction
	case RuleArabic:
		return &row.ArabicAction
	case RuleChinese:
		return &row.ChineseAction
	}
	return nil
}

// ActionOf returns the action stored for rule, or 0 (off) for a rule it does not know.
func ActionOf(row *models.ChatRequirements, rule Rule) int {
	if field := actionField(row, rule); field != nil {
		return *field
	}
	return models.RequirementActionOff
}

// AnyRuleOn reports whether any rule has an action above 0.
func AnyRuleOn(row *models.ChatRequirements) bool {
	for _, rule := range Rules {
		if ActionOf(row, rule) > models.RequirementActionOff {
			return true
		}
	}
	return false
}

// GetSettings returns a chat's requirement settings through the cache. A chat with no row
// reads every rule off and both switches off, and nothing is created. Any other failure is
// returned wrapped, never replaced by silent defaults, so a caller can tell "unreadable"
// from "not configured".
func GetSettings(ctx context.Context, chatID int64) (*models.ChatRequirements, error) {
	return cache.GetFromCacheOrLoad(ctx, cache.CacheKey(cacheModule, chatID), cache.DefaultCacheTTL,
		func(ctx context.Context) (*models.ChatRequirements, error) {
			return GetSettingsFresh(ctx, chatID)
		})
}

// GetSettingsFresh is GetSettings without the cache, for a caller that must see a write
// another replica just made.
func GetSettingsFresh(ctx context.Context, chatID int64) (*models.ChatRequirements, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errors.New("database not initialized"), "member requirement settings")
	}
	var row models.ChatRequirements
	err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.ChatRequirements{ChatID: chatID}, nil
	}
	if err != nil {
		return nil, fukuerrors.Wrapf(err, "read member requirement settings of chat %d", chatID)
	}
	return &row, nil
}

// upsertSettings writes row with one INSERT ... ON CONFLICT (chat_id) DO UPDATE of only
// columns plus updated_at, then drops the cached read. Each write touches its own column,
// so two admins changing two rules at once never overwrite each other, and a Go 0 or false
// is always written. A first write inserts zeros for the other columns, which is "off".
func upsertSettings(ctx context.Context, row *models.ChatRequirements, column string) error {
	if db.DB == nil {
		return fukuerrors.Wrap(errors.New("database not initialized"), "write member requirement settings")
	}
	err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns([]string{column, "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return fukuerrors.Wrapf(err, "write member requirement settings of chat %d", row.ChatID)
	}
	cache.DeleteCache(cache.CacheKey(cacheModule, row.ChatID))
	return nil
}

// SetRuleAction stores the punishment of one rule. An unknown rule returns ErrUnknownRule
// and an action outside 0..4 ErrActionOutOfRange, both before any write.
func SetRuleAction(ctx context.Context, chatID int64, rule Rule, action int) error {
	column, ok := ruleColumns[rule]
	if !ok {
		return ErrUnknownRule
	}
	if action < models.RequirementActionOff || action > models.RequirementActionMute {
		return ErrActionOutOfRange
	}
	row := &models.ChatRequirements{ChatID: chatID}
	*actionField(row, rule) = action
	return upsertSettings(ctx, row, column)
}

// SetCheckOnJoin switches the check of new members at join on or off.
func SetCheckOnJoin(ctx context.Context, chatID int64, on bool) error {
	return upsertSettings(ctx, &models.ChatRequirements{ChatID: chatID, CheckOnJoin: on}, "check_on_join")
}

// SetDeleteMessages switches the deletion of a violator's messages on or off.
func SetDeleteMessages(ctx context.Context, chatID int64, on bool) error {
	return upsertSettings(ctx, &models.ChatRequirements{ChatID: chatID, DeleteMessages: on}, "delete_messages")
}
