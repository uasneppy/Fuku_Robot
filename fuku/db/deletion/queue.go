package deletion

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// The queue (scheduled_deletions) is read and written only through fresh queries and is
// never cached, so no cache.DeleteCache applies to it. Times are stored as UTC at second
// precision: SQLite compares times as text, so mixed offsets or fractional digits would
// mis-order them.

var (
	// ErrInvalidEntry is returned for an Entry with no chat or a non-group (not negative) chat ID,
	// no message or no sent time.
	ErrInvalidEntry = errors.New("invalid scheduled deletion entry")
	// ErrInvalidToken is returned for an empty or over-long claim token. An empty token would
	// match every pending row, which carries an empty token.
	ErrInvalidToken = errors.New("invalid claim token")
	// ErrInvalidBatch is returned for a claim limit or a retry maximum that is not positive.
	ErrInvalidBatch = errors.New("invalid batch size")
	// ErrInvalidRetryAt is returned by a counted Release with no retry time: the counted tries
	// must be spaced, so the caller names when the row is due again.
	ErrInvalidRetryAt = errors.New("invalid retry time")
)

// maxTokenLen is the width of the claim_token column.
const maxTokenLen = 32

// staffGroupLookup reads the Staff Group of a chat fresh from the database. It is a package
// variable so a test can fail the read without a mock library.
var staffGroupLookup = staff.GetStaffGroupFresh

// Entry is one message to schedule: the chat, the message, its kind, the message's own date
// and the delay in seconds after it (0 means due at SentAt).
type Entry struct {
	ChatID    int64
	MessageID int64
	Kind      string
	SentAt    time.Time
	DelaySecs int
}

// utcSecond returns t as UTC truncated to the second.
func utcSecond(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}

// leastExpr returns the SQL expression that keeps the earlier of the stored and the new
// delete_at on conflict. The table qualifier is required on PostgreSQL: an unqualified
// delete_at is ambiguous there. SQLite spells the two-argument minimum MIN.
func leastExpr(tx *gorm.DB) clause.Expr {
	if tx.Name() == "postgres" {
		return gorm.Expr("LEAST(scheduled_deletions.delete_at, EXCLUDED.delete_at)")
	}
	return gorm.Expr("MIN(scheduled_deletions.delete_at, excluded.delete_at)")
}

// Schedule queues one message for deletion at SentAt plus DelaySecs and reports whether a
// row was written. The kind, the delay (ValidateEntryDelay: the range for a service or bot kind,
// 0 for the two immediate member kinds, the presets for self-destruct) and the IDs are validated before
// any read or write; a chat ID that is not negative is refused with ErrInvalidEntry (group and
// supergroup IDs are negative, so a private chat is never queued, whatever the caller checked). A chat that is a Staff Group is refused after a fresh Staff Group read:
// the result is (false, nil) and nothing is written, and a failed read is (false, err) and
// writes nothing (fail closed). Scheduling one (chat, message) again keeps the earlier
// delete_at and leaves the kind and sent time of the first insert as they were.
func Schedule(ctx context.Context, e Entry) (bool, error) {
	if !ValidKind(e.Kind) {
		return false, ErrUnknownKind
	}
	if err := ValidateEntryDelay(e.Kind, e.DelaySecs); err != nil {
		return false, err
	}
	if e.ChatID >= 0 || e.MessageID <= 0 || e.SentAt.IsZero() {
		return false, ErrInvalidEntry
	}
	if db.DB == nil {
		return false, fukuerrors.Wrap(errNoDatabase, "schedule deletion")
	}
	group, err := staffGroupLookup(e.ChatID)
	if err != nil {
		return false, fukuerrors.Wrapf(err, "check chat %d for a Staff Group before scheduling", e.ChatID)
	}
	if group != nil {
		return false, nil
	}
	sentAt := utcSecond(e.SentAt)
	row := &models.ScheduledDeletion{
		ChatID:     e.ChatID,
		MessageID:  e.MessageID,
		Kind:       e.Kind,
		SentAt:     sentAt,
		DeleteAt:   sentAt.Add(time.Duration(e.DelaySecs) * time.Second),
		State:      models.ScheduledDeletionPending,
		ClaimToken: "",
		Attempts:   0,
		CreatedAt:  utcSecond(time.Now()),
	}
	err = db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}, {Name: "message_id"}},
		DoUpdates: clause.Set{
			{Column: clause.Column{Name: "delete_at"}, Value: leastExpr(db.DB)},
		},
	}).Create(row).Error
	if err != nil {
		return false, fukuerrors.Wrapf(err, "schedule deletion in chat %d", e.ChatID)
	}
	return true, nil
}

