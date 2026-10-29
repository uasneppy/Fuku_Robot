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

// Kinds of lockdown trigger, as stored in chat_lockdowns.trigger_kind. The column has no
// CHECK, so later phases add kinds without a migration.
const (
	// LockdownTriggerManual is the trigger_kind of a lockdown an admin started with
	// /lockdown.
	LockdownTriggerManual = "manual"
	// LockdownTriggerJoinSurge is the trigger_kind of a lockdown the join-surge detector
	// started: trigger_count joins within trigger_window seconds.
	LockdownTriggerJoinSurge = "join_surge"
	// LockdownTriggerFlood is the trigger_kind of a lockdown the flood detector started:
	// trigger_count messages within trigger_window seconds, trigger_percent of them from
	// new members.
	LockdownTriggerFlood = "flood"
	// LockdownTriggerAIBurst is the trigger_kind of a lockdown the AI spam verdict counter
	// started: trigger_count spam verdicts within trigger_window seconds, trigger_percent 0.
	// It is 8 characters and the column holds 16 with no CHECK, so it needs no migration.
	LockdownTriggerAIBurst = "ai_burst"
)

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

// JoinerDetailBanExpired is the detail of a kept joiner row whose lockdown ban had
// already run out when the lift reached it: the person was no longer banned and the
// ban's end date had passed. The state check on chat_lockdown_joiners has no state for
// it, so it is a kept row with this detail, which is never Telegram's escaped text.
const JoinerDetailBanExpired = "ban_expired"

// JoinerDetailResponseBan is the detail of a kept joiner row that "Ban N recent
// joiners" banned for good, with no end date. The lift leaves such an account banned
// and counts it as left alone on purpose, and /lockdownstatus still counts it as removed.
const JoinerDetailResponseBan = "response_ban"

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
	// TriggerCount is the number of joins (join_surge) or messages (flood) that fired an
	// automatic trigger; 0 for a manual lockdown (migration 20261018120000).
	TriggerCount int `gorm:"column:trigger_count;not null;default:0" json:"trigger_count,omitempty"`
	// TriggerWindow is the window TriggerCount was counted in, in seconds; 0 for a manual
	// lockdown.
	TriggerWindow int `gorm:"column:trigger_window;not null;default:0" json:"trigger_window,omitempty"`
	// TriggerPercent is, for a flood, the share (0-100) of the counted messages that came
	// from new members; 0 for every other kind.
	TriggerPercent int `gorm:"column:trigger_percent;not null;default:0" json:"trigger_percent,omitempty"`
	// ChatTitle is the locked group's title when the lockdown started, so an alert
	// names the group without a getChat. Empty for a lockdown from before it was kept.
	ChatTitle string `gorm:"column:chat_title;not null;default:''" json:"chat_title,omitempty"`
	// AlertStartedAt is when the lockdown's alert was decided. It is the exactly-once
	// gate (lockdown.BeginAlert): nil until the one conditional update that decides it.
	AlertStartedAt *time.Time `gorm:"column:alert_started_at" json:"alert_started_at,omitempty"`
	// LinkRevokedAt is when an admin revoked the raid's invite link from the alert; nil
	// until Telegram confirmed it. One revoke is recorded per lockdown
	// (lockdown.RecordLinkRevoked), by LinkRevokedBy (LinkRevokedByName is their name when
	// they pressed it) and for LinkRevokedLink.
	LinkRevokedAt     *time.Time `gorm:"column:link_revoked_at" json:"link_revoked_at,omitempty"`
	LinkRevokedBy     *int64     `gorm:"column:link_revoked_by" json:"link_revoked_by,omitempty"`
	LinkRevokedByName string     `gorm:"column:link_revoked_by_name;not null;default:''" json:"link_revoked_by_name,omitempty"`
	LinkRevokedLink   string     `gorm:"column:link_revoked_link;not null;default:''" json:"link_revoked_link,omitempty"`
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

// Kinds of a lockdown alert copy, as stored in chat_lockdown_alerts.kind.
const (
	// AlertKindAlert is the first alert of a lockdown.
	AlertKindAlert = "alert"
	// AlertKindReminder is a later reminder that the lockdown still runs.
	AlertKindReminder = "reminder"
)

// Destinations of a lockdown alert copy, as stored in chat_lockdown_alerts.dest.
const (
	// AlertDestStaff is the Staff Group the locked group is linked to.
	AlertDestStaff = "staff"
	// AlertDestLog is the locked group's own log channel.
	AlertDestLog = "log"
)

// States of a lockdown alert copy, as stored in chat_lockdown_alerts.state.
const (
	// AlertStatePending means the copy was planned and is waiting to be posted.
	AlertStatePending = "pending"
	// AlertStatePosting means a worker claimed the copy and its send is running.
	AlertStatePosting = "posting"
	// AlertStatePosted means the copy was sent; message_id and body are what was sent.
	AlertStatePosted = "posted"
	// AlertStateFailed means the copy could not be sent and will not be tried again.
	AlertStateFailed = "failed"
)

