-- Lockdown ban run report delivery (chat_lockdown_ban_runs).
--
-- A finished ban run (done or stopped) owes two deliveries: the edit of its progress card
-- to the result, and the one #LOCKDOWN_BAN post in the locked group's log channel. Until
-- now the replica whose FinishBanRun won made both in one pass and nothing recorded them,
-- so a rate limit, a shutdown or a replica that died lost them for good. These columns
-- make the delivery resumable from the database like every other lockdown delivery.
--
--   report_card_done_at  the card edit was delivered, or given up on after its attempts
--   report_log_done_at   the log post was delivered, or given up on after its attempts
--   reported_at          both deliveries are settled; the run is never looked at again
--   report_claimed_at    set while a worker is delivering; a stale claim is given back
--   report_attempts      failed attempts of the delivery step in progress; it goes back to
--                        zero when a step is settled so one step cannot use up the other's
--
-- Runs that finished before this migration were delivered by the old code (or lost with
-- no way to tell), so they are marked reported and never delivered again.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

ALTER TABLE chat_lockdown_ban_runs ADD COLUMN IF NOT EXISTS report_card_done_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE chat_lockdown_ban_runs ADD COLUMN IF NOT EXISTS report_log_done_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE chat_lockdown_ban_runs ADD COLUMN IF NOT EXISTS reported_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE chat_lockdown_ban_runs ADD COLUMN IF NOT EXISTS report_claimed_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE chat_lockdown_ban_runs ADD COLUMN IF NOT EXISTS report_attempts INTEGER NOT NULL DEFAULT 0;

UPDATE chat_lockdown_ban_runs SET reported_at = NOW() WHERE state <> 'running' AND reported_at IS NULL;