// ScheduleBatchMax is the most rows ScheduleBatch writes in one INSERT statement.
const ScheduleBatchMax = 100

// batchKey identifies one queued message.
type batchKey struct{ chat, message int64 }

// ScheduleBatch queues many messages in as few statements as possible and returns how many
// entries it wrote. Every entry is validated before any read or write exactly as Schedule
// validates one (kind, ValidateEntryDelay, a negative chat ID, a positive message ID, a sent
// time); the first refusal is returned wrapped with its index and nothing is written. Entries
// naming one (chat, message) are merged in Go to the earliest delete_at (and the first entry's
// kind and sent time), because one PostgreSQL statement that touches a key twice fails whole. The
// Staff Group of each distinct chat is read once, fresh: the entries of a Staff Group chat are
// dropped without an error (layer two of DEL-07), and those of a chat whose read failed are dropped
// with that error collected, so a failed read keeps the message and the other chats are still
// written. The rows are sorted by (chat, message), so two concurrent batches take their row locks
// in one order, and written in chunks of at most ScheduleBatchMax rows, each one multi-row INSERT
// that keeps the earlier delete_at of an existing row and never rewrites its kind or sent time. A
// database error stops at that chunk and is returned, joined with the collected read errors, with
// the number of entries written before it. The queue is read and written fresh, so no cache key
// applies, and no message ID is logged next to a chat ID.
func ScheduleBatch(ctx context.Context, entries []Entry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	for i, e := range entries {
		if err := validateEntry(e); err != nil {
			return 0, fukuerrors.Wrapf(err, "schedule batch entry %d", i)
		}
	}
	if db.DB == nil {
		return 0, fukuerrors.Wrap(errNoDatabase, "schedule deletion batch")
	}
	rows, chats := mergeBatch(entries)
	var errs []error
	allowed := make(map[int64]bool, len(chats))
	for _, chatID := range chats {
		group, err := staffGroupLookup(chatID)
		if err != nil {
			errs = append(errs, fukuerrors.Wrapf(err, "check chat %d for a Staff Group before scheduling", chatID))
			continue
		}
		allowed[chatID] = group == nil
	}
	writable := rows[:0]
	for _, row := range rows {
		if allowed[row.ChatID] {
			writable = append(writable, row)
		}
	}
	wrote := 0
	for chunk := range slices.Chunk(writable, ScheduleBatchMax) {
		err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "chat_id"}, {Name: "message_id"}},
			DoUpdates: clause.Set{
				{Column: clause.Column{Name: "delete_at"}, Value: leastExpr(db.DB)},
			},
		}).Create(&chunk).Error
		if err != nil {
			errs = append(errs, fukuerrors.Wrapf(err, "schedule deletion batch of %d rows in chat %d", len(chunk), chunk[0].ChatID))
			return wrote, errors.Join(errs...)
		}
		wrote += len(chunk)
	}
	return wrote, errors.Join(errs...)
}

// validateEntry applies Schedule's checks to one entry.
func validateEntry(e Entry) error {
	if !ValidKind(e.Kind) {
		return ErrUnknownKind
	}
	if err := ValidateEntryDelay(e.Kind, e.DelaySecs); err != nil {
		return err
	}
	if e.ChatID >= 0 || e.MessageID <= 0 || e.SentAt.IsZero() {
		return ErrInvalidEntry
	}
	return nil
}

// mergeBatch turns validated entries into queue rows, one per (chat, message), keeping the earliest
// delete_at and the first entry's kind and sent time, sorted by (chat, message). It also returns
// the distinct chat IDs in ascending order.
func mergeBatch(entries []Entry) ([]models.ScheduledDeletion, []int64) {
	created := utcSecond(time.Now())
	byKey := make(map[batchKey]*models.ScheduledDeletion, len(entries))
	for _, e := range entries {
		sentAt := utcSecond(e.SentAt)
		deleteAt := sentAt.Add(time.Duration(e.DelaySecs) * time.Second)
		key := batchKey{e.ChatID, e.MessageID}
		if row, ok := byKey[key]; ok {
			if deleteAt.Before(row.DeleteAt) {
				row.DeleteAt = deleteAt
			}
			continue
		}
		byKey[key] = &models.ScheduledDeletion{
			ChatID: e.ChatID, MessageID: e.MessageID, Kind: e.Kind, SentAt: sentAt, DeleteAt: deleteAt,
			State: models.ScheduledDeletionPending, ClaimToken: "", Attempts: 0, CreatedAt: created,
		}
	}
	rows := make([]models.ScheduledDeletion, 0, len(byKey))
	for _, row := range byKey {
		rows = append(rows, *row)
	}
	slices.SortFunc(rows, func(a, b models.ScheduledDeletion) int {
		if c := cmp.Compare(a.ChatID, b.ChatID); c != 0 {
			return c
		}
		return cmp.Compare(a.MessageID, b.MessageID)
	})
	var chats []int64
	for _, row := range rows {
		if len(chats) == 0 || chats[len(chats)-1] != row.ChatID {
			chats = append(chats, row.ChatID)
		}
	}
	return rows, chats
}

