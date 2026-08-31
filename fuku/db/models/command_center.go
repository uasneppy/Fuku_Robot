package models

import "time"

// CommandCenter is a central admin chat that manages connected chats.
type CommandCenter struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID      int64     `gorm:"column:chat_id;uniqueIndex;not null" json:"chat_id"`
	OwnerID     int64     `gorm:"column:owner_id;uniqueIndex;not null" json:"owner_id"`
	Name        string    `gorm:"column:name;not null;size:64" json:"name"`
	Description string    `gorm:"column:description;default:''" json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the table name for CommandCenter.
func (CommandCenter) TableName() string {
	return "command_centers"
}

// CommandCenterChat links a Telegram chat to a command center.
type CommandCenterChat struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID    int64     `gorm:"column:chat_id;uniqueIndex;not null" json:"chat_id"`
	CommandID uint      `gorm:"column:command_id;not null;index" json:"command_id"`
	IsQuiet   bool      `gorm:"column:is_quiet;default:false" json:"is_quiet"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the table name for CommandCenterChat.
func (CommandCenterChat) TableName() string {
	return "command_center_chats"
}

// CommandCenterActionLog records actions performed through the command center.
type CommandCenterActionLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	CommandID  uint      `gorm:"column:command_id;not null;index" json:"command_id"`
	ChatID     int64     `gorm:"column:chat_id;not null" json:"chat_id"`
	UserID     int64     `gorm:"column:user_id;not null" json:"user_id"`
	ActionType string    `gorm:"column:action_type;not null" json:"action_type"` // "mute", "unmute", "ban", "kick", "fban"
	Reason     string    `gorm:"column:reason;default:''" json:"reason"`
	MessageID  int64     `gorm:"column:message_id;default:0" json:"message_id"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
}

// TableName returns the table name for CommandCenterActionLog.
func (CommandCenterActionLog) TableName() string {
	return "command_center_action_logs"
}
