package models

import "time"

// ChatConversation is one chat's conversation setting: whether the bot answers members who
// address it, which provider it asks, and who may talk to it. A chat without a row means off,
// anthropic, everyone. It is read through the cache key chat_conv:<chat> (see fuku/db/chatconv).
// The boolean carries no default tag, so a Go false is always written.
type ChatConversation struct {
	ChatID    int64     `gorm:"column:chat_id;primaryKey;autoIncrement:false" json:"chat_id"`
	Enabled   bool      `gorm:"column:enabled;not null" json:"enabled"`
	Provider  string    `gorm:"column:provider;not null;size:16;check:chk_chat_conversations_provider,provider IN ('anthropic', 'google')" json:"provider"`
	Audience  int16     `gorm:"column:audience;not null;check:chk_chat_conversations_audience,audience BETWEEN 0 AND 1" json:"audience"`
	UpdatedBy int64     `gorm:"column:updated_by;not null" json:"updated_by"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatConversation) TableName() string {
	return "chat_conversations"
}