// ChatLockdownAlert is one copy of a lockdown's alert or reminder in one
// destination. The slot (LockdownID, Kind, Seq, Dest) is unique, so a copy is never
// planned twice. MessageID and Body are what was posted, kept so a later edit starts
// from exactly the text that was sent. The row is never cached and never part of
// backup, export, import or reset; its link to the lockdown exists in SQL only.
type ChatLockdownAlert struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"-"`
	LockdownID  uint       `gorm:"column:lockdown_id;not null;uniqueIndex:uk_chat_lockdown_alerts_slot,priority:1;index:idx_chat_lockdown_alerts_lockdown_state,priority:1" json:"lockdown_id,omitempty"`
	ChatID      int64      `gorm:"column:chat_id;not null" json:"chat_id,omitempty"`
	Kind        string     `gorm:"column:kind;size:8;not null;uniqueIndex:uk_chat_lockdown_alerts_slot,priority:2;check:chk_chat_lockdown_alert_kind,kind IN ('alert','reminder')" json:"kind,omitempty"`
	Seq         int        `gorm:"column:seq;not null;default:0;uniqueIndex:uk_chat_lockdown_alerts_slot,priority:3" json:"seq,omitempty"`
	Dest        string     `gorm:"column:dest;size:8;not null;uniqueIndex:uk_chat_lockdown_alerts_slot,priority:4;check:chk_chat_lockdown_alert_dest,dest IN ('staff','log')" json:"dest,omitempty"`
	DestChatID  int64      `gorm:"column:dest_chat_id;not null" json:"dest_chat_id,omitempty"`
	State       string     `gorm:"column:state;size:8;not null;default:'pending';index:idx_chat_lockdown_alerts_lockdown_state,priority:2;index:idx_chat_lockdown_alerts_state;check:chk_chat_lockdown_alert_state,state IN ('pending','posting','posted','failed')" json:"state,omitempty"`
	MessageID   int64      `gorm:"column:message_id;not null;default:0" json:"message_id,omitempty"`
	Body        string     `gorm:"column:body;not null;default:''" json:"body,omitempty"`
	Link        string     `gorm:"column:link;not null;default:''" json:"link,omitempty"`
	ButtonsLive bool       `gorm:"column:buttons_live;not null;default:true" json:"buttons_live,omitempty"`
	EditPending bool       `gorm:"column:edit_pending;not null;default:false" json:"edit_pending,omitempty"`
	ClosedAt    *time.Time `gorm:"column:closed_at" json:"closed_at,omitempty"`
	Attempts    int        `gorm:"column:attempts;not null;default:0" json:"attempts,omitempty"`
	ClaimedAt   *time.Time `gorm:"column:claimed_at" json:"claimed_at,omitempty"`
	Detail      string     `gorm:"column:detail;not null;default:''" json:"detail,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt   time.Time  `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for ChatLockdownAlert.
func (ChatLockdownAlert) TableName() string {
	return "chat_lockdown_alerts"
}

// Where an account of a lockdown's burst came from, as stored in
// chat_lockdown_burst.source.
const (
	// BurstSourcePre is an account that joined in the minutes before the lock, copied
	// from the recent-joins tracker when the alert was decided.
	BurstSourcePre = "pre"
	// BurstSourceJoiner is an account the lockdown itself recorded, copied from
	// chat_lockdown_joiners by the ban run.
	BurstSourceJoiner = "joiner"
)

// What a ban run did with an account of the burst, as stored in
// chat_lockdown_burst.state. The response states are moved by the ban run.
const (
	// BurstStateNone means no response has touched the account.
	BurstStateNone = ""
	// BurstStateQueued means a ban run took the account and has not reached it yet.
	BurstStateQueued = "queued"
	// BurstStateActing means a worker claimed the account and its ban call is running.
	BurstStateActing = "acting"
	// BurstStateBanned means the account was banned.
	BurstStateBanned = "banned"
	// BurstStateSkipped means the run left the account alone, with a reason.
	BurstStateSkipped = "skipped"
	// BurstStateFailed means the ban call failed.
	BurstStateFailed = "failed"
)

// BurstReasonLetIn is the reason of a skipped burst row whose account the lockdown
// recorded as exempt, because a live admin let them in. The ban run sets it when it
// starts, so no Telegram call is ever made for them.
const BurstReasonLetIn = "let_in"

