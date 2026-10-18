package models

import "time"

// ChatRaidDetection is one chat's settings for automatic raid detection: whether the join
// surge and the message flood triggers are on and how many it takes to fire each. A chat
// without a row uses the defaults (see alita/db/raiddetection).
//
// The booleans carry no gorm default tag on purpose. With one, GORM's Create drops a Go
// false and the SQL default (true) takes its place, so an admin who switched a trigger
// off would find it on again. The defaults live in the migration's column defaults and in
// raiddetection.Defaults instead.
type ChatRaidDetection struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID         int64     `gorm:"column:chat_id;uniqueIndex:uk_chat_raid_detection_chat;not null" json:"chat_id,omitempty"`
	JoinEnabled    bool      `gorm:"column:join_enabled;not null" json:"join_enabled"`
	JoinThreshold  int       `gorm:"column:join_threshold;not null;check:chk_chat_raid_detection_join,join_threshold BETWEEN 3 AND 500" json:"join_threshold"`
	FloodEnabled   bool      `gorm:"column:flood_enabled;not null" json:"flood_enabled"`
	FloodThreshold int       `gorm:"column:flood_threshold;not null;check:chk_chat_raid_detection_flood,flood_threshold BETWEEN 3 AND 500" json:"flood_threshold"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatRaidDetection) TableName() string {
	return "chat_raid_detection"
}
