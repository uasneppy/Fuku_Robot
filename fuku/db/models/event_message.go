package models

import "time"

// ChatEventMessage is one group's custom message for one event (a manual punishment such as ban
// or mute) at one destination (the group itself, or the private copy sent to the punished
// person). It holds the owner's text, at most one media item (FileID and MediaType, 0 meaning no
// media), the URL buttons and the Silent switch. A chat with no group row for an event, or a row
// whose parts are all empty with Silent off, sends the default text; a private copy is off until
// its Silent switch is written false. The event and destination are validated in Go
// (fuku/db/eventmsg).
//
// The model carries no default: tag on any field, so a Go false, 0 or empty string is written by
// the column-targeted upserts of the repository.
type ChatEventMessage struct {
	ChatID    int64       `gorm:"column:chat_id;primaryKey;autoIncrement:false" json:"chat_id"`
	Event     string      `gorm:"column:event;primaryKey;size:32" json:"event"`
	Dest      string      `gorm:"column:dest;primaryKey;size:16" json:"dest"`
	Text      string      `gorm:"column:text;not null" json:"text"`
	FileID    string      `gorm:"column:file_id;not null" json:"file_id"`
	MediaType int         `gorm:"column:media_type;not null" json:"media_type"`
	Buttons   ButtonArray `gorm:"column:buttons;type:jsonb" json:"buttons"`
	Silent    bool        `gorm:"column:silent;not null" json:"silent"`
	UpdatedBy int64       `gorm:"column:updated_by;not null" json:"updated_by"`
	UpdatedAt time.Time   `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatEventMessage) TableName() string {
	return "chat_event_messages"
}

// ChatEventMessageOptions is one group's two options of the private copies of its event messages:
// PMDetails adds a Details button to the group message when a private copy was not delivered, and
// PMLog posts one line in the group's log channel then. A chat with no row has both options off.
// It is read with the chat's event messages (fuku/db/eventmsg) under one cache key.
//
// The model carries no default: tag on any field, so a Go false is written by the column-targeted
// upserts of the repository.
type ChatEventMessageOptions struct {
	ChatID    int64     `gorm:"column:chat_id;primaryKey;autoIncrement:false" json:"chat_id"`
	PMDetails bool      `gorm:"column:pm_details;not null" json:"pm_details"`
	PMLog     bool      `gorm:"column:pm_log;not null" json:"pm_log"`
	UpdatedBy int64     `gorm:"column:updated_by;not null" json:"updated_by"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatEventMessageOptions) TableName() string {
	return "chat_event_message_options"
}
