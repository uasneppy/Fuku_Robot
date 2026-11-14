package deletion

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
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
// seconds. An unknown kind returns ErrUnknownKind and a delay the kind does not accept
// (ValidateRuleDelay: the range for a service or bot kind, the presets for a member kind)
// ErrDelayOutOfRange, both before any write. A Go 0 is written, and the cached read is
// dropped.
func SetRule(ctx context.Context, chatID int64, kind string, delay int) error {
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	if err := ValidateRuleDelay(kind, delay); err != nil {
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

// dropPendingKindsTx deletes, inside tx, the PENDING queue rows of a chat whose kind is exactly
// one of kinds. It never takes a prefix (a kind that merely starts like another is not a match),
// never touches an acting row (a worker already claimed it and finishes in seconds), and never
// another chat's rows. It is the part of a switch-off that stops what was already queued (WR-03).
func dropPendingKindsTx(tx *gorm.DB, chatID int64, kinds ...string) error {
	return tx.Where("chat_id = ? AND state = ? AND kind IN ?", chatID, models.ScheduledDeletionPending, kinds).
		Delete(&models.ScheduledDeletion{}).Error
}

// RemoveRule deletes the row of one kind, so the chat keeps (or, for a bot kind, never
// deletes) it again, and in the same transaction drops the chat's pending queue rows of that
// exact kind: a rule switched off must stop acting on what was already queued, not only on the
// next message (WR-03). Acting rows are left to the worker that claimed them. Removing an
// absent row is not an error. An unknown kind returns ErrUnknownKind before any write.
func RemoveRule(ctx context.Context, chatID int64, kind string) error {
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "remove deletion rule")
	}
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("chat_id = ? AND kind = ?", chatID, kind).Delete(&models.DeleteRule{}).Error; err != nil {
			return err
		}
		return dropPendingKindsTx(tx, chatID, kind)
	})
	if err != nil {
		return fukuerrors.Wrapf(err, "remove deletion rule of chat %d", chatID)
	}
	cache.DeleteCache(rulesKey(chatID))
	return nil
}

// JoinLeftMode is what a group chose for the joined or the left service message.
type JoinLeftMode int

// The three choices of the joined and left service messages.
const (
	// JoinLeftKeep keeps the message: the shared flag is off and both delay rows are gone.
	JoinLeftKeep JoinLeftMode = iota + 1
	// JoinLeftNow deletes the message at once: the flag is on and this kind has no row.
	JoinLeftNow
	// JoinLeftAfter deletes the message after DelaySecs: the flag is on and this kind has a row.
	JoinLeftAfter
)

// JoinLeftState is the choice for one joined or left kind. DelaySecs is read only for
// JoinLeftAfter and must then be 1..MaxDelaySecs.
type JoinLeftState struct {
	Mode      JoinLeftMode
	DelaySecs int
}

var (
	// ErrNotJoinLeft is returned when a kind is not the joined or the left service message.
	ErrNotJoinLeft = errors.New("kind does not share the clean service flag")
	// ErrInvalidJoinLeftState is returned for a JoinLeftMode that is not one of the three.
	ErrInvalidJoinLeftState = errors.New("invalid joined or left state")
)

// applyHook is called inside the transaction of ApplyJoinLeftRule and ApplyCleanServiceFlag
// between the flag write and the rule writes. It is nil in production; a test installs one
// with SetApplyHookForTest (testtools builds only) to fail the second step.
var applyHook func() error

// runApplyHook calls the hook, if any.
func runApplyHook() error {
	if applyHook == nil {
		return nil
	}
	return applyHook()
}

// dropSharedCaches drops the greetings and the delete_rules cache keys of a chat after a
// transaction that wrote both stores.
func dropSharedCaches(chatID int64) {
	cache.DeleteCache(cache.CacheKey("greetings", chatID))
	cache.DeleteCache(rulesKey(chatID))
}

// removeJoinLeftTx removes both the joined and the left delay rows inside tx and, with dropQueued,
// also the chat's pending queue rows of those two kinds: Keep and /cleanservice off stop what was
// already queued (WR-03), while /cleanservice on and a Now or After choice leave the queue alone.
func removeJoinLeftTx(tx *gorm.DB, chatID int64, dropQueued bool) error {
	if err := tx.Where("chat_id = ? AND kind IN ?", chatID, []string{KindJoined, KindLeft}).
		Delete(&models.DeleteRule{}).Error; err != nil {
		return err
	}
	if !dropQueued {
		return nil
	}
	return dropPendingKindsTx(tx, chatID, KindJoined, KindLeft)
}

// ApplyJoinLeftRule applies a group's choice for the joined or the left service message.
// The shared /cleanservice flag and the delay rows are written in ONE transaction, so a
// failure leaves both as they were: Keep writes the flag false and removes both the joined
// and the left rows (with the flag off both read Keep, and a stale delay must never come
// back) and drops the chat's pending queue rows of both kinds (WR-03); Now writes the flag
// true and removes this kind's row; After N writes the flag true and upserts this kind's row,
// and neither of those drops a queued row. The kind, the mode and the delay are validated before any
// write. After the commit it drops the greetings and the delete_rules cache keys.
func ApplyJoinLeftRule(ctx context.Context, chatID int64, kind string, state JoinLeftState) error {
	if !SharesCleanServiceFlag(kind) {
		return ErrNotJoinLeft
	}
	switch state.Mode {
	case JoinLeftKeep, JoinLeftNow:
	case JoinLeftAfter:
		if state.DelaySecs < 1 || state.DelaySecs > MaxDelaySecs {
			return ErrDelayOutOfRange
		}
	default:
		return ErrInvalidJoinLeftState
	}
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "apply joined or left rule")
	}
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := greetings.SetCleanServiceTx(tx, chatID, state.Mode != JoinLeftKeep); err != nil {
			return err
		}
		if err := runApplyHook(); err != nil {
			return err
		}
		switch state.Mode {
		case JoinLeftKeep:
			return removeJoinLeftTx(tx, chatID, true)
		case JoinLeftNow:
			return tx.Where("chat_id = ? AND kind = ?", chatID, kind).Delete(&models.DeleteRule{}).Error
		default:
			return tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "chat_id"}, {Name: "kind"}},
				DoUpdates: clause.AssignmentColumns([]string{"delay_secs", "updated_at"}),
			}).Create(&models.DeleteRule{ChatID: chatID, Kind: kind, DelaySecs: state.DelaySecs}).Error
		}
	})
	if err != nil {
		return fukuerrors.Wrapf(err, "apply joined or left rule of chat %d", chatID)
	}
	dropSharedCaches(chatID)
	return nil
}

// ApplyCleanServiceFlag is the /cleanservice command's write: it sets the shared flag and
// removes BOTH the joined and the left delay rows in one transaction, so a delay chosen
// before an off and on never comes back; turning it off also drops the chat's pending queue
// rows of both kinds in that transaction (WR-03). It then drops the greetings and delete_rules
// cache keys.
func ApplyCleanServiceFlag(ctx context.Context, chatID int64, on bool) error {
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "apply clean service flag")
	}
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := greetings.SetCleanServiceTx(tx, chatID, on); err != nil {
			return err
		}
		if err := runApplyHook(); err != nil {
			return err
		}
		return removeJoinLeftTx(tx, chatID, !on)
	})
	if err != nil {
		return fukuerrors.Wrapf(err, "apply clean service flag of chat %d", chatID)
	}
	dropSharedCaches(chatID)
	return nil
}