// CountPending counts the queued rows (pending and acting) of one chat and kind, up to limit, with
// SELECT count(*) FROM (SELECT 1 ... LIMIT limit): the unique (chat, message) index serves it, and
// the count never reads past the limit, so it is exact below the limit and equals it at or above
// it. A limit of 0 or less returns 0 without a query. The queue is read fresh, so no cache key
// applies.
func CountPending(ctx context.Context, chatID int64, kind string, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	if db.DB == nil {
		return 0, fukuerrors.Wrap(errNoDatabase, "count queued deletions")
	}
	var n int64
	err := db.DB.WithContext(ctx).Raw(
		`SELECT count(*) FROM (SELECT 1 FROM scheduled_deletions WHERE chat_id = ? AND kind = ? LIMIT ?) s`,
		chatID, kind, limit,
	).Scan(&n).Error
	if err != nil {
		return 0, fukuerrors.Wrapf(err, "count queued deletions of chat %d", chatID)
	}
	return n, nil
}

// DropPendingMessage deletes the pending row of one (chat, message, kind) and nothing else: an
// acting row, another kind's row of the same message and the same message ID in another chat are
// left alone. It returns how many rows it deleted (0 or 1) and is how a pinned message leaves the
// self-destruct queue. The queue is written fresh, so no cache key applies.
func DropPendingMessage(ctx context.Context, chatID, messageID int64, kind string) (int64, error) {
	if db.DB == nil {
		return 0, fukuerrors.Wrap(errNoDatabase, "drop a queued deletion")
	}
	result := db.DB.WithContext(ctx).
		Where("chat_id = ? AND message_id = ? AND kind = ? AND state = ?", chatID, messageID, kind, models.ScheduledDeletionPending).
		Delete(&models.ScheduledDeletion{})
	if result.Error != nil {
		return 0, fukuerrors.Wrapf(result.Error, "drop a queued deletion of chat %d", chatID)
	}
	return result.RowsAffected, nil
}

// NewClaimToken returns a fresh claim token: 16 random bytes as 32 hex characters. Use one
// token per claim, because ClaimDue reads its rows back by token.
func NewClaimToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("deletion.NewClaimToken: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(buf[:])
}

// checkToken refuses an empty or over-long token.
func checkToken(token string) error {
	if token == "" || len(token) > maxTokenLen {
		return ErrInvalidToken
	}
	return nil
}

// ClaimDue claims up to limit pending rows whose delete_at is at or before now, oldest
// first, with one conditional UPDATE that sets state acting, the token and claimed_at, and
// reads back the rows that carry the token. The UPDATE re-checks that a row is still
// pending, so among concurrent callers each row has exactly one winner. Use a fresh token
// for every call: a repeated token would read back rows an earlier call still holds.
func ClaimDue(ctx context.Context, now time.Time, limit int, token string) ([]models.ScheduledDeletion, error) {
	if err := checkToken(token); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, ErrInvalidBatch
	}
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errNoDatabase, "claim due deletions")
	}
	at := utcSecond(now)
	err := db.DB.WithContext(ctx).Exec(
		`UPDATE scheduled_deletions SET state = ?, claim_token = ?, claimed_at = ?
		 WHERE id IN (
		   SELECT id FROM scheduled_deletions
		   WHERE state = ? AND delete_at <= ?
		   ORDER BY delete_at, id LIMIT ?
		 ) AND state = ?`,
		models.ScheduledDeletionActing, token, at,
		models.ScheduledDeletionPending, at, limit,
		models.ScheduledDeletionPending,
	).Error
	if err != nil {
		return nil, fukuerrors.Wrap(err, "claim due deletions")
	}
	var rows []models.ScheduledDeletion
	if err := db.DB.WithContext(ctx).
		Where("claim_token = ? AND state = ?", token, models.ScheduledDeletionActing).
		Order("delete_at, id").Find(&rows).Error; err != nil {
		return nil, fukuerrors.Wrap(err, "read claimed deletions")
	}
	return rows, nil
}

