-- Automatic raid detection settings (chat_raid_detection).
--
-- One row per chat holds the two automatic triggers' switches and thresholds: the join
-- surge (this many different accounts joining within 60 seconds) and the message flood
-- (this many messages from a chat within 30 seconds, most of them from new members). A
-- chat without a row uses the defaults, both triggers on, join surge at 15 and flood at
-- 30, so every group is protected until an admin changes it.
--
-- The table is read through the cache and every write invalidates its key. It is never
-- part of backup, export, import or reset. There is deliberately no foreign key on the
-- chat ID (the staff, federation and lockdown precedent).
--
-- The thresholds are 3 to 500. The old antiraid auto trigger (antiraid_settings
-- .auto_antiraid_threshold, 0 meaning off) is carried over for every chat that had it
-- on, as that chat's join surge threshold, clamped into the same range in both
-- directions: a value below 3 becomes 3, and a value above 500 becomes 500. The old
-- /autoantiraid accepted values far above 500, and a copy that broke the CHECK below
-- would abort the migration and so the bot's startup (D-22).
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS chat_raid_detection (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    join_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    join_threshold INTEGER NOT NULL DEFAULT 15,
    flood_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    flood_threshold INTEGER NOT NULL DEFAULT 30,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_raid_detection_chat UNIQUE (chat_id),
    CONSTRAINT chk_chat_raid_detection_join CHECK (join_threshold BETWEEN 3 AND 500),
    CONSTRAINT chk_chat_raid_detection_flood CHECK (flood_threshold BETWEEN 3 AND 500)
);

-- Carry-over of the retired auto threshold (D-03, D-18, D-22): a chat that had it on keeps
-- its number as the join surge threshold. LEAST(GREATEST(.., 3), 500) raises a value below
-- 3 to 3 (D-18) and lowers one above 500 to 500 (D-22), so the row always satisfies the
-- CHECK above.
INSERT INTO chat_raid_detection (chat_id, join_enabled, join_threshold, flood_enabled, flood_threshold)
SELECT chat_id, TRUE, LEAST(GREATEST(auto_antiraid_threshold, 3), 500), TRUE, 30
FROM antiraid_settings
WHERE auto_antiraid_threshold > 0
ON CONFLICT (chat_id) DO NOTHING;
