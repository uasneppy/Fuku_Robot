package models

import "time"

// The two roles a ChatAdminIndex row can hold, the same words as the Telegram member
// statuses they come from.
const (
	AdminIndexRoleCreator       = "creator"
	AdminIndexRoleAdministrator = "administrator"
)

// ChatAdminIndex is the bot's record that a user is the creator or a human administrator
// of a group. It is a hint for the private settings list and never authority: every pick
// is checked against a live getChatMember. Rows are read fresh and are never cached or
// part of backup, export, import or reset.
type ChatAdminIndex struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID    int64     `gorm:"column:chat_id;uniqueIndex:uk_chat_admin_index_chat_user,priority:1;not null" json:"chat_id,omitempty"`
	UserID    int64     `gorm:"column:user_id;uniqueIndex:uk_chat_admin_index_chat_user,priority:2;index:idx_chat_admin_index_user_id;not null" json:"user_id,omitempty"`
	Role      string    `gorm:"column:role;size:16;not null;check:chk_chat_admin_index_role,role IN ('creator', 'administrator')" json:"role"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatAdminIndex) TableName() string {
	return "chat_admin_index"
}
