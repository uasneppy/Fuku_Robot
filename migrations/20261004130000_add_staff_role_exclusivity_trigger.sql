-- Database-level role exclusivity for Staff Groups (D-10).
--
-- A chat must never be both a Staff Group (a staff_groups row) and a linked
-- group (a staff_group_links row), and a Staff Group must never itself be linked
-- as a group. The rule spans two tables, so UNIQUE and CHECK constraints cannot
-- express it. The application checks it inside its own transactions, but two bot
-- replicas can interleave a check-then-insert and both commit.
--
-- This trigger closes that race. Before checking the other table it takes a
-- transaction-scoped advisory lock keyed on the chat ID. A concurrent writer
-- touching the same chat waits for the first transaction to finish, then its own
-- check sees the committed row and rejects the write (READ COMMITTED gives each
-- statement inside the function a fresh snapshot).
--
-- Lock ordering: a link locks both of its chat IDs, lower ID first, and a Staff
-- Group locks only its own ID. One consistent ascending order means no deadlock
-- cycle between the two kinds of write.
--
-- The key prefix 'alita:staff_role:' cannot collide with the advisory keys used
-- by the migration runner or the captcha repository.
--
-- A rejected write raises check_violation with a message starting with
-- 'staff role conflict'. The repository sees a generic database error and fails
-- closed; a retry reaches the application check, which reports the precise reason.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

CREATE OR REPLACE FUNCTION staff_role_exclusivity_guard()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_TABLE_NAME = 'staff_groups' THEN
        PERFORM pg_advisory_xact_lock(hashtextextended('alita:staff_role:' || NEW.chat_id::text, 0));

        IF EXISTS (SELECT 1 FROM staff_group_links WHERE group_chat_id = NEW.chat_id) THEN
            RAISE EXCEPTION 'staff role conflict: chat % is a linked group', NEW.chat_id
                USING ERRCODE = 'check_violation';
        END IF;
    ELSE
        PERFORM pg_advisory_xact_lock(hashtextextended(
            'alita:staff_role:' || LEAST(NEW.group_chat_id, NEW.staff_chat_id)::text, 0));
        PERFORM pg_advisory_xact_lock(hashtextextended(
            'alita:staff_role:' || GREATEST(NEW.group_chat_id, NEW.staff_chat_id)::text, 0));

        IF EXISTS (SELECT 1 FROM staff_groups WHERE chat_id = NEW.group_chat_id) THEN
            RAISE EXCEPTION 'staff role conflict: chat % is a Staff Group', NEW.group_chat_id
                USING ERRCODE = 'check_violation';
        END IF;

        IF EXISTS (
            SELECT 1 FROM staff_group_links
            WHERE group_chat_id = NEW.staff_chat_id AND id <> NEW.id
        ) THEN
            RAISE EXCEPTION 'staff role conflict: Staff Group % is itself linked', NEW.staff_chat_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_staff_groups_role_exclusivity ON staff_groups;
CREATE TRIGGER trg_staff_groups_role_exclusivity
    BEFORE INSERT OR UPDATE OF chat_id ON staff_groups
    FOR EACH ROW
    EXECUTE FUNCTION staff_role_exclusivity_guard();

DROP TRIGGER IF EXISTS trg_staff_group_links_role_exclusivity ON staff_group_links;
CREATE TRIGGER trg_staff_group_links_role_exclusivity
    BEFORE INSERT OR UPDATE OF group_chat_id, staff_chat_id ON staff_group_links
    FOR EACH ROW
    EXECUTE FUNCTION staff_role_exclusivity_guard();
