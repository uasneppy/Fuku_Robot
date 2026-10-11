-- Lockdown alerts: one row per posted copy of an alert or a reminder
-- (chat_lockdown_alerts) and two columns on chat_lockdowns.
--
-- chat_lockdowns.chat_title is the locked group's title when the lockdown started, so
-- the worker never needs a getChat to name the group in an alert. alert_started_at is
-- the exactly-once gate: the alert of a lockdown is decided by one conditional update
-- that matches only while it is NULL, and the same transaction creates the copy rows.
--
-- A copy is one message in one destination (the group's Staff Group or its log
-- channel). kind and seq tell the first alert (alert, 0) from its reminders
-- (reminder, n); the slot (lockdown_id, kind, seq, dest) is unique, so a copy is never
-- created twice. message_id and body are what was posted, kept so a later edit
-- (closing the alert, revoking its buttons) starts from exactly the text that was sent.
-- buttons_live, edit_pending and closed_at are the state of that later edit.
--
-- There is deliberately no foreign key on chat IDs (the staff and federation
-- precedent). The copy table keeps a SQL-only foreign key to its lockdown; the Go
-- models declare no relation.
--
-- These rows are never cached and never part of backup, export, import or reset.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS chat_title TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS alert_started_at TIMESTAMP WITH TIME ZONE;

CREATE TABLE IF NOT EXISTS chat_lockdown_alerts (
    id BIGSERIAL PRIMARY KEY,
    lockdown_id BIGINT NOT NULL REFERENCES chat_lockdowns(id) ON DELETE CASCADE,
    -- The locked group.
    chat_id BIGINT NOT NULL,
    kind VARCHAR(8) NOT NULL,
    seq INTEGER NOT NULL DEFAULT 0,
    dest VARCHAR(8) NOT NULL,
    dest_chat_id BIGINT NOT NULL,
    state VARCHAR(8) NOT NULL DEFAULT 'pending',
    message_id BIGINT NOT NULL DEFAULT 0,
    -- The exact text that was sent.
    body TEXT NOT NULL DEFAULT '',
    -- A link shown in the copy, empty when there is none.
    link TEXT NOT NULL DEFAULT '',
    buttons_live BOOLEAN NOT NULL DEFAULT TRUE,
    edit_pending BOOLEAN NOT NULL DEFAULT FALSE,
    closed_at TIMESTAMP WITH TIME ZONE,
    attempts INTEGER NOT NULL DEFAULT 0,
    claimed_at TIMESTAMP WITH TIME ZONE,
    -- Telegram's error text, already HTML-escaped.
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_lockdown_alerts_slot UNIQUE (lockdown_id, kind, seq, dest),
    CONSTRAINT chk_chat_lockdown_alert_kind CHECK (kind IN ('alert', 'reminder')),
    CONSTRAINT chk_chat_lockdown_alert_dest CHECK (dest IN ('staff', 'log')),
    CONSTRAINT chk_chat_lockdown_alert_state CHECK (state IN ('pending', 'posting', 'posted', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_chat_lockdown_alerts_lockdown_state ON chat_lockdown_alerts (lockdown_id, state);
CREATE INDEX IF NOT EXISTS idx_chat_lockdown_alerts_state ON chat_lockdown_alerts (state);
