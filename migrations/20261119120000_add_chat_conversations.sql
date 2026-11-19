-- Per-group conversation setting (chat_conversations).
--
-- One row per chat whose owner changed the conversation setting in the private settings
-- menu: whether the bot answers members who address it, which provider it asks (anthropic or
-- google) and who may talk to it (0 everyone, 1 admins and approved members). A chat without a
-- row means off, anthropic, everyone. It is read through the cache key chat_conv:<chat>, which
-- skips the in-process layer, and every write drops that key. It is never part of backup,
-- export, import or reset, so a restored backup can never switch conversations on, and it has
-- no foreign key on the chat (the staff and settings precedent).
--
-- The statement is idempotent and its constraints are inline, because both migration appliers
-- swallow duplicate_object on a bare constraint addition.

CREATE TABLE IF NOT EXISTS chat_conversations (
    chat_id BIGINT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    provider VARCHAR(16) NOT NULL DEFAULT 'anthropic',
    audience SMALLINT NOT NULL DEFAULT 0,
    updated_by BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_chat_conversations_provider CHECK (provider IN ('anthropic', 'google')),
    CONSTRAINT chk_chat_conversations_audience CHECK (audience BETWEEN 0 AND 1)
);
