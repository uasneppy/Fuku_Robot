package models

import "time"

// StaffForwardBlock is one listed source of a Staff Group's forward blocklist: a message forwarded
// from SourceChatID into a group linked to the Staff Group is deleted. SourceChatID is always a
// negative Telegram chat ID. SourceTitle is the title known when the source was added and may be
// empty or out of date. A source is listed once per Staff Group.
type StaffForwardBlock struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"-"`
	StaffChatID  int64     `gorm:"column:staff_chat_id;not null;uniqueIndex:uk_staff_forward_blocks_source,priority:1" json:"staff_chat_id,omitempty"`
	SourceChatID int64     `gorm:"column:source_chat_id;not null;uniqueIndex:uk_staff_forward_blocks_source,priority:2;check:chk_staff_forward_blocks_source,source_chat_id < 0" json:"source_chat_id,omitempty"`
	SourceTitle  string    `gorm:"column:source_title;not null" json:"source_title,omitempty"`
	CreatedBy    int64     `gorm:"column:created_by;not null" json:"created_by,omitempty"`
	CreatedAt    time.Time `gorm:"column:created_at;not null" json:"created_at,omitempty"`
}

// TableName returns the migration's table name for StaffForwardBlock.
func (StaffForwardBlock) TableName() string {
	return "staff_forward_blocks"
}

// StaffForwardBlockSettings is the one action of a Staff Group's forward blocklist: none (delete
// the forward only), warn, mute, kick or ban. A Staff Group without a row means none. The field
// carries no gorm default tag, so a value is always written by the column-targeted upsert.
type StaffForwardBlockSettings struct {
	StaffChatID int64     `gorm:"column:staff_chat_id;primaryKey;autoIncrement:false" json:"staff_chat_id,omitempty"`
	Action      string    `gorm:"column:action;type:varchar(8);not null;check:chk_staff_forward_block_action,action IN ('none','warn','mute','kick','ban')" json:"action,omitempty"`
	UpdatedBy   int64     `gorm:"column:updated_by;not null" json:"updated_by,omitempty"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for StaffForwardBlockSettings.
func (StaffForwardBlockSettings) TableName() string {
	return "staff_forward_block_settings"
}
