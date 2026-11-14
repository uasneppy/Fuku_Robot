-- Rules media, Media under text and Greet once.
--
-- greetings gets three switches, every one off for a row saved before this migration, so
-- every existing welcome and goodbye reads and sends exactly as it did:
--
--   welcome_media_below  show the welcome's media under its text instead of above it
--   goodbye_media_below  the same for the goodbye
--   welcome_once         greet a member only the first time they ever join the group
--
-- rules gets the media and buttons a rules message can carry, plus its own Media under
-- text switch. A rules row saved before this migration has none of them (text only):
--
--   file_id      the stored media file ID, empty for no media
--   media_type   the media kind, 0 means no media
--   btns         the rules message's URL buttons as JSON, '[]' for none
--   media_below  show the media under the text instead of above it
--
-- greeted_members is the Greet once record: one row per (chat, member) the bot has
-- welcomed while the switch was on, kept for good. It has no foreign key on the chat or
-- the user, is read fresh and never cached, and is never part of backup, export, import
-- or reset. The primary key is what makes a claim by two replicas at once win once.
--
-- Every statement is idempotent. The Go runner and scripts/migrate_psql.sh both run this
-- file unchanged; the runner applies each file once inside a transaction and records its
-- sha256 in schema_migrations.

ALTER TABLE greetings ADD COLUMN IF NOT EXISTS welcome_media_below BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE greetings ADD COLUMN IF NOT EXISTS goodbye_media_below BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE greetings ADD COLUMN IF NOT EXISTS welcome_once BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE rules ADD COLUMN IF NOT EXISTS file_id TEXT NOT NULL DEFAULT '';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS media_type BIGINT NOT NULL DEFAULT 0;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS btns JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS media_below BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS greeted_members (
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, user_id)
);
