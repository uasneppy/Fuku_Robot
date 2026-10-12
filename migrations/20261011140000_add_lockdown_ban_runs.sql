-- Lockdown ban runs: one row per "Ban N recent joiners" run (chat_lockdown_ban_runs).
--
-- A run answers a lockdown's burst by banning its accounts for good, one account at a
-- time (chat_lockdown_burst rows carry the per-account state and point back here
-- through run_id). The run row holds who started it, the destination and message of its
-- progress card, and whether it is still going.
--
-- At most one run per lockdown is running at a time: the partial unique index
-- uk_chat_lockdown_ban_runs_running makes a second concurrent run impossible, the same
-- idiom as uk_chat_lockdowns_active. A finished run (done or stopped) no longer holds
-- the index, so a later run can take the accounts no earlier run handled.
--
-- There is deliberately no foreign key on chat IDs (the staff and federation
-- precedent). The table keeps a SQL-only foreign key to its lockdown; the Go model
-- declares no relation.
--
-- These rows are never cached and never part of backup, export, import or reset.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS chat_lockdown_ban_runs (
    id BIGSERIAL PRIMARY KEY,
    lockdown_id BIGINT NOT NULL REFERENCES chat_lockdowns(id) ON DELETE CASCADE,
    -- The locked group.
    chat_id BIGINT NOT NULL,
    started_by BIGINT NOT NULL,
    started_by_name TEXT NOT NULL DEFAULT '',
    -- The progress card: where it was posted and its message.
    card_chat_id BIGINT NOT NULL DEFAULT 0,
    card_msg_id BIGINT NOT NULL DEFAULT 0,
    card_dest VARCHAR(8) NOT NULL DEFAULT 'staff',
    state VARCHAR(8) NOT NULL DEFAULT 'running',
    finished_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_chat_lockdown_ban_run_state CHECK (state IN ('running', 'done', 'stopped')),
    CONSTRAINT chk_chat_lockdown_ban_run_dest CHECK (card_dest IN ('staff', 'log'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_chat_lockdown_ban_runs_running ON chat_lockdown_ban_runs (lockdown_id) WHERE state = 'running';
