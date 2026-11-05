-- Settings hub state (chat_settings_access and chat_admin_index).
--
-- chat_settings_access holds the owner-only switch "Admins can change settings", one row
-- per chat. A chat without a row means owner only, the safe direction. The row is read
-- fresh from this table on every authority decision and is never cached. It is never part
-- of backup, export, import or reset, so restoring a backup can never widen who may change
-- a group's settings. There is deliberately no foreign key on the chat ID (the staff,
-- federation and lockdown precedent).
--
-- chat_admin_index is the bot's record of each group's creator and human administrators.
-- It is a hint for the private settings list only and never authority: every pick from the
-- list is checked against a live getChatMember. It is read fresh, never part of backup,
-- export, import or reset, and has no foreign key on the chat or the user.
--
-- Every statement is idempotent and every constraint is inline in its table, because both
-- migration appliers swallow duplicate_object on a bare constraint addition. The Go runner
-- applies each file once inside a transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS chat_settings_access (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    admins_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_settings_access_chat UNIQUE (chat_id)
);

CREATE TABLE IF NOT EXISTS chat_admin_index (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    role VARCHAR(16) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_admin_index_chat_user UNIQUE (chat_id, user_id),
    CONSTRAINT chk_chat_admin_index_role CHECK (role IN ('creator', 'administrator'))
);

CREATE INDEX IF NOT EXISTS idx_chat_admin_index_user_id ON chat_admin_index (user_id);
