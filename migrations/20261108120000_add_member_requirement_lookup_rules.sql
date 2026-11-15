-- Member requirements, the lookup rules: a profile photo and a channel subscription.
--
-- chat_requirements gets two more action columns next to the five name rules, one per new
-- rule, with the same values: 0 off, 1 warn, 2 kick, 3 ban, 4 mute.
--
--   photo_action    the punishment for a member who has no profile photo
--   channel_action  the punishment for a member who is not subscribed to the channel the
--                   group is the comment group of
--
-- Each column is NOT NULL DEFAULT 0 under its own named CHECK (0..4). A chat that has no
-- row, and a row written before this file, therefore reads both rules off, so no group
-- screens anybody for them until its owner chooses a punishment. The CHECK is part of the
-- column definition because PostgreSQL has no ADD CONSTRAINT IF NOT EXISTS, which is what
-- keeps each statement idempotent: ADD COLUMN IF NOT EXISTS skips the whole clause, the
-- constraint included, when the column already exists.
--
-- The table is still read through the cache, every write still invalidates its key, and
-- it is never part of backup, export, import or reset.
--
-- The Go runner and scripts/migrate_psql.sh both run this file unchanged; the runner
-- applies each file once inside a transaction and records its sha256 in schema_migrations.

ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS photo_action SMALLINT NOT NULL DEFAULT 0 CONSTRAINT chk_chat_requirements_photo CHECK (photo_action BETWEEN 0 AND 4);
ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS channel_action SMALLINT NOT NULL DEFAULT 0 CONSTRAINT chk_chat_requirements_channel CHECK (channel_action BETWEEN 0 AND 4);