// Complete deletes the listed rows that still carry token in state acting. A row that a
// stale release handed to another claimer carries that claimer's token, so a late
// completion of the first claim removes nothing.
func Complete(ctx context.Context, token string, ids []int64) error {
	if err := checkToken(token); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "complete deletions")
	}
	err := db.DB.WithContext(ctx).
		Where("claim_token = ? AND state = ? AND id IN ?", token, models.ScheduledDeletionActing, ids).
		Delete(&models.ScheduledDeletion{}).Error
	if err != nil {
		return fukuerrors.Wrapf(err, "complete %d deletions", len(ids))
	}
	return nil
}

// Release returns the listed rows that still carry token to pending, with the token and
// the claim time cleared. With countAttempt it adds one attempt to each, moves each one's
// delete_at forward to retryAt (UTC, whole seconds) so the counted tries are spaced instead
// of being re-claimed on the next tick, and deletes instead every row whose counted attempts
// would reach maxAttempts, so the third counted failure drops the row. Without countAttempt
// (a rate limit, a shutdown) no attempt is counted, delete_at is left as it was and retryAt
// is ignored. retryAt is computed by the caller, so SQLite and PostgreSQL run the same
// statement and a test clock applies; a counted release needs a non-zero one.
func Release(ctx context.Context, token string, ids []int64, countAttempt bool, maxAttempts int, retryAt time.Time) error {
	if err := checkToken(token); err != nil {
		return err
	}
	if countAttempt && maxAttempts <= 0 {
		return ErrInvalidBatch
	}
	if countAttempt && retryAt.IsZero() {
		return ErrInvalidRetryAt
	}
	if len(ids) == 0 {
		return nil
	}
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "release deletions")
	}
	held := func(tx *gorm.DB) *gorm.DB {
		return tx.Where("claim_token = ? AND state = ? AND id IN ?", token, models.ScheduledDeletionActing, ids)
	}
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"state":       models.ScheduledDeletionPending,
			"claim_token": "",
			"claimed_at":  nil,
		}
		if countAttempt {
			if err := held(tx.Model(&models.ScheduledDeletion{})).
				Where("attempts + 1 >= ?", maxAttempts).
				Delete(&models.ScheduledDeletion{}).Error; err != nil {
				return err
			}
			updates["attempts"] = gorm.Expr("attempts + 1")
			updates["delete_at"] = utcSecond(retryAt)
		}
		return held(tx.Model(&models.ScheduledDeletion{})).Updates(updates).Error
	})
	if err != nil {
		return fukuerrors.Wrapf(err, "release %d deletions", len(ids))
	}
	return nil
}

// ReleaseStale returns acting rows claimed before cutoff to pending, with the token and the
// claim time cleared and no attempt counted: the claimer stopped without finishing. It
// returns how many rows it released.
func ReleaseStale(ctx context.Context, cutoff time.Time) (int64, error) {
	if db.DB == nil {
		return 0, fukuerrors.Wrap(errNoDatabase, "release stale deletion claims")
	}
	result := db.DB.WithContext(ctx).Model(&models.ScheduledDeletion{}).
		Where("state = ? AND (claimed_at IS NULL OR claimed_at < ?)", models.ScheduledDeletionActing, utcSecond(cutoff)).
		Updates(map[string]any{
			"state":       models.ScheduledDeletionPending,
			"claim_token": "",
			"claimed_at":  nil,
		})
	if result.Error != nil {
		return 0, fukuerrors.Wrap(result.Error, "release stale deletion claims")
	}
	return result.RowsAffected, nil
}

// likeEscaper escapes the characters LIKE treats specially, with a backslash.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// DropChatPending deletes the pending rows of a chat whose kind starts with kindPrefix (an
// empty prefix drops every pending row of the chat). Acting rows and other chats are left
// alone. It returns how many rows it deleted.
func DropChatPending(ctx context.Context, chatID int64, kindPrefix string) (int64, error) {
	if db.DB == nil {
		return 0, fukuerrors.Wrap(errNoDatabase, "drop pending deletions")
	}
	query := db.DB.WithContext(ctx).Where("chat_id = ? AND state = ?", chatID, models.ScheduledDeletionPending)
	if kindPrefix != "" {
		query = query.Where(`kind LIKE ? ESCAPE '\'`, likeEscaper.Replace(kindPrefix)+"%")
	}
	result := query.Delete(&models.ScheduledDeletion{})
	if result.Error != nil {
		return 0, fukuerrors.Wrapf(result.Error, "drop pending deletions of chat %d", chatID)
	}
	return result.RowsAffected, nil
}
