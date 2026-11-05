-- Member requirements: the settings of each group and the hold of each member.
--
-- chat_requirements is one chat's choice of punishment for each member requirement rule
-- (last name, username, Russian, Arabic and Chinese letters): 0 off, 1 warn, 2 kick, 3 ban,
-- 4 mute, each under its own named CHECK, plus the check-on-join and delete-messages
-- switches. A chat without a row means every rule off and both switches off, so a group
-- changes nothing until its owner chooses.
--
-- chat_requirement_holds is one member's episode in one chat. It is the latch that makes a
-- violation punished once however many replicas see it (the unique key on chat and user,
-- claimed with INSERT ... ON CONFLICT DO NOTHING) and the record of a mute the bot placed.
-- It is read fresh and never cached.
--
-- Neither table has a foreign key, and neither is part of backup, export, import or reset.
--
-- Every statement is idempotent and carries its constraints inline. The Go runner and
-- scripts/migrate_psql.sh both run this file unchanged; the runner applies each file once
-- inside a transaction and records its sha256 in schema_migrations.

CREATE TABLE IF NOT EXISTS chat_requirements (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    last_name_action SMALLINT NOT NULL DEFAULT 0,
    username_action SMALLINT NOT NULL DEFAULT 0,
    russian_action SMALLINT NOT NULL DEFAULT 0,
    arabic_action SMALLINT NOT NULL DEFAULT 0,
    chinese_action SMALLINT NOT NULL DEFAULT 0,
    check_on_join BOOLEAN NOT NULL DEFAULT FALSE,
    delete_messages BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_requirements_chat UNIQUE (chat_id),
    CONSTRAINT chk_chat_requirements_last_name CHECK (last_name_action BETWEEN 0 AND 4),
    CONSTRAINT chk_chat_requirements_username CHECK (username_action BETWEEN 0 AND 4),
    CONSTRAINT chk_chat_requirements_russian CHECK (russian_action BETWEEN 0 AND 4),
    CONSTRAINT chk_chat_requirements_arabic CHECK (arabic_action BETWEEN 0 AND 4),
    CONSTRAINT chk_chat_requirements_chinese CHECK (chinese_action BETWEEN 0 AND 4)
);

CREATE TABLE IF NOT EXISTS chat_requirement_holds (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    rules INTEGER NOT NULL,
    action SMALLINT NOT NULL,
    mute_until BIGINT NOT NULL DEFAULT 0,
    welcome_held BOOLEAN NOT NULL DEFAULT FALSE,
    notice_msg_id BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    punished_at TIMESTAMP WITH TIME ZONE NULL,
    CONSTRAINT uk_chat_requirement_holds_chat_user UNIQUE (chat_id, user_id),
    CONSTRAINT chk_chat_requirement_holds_rules CHECK (rules > 0),
    CONSTRAINT chk_chat_requirement_holds_action CHECK (action BETWEEN 1 AND 4)
);
