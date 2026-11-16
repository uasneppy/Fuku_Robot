-- Drop the legacy chk_blacklists_action check of blacklists.action (Phase 19, research C-16).
--
-- Migration 20250805204145 added chk_blacklists_action with the seven values warn, mute, ban,
-- kick, tban, tmute and delete, which lacks none. Migration 20260412000000 later dropped only
-- the singular chk_blacklist_action before adding its own eight-value check (with none), so a
-- database the chain built carries both and the legacy one makes `/blacklistaction none`
-- (and the blacklist page's None choice) fail with SQLSTATE 23514. Dropping it leaves
-- chk_blacklist_action, the eight-value check, as the only check on the column.
ALTER TABLE blacklists DROP CONSTRAINT IF EXISTS chk_blacklists_action;
