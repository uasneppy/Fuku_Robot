package models

import "time"

// States of a chat lockdown, as stored in chat_lockdowns.state.
const (
	// LockdownStateActive means the group is locked. At most one row per chat is
	// active, enforced by the uk_chat_lockdowns_active partial unique index.
	LockdownStateActive = "active"
	// LockdownStateLifting means /unlockdown restored the permissions and the
	// recorded joiners are being unbanned.
	LockdownStateLifting = "lifting"
	// LockdownStateLifted means the lockdown is over.
	LockdownStateLifted = "lifted"
)

// LockdownTriggerManual is the trigger_kind of a lockdown an admin started with
// /lockdown. The column has no CHECK, so later phases add kinds without a migration.
const LockdownTriggerManual = "manual"

// States of one joiner row of a lockdown, as stored in chat_lockdown_joiners.state.
const (
	// JoinerStatePending means the joiner was recorded and is waiting to be banned.
	JoinerStatePending = "pending"
	// JoinerStateActing means a worker claimed the row and its ban call is running.
	JoinerStateActing = "acting"
	// JoinerStateBanned means the lockdown's own ban is in place.
	JoinerStateBanned = "banned"
	// JoinerStateBanFailed means the ban call failed.
	JoinerStateBanFailed = "ban_failed"
	// JoinerStateDeclined means a join request was declined.
	JoinerStateDeclined = "declined"
	// JoinerStateDeclineFailed means declining a join request failed.
	JoinerStateDeclineFailed = "decline_failed"
	// JoinerStateExempt means a live admin added the person directly, so they stay.
	JoinerStateExempt = "exempt"
	// JoinerStateCancelled means the lockdown ended before the row was acted on.
	JoinerStateCancelled = "cancelled"
	// JoinerStateUnbanning means a worker claimed the row to unban at the lift.
	JoinerStateUnbanning = "unbanning"
	// JoinerStateUnbanned means the lift removed the lockdown's own ban.
	JoinerStateUnbanned = "unbanned"
	// JoinerStateKept means the ban was no longer the lockdown's own, so the lift left it.
	JoinerStateKept = "kept"
	// JoinerStateUnbanFailed means the lift could not unban the person.
	JoinerStateUnbanFailed = "unban_failed"
)

// How a joiner reached the group, as stored in chat_lockdown_joiners.join_path.
const (
	// JoinPathMember is the chat_member update.
	JoinPathMember = "member"
	// JoinPathService is the new_chat_members service message.
	JoinPathService = "service"
	// JoinPathRequest is a chat_join_request.
	JoinPathRequest = "request"
)

// ChatLockdown is one lockdown of one chat. PrePermissions is the raw JSON of the
// permissions member of the getChat answer read before the lock call, replayed
// verbatim at the lift; it is never produced from a typed struct. LockedAt stays
// nil until Telegram confirmed the lock. The row is never cached and never part of
// backup, export, import or reset.
type ChatLockdown struct {
	ID                uint       `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID            int64      `gorm:"column:chat_id;not null;uniqueIndex:uk_chat_lockdowns_active,where:state = 'active'" json:"chat_id,omitempty"`
	State             string     `gorm:"column:state;size:8;not null;default:'active';index:idx_chat_lockdowns_state;check:chk_chat_lockdown_state,state IN ('active','lifting','lifted')" json:"state,omitempty"`
	TriggerKind       string     `gorm:"column:trigger_kind;size:16;not null;default:'manual'" json:"trigger_kind,omitempty"`
	Reason            string     `gorm:"column:reason;not null;default:''" json:"reason,omitempty"`
	StartedBy         int64      `gorm:"column:started_by;not null;default:0" json:"started_by,omitempty"`
	StartedByName     string     `gorm:"column:started_by_name;not null;default:''" json:"started_by_name,omitempty"`
	PrePermissions    string     `gorm:"column:pre_permissions;not null" json:"pre_permissions,omitempty"`
	LockedPermissions string     `gorm:"column:locked_permissions;not null;default:''" json:"locked_permissions,omitempty"`
	LockedAt          *time.Time `gorm:"column:locked_at" json:"locked_at,omitempty"`
	LiftedBy          *int64     `gorm:"column:lifted_by" json:"lifted_by,omitempty"`
	LiftedByName      string     `gorm:"column:lifted_by_name;not null;default:''" json:"lifted_by_name,omitempty"`
	LiftStartedAt     *time.Time `gorm:"column:lift_started_at" json:"lift_started_at,omitempty"`
	LiftedAt          *time.Time `gorm:"column:lifted_at" json:"lifted_at,omitempty"`
	ManualChange      bool       `gorm:"column:manual_change;not null;default:false" json:"manual_change,omitempty"`
	CreatedAt         time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt         time.Time  `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for ChatLockdown.
func (ChatLockdown) TableName() string {
	return "chat_lockdowns"
}

// LockdownJoiner is one user handled during a lockdown. BanUntil is the marker
// that tells the lockdown's own ban from a deliberate one. The link to the parent
// exists in SQL only.
type LockdownJoiner struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"-"`
	LockdownID     uint       `gorm:"column:lockdown_id;not null;uniqueIndex:uk_chat_lockdown_joiners_user,priority:1;index:idx_chat_lockdown_joiners_lockdown_state,priority:1" json:"lockdown_id,omitempty"`
	ChatID         int64      `gorm:"column:chat_id;not null;index:idx_chat_lockdown_joiners_chat_user,priority:1" json:"chat_id,omitempty"`
	UserID         int64      `gorm:"column:user_id;not null;uniqueIndex:uk_chat_lockdown_joiners_user,priority:2;index:idx_chat_lockdown_joiners_chat_user,priority:2" json:"user_id,omitempty"`
	FirstName      string     `gorm:"column:first_name;not null;default:''" json:"first_name,omitempty"`
	Username       string     `gorm:"column:username;not null;default:''" json:"username,omitempty"`
	IsBot          bool       `gorm:"column:is_bot;not null;default:false" json:"is_bot,omitempty"`
	JoinPath       string     `gorm:"column:join_path;size:8;not null;default:'';check:chk_chat_lockdown_joiner_path,join_path IN ('','member','service','request')" json:"join_path,omitempty"`
	InviteLink     string     `gorm:"column:invite_link;not null;default:''" json:"invite_link,omitempty"`
	ViaJoinRequest bool       `gorm:"column:via_join_request;not null;default:false" json:"via_join_request,omitempty"`
	PerformerID    int64      `gorm:"column:performer_id;not null;default:0" json:"performer_id,omitempty"`
	State          string     `gorm:"column:state;size:16;not null;default:'pending';index:idx_chat_lockdown_joiners_lockdown_state,priority:2;check:chk_chat_lockdown_joiner_state,state IN ('pending','acting','banned','ban_failed','declined','decline_failed','exempt','cancelled','unbanning','unbanned','kept','unban_failed')" json:"state,omitempty"`
	BanUntil       int64      `gorm:"column:ban_until;not null;default:0" json:"ban_until,omitempty"`
	JoinMsgID      int64      `gorm:"column:join_msg_id;not null;default:0" json:"join_msg_id,omitempty"`
	Attempts       int        `gorm:"column:attempts;not null;default:0" json:"attempts,omitempty"`
	ClaimedAt      *time.Time `gorm:"column:claimed_at" json:"claimed_at,omitempty"`
	Detail         string     `gorm:"column:detail;not null;default:''" json:"detail,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for LockdownJoiner.
func (LockdownJoiner) TableName() string {
	return "chat_lockdown_joiners"
}
