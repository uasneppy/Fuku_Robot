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

const warnModeCheckMigration = "20261116120000_drop_legacy_warns_mode_check.sql"

// TestWarnModeCheckMigrationStatements proves, without a database, what the Go runner hands to
// PostgreSQL for the legacy warn mode check migration: one idempotent drop of chk_warns_mode
// (the legacy plural name, never chk_warn_mode, which allows the empty mode), no transaction
// control, and a file name that sorts after the migration that was newest when it was written.
func TestWarnModeCheckMigrationStatements(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", warnModeCheckMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", warnModeCheckMigration, err)
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
		t.Fatalf("%s gives %d statements, want 1:\n%s", warnModeCheckMigration, len(run), strings.Join(run, "\n---\n"))
	}
	if want := "ALTER TABLE warns_settings DROP CONSTRAINT IF EXISTS chk_warns_mode"; strings.TrimSuffix(run[0], ";") != want {
		t.Errorf("statement = %q, want %q", run[0], want)
	}
	upper := strings.ToUpper(string(content))
	for _, banned := range []string{"BEGIN", "COMMIT", "ROLLBACK", "CONCURRENTLY"} {
		if strings.Contains(upper, banned) {
			t.Errorf("the migration contains %s", banned)
		}
	}

	// The migration must sort after the one that was newest when it was written. A check that no
	// migration sorts after it would fail the moment any later migration is added.
	if previous := staffForwardBlockMigration; warnModeCheckMigration <= previous {
		t.Errorf("%s must sort after %s", warnModeCheckMigration, previous)
	}
}

// TestWarnModeCheckOnPostgres proves, on PostgreSQL only, that after the migration chain
// warns_settings.warn_mode accepts NULL, the empty mode (written by `/reset warnings` and a
// mode-less backup import before they stored mute, and still possible from an older build) and
// the five modes, refuses any other value with SQLSTATE 23514, and has exactly one CHECK
// constraint that mentions the column. Migration 20250805204145 added chk_warns_mode without
// the empty mode, and migration 20260412000000 dropped only the singular chk_warn_mode before
// adding its own check, so a chain-built database kept the legacy check until the migration
// that drops it.
//
// It needs FUKU_TEST_DATABASE=true and DATABASE_URL, and a database the migration chain
// already ran against (the CI integrity step). It runs inside one transaction that is always
// rolled back, with a fresh chat ID, so it leaves nothing behind.
func TestWarnModeCheckOnPostgres(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the warn mode check test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the warn mode check test on PostgreSQL")
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

	for _, table := range []string{"warns_settings", "chats"} {
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
			t.Errorf("roll back the warn mode transaction: %v", err)
		}
	}()

	var checks []string
	if err := tx.Raw(
		`SELECT conname FROM pg_constraint
		 WHERE conrelid = 'warns_settings'::regclass AND contype = 'c'
		   AND pg_get_constraintdef(oid) LIKE '%warn_mode%'`,
	).Scan(&checks).Error; err != nil {
		t.Fatalf("list the warn_mode checks: %v", err)
	}
	t.Logf("CHECK constraints mentioning warn_mode on warns_settings: %v", checks)
	if len(checks) != 1 || checks[0] != "chk_warn_mode" {
		t.Errorf("checks mentioning warn_mode = %v, want exactly [chk_warn_mode]", checks)
	}

	chatID := captchaModeChatID(t)
	now := time.Now().UTC()
	if err := tx.Exec(
		`INSERT INTO chats (chat_id, chat_name, language, is_inactive, created_at, updated_at)
		 VALUES (?, ?, ?, FALSE, ?, ?)`,
		chatID, "warn mode test", "en", now, now,
	).Error; err != nil {
		t.Fatalf("insert chat %d: %v", chatID, err)
	}
	if err := tx.Exec(
		`INSERT INTO warns_settings (chat_id, warn_limit, warn_mode, created_at, updated_at)
		 VALUES (?, 3, 'mute', ?, ?)`,
		chatID, now, now,
	).Error; err != nil {
		t.Fatalf("insert a warns_settings row with mode mute: %v", err)
	}

	// A refused statement aborts the transaction in PostgreSQL, so each runs behind a savepoint.
	update := func(mode any) error {
		if err := tx.Exec("SAVEPOINT warn_mode_probe").Error; err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		err := tx.Exec("UPDATE warns_settings SET warn_mode = ? WHERE chat_id = ?", mode, chatID).Error
		if rollbackErr := tx.Exec("ROLLBACK TO SAVEPOINT warn_mode_probe").Error; rollbackErr != nil {
			t.Fatalf("roll back to savepoint: %v", rollbackErr)
		}
		return err
	}
	if err := update(nil); err != nil {
		t.Errorf("warn_mode NULL must be accepted, got: %v", err)
	}
	for _, mode := range []string{"", "ban", "kick", "mute", "tban", "tmute"} {
		if err := update(mode); err != nil {
			t.Errorf("warn_mode %q must be accepted, got: %v", mode, err)
		}
	}
	err = update("x")
	if err == nil {
		t.Error("warn_mode \"x\" was accepted by the database")
	} else if !strings.Contains(err.Error(), "SQLSTATE 23514") {
		t.Errorf("warn_mode \"x\" failed with %v, want SQLSTATE 23514", err)
	}
}
