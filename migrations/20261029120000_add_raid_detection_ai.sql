-- AI burst trigger settings (chat_raid_detection).
--
-- Automatic raid detection gets a third trigger next to the join surge and the message
-- flood: the AI burst, which counts the messages the AI spam check deleted (this many
-- verdicts from at least 3 different accounts within 120 seconds). Each chat stores its own
-- switch and threshold for it:
--
--   ai_enabled    whether the AI burst trigger is on, default on
--   ai_threshold  how many AI spam verdicts fire it, default 5
--
-- The threshold is 3 to 500, the same range as the other two triggers, enforced by the
-- named CHECK on the column. Existing rows are filled by the column defaults, so every
-- chat that already has a row gets the trigger on at 5, and a chat without a row still
-- uses the defaults in Go (raiddetection.Defaults). The CHECK is part of the column
-- definition because PostgreSQL has no ADD CONSTRAINT IF NOT EXISTS, which keeps the
-- statement idempotent.
--
-- The table is still read through the cache, every write still invalidates its key, and
-- it is never part of backup, export, import or reset.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

ALTER TABLE chat_raid_detection ADD COLUMN IF NOT EXISTS ai_enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE chat_raid_detection ADD COLUMN IF NOT EXISTS ai_threshold INTEGER NOT NULL DEFAULT 5 CONSTRAINT chk_chat_raid_detection_ai CHECK (ai_threshold BETWEEN 3 AND 500);
