-- Staff action audit record: one row per confirmed staff action (staff_actions)
-- and one row per linked group of that action (staff_action_groups).
--
-- There is deliberately no foreign key on chat IDs (the federation_chats and
-- staff_group_links precedent). The child keeps a SQL-only foreign key to its
-- parent, because both rows are written by the same transaction; the Go models
-- declare no relation.
--
-- These tables are never cached and never part of backup, export, import or
-- reset: they hold names, user IDs and free-text reasons that are kept forever.
--
-- The target's state before the action (prior_status, prior_is_member,
-- prior_until, prior_permissions) is written BEFORE each Telegram write, so a
-- later Undo can put back exactly what was there. prior_permissions is the JSON
-- of the target's permission set and is filled only for a restricted target.
-- prior_status '' means the state was never captured and is never undoable.
--
-- staff_actions.staff_chat_id follows the Staff Group through staff.RekeyChat.
-- summary_chat_id and staff_action_groups.group_chat_id are never re-keyed: a
-- message ID only means something in the chat it was sent to, and a restriction
-- lives in the group it was made in.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS staff_actions (
    id BIGSERIAL PRIMARY KEY,
    staff_chat_id BIGINT NOT NULL,
    issuer_user_id BIGINT NOT NULL,
    issuer_name TEXT NOT NULL DEFAULT '',
    target_user_id BIGINT NOT NULL,
    target_name TEXT NOT NULL DEFAULT '',
    action VARCHAR(8) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    -- The duration as typed, where 0 seconds is permanent.
    duration_sec BIGINT NOT NULL DEFAULT 0,
    duration_amount BIGINT NOT NULL DEFAULT 0,
    duration_unit VARCHAR(1) NOT NULL DEFAULT '',
    over_limit BOOLEAN NOT NULL DEFAULT FALSE,
    -- The one end date sent to every group, where 0 is permanent.
    until_date BIGINT NOT NULL DEFAULT 0,
    group_count INTEGER NOT NULL DEFAULT 0,
    -- Where summary_msg_id lives, never re-keyed.
    summary_chat_id BIGINT NOT NULL DEFAULT 0,
    summary_msg_id BIGINT NOT NULL DEFAULT 0,
    -- NULL while the run is going, and after a crash that never finalized it.
    finished_at TIMESTAMP WITH TIME ZONE,
    -- The one undo of this action, claimed by a conditional UPDATE.
    undo_by BIGINT,
    undo_by_name TEXT NOT NULL DEFAULT '',
    undo_started_at TIMESTAMP WITH TIME ZONE,
    undo_finished_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_staff_action_kind CHECK (action IN ('ban', 'mute', 'kick', 'unban', 'unmute'))
);

CREATE INDEX IF NOT EXISTS idx_staff_actions_staff_chat_id_id ON staff_actions(staff_chat_id, id);

CREATE TABLE IF NOT EXISTS staff_action_groups (
    id BIGSERIAL PRIMARY KEY,
    action_id BIGINT NOT NULL REFERENCES staff_actions(id) ON DELETE CASCADE,
    -- Position in the link order the run used.
    seq INTEGER NOT NULL,
    group_chat_id BIGINT NOT NULL,
    -- The group's title at action time.
    group_title TEXT NOT NULL DEFAULT '',
    outcome VARCHAR(8) NOT NULL DEFAULT 'pending',
    -- The staffReason code, deliberately without a CHECK so a new reason code needs no migration.
    reason VARCHAR(40) NOT NULL DEFAULT '',
    -- Telegram's error text, already HTML-escaped.
    detail TEXT NOT NULL DEFAULT '',
    prior_status VARCHAR(16) NOT NULL DEFAULT '',
    prior_is_member BOOLEAN NOT NULL DEFAULT FALSE,
    prior_until BIGINT NOT NULL DEFAULT 0,
    prior_permissions TEXT NOT NULL DEFAULT '',
    -- Set when the outcome is done.
    applied_at TIMESTAMP WITH TIME ZONE,
    -- This group's part of the one undo of the action.
    undo_outcome VARCHAR(8) NOT NULL DEFAULT '',
    undo_reason VARCHAR(40) NOT NULL DEFAULT '',
    undo_detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(action_id, group_chat_id),
    CONSTRAINT chk_staff_action_group_outcome CHECK (outcome IN ('pending', 'done', 'skipped', 'failed')),
    CONSTRAINT chk_staff_action_group_undo_outcome CHECK (undo_outcome IN ('', 'pending', 'done', 'skipped', 'failed'))
);
