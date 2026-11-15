package staff

import (
	"context"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

const (
	cachePrefixStaffForwardBlock = "staff_forward_block"

	// ForwardBlockMax is the soft cap on the sources one Staff Group can list. It is read inside the
	// insert transaction, so two concurrent adds can pass it by one.
	ForwardBlockMax = 200

	// ForwardBlockActionNone deletes the forward and does nothing more to the member.
	ForwardBlockActionNone = "none"
)

// ForwardBlockActions is the one action vocabulary of a forward blocklist: the vocabulary of the
// blacklist, where none means "delete only". The CHECK on staff_forward_block_settings.action
// holds exactly these five values.
var ForwardBlockActions = []string{ForwardBlockActionNone, "warn", "mute", "kick", "ban"}

// ErrForwardBlockFull is returned by AddForwardBlock when the Staff Group already lists
// ForwardBlockMax sources.
var ErrForwardBlockFull = errors.New("staff: forward block list is full")

// ErrForwardBlockAction is returned by SetForwardBlockAction for an action outside
// ForwardBlockActions, before any write.
var ErrForwardBlockAction = errors.New("staff: unknown forward block action")

// ForwardBlockRules is one Staff Group's forward blocklist as the watcher reads it: the action
// and the source chat IDs. It is the value cached under staff_forward_block:<staff chat>.
type ForwardBlockRules struct {
	Action  string  `msgpack:"a"`
	Sources []int64 `msgpack:"s"`
}

// IsBlocked reports whether sourceChatID is listed.
func (r ForwardBlockRules) IsBlocked(sourceChatID int64) bool {
	return slices.Contains(r.Sources, sourceChatID)
}

// invalidateForwardBlockKeys drops the cached forward block rules of every given Staff Group ID.
// Callers invoke it after the write commits.
func invalidateForwardBlockKeys(staffChatIDs ...int64) {
	for _, id := range staffChatIDs {
		cache.DeleteCache(cache.CacheKey(cachePrefixStaffForwardBlock, id))
	}
}

func forwardBlockDB(ctx context.Context) (*gorm.DB, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errors.New("database not initialized"), "staff forward block")
	}
	return db.DB.WithContext(ctx), nil
}

// GetForwardBlockRules returns a Staff Group's forward blocklist through the cache (the key is in
// skipLocal, so a write on another replica is seen at the next read). A Staff Group with no rows
// reads action none and no sources. A failed load is returned wrapped and is never replaced by
// defaults, so a caller can tell "unreadable" from "empty".
func GetForwardBlockRules(ctx context.Context, staffChatID int64) (ForwardBlockRules, error) {
	return cache.GetFromCacheOrLoad(ctx, cache.CacheKey(cachePrefixStaffForwardBlock, staffChatID), cache.DefaultCacheTTL,
		func(ctx context.Context) (ForwardBlockRules, error) {
			return GetForwardBlockRulesFresh(ctx, staffChatID)
		})
}

// GetForwardBlockRulesFresh is GetForwardBlockRules without the cache.
func GetForwardBlockRulesFresh(ctx context.Context, staffChatID int64) (ForwardBlockRules, error) {
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return ForwardBlockRules{}, err
	}
	rules := ForwardBlockRules{Action: ForwardBlockActionNone, Sources: []int64{}}
	var settings models.StaffForwardBlockSettings
	err = q.Where("staff_chat_id = ?", staffChatID).First(&settings).Error
	switch {
	case err == nil:
		rules.Action = settings.Action
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return ForwardBlockRules{}, fukuerrors.Wrapf(err, "read forward block action of staff chat %d", staffChatID)
	}
	var ids []int64
	err = q.Model(&models.StaffForwardBlock{}).Where("staff_chat_id = ?", staffChatID).
		Order("id ASC").Pluck("source_chat_id", &ids).Error
	if err != nil {
		return ForwardBlockRules{}, fukuerrors.Wrapf(err, "read forward block sources of staff chat %d", staffChatID)
	}
	if ids != nil {
		rules.Sources = ids
	}
	return rules, nil
}

// ListForwardBlocksFresh returns a Staff Group's listed sources ordered by ID, straight from the
// database. It creates nothing and uses no cache.
func ListForwardBlocksFresh(ctx context.Context, staffChatID int64) ([]models.StaffForwardBlock, error) {
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.StaffForwardBlock
	if err := q.Where("staff_chat_id = ?", staffChatID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fukuerrors.Wrapf(err, "list forward blocks of staff chat %d", staffChatID)
	}
	return rows, nil
}

// GetForwardBlockFresh returns one listed source by its row ID, only when it belongs to the Staff
// Group. A missing row and another Staff Group's row both return (nil, nil).
func GetForwardBlockFresh(ctx context.Context, staffChatID, id int64) (*models.StaffForwardBlock, error) {
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return nil, err
	}
	var row models.StaffForwardBlock
	err = q.Where("id = ? AND staff_chat_id = ?", id, staffChatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fukuerrors.Wrapf(err, "get forward block %d of staff chat %d", id, staffChatID)
	}
	return &row, nil
}

// CountForwardBlocksFresh counts a Staff Group's listed sources.
func CountForwardBlocksFresh(ctx context.Context, staffChatID int64) (int64, error) {
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.Model(&models.StaffForwardBlock{}).Where("staff_chat_id = ?", staffChatID).Count(&n).Error; err != nil {
		return 0, fukuerrors.Wrapf(err, "count forward blocks of staff chat %d", staffChatID)
	}
	return n, nil
}

