package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const blacklistActionMigration = "20261113120000_drop_legacy_blacklist_action_check.sql"

// TestBlacklistActionMigrationStatements proves, without a database, what the Go runner hands
// to PostgreSQL for the legacy blacklist action check migration: one idempotent drop of
// chk_blacklists_action (the legacy plural name, never the eight-value chk_blacklist_action),
// no transaction control, and a file name that sorts after every other migration.
func TestBlacklistActionMigrationStatements(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", blacklistActionMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", blacklistActionMigration, err)
	}
	runner := &MigrationRunner{}
	var run []string
	for _, stmt := range runner.splitSQLStatements(cleanSupabaseSQL(string(content))) {
		if strings.TrimSpace(stmt) == "" || isTransactionControlStatement(stmt) {
			continue
		}
		run = append(run, strings.TrimSpace(stripSQLComments(stmt)))
	}
	if len(run) != 1 {
		t.Fatalf("%s gives %d statements, want 1:\n%s", blacklistActionMigration, len(run), strings.Join(run, "\n---\n"))
	}
	if want := "ALTER TABLE blacklists DROP CONSTRAINT IF EXISTS chk_blacklists_action"; strings.TrimSuffix(run[0], ";") != want {
		t.Errorf("statement = %q, want %q", run[0], want)
	}
	upper := strings.ToUpper(string(content))
	for _, banned := range []string{"BEGIN", "COMMIT", "ROLLBACK", "CONCURRENTLY"} {
		if strings.Contains(upper, banned) {
			t.Errorf("the migration contains %s", banned)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	for _, entry := range entries {
		if name := entry.Name(); strings.HasSuffix(name, ".sql") && name > blacklistActionMigration {
			t.Errorf("%s sorts after %s: a newer migration must carry a later timestamp", name, blacklistActionMigration)
		}
	}
}

// TestBlacklistActionValuesOnPostgres proves, on PostgreSQL only, that after the migration
// chain blacklists.action accepts every value /blacklistaction writes (warn, mute, kick, ban
// and none), refuses any other with SQLSTATE 23514, and has exactly one CHECK constraint
// that mentions the column. Research C-16: migration 20250805204145 added chk_blacklists_action
// without none, and migration 20260412000000 dropped only the singular chk_blacklist_action
// before adding its own eight-value check, so a chain-built database kept the legacy check and
// refused `/blacklistaction none` until the migration that drops it.
//
// It needs FUKU_TEST_DATABASE=true and DATABASE_URL, and a database the migration chain
// already ran against (the CI integrity step). It runs inside one transaction that is always
// rolled back, with a fresh chat ID, so it leaves nothing behind.
func TestBlacklistActionValuesOnPostgres(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the blacklist action test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the blacklist action test on PostgreSQL")
	}

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL handle: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close PostgreSQL: %v", err)
		}
	})

	for _, table := range []string{"blacklists", "chats"} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("table %s is missing: the migration chain must run before this test", table)
		}
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below vanish with the rollback.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the blacklist action transaction: %v", err)
		}
	}()

	var checks []string
	if err := tx.Raw(
		`SELECT conname FROM pg_constraint
		 WHERE conrelid = 'blacklists'::regclass AND contype = 'c'
		   AND pg_get_constraintdef(oid) LIKE '%action%'`,
	).Scan(&checks).Error; err != nil {
		t.Fatalf("list the action checks: %v", err)
	}
	t.Logf("CHECK constraints mentioning action on blacklists: %v", checks)
	if len(checks) != 1 || checks[0] != "chk_blacklist_action" {
		t.Errorf("checks mentioning action = %v, want exactly [chk_blacklist_action]", checks)
	}

	chatID := captchaModeChatID(t)
	now := time.Now().UTC()
	if err := tx.Exec(
		`INSERT INTO chats (chat_id, chat_name, language, is_inactive, created_at, updated_at)
		 VALUES (?, ?, ?, FALSE, ?, ?)`,
		chatID, "blacklist action test", "en", now, now,
	).Error; err != nil {
		t.Fatalf("insert chat %d: %v", chatID, err)
	}
	if err := tx.Exec(
		`INSERT INTO blacklists (chat_id, word, action, reason, created_at, updated_at)
		 VALUES (?, 'probe', 'warn', 'Blacklisted word: ''%s''', ?, ?)`,
		chatID, now, now,
	).Error; err != nil {
		t.Fatalf("insert a blacklist row with action warn: %v", err)
	}

	// A refused statement aborts the transaction in PostgreSQL, so each runs behind a savepoint.
	update := func(action string) error {
		if err := tx.Exec("SAVEPOINT action_probe").Error; err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		err := tx.Exec("UPDATE blacklists SET action = ? WHERE chat_id = ?", action, chatID).Error
		if rollbackErr := tx.Exec("ROLLBACK TO SAVEPOINT action_probe").Error; rollbackErr != nil {
			t.Fatalf("roll back to savepoint: %v", rollbackErr)
		}
		return err
	}
	for _, action := range []string{"warn", "mute", "kick", "ban", "none"} {
		if err := update(action); err != nil {
			t.Errorf("action %q must be accepted, got: %v", action, err)
		}
	}
	err = update("x")
	if err == nil {
		t.Error("action \"x\" was accepted by the database")
	} else if !strings.Contains(err.Error(), "SQLSTATE 23514") {
		t.Errorf("action \"x\" failed with %v, want SQLSTATE 23514", err)
	}
}
