-- Scheduled deletion of service and bot messages: the rules of each group and the queue.
--
-- chat_delete_rules is one chat's delay for one kind of message, one row per (chat, kind).
-- A kind is "service:<type>" (a Telegram service message) or "bot:<kind>" (a message the bot
-- sent). delay_secs is 0 for "delete now" and 1..86400 for "delete after N seconds", capped
-- below Telegram's 48 hour deletion window by the named CHECK. A chat with no row for a kind
-- means Keep (or Off for a bot kind), so a group changes nothing until its owner chooses.
-- Kinds are validated in Go, never by a CHECK, so a later message type needs no migration.
--
-- scheduled_deletions is the durable queue of messages waiting for their time. One row per
-- (chat, message), unique on that pair, so scheduling a message twice keeps one row and the
-- earlier delete_at. sent_at is the message's own date, which the worker uses to drop a row
-- Telegram can no longer delete. state is pending or acting: a worker claims due rows with
-- one conditional update that sets acting, a token and claimed_at, and completes or releases
-- them by that token. attempts counts failed tries.
--
-- Neither table has a foreign key. chat_delete_rules is read through the cache under the
-- "delete_rules" key and every write drops it; scheduled_deletions is read fresh and never
-- cached. Neither table is part of backup, export, import or reset.
--
-- Every statement is idempotent and carries its constraints inline. The Go runner and
-- scripts/migrate_psql.sh both run this file unchanged; the runner applies each file once
-- inside a transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS chat_delete_rules (
    chat_id BIGINT NOT NULL,
    kind VARCHAR(40) NOT NULL,
    delay_secs INTEGER NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, kind),
    CONSTRAINT chk_chat_delete_rules_delay CHECK (delay_secs BETWEEN 0 AND 86400)
);

CREATE TABLE IF NOT EXISTS scheduled_deletions (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    kind VARCHAR(40) NOT NULL,
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delete_at TIMESTAMP WITH TIME ZONE NOT NULL,
    state VARCHAR(8) NOT NULL DEFAULT 'pending',
    claim_token VARCHAR(32) NOT NULL DEFAULT '',
    claimed_at TIMESTAMP WITH TIME ZONE NULL,
    attempts SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_scheduled_deletions_message UNIQUE (chat_id, message_id),
    CONSTRAINT chk_scheduled_deletions_state CHECK (state IN ('pending', 'acting')),
    CONSTRAINT chk_scheduled_deletions_attempts CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_deletions_due ON scheduled_deletions (delete_at) WHERE state = 'pending';

CREATE INDEX IF NOT EXISTS idx_scheduled_deletions_acting ON scheduled_deletions (claimed_at) WHERE state = 'acting';
