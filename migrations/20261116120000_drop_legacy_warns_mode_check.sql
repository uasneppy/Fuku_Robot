-- Drop the legacy chk_warns_mode check of warns_settings.warn_mode.
--
-- Migration 20250805204145 added chk_warns_mode as NULL or one of ban, kick, mute, tban and
-- tmute, which refuses the empty mode. Migration 20260412000000 later dropped only the singular
-- chk_warn_mode before adding its own check (NULL, empty or one of the five), so a database the
-- chain built carries both, and the legacy one makes a write of an empty mode (an older
-- `/reset warnings`, a mode-less backup import) fail with SQLSTATE 23514. Dropping it leaves
-- chk_warn_mode as the only check on the column.
ALTER TABLE warns_settings DROP CONSTRAINT IF EXISTS chk_warns_mode;
