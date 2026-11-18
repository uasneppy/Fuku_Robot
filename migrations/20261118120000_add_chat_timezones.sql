-- Per-group timezone (chat_timezones).
--
-- One row per chat that chose a timezone for the times the bot shows: an IANA zone name
-- (Europe/Kyiv) or a fixed offset written as UTC+hh:mm. A chat without a row uses the bot's
-- default (DEFAULT_TIMEZONE). It is read through the cache key chat_tz:<chat> and every write
-- drops that key. It is never part of backup, export, import or reset, and has no foreign key
-- on the chat (the staff and settings precedent).
--
-- The statement is idempotent and its constraints are inline, because both migration appliers
-- swallow duplicate_object on a bare constraint addition.

CREATE TABLE IF NOT EXISTS chat_timezones (
    chat_id BIGINT PRIMARY KEY,
    tz VARCHAR(64) NOT NULL,
    updated_by BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_chat_timezones_tz CHECK (length(tz) BETWEEN 1 AND 64)
);
