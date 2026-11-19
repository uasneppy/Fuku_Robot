-- The per-group options of the private copies of event messages (chat_event_message_options).
--
-- One row per chat: the two options that decide what happens when a private copy of an event
-- message could not be delivered to the punished person. pm_details adds a Details button to the
-- group message (it opens the bot's private chat and shows that person their copy), and pm_log
-- posts one line in the group's log channel. Both are off for a chat with no row, plus who
-- changed them last.
--
-- The table is read together with chat_event_messages through the cache key
-- event_messages:<chat> (one struct per chat), which is listed in skipLocal, and every write is a
-- column-targeted upsert of its own option that drops that key. It has no foreign key on the chat
-- and is never part of backup, export, import or reset in this phase (a later phase decides it
-- together with the private copies).
--
-- Every column is NOT NULL with a zero default, so a row inserted with only its key reads both
-- options off. The statement is idempotent and its primary key is inline, because both migration
-- appliers swallow duplicate_object on a bare constraint addition. The Go runner and
-- scripts/migrate_psql.sh both run this file unchanged, once, inside one transaction.

CREATE TABLE IF NOT EXISTS chat_event_message_options (
    chat_id BIGINT NOT NULL,
    pm_details BOOLEAN NOT NULL DEFAULT FALSE,
    pm_log BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_chat_event_message_options PRIMARY KEY (chat_id)
);
