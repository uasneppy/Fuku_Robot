package models

import "time"

// The MediaBelow and Once switches carry no gorm default tag on purpose. With one, GORM's
// Create drops a Go false and the SQL default takes its place. The default lives in the
// migration's column default (FALSE), so a row saved before it reads as off (see
// chat_settings_access.go).

type WelcomeSettings struct {
	CleanWelcome  bool        `gorm:"column:clean_old;default:false" json:"clean_old" default:"false"`
	LastMsgId     int64       `gorm:"column:last_msg_id" json:"last_msg_id,omitempty"`
	ShouldWelcome bool        `gorm:"column:enabled;default:true" json:"welcome_enabled" default:"true"`
	WelcomeText   string      `gorm:"column:text" json:"welcome_text,omitempty"`
	FileID        string      `gorm:"column:file_id" json:"file_id,omitempty"`
	WelcomeType   int         `gorm:"column:type;default:1" json:"welcome_type,omitempty"`
	Button        ButtonArray `gorm:"column:btns;type:jsonb" json:"btns,omitempty"`
	// MediaBelow shows the welcome's media under its text (column welcome_media_below).
	MediaBelow bool `gorm:"column:media_below" json:"media_below"`
	// Once greets a member only the first time they join (column welcome_once).
	Once bool `gorm:"column:once" json:"once"`
}

type GoodbyeSettings struct {
	CleanGoodbye  bool        `gorm:"column:clean_old;default:false" json:"clean_old" default:"false"`
	LastMsgId     int64       `gorm:"column:last_msg_id" json:"last_msg_id,omitempty"`
	ShouldGoodbye bool        `gorm:"column:enabled;default:true" json:"enabled" default:"true"`
	GoodbyeText   string      `gorm:"column:text" json:"text,omitempty"`
	FileID        string      `gorm:"column:file_id" json:"file_id,omitempty"`
	GoodbyeType   int         `gorm:"column:type;default:1" json:"type,omitempty"`
	Button        ButtonArray `gorm:"column:btns;type:jsonb" json:"btns,omitempty"`
	// MediaBelow shows the goodbye's media under its text (column goodbye_media_below).
	MediaBelow bool `gorm:"column:media_below" json:"media_below"`
}

type GreetingSettings struct {
	ID                 uint             `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID             int64            `gorm:"column:chat_id;uniqueIndex;not null" json:"_id,omitempty"`
	ShouldCleanService bool             `gorm:"column:clean_service_settings;default:false" json:"clean_service_settings" default:"false"`
	WelcomeSettings    *WelcomeSettings `gorm:"embedded;embeddedPrefix:welcome_" json:"welcome_settings" default:"false"`
	GoodbyeSettings    *GoodbyeSettings `gorm:"embedded;embeddedPrefix:goodbye_" json:"goodbye_settings" default:"false"`
	ShouldAutoApprove  bool             `gorm:"column:auto_approve;default:false" json:"auto_approve" default:"false"`
	CreatedAt          time.Time        `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt          time.Time        `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

func (GreetingSettings) TableName() string {
	return "greetings"
}
