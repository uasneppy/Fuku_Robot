package models

import "time"

// DeleteRule is one chat's delay for one kind of message (chat_delete_rules): kind is
// "service:<type>" or "bot:<kind>", DelaySecs is 0 for "delete now" and 1..86400 for
// "delete after N seconds". A chat with no row for a kind keeps (or, for a bot kind, never
// deletes) that kind. Rules are read through the cache and never part of backup, export,
// import or reset (see fuku/db/deletion).
//
// DelaySecs carries no gorm default tag on purpose: a Go 0 is "delete now" and must be
// written, not replaced by a column default.
type DeleteRule struct {
	ChatID    int64     `gorm:"column:chat_id;primaryKey;autoIncrement:false"`
	Kind      string    `gorm:"column:kind;primaryKey;size:40;not null"`
	DelaySecs int       `gorm:"column:delay_secs;not null;check:chk_chat_delete_rules_delay,delay_secs BETWEEN 0 AND 86400"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

// TableName returns the migration's table name.
func (DeleteRule) TableName() string {
	return "chat_delete_rules"
}
