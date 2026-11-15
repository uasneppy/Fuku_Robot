package models

import "time"

// ChatSettingsAccess is one chat's owner switch "Admins can change settings". A chat
// without a row means owner only. The row is read fresh from the database on every
// authority decision and is never cached (see fuku/db/menuaccess).
//
// AdminsAllowed carries no gorm default tag on purpose. With one, GORM's Create drops a Go
// false and the SQL default takes its place, so an owner who switched admins off could find
// the switch in a state they did not choose. The default lives in the migration's column
// default (FALSE, owner only) and in the repository's missing-row rule instead.
type ChatSettingsAccess struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID        int64     `gorm:"column:chat_id;uniqueIndex:uk_chat_settings_access_chat;not null" json:"chat_id,omitempty"`
	AdminsAllowed bool      `gorm:"column:admins_allowed;not null" json:"admins_allowed"`
	UpdatedBy     int64     `gorm:"column:updated_by;not null" json:"updated_by"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatSettingsAccess) TableName() string {
	return "chat_settings_access"
}
