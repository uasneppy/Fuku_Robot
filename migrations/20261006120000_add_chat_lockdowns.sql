-- Manual lockdown state: one row per lockdown of a chat (chat_lockdowns) and one
-- row per user handled during it (chat_lockdown_joiners).
--
-- There is deliberately no foreign key on chat IDs (the staff and federation
-- precedent). The joiner table keeps a SQL-only foreign key to its lockdown; the Go
-- models declare no relation.
--
-- These tables are never cached and never part of backup, export, import or reset.
-- Every replica reads them fresh, so a new lockdown is enforced everywhere at once
-- and survives a restart and a Redis flush.
--
-- pre_permissions is the raw JSON of the permissions member of the getChat answer
-- read before the lock call, replayed verbatim at the lift. It is never produced
-- from a typed struct. locked_permissions is the set the lock call sent.
-- locked_at stays NULL until Telegram confirmed the lock, so a row whose lock never
-- took effect is never mistaken for a lockdown.
--
-- A chat has at most one active lockdown (uk_chat_lockdowns_active). It may have
-- older rows that are lifting or lifted.
--
-- chat_lockdown_joiners.ban_until is the marker that tells the lockdown's own ban
-- from a deliberate one: the lift unbans only a user whose live ban still ends at
-- exactly this value.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS chat_lockdowns (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    state VARCHAR(8) NOT NULL DEFAULT 'active',
    -- 'manual' today. Deliberately without a CHECK so a later phase adds a trigger
    -- kind without a migration.
    trigger_kind VARCHAR(16) NOT NULL DEFAULT 'manual',
    reason TEXT NOT NULL DEFAULT '',
    started_by BIGINT NOT NULL DEFAULT 0,
    started_by_name TEXT NOT NULL DEFAULT '',
    pre_permissions TEXT NOT NULL,
    locked_permissions TEXT NOT NULL DEFAULT '',
    -- NULL until Telegram confirmed the lock.
    locked_at TIMESTAMP WITH TIME ZONE,
    lifted_by BIGINT,
    lifted_by_name TEXT NOT NULL DEFAULT '',
    lift_started_at TIMESTAMP WITH TIME ZONE,
    lifted_at TIMESTAMP WITH TIME ZONE,
    -- Set when the live permissions were no longer the locked set at the lift.
    manual_change BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_chat_lockdown_state CHECK (state IN ('active', 'lifting', 'lifted'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_chat_lockdowns_active ON chat_lockdowns (chat_id) WHERE state = 'active';
CREATE INDEX IF NOT EXISTS idx_chat_lockdowns_state ON chat_lockdowns (state);

CREATE TABLE IF NOT EXISTS chat_lockdown_joiners (
    id BIGSERIAL PRIMARY KEY,
    lockdown_id BIGINT NOT NULL REFERENCES chat_lockdowns(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    -- What the joiner alert and "Revoke link" need, stored now so no column is added later.
    first_name TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    join_path VARCHAR(8) NOT NULL DEFAULT '',
    invite_link TEXT NOT NULL DEFAULT '',
    via_join_request BOOLEAN NOT NULL DEFAULT FALSE,
    performer_id BIGINT NOT NULL DEFAULT 0,
    state VARCHAR(16) NOT NULL DEFAULT 'pending',
    -- The end date of the lockdown's own ban, unix seconds; 0 until one is chosen.
    ban_until BIGINT NOT NULL DEFAULT 0,
    -- The join service message to delete, 0 when there is none.
    join_msg_id BIGINT NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    claimed_at TIMESTAMP WITH TIME ZONE,
    -- Telegram's error text, already HTML-escaped.
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_lockdown_joiners_user UNIQUE (lockdown_id, user_id),
    CONSTRAINT chk_chat_lockdown_joiner_state CHECK (state IN ('pending', 'acting', 'banned', 'ban_failed', 'declined', 'decline_failed', 'exempt', 'cancelled', 'unbanning', 'unbanned', 'kept', 'unban_failed')),
    CONSTRAINT chk_chat_lockdown_joiner_path CHECK (join_path IN ('', 'member', 'service', 'request'))
);

CREATE INDEX IF NOT EXISTS idx_chat_lockdown_joiners_lockdown_state ON chat_lockdown_joiners (lockdown_id, state);
CREATE INDEX IF NOT EXISTS idx_chat_lockdown_joiners_chat_user ON chat_lockdown_joiners (chat_id, user_id);
