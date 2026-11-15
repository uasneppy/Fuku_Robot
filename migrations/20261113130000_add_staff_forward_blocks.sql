-- Staff Group forward source blocklist: the channels or groups whose forwards the bot deletes in
-- every group linked to a Staff Group, and the one action applied to the member who forwarded.
--
-- staff_forward_blocks is one listed source of one Staff Group. source_chat_id is the numeric
-- Telegram chat ID of the source (always negative: a channel or a group), stored as typed or as
-- resolved from an @username once, and never re-resolved. source_title is the title Telegram gave
-- when the source was added (empty when the bot could not see it) and is shown only to the Staff
-- Group's own administrators. A source is listed once per Staff Group, under its unique key.
--
-- staff_forward_block_settings is the list's one action, one row per Staff Group: none (delete the
-- forward only), warn, mute, kick or ban, the vocabulary of the blacklist. A Staff Group without a
-- row means none. The action is written by a column-targeted upsert so a value is always stored.
--
-- Neither table has a foreign key: RekeyChat moves staff_chat_id with the Staff Group on a chat
-- migration, and the rows are deleted with the Staff Group in the same transaction. Neither table
-- is part of backup, export, import or reset.
--
-- Every statement is idempotent and carries its constraints inline. The Go runner and
-- scripts/migrate_psql.sh both run this file unchanged; the runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS staff_forward_blocks (
    id BIGSERIAL PRIMARY KEY,
    staff_chat_id BIGINT NOT NULL,
    source_chat_id BIGINT NOT NULL,
    source_title TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_staff_forward_blocks_source UNIQUE (staff_chat_id, source_chat_id),
    CONSTRAINT chk_staff_forward_blocks_source CHECK (source_chat_id < 0)
);

CREATE TABLE IF NOT EXISTS staff_forward_block_settings (
    staff_chat_id BIGINT PRIMARY KEY,
    action VARCHAR(8) NOT NULL DEFAULT 'none',
    updated_by BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_staff_forward_block_action CHECK (action IN ('none','warn','mute','kick','ban'))
);
