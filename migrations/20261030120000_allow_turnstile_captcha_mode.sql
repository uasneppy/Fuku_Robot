-- Allow the turnstile captcha mode (captcha_settings.captcha_mode).
--
-- A group can now choose the Turnstile web captcha next to the math and text ones, so the
-- column accepts 'turnstile' as well. The column is VARCHAR(10) and the new value has 9
-- characters, so only the CHECK changes.
--
-- Two CHECKs on this column exist in every database the chain built, not one:
--
--   chk_captcha_mode                    added by 20260412000000, allows math and text
--   captcha_settings_captcha_mode_check the unnamed inline check of 20250807103246, which
--                                       PostgreSQL named itself, allows math and text and
--                                       was never dropped by a later migration
--
-- Dropping only the first would leave the second refusing 'turnstile'. Both appliers also
-- swallow duplicate_object around a top-level constraint add, so an add that ran while the
-- old check still existed would change nothing and say nothing. The order is therefore:
--
--   1. drop chk_captcha_mode if it exists
--   2. drop every other CHECK on the table whose definition mentions captcha_mode
--   3. add chk_captcha_mode again, with turnstile
--
-- The re-added check is NOT VALID: it skips a scan of the existing rows (all of them are
-- math or text, so they would pass) and every new or updated row is checked. The rows are
-- not touched.
--
-- Every statement is idempotent; the Go runner applies each file once inside a
-- transaction and records its sha256 in schema_migrations.

ALTER TABLE captcha_settings DROP CONSTRAINT IF EXISTS chk_captcha_mode;

DO $$
DECLARE
    legacy RECORD;
BEGIN
    FOR legacy IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'captcha_settings'::regclass
          AND contype = 'c'
          AND conname <> 'chk_captcha_mode'
          AND pg_get_constraintdef(oid) LIKE '%captcha_mode%'
    LOOP
        EXECUTE format('ALTER TABLE captcha_settings DROP CONSTRAINT %I', legacy.conname);
    END LOOP;
END $$;

ALTER TABLE captcha_settings
ADD CONSTRAINT chk_captcha_mode
CHECK (captcha_mode IN ('math', 'text', 'turnstile')) NOT VALID;
