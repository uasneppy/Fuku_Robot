-- Staff Groups and the groups linked to them.
--
-- A Staff Group is one Telegram group of trusted admins that manages other
-- groups. staff_groups holds the Staff Group itself; staff_group_links holds
-- one row per linked group.
--
-- There is deliberately no foreign key on the chat IDs (the federation_chats
-- precedent): a Staff Group or a linked group may be known to the bot before or
-- after it has a chats row, and tests run the same schema on SQLite.
--
-- staff_groups.owner_user_id is NOT unique: one owner may run several Staff
-- Groups. staff_group_links.owner_user_id is the user who made the link.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS staff_groups (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    owner_user_id BIGINT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- The unique constraint already provides the index every gate lookup uses.
    UNIQUE(chat_id)
);

CREATE INDEX IF NOT EXISTS idx_staff_groups_owner_user_id ON staff_groups(owner_user_id);

CREATE TABLE IF NOT EXISTS staff_group_links (
    id BIGSERIAL PRIMARY KEY,
    group_chat_id BIGINT NOT NULL,
    staff_chat_id BIGINT NOT NULL,
    owner_user_id BIGINT NOT NULL,
    group_title TEXT NOT NULL DEFAULT '',
    -- 24 characters wide: 'bot_cannot_restrict' is 19 characters long.
    health VARCHAR(24) NOT NULL DEFAULT 'ok',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- A group belongs to at most one Staff Group; this also serves group lookups.
    UNIQUE(group_chat_id),
    CONSTRAINT chk_staff_link_distinct CHECK (group_chat_id <> staff_chat_id),
    CONSTRAINT chk_staff_link_health CHECK (health IN ('ok', 'bot_missing', 'bot_not_admin', 'bot_cannot_restrict'))
);

CREATE INDEX IF NOT EXISTS idx_staff_group_links_staff_chat_id ON staff_group_links(staff_chat_id);
