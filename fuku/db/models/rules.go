package models

import "time"

// RulesSettings is one chat's rules message. A row saved before the media columns
// existed holds text only, and its FileID, MediaType, Buttons and MediaBelow read as zero
// (migration 20261105120000 defaults them on PostgreSQL; a NULL reads as zero on SQLite).
//
// MediaBelow carries no gorm default tag on purpose, for the reason given on
// ChatSettingsAccess: GORM's Create would turn a Go false back into the SQL default.
type RulesSettings struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatId   int64  `gorm:"column:chat_id;uniqueIndex;not null" json:"chat_id,omitempty"`
	Rules    string `gorm:"column:rules;type:text" json:"rules,omitempty"`
	RulesBtn string `gorm:"column:rules_btn" json:"rules_btn,omitempty"`
	Private  bool   `gorm:"column:private;default:false" json:"private,omitempty"`
	// FileID is the stored media file ID, empty for no media.
	FileID string `gorm:"column:file_id" json:"file_id,omitempty"`
	// MediaType is the media kind, 0 for no media.
	MediaType int `gorm:"column:media_type" json:"media_type,omitempty"`
	// Buttons are the rules message's URL buttons.
	Buttons ButtonArray `gorm:"column:btns;type:jsonb" json:"btns,omitempty"`
	// MediaBelow shows the media under the text instead of above it.
	MediaBelow bool      `gorm:"column:media_below" json:"media_below"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

func (RulesSettings) TableName() string {
	return "rules"
}

// HasContent reports whether the rules message is set. Red stub: it is filled in by the
// next commit.
func (r RulesSettings) HasContent() bool {
	return false
}
