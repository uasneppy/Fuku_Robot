package models

import "time"

// The punishments a group can choose for a broken member requirement. The values are
// stored in the eight action columns of chat_requirements (CHECK 0..4) and, from 1 to 4,
// in chat_requirement_holds.action.
const (
	// RequirementActionOff means the rule is not enforced.
	RequirementActionOff = 0
	// RequirementActionWarn warns the member.
	RequirementActionWarn = 1
	// RequirementActionKick removes the member and lets them rejoin.
	RequirementActionKick = 2
	// RequirementActionBan removes the member for good.
	RequirementActionBan = 3
	// RequirementActionMute mutes the member until they fix what the rule asks.
	RequirementActionMute = 4
)

// ChatRequirements is one chat's member requirement settings: the punishment chosen for
// each rule, whether new members are checked at join, and whether a violator's messages
// are deleted. A chat without a row has every rule off (see fuku/db/memberreq).
//
// The booleans carry no gorm default tag on purpose. With one, GORM's Create drops a Go
// false and the SQL default takes its place, so a switch an admin turned off could come
// back on. The defaults live in the migration's column defaults.
type ChatRequirements struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID         int64     `gorm:"column:chat_id;uniqueIndex:uk_chat_requirements_chat;not null" json:"chat_id,omitempty"`
	LastNameAction int       `gorm:"column:last_name_action;type:smallint;not null;check:chk_chat_requirements_last_name,last_name_action BETWEEN 0 AND 4" json:"last_name_action"`
	UsernameAction int       `gorm:"column:username_action;type:smallint;not null;check:chk_chat_requirements_username,username_action BETWEEN 0 AND 4" json:"username_action"`
	RussianAction  int       `gorm:"column:russian_action;type:smallint;not null;check:chk_chat_requirements_russian,russian_action BETWEEN 0 AND 4" json:"russian_action"`
	ArabicAction   int       `gorm:"column:arabic_action;type:smallint;not null;check:chk_chat_requirements_arabic,arabic_action BETWEEN 0 AND 4" json:"arabic_action"`
	ChineseAction  int       `gorm:"column:chinese_action;type:smallint;not null;check:chk_chat_requirements_chinese,chinese_action BETWEEN 0 AND 4" json:"chinese_action"`
	PhotoAction    int       `gorm:"column:photo_action;type:smallint;not null;check:chk_chat_requirements_photo,photo_action BETWEEN 0 AND 4" json:"photo_action"`
	ChannelAction  int       `gorm:"column:channel_action;type:smallint;not null;check:chk_chat_requirements_channel,channel_action BETWEEN 0 AND 4" json:"channel_action"`
	AIAction       int       `gorm:"column:ai_action;type:smallint;not null;check:chk_chat_requirements_ai,ai_action BETWEEN 0 AND 4" json:"ai_action"`
	CheckOnJoin    bool      `gorm:"column:check_on_join;not null" json:"check_on_join"`
	DeleteMessages bool      `gorm:"column:delete_messages;not null" json:"delete_messages"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name.
func (ChatRequirements) TableName() string {
	return "chat_requirements"
}

// ChatRequirementHold is one member's requirement episode in one chat. It is both the
// latch that makes a violation punished once across replicas (a unique key on chat and
// user, claimed insert-first) and the record of a mute the bot placed. A hold with a nil
// PunishedAt is pending: claimed but not yet confirmed punished, or a backoff record.
type ChatRequirementHold struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ChatID      int64      `gorm:"column:chat_id;uniqueIndex:uk_chat_requirement_holds_chat_user,priority:1;not null" json:"chat_id"`
	UserID      int64      `gorm:"column:user_id;uniqueIndex:uk_chat_requirement_holds_chat_user,priority:2;not null" json:"user_id"`
	Rules       int        `gorm:"column:rules;not null;check:chk_chat_requirement_holds_rules,rules > 0" json:"rules"`
	Action      int        `gorm:"column:action;type:smallint;not null;check:chk_chat_requirement_holds_action,action BETWEEN 1 AND 4" json:"action"`
	MuteUntil   int64      `gorm:"column:mute_until;not null" json:"mute_until"`
	WelcomeHeld bool       `gorm:"column:welcome_held;not null" json:"welcome_held"`
	NoticeMsgID int64      `gorm:"column:notice_msg_id;not null" json:"notice_msg_id"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	PunishedAt  *time.Time `gorm:"column:punished_at" json:"punished_at"`
}

// TableName returns the migration's table name.
func (ChatRequirementHold) TableName() string {
	return "chat_requirement_holds"
}
