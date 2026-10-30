-- Lockdown burst: one row per account of a lockdown's burst (chat_lockdown_burst).
--
-- The burst is the raid a lockdown answers. Source 'pre' rows come from the recent-joins
-- tracker (Redis, the last 10 minutes before the lock): they are copied here in the same
-- transaction that decides the lockdown's alert, so the alert, the "Ban N recent joiners"
-- button and the reminders never need Redis afterwards. Source 'joiner' rows are copied
-- from chat_lockdown_joiners by the ban run, for the accounts that joined while the
-- lockdown was active.
--
-- The response columns (state, reason, detail, run_id, attempts, claimed_at) are what a
-- ban run moves per account; they are created with the table so every later statement
-- stays an idempotent CREATE. state '' means no response has touched the account.
--
-- There is deliberately no foreign key on chat IDs (the staff and federation
-- precedent). The table keeps a SQL-only foreign key to its lockdown; the Go model
-- declares no relation.
--
-- These rows are never cached and never part of backup, export, import or reset.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS chat_lockdown_burst (
    id BIGSERIAL PRIMARY KEY,
    lockdown_id BIGINT NOT NULL REFERENCES chat_lockdowns(id) ON DELETE CASCADE,
    -- The locked group.
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    first_name TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    invite_link TEXT NOT NULL DEFAULT '',
    source VARCHAR(8) NOT NULL DEFAULT 'pre',
    joined_at TIMESTAMP WITH TIME ZONE,
    state VARCHAR(8) NOT NULL DEFAULT '',
    reason VARCHAR(16) NOT NULL DEFAULT '',
    -- Telegram's error text, already HTML-escaped.
    detail TEXT NOT NULL DEFAULT '',
    run_id BIGINT,
    attempts INTEGER NOT NULL DEFAULT 0,
    claimed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_lockdown_burst_user UNIQUE (lockdown_id, user_id),
    CONSTRAINT chk_chat_lockdown_burst_source CHECK (source IN ('pre', 'joiner')),
    CONSTRAINT chk_chat_lockdown_burst_state CHECK (state IN ('', 'queued', 'acting', 'banned', 'skipped', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_chat_lockdown_burst_lockdown_state ON chat_lockdown_burst (lockdown_id, state);
