package models

import "time"

// Outcomes of one linked group in a staff action, as stored in
// staff_action_groups.outcome.
const (
	// StaffActionOutcomePending means the group has not been finished yet.
	StaffActionOutcomePending = "pending"
	// StaffActionOutcomeDone means the action was applied in the group.
	StaffActionOutcomeDone = "done"
	// StaffActionOutcomeSkipped means the group was deliberately left alone.
	StaffActionOutcomeSkipped = "skipped"
	// StaffActionOutcomeFailed means the group could not be judged or acted on.
	StaffActionOutcomeFailed = "failed"
)

// StaffAction is the audit record of one confirmed staff action: who issued it,
// against whom, what it was and how it ended. StaffChatID follows the Staff Group
// through staff.RekeyChat; SummaryChatID never does, because SummaryMsgID only
// names a message in the chat it was sent to. The Undo columns are claimed by one
// conditional update, so an action is undone at most once.
type StaffAction struct {
	ID             uint   `gorm:"primaryKey;autoIncrement;index:idx_staff_actions_staff_chat_id_id,priority:2" json:"-"`
	StaffChatID    int64  `gorm:"column:staff_chat_id;index:idx_staff_actions_staff_chat_id_id,priority:1;not null" json:"staff_chat_id,omitempty"`
	IssuerUserID   int64  `gorm:"column:issuer_user_id;not null" json:"issuer_user_id,omitempty"`
	IssuerName     string `gorm:"column:issuer_name;not null;default:''" json:"issuer_name,omitempty"`
	TargetUserID   int64  `gorm:"column:target_user_id;not null" json:"target_user_id,omitempty"`
	TargetName     string `gorm:"column:target_name;not null;default:''" json:"target_name,omitempty"`
	Action         string `gorm:"column:action;size:8;not null;check:chk_staff_action_kind,action IN ('ban','mute','kick','unban','unmute')" json:"action,omitempty"`
	Reason         string `gorm:"column:reason;not null;default:''" json:"reason,omitempty"`
	DurationSec    int64  `gorm:"column:duration_sec;not null;default:0" json:"duration_sec,omitempty"`
	DurationAmount int64  `gorm:"column:duration_amount;not null;default:0" json:"duration_amount,omitempty"`
	DurationUnit   string `gorm:"column:duration_unit;size:1;not null;default:''" json:"duration_unit,omitempty"`
	OverLimit      bool   `gorm:"column:over_limit;not null;default:false" json:"over_limit,omitempty"`
	// UntilDate is the one end date sent to every group; 0 is permanent.
	UntilDate  int64 `gorm:"column:until_date;not null;default:0" json:"until_date,omitempty"`
	GroupCount int   `gorm:"column:group_count;not null;default:0" json:"group_count,omitempty"`
	// SummaryChatID is the chat SummaryMsgID lives in. It is never re-keyed.
	SummaryChatID int64 `gorm:"column:summary_chat_id;not null;default:0" json:"summary_chat_id,omitempty"`
	SummaryMsgID  int64 `gorm:"column:summary_msg_id;not null;default:0" json:"summary_msg_id,omitempty"`
	// FinishedAt is set once the run ended; nil while it is going or after a crash.
	FinishedAt *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	// UndoBy, UndoByName, UndoStartedAt and UndoFinishedAt record the one undo.
	UndoBy         *int64     `gorm:"column:undo_by" json:"undo_by,omitempty"`
	UndoByName     string     `gorm:"column:undo_by_name;not null;default:''" json:"undo_by_name,omitempty"`
	UndoStartedAt  *time.Time `gorm:"column:undo_started_at" json:"undo_started_at,omitempty"`
	UndoFinishedAt *time.Time `gorm:"column:undo_finished_at" json:"undo_finished_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for StaffAction.
func (StaffAction) TableName() string {
	return "staff_actions"
}

// StaffActionGroup is one linked group's part of a StaffAction: its outcome, the
// target's state there before the action (written before the Telegram write, so
// Undo can put it back) and the group's undo result. GroupChatID is never
// re-keyed: the restriction lives in the chat it was made in. The link to the
// parent exists in SQL only.
type StaffActionGroup struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ActionID    uint   `gorm:"column:action_id;not null;uniqueIndex:idx_staff_action_groups_action_group,priority:1" json:"action_id,omitempty"`
	Seq         int    `gorm:"column:seq;not null" json:"seq,omitempty"`
	GroupChatID int64  `gorm:"column:group_chat_id;not null;uniqueIndex:idx_staff_action_groups_action_group,priority:2" json:"group_chat_id,omitempty"`
	GroupTitle  string `gorm:"column:group_title;not null;default:''" json:"group_title,omitempty"`
	Outcome     string `gorm:"column:outcome;size:8;not null;default:'pending';check:chk_staff_action_group_outcome,outcome IN ('pending','done','skipped','failed')" json:"outcome,omitempty"`
	// Reason is the staffReason code; deliberately not a CHECK list.
	Reason string `gorm:"column:reason;size:40;not null;default:''" json:"reason,omitempty"`
	// Detail is Telegram's error text, already HTML-escaped.
	Detail string `gorm:"column:detail;not null;default:''" json:"detail,omitempty"`
	// PriorStatus is the target's gotgbot status before the action; "" means the
	// state was never captured, which is never undoable.
	PriorStatus   string `gorm:"column:prior_status;size:16;not null;default:''" json:"prior_status,omitempty"`
	PriorIsMember bool   `gorm:"column:prior_is_member;not null;default:false" json:"prior_is_member,omitempty"`
	PriorUntil    int64  `gorm:"column:prior_until;not null;default:0" json:"prior_until,omitempty"`
	// PriorPermissions is the JSON of the target's gotgbot.ChatPermissions, filled
	// only for a restricted target.
	PriorPermissions string     `gorm:"column:prior_permissions;not null;default:''" json:"prior_permissions,omitempty"`
	AppliedAt        *time.Time `gorm:"column:applied_at" json:"applied_at,omitempty"`
	UndoOutcome      string     `gorm:"column:undo_outcome;size:8;not null;default:'';check:chk_staff_action_group_undo_outcome,undo_outcome IN ('','pending','done','skipped','failed')" json:"undo_outcome,omitempty"`
	UndoReason       string     `gorm:"column:undo_reason;size:40;not null;default:''" json:"undo_reason,omitempty"`
	UndoDetail       string     `gorm:"column:undo_detail;not null;default:''" json:"undo_detail,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt        time.Time  `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for StaffActionGroup.
func (StaffActionGroup) TableName() string {
	return "staff_action_groups"
}
