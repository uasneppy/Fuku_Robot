package models

import "time"

// Health values of a Staff Group link, as stored in staff_group_links.health.
const (
	// StaffHealthOK means the bot is an admin with ban rights in the linked group.
	StaffHealthOK = "ok"
	// StaffHealthBotMissing means the bot is no longer a member of the linked group.
	StaffHealthBotMissing = "bot_missing"
	// StaffHealthBotNotAdmin means the bot is a member but not an admin.
	StaffHealthBotNotAdmin = "bot_not_admin"
	// StaffHealthBotCannotRestrict means the bot is an admin without ban rights.
	StaffHealthBotCannotRestrict = "bot_cannot_restrict"
)

// StaffGroup is a Telegram group that manages other groups. OwnerUserID is the
// live creator who declared it with /setstaff; it is not unique, so one owner
// may run several Staff Groups.
type StaffGroup struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID      int64     `gorm:"column:chat_id;uniqueIndex;not null" json:"chat_id,omitempty"`
	OwnerUserID int64     `gorm:"column:owner_user_id;index:idx_staff_groups_owner_user_id;not null" json:"owner_user_id,omitempty"`
	Title       string    `gorm:"column:title;not null;default:''" json:"title,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for StaffGroup.
func (StaffGroup) TableName() string {
	return "staff_groups"
}

// StaffGroupLink ties one managed group to the Staff Group that manages it. A
// group has at most one link, and a link never points a group at itself.
// OwnerUserID is the user who made the link.
type StaffGroupLink struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	GroupChatID int64     `gorm:"column:group_chat_id;uniqueIndex;not null;check:chk_staff_link_distinct,group_chat_id <> staff_chat_id" json:"group_chat_id,omitempty"`
	StaffChatID int64     `gorm:"column:staff_chat_id;index:idx_staff_group_links_staff_chat_id;not null" json:"staff_chat_id,omitempty"`
	OwnerUserID int64     `gorm:"column:owner_user_id;not null" json:"owner_user_id,omitempty"`
	GroupTitle  string    `gorm:"column:group_title;not null;default:''" json:"group_title,omitempty"`
	Health      string    `gorm:"column:health;size:24;not null;default:'ok';check:chk_staff_link_health,health IN ('ok','bot_missing','bot_not_admin','bot_cannot_restrict')" json:"health,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for StaffGroupLink.
func (StaffGroupLink) TableName() string {
	return "staff_group_links"
}