// ChatLockdownBurst is one account of a lockdown's burst: someone who joined in the
// minutes before the lock (source pre, saved from the recent-joins tracker when the
// alert was decided) or while the lockdown was active (source joiner). The state,
// reason, detail, run, attempt and claim columns are what a ban run moves per account.
// (LockdownID, UserID) is unique. The row is never cached and never part of backup,
// export, import or reset; its link to the lockdown exists in SQL only.
type ChatLockdownBurst struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"-"`
	LockdownID uint       `gorm:"column:lockdown_id;not null;uniqueIndex:uk_chat_lockdown_burst_user,priority:1;index:idx_chat_lockdown_burst_lockdown_state,priority:1" json:"lockdown_id,omitempty"`
	ChatID     int64      `gorm:"column:chat_id;not null" json:"chat_id,omitempty"`
	UserID     int64      `gorm:"column:user_id;not null;uniqueIndex:uk_chat_lockdown_burst_user,priority:2" json:"user_id,omitempty"`
	FirstName  string     `gorm:"column:first_name;not null;default:''" json:"first_name,omitempty"`
	Username   string     `gorm:"column:username;not null;default:''" json:"username,omitempty"`
	IsBot      bool       `gorm:"column:is_bot;not null;default:false" json:"is_bot,omitempty"`
	InviteLink string     `gorm:"column:invite_link;not null;default:''" json:"invite_link,omitempty"`
	Source     string     `gorm:"column:source;size:8;not null;default:'pre';check:chk_chat_lockdown_burst_source,source IN ('pre','joiner')" json:"source,omitempty"`
	JoinedAt   *time.Time `gorm:"column:joined_at" json:"joined_at,omitempty"`
	State      string     `gorm:"column:state;size:8;not null;default:'';index:idx_chat_lockdown_burst_lockdown_state,priority:2;check:chk_chat_lockdown_burst_state,state IN ('','queued','acting','banned','skipped','failed')" json:"state,omitempty"`
	Reason     string     `gorm:"column:reason;size:16;not null;default:''" json:"reason,omitempty"`
	Detail     string     `gorm:"column:detail;not null;default:''" json:"detail,omitempty"`
	RunID      *int64     `gorm:"column:run_id" json:"run_id,omitempty"`
	Attempts   int        `gorm:"column:attempts;not null;default:0" json:"attempts,omitempty"`
	ClaimedAt  *time.Time `gorm:"column:claimed_at" json:"claimed_at,omitempty"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt  time.Time  `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for ChatLockdownBurst.
func (ChatLockdownBurst) TableName() string {
	return "chat_lockdown_burst"
}

// States of a "Ban N recent joiners" run, as stored in chat_lockdown_ban_runs.state.
const (
	// BanRunStateRunning means the run is queueing and banning accounts. At most one run
	// per lockdown is running, enforced by the uk_chat_lockdown_ban_runs_running partial
	// unique index.
	BanRunStateRunning = "running"
	// BanRunStateDone means every account of the run reached a final state.
	BanRunStateDone = "done"
	// BanRunStateStopped means the run ended early (the lockdown was lifted, or an admin
	// stopped it) with accounts left unhandled.
	BanRunStateStopped = "stopped"
)

// ChatLockdownBanRun is one run of "Ban N recent joiners" for one lockdown. CardChatID
// and CardMsgID are where the run's progress card lives (CardDest says whether that is
// the Staff Group or the locked group's log channel). The row is never cached and never
// part of backup, export, import or reset; its link to the lockdown exists in SQL only.
type ChatLockdownBanRun struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"-"`
	LockdownID    uint       `gorm:"column:lockdown_id;not null;uniqueIndex:uk_chat_lockdown_ban_runs_running,where:state = 'running'" json:"lockdown_id,omitempty"`
	ChatID        int64      `gorm:"column:chat_id;not null" json:"chat_id,omitempty"`
	StartedBy     int64      `gorm:"column:started_by;not null" json:"started_by,omitempty"`
	StartedByName string     `gorm:"column:started_by_name;not null;default:''" json:"started_by_name,omitempty"`
	CardChatID    int64      `gorm:"column:card_chat_id;not null;default:0" json:"card_chat_id,omitempty"`
	CardMsgID     int64      `gorm:"column:card_msg_id;not null;default:0" json:"card_msg_id,omitempty"`
	CardDest      string     `gorm:"column:card_dest;size:8;not null;default:'staff';check:chk_chat_lockdown_ban_run_dest,card_dest IN ('staff','log')" json:"card_dest,omitempty"`
	State         string     `gorm:"column:state;size:8;not null;default:'running';check:chk_chat_lockdown_ban_run_state,state IN ('running','done','stopped')" json:"state,omitempty"`
	FinishedAt    *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	// The delivery of the finished run's report (migration 20261011160000). The card edit
	// and the log post are settled separately (delivered, or given up on after
	// the attempt budget); ReportedAt is set once both are, and ReportClaimedAt
	// is the worker's claim while it delivers.
	ReportCardDoneAt *time.Time `gorm:"column:report_card_done_at" json:"report_card_done_at,omitempty"`
	ReportLogDoneAt  *time.Time `gorm:"column:report_log_done_at" json:"report_log_done_at,omitempty"`
	ReportedAt       *time.Time `gorm:"column:reported_at" json:"reported_at,omitempty"`
	ReportClaimedAt  *time.Time `gorm:"column:report_claimed_at" json:"report_claimed_at,omitempty"`
	ReportAttempts   int        `gorm:"column:report_attempts;not null;default:0" json:"report_attempts,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt        time.Time  `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the migration's table name for ChatLockdownBanRun.
func (ChatLockdownBanRun) TableName() string {
	return "chat_lockdown_ban_runs"
}
