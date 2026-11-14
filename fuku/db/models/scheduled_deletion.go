package models

import "time"

// The states of a scheduled_deletions row.
const (
	// ScheduledDeletionPending is a row waiting for its delete_at.
	ScheduledDeletionPending = "pending"
	// ScheduledDeletionActing is a row a worker claimed and is deleting.
	ScheduledDeletionActing = "acting"
)

// ScheduledDeletion is one message waiting to be deleted (scheduled_deletions), unique on
// (chat, message). SentAt is the message's own date and DeleteAt the time it is due; both
// are stored as UTC at second precision so SQLite compares them as text correctly. A worker
// claims a due row (state acting, ClaimToken, ClaimedAt) and completes or releases it by
// that token. The queue is read only through fresh queries, never cached, and never part of
// backup, export, import or reset.
//
// No field carries a gorm default tag: Attempts must be writable as a Go 0, and the
// defaults live in the migration.
type ScheduledDeletion struct {
	ID         int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ChatID     int64      `gorm:"column:chat_id;not null;uniqueIndex:uk_scheduled_deletions_message,priority:1"`
	MessageID  int64      `gorm:"column:message_id;not null;uniqueIndex:uk_scheduled_deletions_message,priority:2"`
	Kind       string     `gorm:"column:kind;size:40;not null"`
	SentAt     time.Time  `gorm:"column:sent_at;not null"`
	DeleteAt   time.Time  `gorm:"column:delete_at;not null;index:idx_scheduled_deletions_due,where:state = 'pending'"`
	State      string     `gorm:"column:state;size:8;not null;check:chk_scheduled_deletions_state,state IN ('pending','acting')"`
	ClaimToken string     `gorm:"column:claim_token;size:32;not null"`
	ClaimedAt  *time.Time `gorm:"column:claimed_at;index:idx_scheduled_deletions_acting,where:state = 'acting'"`
	Attempts   int16      `gorm:"column:attempts;not null;check:chk_scheduled_deletions_attempts,attempts >= 0"`
	CreatedAt  time.Time  `gorm:"column:created_at;not null"`
}

// TableName returns the migration's table name.
func (ScheduledDeletion) TableName() string {
	return "scheduled_deletions"
}
