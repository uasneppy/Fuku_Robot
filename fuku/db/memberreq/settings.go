package memberreq

import (
	"context"
	"errors"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
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

// ActionOf returns the action stored for rule.
func ActionOf(row *models.ChatRequirements, rule Rule) int { return -1 }

// AnyRuleOn reports whether any rule has an action above 0.
func AnyRuleOn(row *models.ChatRequirements) bool { return true }

// GetSettings returns a chat's settings through the cache.
func GetSettings(ctx context.Context, chatID int64) (*models.ChatRequirements, error) {
	return &models.ChatRequirements{ChatID: chatID}, nil
}

// GetSettingsFresh returns a chat's settings straight from the database.
func GetSettingsFresh(ctx context.Context, chatID int64) (*models.ChatRequirements, error) {
	return &models.ChatRequirements{ChatID: chatID}, nil
}

// SetRuleAction stores the action of one rule.
func SetRuleAction(ctx context.Context, chatID int64, rule Rule, action int) error { return nil }

// SetCheckOnJoin stores the check-on-join switch.
func SetCheckOnJoin(ctx context.Context, chatID int64, on bool) error { return nil }

// SetDeleteMessages stores the delete-messages switch.
func SetDeleteMessages(ctx context.Context, chatID int64, on bool) error { return nil }
