-- Lockdown trigger columns (chat_lockdowns).
--
-- A lockdown can now be started by something other than an admin's /lockdown: the join
-- surge and flood triggers of automatic raid detection. The row records what fired and
-- the numbers behind it, so the alert and the status texts can render them in each
-- reader's language instead of carrying them inside the free-text reason.
--
--   trigger_count    joins (join_surge) or messages (flood) that fired the trigger
--   trigger_window   the window those were counted in, in seconds
--   trigger_percent  for a flood, the share of those messages from new members (0-100)
--
-- All three are 0 for a manual lockdown. trigger_kind itself has no CHECK, so the new
-- kinds join_surge and flood need no change to it.
--
-- idx_chat_lockdowns_chat_lift serves the quiet period after a lift: "was this chat's
-- last lockdown lifted less than N minutes ago" is a lookup by chat_id ordered by
-- lift_started_at.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS trigger_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS trigger_window INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS trigger_percent INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_chat_lockdowns_chat_lift ON chat_lockdowns (chat_id, lift_started_at);