// requireStaffGroupRow returns ErrStaffGroupMissing unless q holds a staff_groups row for chatID.
func requireStaffGroupRow(q *gorm.DB, chatID int64) error {
	var rows int64
	if err := q.Model(&models.StaffGroup{}).Where("chat_id = ?", chatID).Count(&rows).Error; err != nil {
		return err
	}
	if rows == 0 {
		return ErrStaffGroupMissing
	}
	return nil
}

// AddForwardBlock lists sourceChatID for the Staff Group in one transaction: the Staff Group row
// must exist (ErrStaffGroupMissing), the list must be below ForwardBlockMax (ErrForwardBlockFull)
// and the insert is ON CONFLICT DO NOTHING on the unique key, so a repeat reports added=false and
// changes nothing. The title is stored as given. The cached rules are dropped after the commit.
func AddForwardBlock(ctx context.Context, staffChatID, sourceChatID int64, title string, userID int64) (added bool, err error) {
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return false, err
	}
	err = q.Transaction(func(tx *gorm.DB) error {
		if err := requireStaffGroupRow(tx, staffChatID); err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&models.StaffForwardBlock{}).Where("staff_chat_id = ?", staffChatID).Count(&n).Error; err != nil {
			return err
		}
		if n >= ForwardBlockMax {
			return ErrForwardBlockFull
		}
		row := &models.StaffForwardBlock{
			StaffChatID:  staffChatID,
			SourceChatID: sourceChatID,
			SourceTitle:  title,
			CreatedBy:    userID,
			CreatedAt:    time.Now().UTC(),
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
		if result.Error != nil {
			return result.Error
		}
		added = result.RowsAffected == 1
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrStaffGroupMissing) || errors.Is(err, ErrForwardBlockFull) {
			return false, err
		}
		return false, fukuerrors.Wrapf(err, "add forward block to staff chat %d", staffChatID)
	}
	invalidateForwardBlockKeys(staffChatID)
	return added, nil
}

// RemoveForwardBlockByID deletes one listed source by its row ID, only when it belongs to the Staff
// Group, in one conditional statement, and returns the row that went. A missing ID and another
// Staff Group's row delete nothing and return nil. The cached rules are dropped only when a row
// went.
func RemoveForwardBlockByID(ctx context.Context, staffChatID, id int64) (*models.StaffForwardBlock, error) {
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.StaffForwardBlock
	result := q.Clauses(clause.Returning{}).Where("id = ? AND staff_chat_id = ?", id, staffChatID).Delete(&rows)
	if result.Error != nil {
		return nil, fukuerrors.Wrapf(result.Error, "remove forward block %d of staff chat %d", id, staffChatID)
	}
	if len(rows) != 1 {
		return nil, nil
	}
	invalidateForwardBlockKeys(staffChatID)
	return &rows[0], nil
}

// RemoveForwardBlockBySource deletes the listing of sourceChatID for the Staff Group and reports
// whether a row went. The cached rules are dropped only when a row went.
func RemoveForwardBlockBySource(ctx context.Context, staffChatID, sourceChatID int64) (removed bool, err error) {
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return false, err
	}
	result := q.Where("staff_chat_id = ? AND source_chat_id = ?", staffChatID, sourceChatID).Delete(&models.StaffForwardBlock{})
	if result.Error != nil {
		return false, fukuerrors.Wrapf(result.Error, "remove forward block source of staff chat %d", staffChatID)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	invalidateForwardBlockKeys(staffChatID)
	return true, nil
}

// SetForwardBlockAction stores the list's one action. An action outside ForwardBlockActions returns
// ErrForwardBlockAction and a Staff Group with no row ErrStaffGroupMissing, both before the write.
// The write is one column-targeted INSERT ... ON CONFLICT (staff_chat_id) DO UPDATE of action,
// updated_by and updated_at, so a value is always stored, then the cached rules are dropped.
func SetForwardBlockAction(ctx context.Context, staffChatID, userID int64, action string) error {
	if !slices.Contains(ForwardBlockActions, action) {
		return ErrForwardBlockAction
	}
	q, err := forwardBlockDB(ctx)
	if err != nil {
		return err
	}
	err = q.Transaction(func(tx *gorm.DB) error {
		if err := requireStaffGroupRow(tx, staffChatID); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "staff_chat_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"action", "updated_by", "updated_at"}),
		}).Create(&models.StaffForwardBlockSettings{
			StaffChatID: staffChatID,
			Action:      action,
			UpdatedBy:   userID,
			UpdatedAt:   time.Now().UTC(),
		}).Error
	})
	if err != nil {
		if errors.Is(err, ErrStaffGroupMissing) {
			return err
		}
		return fukuerrors.Wrapf(err, "set forward block action of staff chat %d", staffChatID)
	}
	invalidateForwardBlockKeys(staffChatID)
	return nil
}

// deleteForwardBlocksTx deletes both forward block tables' rows of one Staff Group inside tx.
func deleteForwardBlocksTx(tx *gorm.DB, staffChatID int64) error {
	if err := tx.Where("staff_chat_id = ?", staffChatID).Delete(&models.StaffForwardBlock{}).Error; err != nil {
		return err
	}
	return tx.Where("staff_chat_id = ?", staffChatID).Delete(&models.StaffForwardBlockSettings{}).Error
}
