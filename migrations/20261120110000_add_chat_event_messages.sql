-- Custom event messages of a group (chat_event_messages).
--
-- One row per (chat_id, event, dest): the message the bot posts for one event of one group
-- (today the manual punishments: ban, tban, kick, mute, tmute, unban, unmute) at one destination
-- (today only group). A row holds the owner's text, at most one media item (file_id and its
-- media_type, 0 meaning no media), the URL buttons (jsonb, '[]' for none, as the greetings and
-- rules rows store them) and the Silent switch, plus who changed it last. A chat with no row for
-- an event, or a row with every part empty and Silent off, sends today's default text.
--
-- The event and the destination are validated in Go (fuku/db/eventmsg: ValidEvent and
-- ValidDest), never by a database rule, so later phases add events and the pm destination with
-- no migration. The table is read through the cache key event_messages:<chat> (one struct per
-- chat), which is listed in skipLocal, and every write is a column-targeted upsert that drops
-- that key. It has no foreign key on the chat and is never part of backup, export, import or
-- reset in this phase (a later phase adds it to backups).
--
-- Every column is NOT NULL with a zero default, so a row inserted with only its key reads as
-- empty. The statement is idempotent and its primary key is inline, because both migration
-- appliers swallow duplicate_object on a bare constraint addition. The Go runner and
-- scripts/migrate_psql.sh both run this file unchanged, once, inside one transaction.

CREATE TABLE IF NOT EXISTS chat_event_messages (
    chat_id BIGINT NOT NULL,
    event VARCHAR(32) NOT NULL,
    dest VARCHAR(16) NOT NULL,
    text TEXT NOT NULL DEFAULT '',
    file_id TEXT NOT NULL DEFAULT '',
    media_type INTEGER NOT NULL DEFAULT 0,
    buttons JSONB NOT NULL DEFAULT '[]'::jsonb,
    silent BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_chat_event_messages PRIMARY KEY (chat_id, event, dest)
);
