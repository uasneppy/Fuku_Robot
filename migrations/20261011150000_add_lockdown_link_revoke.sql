-- Lockdown invite-link revoke: who revoked the raid's invite link from the alert.
--
-- An admin of the locked group can revoke the bot-created invite link a raid came
-- through with the alert's "Revoke link" button. One revoke is recorded per lockdown:
-- link_revoked_at is NULL until Telegram confirmed the revoke, and the conditional update
-- that sets it (WHERE link_revoked_at IS NULL) decides which of two simultaneous
-- confirmations won. link_revoked_link is the link that was revoked, kept so the record
-- stays true if the alert copies are edited later.
--
-- These columns are never cached and never part of backup, export, import or reset.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS link_revoked_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS link_revoked_by BIGINT;
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS link_revoked_by_name TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS link_revoked_link TEXT NOT NULL DEFAULT '';
