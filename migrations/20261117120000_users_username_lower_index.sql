-- Index users by LOWER(username).
--
-- user.FindUsersByUsername (the Staff Group target resolver, and through it
-- user.GetUserIdByUserName and every @username lookup) matches with
-- LOWER(username) = LOWER(?), which the plain idx_users_user_name on username cannot
-- serve, so every lookup scanned the users table. This expression index serves it.
-- It is a plain CREATE INDEX because each migration file runs in one transaction.
CREATE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username));
