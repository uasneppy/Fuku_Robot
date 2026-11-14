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

// TakeHoldByID deletes the hold with this ID, but only when it belongs to this chat and
// user, and returns the row it deleted (DELETE ... RETURNING). Of two concurrent takes
// exactly one gets taken true; a wrong chat or user, or a hold that is already gone, takes
// nothing and returns (nil, false, nil).
func TakeHoldByID(ctx context.Context, id, chatID, userID int64) (*models.ChatRequirementHold, bool, error) {
	conn, err := holdsDB(ctx, "take requirement hold")
	if err != nil {
		return nil, false, err
	}
	var rows []models.ChatRequirementHold
	result := conn.Clauses(clause.Returning{}).
		Where("id = ? AND chat_id = ? AND user_id = ?", id, chatID, userID).
		Delete(&rows)
	if result.Error != nil {
		return nil, false, fukuerrors.Wrapf(result.Error, "take requirement hold %d of user %d in chat %d", id, userID, chatID)
	}
	if len(rows) != 1 {
		return nil, false, nil
	}
	return &rows[0], true, nil
}

// RestoreHold puts back a hold TakeHoldByID took, with its original ID and every column as
// read: INSERT with the explicit ID and ON CONFLICT DO NOTHING without a target, so a hold
// that exists again (under the same ID or for the same member) is left alone and the call
// returns nil. The sequence is never moved, and the old ID was never reused.
func RestoreHold(ctx context.Context, hold *models.ChatRequirementHold) error {
	conn, err := holdsDB(ctx, "restore requirement hold")
	if err != nil {
		return err
	}
	if err := conn.Clauses(clause.OnConflict{DoNothing: true}).Create(hold).Error; err != nil {
		return fukuerrors.Wrapf(err, "restore requirement hold %d of user %d in chat %d", hold.ID, hold.UserID, hold.ChatID)
	}
	return nil
}

// cleanMargin is how far before a message's date a hold must have been created, or a mute
// confirmed, for the message to count as proof that the member is clean now. It covers the
// clock difference between Telegram and this server and out-of-order processing of two
// quick messages.
const cleanMargin = 2 * time.Second

// ClearHoldOnClean deletes the member's hold when a message dated msgDate shows they have
// fixed what the rules ask, and reports whether it deleted one. A hold created within
// cleanMargin of the message, or after it, is never deleted (an older message processed
// late must not undo a fresh hold). Of the older holds, any that is not a mute goes; a mute
// goes only when it was confirmed more than cleanMargin before the message (a member the
// bot muted cannot send, so such a message proves an admin lifted the mute) or is a
// pending mute older than StaleClaim (a claim that never confirmed).
func ClearHoldOnClean(ctx context.Context, chatID, userID int64, msgDate time.Time) (bool, error) {
	conn, err := holdsDB(ctx, "clear requirement hold on clean message")
	if err != nil {
		return false, err
	}
	cutoff := msgDate.Add(-cleanMargin)
	staleBefore := time.Now().Add(-StaleClaim)
	result := conn.Where("chat_id = ? AND user_id = ? AND created_at < ?", chatID, userID, cutoff).
		Where("action <> ? OR (punished_at IS NOT NULL AND punished_at < ?) OR (punished_at IS NULL AND created_at < ?)",
			models.RequirementActionMute, cutoff, staleBefore).
		Delete(&models.ChatRequirementHold{})
	if result.Error != nil {
		return false, fukuerrors.Wrapf(result.Error, "clear requirement hold of user %d in chat %d", userID, chatID)
	}
	return result.RowsAffected > 0, nil
}

// ClearHolds deletes every hold of the member in the chat (the member left) and returns
// how many it deleted. Other members and other chats are untouched.
func ClearHolds(ctx context.Context, chatID, userID int64) (int64, error) {
	conn, err := holdsDB(ctx, "clear requirement holds")
	if err != nil {
		return 0, err
	}
	result := conn.Where("chat_id = ? AND user_id = ?", chatID, userID).Delete(&models.ChatRequirementHold{})
	if result.Error != nil {
		return 0, fukuerrors.Wrapf(result.Error, "clear requirement holds of user %d in chat %d", userID, chatID)
	}
	return result.RowsAffected, nil
}

// TakeHoldsBefore deletes and returns the member's holds created before joinDate: the
// leftovers of an earlier episode that a fresh join resets. A hold created at or after the
// join is left alone.
func TakeHoldsBefore(ctx context.Context, chatID, userID int64, joinDate time.Time) ([]models.ChatRequirementHold, error) {
	conn, err := holdsDB(ctx, "take requirement holds before a join")
	if err != nil {
		return nil, err
	}
	var rows []models.ChatRequirementHold
	result := conn.Clauses(clause.Returning{}).
		Where("chat_id = ? AND user_id = ? AND created_at < ?", chatID, userID, joinDate).
		Delete(&rows)
	if result.Error != nil {
		return nil, fukuerrors.Wrapf(result.Error, "take requirement holds of user %d in chat %d", userID, chatID)
	}
	return rows, nil
}

// firstHold runs a fresh query for one hold and returns nil, nil when there is none.
func firstHold(ctx context.Context, what, where string, args ...any) (*models.ChatRequirementHold, error) {
	conn, err := holdsDB(ctx, what)
	if err != nil {
		return nil, err
	}
	var rows []models.ChatRequirementHold
	if err := conn.Where(where, args...).Limit(1).Find(&rows).Error; err != nil {
		return nil, fukuerrors.Wrap(err, what)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// GetHoldFresh reads the member's hold in the chat straight from the database, or nil
// when there is none.
func GetHoldFresh(ctx context.Context, chatID, userID int64) (*models.ChatRequirementHold, error) {
	return firstHold(ctx, "read requirement hold", "chat_id = ? AND user_id = ?", chatID, userID)
}

// GetHoldByIDFresh reads the hold with this ID straight from the database, or nil when
// there is none.
func GetHoldByIDFresh(ctx context.Context, id int64) (*models.ChatRequirementHold, error) {
	return firstHold(ctx, "read requirement hold by ID", "id = ?", id)
}
