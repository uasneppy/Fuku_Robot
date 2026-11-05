package memberreq

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// A hold is one member's requirement episode in one chat: the latch that makes a
// violation punished once however many replicas see it, and the record of a mute the bot
// placed. Everything in this file reads and writes the database fresh. It never touches
// the cache or Redis, so no cache key is deleted and the latch holds across replicas and
// with no Redis at all (D-07, D-15). Errors name chat and user IDs only.

// StaleClaim is how old a pending hold (claimed, never confirmed punished) must be before
// ReclaimStaleHold may take it over. It is a variable only so tests can shorten it.
var StaleClaim = 2 * time.Minute

// holdsDB returns the database handle bound to ctx, or an error before the database exists.
func holdsDB(ctx context.Context, what string) (*gorm.DB, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errors.New("database not initialized"), what)
	}
	return db.DB.WithContext(ctx), nil
}

// ClaimHold inserts hold with INSERT ... ON CONFLICT (chat_id, user_id) DO NOTHING and
// reports whether this call's insert landed. Of any number of concurrent claims of one
// member, on one replica or several, exactly one gets won true; the others get false and
// no error. The winner's hold carries its new ID.
func ClaimHold(ctx context.Context, hold *models.ChatRequirementHold) (won bool, err error) {
	conn, err := holdsDB(ctx, "claim requirement hold")
	if err != nil {
		return false, err
	}
	result := conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).Create(hold)
	if result.Error != nil {
		return false, fukuerrors.Wrapf(result.Error, "claim requirement hold of user %d in chat %d", hold.UserID, hold.ChatID)
	}
	return result.RowsAffected == 1, nil
}

// ReclaimStaleHold takes over a pending hold that is older than StaleClaim, with one
// conditional UPDATE: it sets rules, action, mute_until and welcome_held from fresh, clears
// the notice ID and sets created_at to now, only where the member's hold is not punished
// and was created before now minus StaleClaim. won is true for the one caller whose update
// matched, with the hold's ID; a punished hold and a younger pending one are never
// reclaimed.
func ReclaimStaleHold(ctx context.Context, fresh *models.ChatRequirementHold) (won bool, id int64, err error) {
	conn, err := holdsDB(ctx, "reclaim requirement hold")
	if err != nil {
		return false, 0, err
	}
	now := time.Now()
	var rows []models.ChatRequirementHold
	result := conn.Model(&rows).Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}}}).
		Where("chat_id = ? AND user_id = ? AND punished_at IS NULL AND created_at < ?", fresh.ChatID, fresh.UserID, now.Add(-StaleClaim)).
		Updates(map[string]any{
			"rules":         fresh.Rules,
			"action":        fresh.Action,
			"mute_until":    fresh.MuteUntil,
			"welcome_held":  fresh.WelcomeHeld,
			"notice_msg_id": 0,
			"created_at":    now,
		})
	if result.Error != nil {
		return false, 0, fukuerrors.Wrapf(result.Error, "reclaim requirement hold of user %d in chat %d", fresh.UserID, fresh.ChatID)
	}
	if result.RowsAffected != 1 || len(rows) != 1 {
		return false, 0, nil
	}
	return true, rows[0].ID, nil
}

// updateHold runs one UPDATE of values on the hold with this ID, narrowed by where. It
// returns how many rows matched, so a caller can tell "changed" from "nothing to change".
func updateHold(ctx context.Context, id int64, where string, values map[string]any, what string) (bool, error) {
	conn, err := holdsDB(ctx, what)
	if err != nil {
		return false, err
	}
	result := conn.Model(&models.ChatRequirementHold{}).Where("id = ? AND "+where, id).Updates(values)
	if result.Error != nil {
		return false, fukuerrors.Wrapf(result.Error, "%s of hold %d", what, id)
	}
	return result.RowsAffected == 1, nil
}

// MarkHoldPunished stamps punished_at on the hold with this ID, once Telegram (or the warn
// write) confirmed the punishment. It reports whether a row changed.
func MarkHoldPunished(ctx context.Context, id int64) (bool, error) {
	return updateHold(ctx, id, "1 = 1", map[string]any{"punished_at": time.Now()}, "mark requirement hold punished")
}

// SetHoldNotice stores the ID of the notice message on the hold with this ID.
func SetHoldNotice(ctx context.Context, id, msgID int64) (bool, error) {
	return updateHold(ctx, id, "1 = 1", map[string]any{"notice_msg_id": msgID}, "store requirement hold notice")
}

// SetHoldWelcomeHeld records on the hold with this ID that a join mute held the welcome.
func SetHoldWelcomeHeld(ctx context.Context, id int64) (bool, error) {
	return updateHold(ctx, id, "1 = 1", map[string]any{"welcome_held": true}, "mark requirement hold welcome held")
}

// BackoffHold refreshes created_at of a pending hold, so it stays as a backoff record
// that ReclaimStaleHold may take over only after StaleClaim. A punished hold is left alone.
func BackoffHold(ctx context.Context, id int64) (bool, error) {
	return updateHold(ctx, id, "punished_at IS NULL", map[string]any{"created_at": time.Now()}, "back off requirement hold")
}

// RevertHoldToPending turns a punished hold back into a pending one created now: the row
// stays only as the backoff record after a punishment that was lifted again. A pending
// hold is left alone.
func RevertHoldToPending(ctx context.Context, id int64) (bool, error) {
	return updateHold(ctx, id, "punished_at IS NOT NULL",
		map[string]any{"punished_at": nil, "created_at": time.Now()}, "revert requirement hold to pending")
}
