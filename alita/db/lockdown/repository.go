package lockdown

// Lockdown state (chat_lockdowns, chat_lockdown_joiners) is authority data: whether
// a group is locked decides who may talk, who is banned on joining and what a lift
// restores. It is always read fresh from the database on every replica and never
// cached, so no cache key is read here and none needs invalidation. Writes use
// db.DB without a request context, so a cancelled context never loses a record
// write. A cache for these reads would need skipLocal and a DeleteCache on every
// write below.

import (
	"errors"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// unfinishedJoinerStates are the joiner states in which a lockdown still has work
// to do. A lift is finished only when no joiner row of the lockdown is in one.
var unfinishedJoinerStates = []string{
	models.JoinerStatePending,
	models.JoinerStateActing,
	models.JoinerStateBanned,
	models.JoinerStateUnbanning,
}

// now is the time written to a row, cut to the microsecond so the value round-trips
// exactly through PostgreSQL's microsecond timestamps.
func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

// isUniqueViolation reports whether err says a unique constraint or index refused a
// write, on PostgreSQL (SQLSTATE 23505) and on SQLite.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "23505") ||
		strings.Contains(text, "unique constraint") ||
		strings.Contains(text, "duplicate key")
}

// Start records a new active lockdown. It is the one-active-lockdown-per-chat
// guarantee: the insert is refused by the uk_chat_lockdowns_active partial unique
// index while the chat has an active row, so exactly one caller ever sees true for
// a chat, on any replica. false with no error means the chat is already locked and
// nothing was written. On true the row's ID, State and timestamps are filled in.
func Start(row *models.ChatLockdown) (bool, error) {
	stamp := now()
	row.State = models.LockdownStateActive
	row.CreatedAt = stamp
	row.UpdatedAt = stamp
	result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return false, nil
		}
		log.Errorf("[Lockdown] Start: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "start lockdown of chat %d", row.ChatID)
	}
	return result.RowsAffected == 1, nil
}

// GetActiveFresh reads the chat's active lockdown straight from the database,
// whether or not Telegram confirmed its lock. It returns (nil, nil) when the chat
// has none.
func GetActiveFresh(chatID int64) (*models.ChatLockdown, error) {
	var row models.ChatLockdown
	err := db.DB.Where("chat_id = ? AND state = ?", chatID, models.LockdownStateActive).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Lockdown] GetActiveFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get active lockdown of chat %d", chatID)
	}
	return &row, nil
}

// GetFresh reads one lockdown by its row ID straight from the database. It returns
// (nil, nil) when there is no such row.
func GetFresh(id uint) (*models.ChatLockdown, error) {
	var row models.ChatLockdown
	err := db.DB.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Lockdown] GetFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get lockdown %d", id)
	}
	return &row, nil
}

// GetCurrentFresh reads the lockdown a status command reports, straight from the
// database: the chat's active row, else its lifting row with the highest ID. A lifted
// lockdown is over and never current. It returns (nil, nil) when the chat has
// neither.
func GetCurrentFresh(chatID int64) (*models.ChatLockdown, error) {
	active, err := GetActiveFresh(chatID)
	if err != nil || active != nil {
		return active, err
	}
	var rows []models.ChatLockdown
	err = db.DB.Where("chat_id = ? AND state = ?", chatID, models.LockdownStateLifting).
		Order("id DESC").Limit(1).Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] GetCurrentFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get current lockdown of chat %d", chatID)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// ListLiftingFresh reads every lockdown that is being lifted, oldest first, straight
// from the database: the lifts the worker still has joiners to unban for.
func ListLiftingFresh() ([]models.ChatLockdown, error) {
	var rows []models.ChatLockdown
	err := db.DB.Where("state = ?", models.LockdownStateLifting).Order("id").Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListLiftingFresh: %v", err)
		return nil, alitaerrors.Wrap(err, "list lifting lockdowns")
	}
	return rows, nil
}

// TallyJoiners counts the joiner rows of one lockdown by state, straight from the
// database. A state with no rows is absent from the map, and a lockdown with no
// joiner rows gets an empty map.
func TallyJoiners(lockdownID uint) (map[string]int64, error) {
	var counts []struct {
		State string
		N     int64
	}
	err := db.DB.Model(&models.LockdownJoiner{}).
		Select("state, COUNT(*) AS n").
		Where("lockdown_id = ?", lockdownID).
		Group("state").
		Scan(&counts).Error
	if err != nil {
		log.Errorf("[Lockdown] TallyJoiners: %v", err)
		return nil, alitaerrors.Wrapf(err, "tally joiners of lockdown %d", lockdownID)
	}
	tally := make(map[string]int64, len(counts))
	for _, count := range counts {
		tally[count.State] = count.N
	}
	return tally, nil
}

// ConfirmLocked records that Telegram confirmed the lock. It matches only an active
// row that is not confirmed yet, and reports whether this call wrote the time.
func ConfirmLocked(id uint) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdown{}).
		Where("id = ? AND state = ? AND locked_at IS NULL", id, models.LockdownStateActive).
		Updates(map[string]any{"locked_at": stamp, "updated_at": stamp})
	if result.Error != nil {
		log.Errorf("[Lockdown] ConfirmLocked: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "confirm lock of lockdown %d", id)
	}
	return result.RowsAffected == 1, nil
}

// DeleteUnconfirmed removes an active lockdown whose lock Telegram never confirmed,
// so a refused lock leaves no row behind. It never touches a confirmed row, and
// reports whether it deleted one.
func DeleteUnconfirmed(id uint) (bool, error) {
	result := db.DB.
		Where("id = ? AND state = ? AND locked_at IS NULL", id, models.LockdownStateActive).
		Delete(&models.ChatLockdown{})
	if result.Error != nil {
		log.Errorf("[Lockdown] DeleteUnconfirmed: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "delete unconfirmed lockdown %d", id)
	}
	return result.RowsAffected == 1, nil
}

// BeginLift moves an active lockdown to lifting and records who lifted it. It is
// the one-lifter guarantee: a single conditional update matches only while the row
// is active, so exactly one caller ever sees true. manualChange records that the
// live permissions were no longer the locked set when the lift was made.
func BeginLift(id uint, by int64, byName string, manualChange bool) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdown{}).
		Where("id = ? AND state = ?", id, models.LockdownStateActive).
		Updates(map[string]any{
			"state":           models.LockdownStateLifting,
			"lifted_by":       by,
			"lifted_by_name":  byName,
			"lift_started_at": stamp,
			"manual_change":   manualChange,
			"updated_at":      stamp,
		})
	if result.Error != nil {
		log.Errorf("[Lockdown] BeginLift: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "begin lift of lockdown %d", id)
	}
	return result.RowsAffected == 1, nil
}

// FinishLift marks a lifting lockdown lifted. It matches only while no joiner row of
// the lockdown is pending, acting, banned or unbanning, in one conditional update,
// and reports whether this call finished it. false with no error means the lockdown
// is not lifting or still has joiners to handle.
func FinishLift(id uint) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdown{}).
		Where("id = ? AND state = ? AND NOT EXISTS ("+
			"SELECT 1 FROM chat_lockdown_joiners WHERE lockdown_id = ? AND state IN ?)",
			id, models.LockdownStateLifting, id, unfinishedJoinerStates).
		Updates(map[string]any{
			"state":      models.LockdownStateLifted,
			"lifted_at":  stamp,
			"updated_at": stamp,
		})
	if result.Error != nil {
		log.Errorf("[Lockdown] FinishLift: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "finish lift of lockdown %d", id)
	}
	return result.RowsAffected == 1, nil
}
