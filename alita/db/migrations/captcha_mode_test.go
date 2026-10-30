package migrations

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const captchaModeMigration = "20261030120000_allow_turnstile_captcha_mode.sql"

// captchaModeStatements reads the captcha mode migration and returns the statements the
// runner would execute: the file goes through cleanSupabaseSQL and splitSQLStatements,
// and empty and transaction-control statements are dropped, exactly as applyMigration does.
func captchaModeStatements(t *testing.T) []string {
	t.Helper()
	migrationPath, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", captchaModeMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(migrationPath) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", captchaModeMigration, err)
	}
	runner := &MigrationRunner{}
	var run []string
	for _, stmt := range runner.splitSQLStatements(cleanSupabaseSQL(string(content))) {
		if strings.TrimSpace(stmt) == "" || isTransactionControlStatement(stmt) {
			continue
		}
		run = append(run, stmt)
	}
	return run
}

// TestCaptchaModeMigrationStatements proves, without a database, what the Go runner hands
// to PostgreSQL for the captcha mode migration: three statements in a fixed order (drop the
// named check, drop every other check on the column, add the widened check), none of them
// transaction control. The order is the point (D-29): both appliers swallow duplicate_object
// on an ADD CONSTRAINT, so an ADD that ran while the old check still existed would be a
// silent no-op and turnstile would stay refused in production.
func TestCaptchaModeMigrationStatements(t *testing.T) {
	run := captchaModeStatements(t)
	if len(run) != 3 {
		t.Fatalf("%s gives %d statements, want 3 (drop the named check, drop the legacy checks, add the widened check):\n%s",
			captchaModeMigration, len(run), strings.Join(run, "\n---\n"))
	}

	drop, legacy, add := run[0], run[1], run[2]

	if !strings.Contains(drop, "DROP CONSTRAINT IF EXISTS chk_captcha_mode") {
		t.Errorf("statement 1 must drop chk_captcha_mode if it exists, got:\n%s", drop)
	}

	if !strings.Contains(legacy, "pg_constraint") || !strings.Contains(legacy, "captcha_mode") {
		t.Errorf("statement 2 must walk pg_constraint for the checks that mention captcha_mode, got:\n%s", legacy)
	}
	if !strings.Contains(legacy, "DROP CONSTRAINT") {
		t.Errorf("statement 2 must drop the checks it finds, got:\n%s", legacy)
	}
	if strings.Contains(legacy, "duplicate_object") {
		t.Errorf("statement 2 must not be wrapped by the duplicate_object rewrite, got:\n%s", legacy)
	}

	if !strings.Contains(add, "'turnstile'") || !strings.Contains(add, "NOT VALID") {
		t.Errorf("statement 3 must add the check with turnstile as NOT VALID, got:\n%s", add)
	}
	if !strings.Contains(add, "duplicate_object") {
		t.Errorf("statement 3 should arrive wrapped by the runner's duplicate_object rewrite, got:\n%s", add)
	}

	for i, stmt := range run {
		if isTransactionControlStatement(stmt) {
			t.Errorf("statement %d is transaction control: %s", i+1, stmt)
		}
	}
}

// captchaModeChatID returns a random large negative chat ID, so the rows the test inserts
// cannot collide with anything already in the CI database.
func captchaModeChatID(t *testing.T) int64 {
	t.Helper()
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return -2000000000000 - int64(binary.BigEndian.Uint64(buf[:])&0x3fffffffffffffff)
}

// TestCaptchaModeTurnstileConstraint proves, on PostgreSQL only, that after the migration
// chain captcha_settings.captcha_mode accepts math, text and turnstile and nothing else, that
// exactly one CHECK on the table mentions captcha_mode and it is chk_captcha_mode (the
// legacy inline check of migration 20250807103246 is gone), and that the migration's
// statements can run twice (D-05, D-29).
//
// It needs ALITA_TEST_DATABASE=true and DATABASE_URL, and a database the migration chain
// already ran against (the CI integrity step). It runs inside one transaction that is always
// rolled back, with fresh chat IDs, so it leaves nothing behind. It is never run locally.
func TestCaptchaModeTurnstileConstraint(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ALITA_TEST_DATABASE")), "true") {
		t.Skip("set ALITA_TEST_DATABASE=true and DATABASE_URL to run the captcha mode constraint test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the captcha mode constraint test on PostgreSQL")
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

	for _, table := range []string{"captcha_settings", "chats"} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("table %s is missing: the migration chain must run before this test", table)
		}
	}

	run := captchaModeStatements(t)
	if len(run) == 0 {
		t.Fatalf("%s holds no statements", captchaModeMigration)
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below vanish, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the captcha mode transaction: %v", err)
		}
	}()

	execMigration := func(pass string) {
		t.Helper()
		for i, stmt := range run {
			if err := tx.Exec(stmt).Error; err != nil {
				t.Fatalf("%s: statement %d/%d of %s failed: %v", pass, i+1, len(run), captchaModeMigration, err)
			}
		}
	}

	checksOnMode := func(pass string) {
		t.Helper()
		var names []string
		if err := tx.Raw(
			`SELECT conname FROM pg_constraint
			 WHERE conrelid = 'captcha_settings'::regclass AND contype = 'c'
			   AND pg_get_constraintdef(oid) LIKE '%captcha_mode%'`,
		).Scan(&names).Error; err != nil {
			t.Fatalf("%s: list the captcha_mode checks: %v", pass, err)
		}
		if len(names) != 1 || names[0] != "chk_captcha_mode" {
			t.Errorf("%s: checks mentioning captcha_mode = %v, want exactly [chk_captcha_mode]", pass, names)
		}
	}

	now := time.Now().UTC()
	insertMode := func(mode string) error {
		chatID := captchaModeChatID(t)
		if err := tx.Exec(
			`INSERT INTO chats (chat_id, chat_name, language, is_inactive, created_at, updated_at)
			 VALUES (?, ?, ?, FALSE, ?, ?)`,
			chatID, "captcha mode test", "en", now, now,
		).Error; err != nil {
			t.Fatalf("insert chat %d: %v", chatID, err)
		}
		return tx.Exec(
			`INSERT INTO captcha_settings (chat_id, captcha_mode) VALUES (?, ?)`,
			chatID, mode,
		).Error
	}

	execMigration("first run")
	checksOnMode("first run")
	execMigration("second run")
	checksOnMode("second run (idempotent)")

	for _, mode := range []string{"turnstile", "math", "text"} {
		if err := insertMode(mode); err != nil {
			t.Errorf("captcha_mode %q must be accepted after the migration, got: %v", mode, err)
		}
	}

	// A refused insert aborts the transaction in PostgreSQL, so it runs behind a savepoint.
	if err := tx.Exec("SAVEPOINT bogus_mode").Error; err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	err = insertMode("bogus")
	if err == nil {
		t.Fatalf("captcha_mode 'bogus' was accepted, want a check violation")
	}
	if !strings.Contains(err.Error(), "SQLSTATE 23514") {
		t.Errorf("captcha_mode 'bogus' failed with %v, want SQLSTATE 23514 (check_violation)", err)
	}
	if err := tx.Exec("ROLLBACK TO SAVEPOINT bogus_mode").Error; err != nil {
		t.Fatalf("roll back to savepoint: %v", err)
	}
}
