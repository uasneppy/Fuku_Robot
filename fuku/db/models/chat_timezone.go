package models

import "time"

// ChatTimezone is one chat's choice of timezone for the times the bot shows: an IANA zone
// name or a fixed offset written as UTC+hh:mm. A chat without a row uses the bot's default.
// It is read through the cache key chat_tz:<chat> (see fuku/db/timezone).
type ChatTimezone struct {
	ChatID    int64     `gorm:"column:chat_id;primaryKey;autoIncrement:false" json:"chat_id"`
	TZ        string    `gorm:"column:tz;not null;size:64;check:chk_chat_timezones_tz,length(tz) BETWEEN 1 AND 64" json:"tz"`
	UpdatedBy int64     `gorm:"column:updated_by;not null" json:"updated_by"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatTimezone) TableName() string {
	return "chat_timezones"
}
