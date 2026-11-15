package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const staffForwardBlockMigration = "20261113130000_add_staff_forward_blocks.sql"

// staffForwardBlockStatements reads the forward block migration and returns the statements the
// runner would execute, dropping empty and transaction-control statements as applyMigration does.
func staffForwardBlockStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", staffForwardBlockMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", staffForwardBlockMigration, err)
	}
	runner := &MigrationRunner{}
	var run []string
	for _, stmt := range runner.splitSQLStatements(cleanSupabaseSQL(string(content))) {
		if strings.TrimSpace(stmt) == "" || isTransactionControlStatement(stmt) {
			continue
		}
		run = append(run, stripSQLComments(stmt))
	}
	return run
}

// TestStaffForwardBlockMigrationStatements proves, without a database, what the Go runner hands to
// PostgreSQL for the forward block migration: two idempotent CREATE TABLE statements carrying the
// three named constraints inline, no transaction control, no CONCURRENTLY, no NOT VALID and no
// top-level ADD CONSTRAINT, and a file name that sorts after the previous migration.
func TestStaffForwardBlockMigrationStatements(t *testing.T) {
	run := staffForwardBlockStatements(t)
	if len(run) != 2 {
		t.Fatalf("%s gives %d statements, want 2:\n%s", staffForwardBlockMigration, len(run), strings.Join(run, "\n---\n"))
	}
	wantPrefix := []string{
		"CREATE TABLE IF NOT EXISTS staff_forward_blocks ",
		"CREATE TABLE IF NOT EXISTS staff_forward_block_settings ",
	}
	for i, prefix := range wantPrefix {
		if !strings.HasPrefix(run[i], prefix) {
			t.Errorf("statement %d must start with %q, got:\n%s", i+1, prefix, run[i])
		}
	}
	all := strings.Join(run, "\n")
	for _, name := range []string{
		"uk_staff_forward_blocks_source",
		"chk_staff_forward_blocks_source",
		"chk_staff_forward_block_action",
	} {
		if !strings.Contains(all, "CONSTRAINT "+name+" ") {
			t.Errorf("the statements must carry constraint %s inline", name)
		}
	}
	for i, stmt := range run {
		upper := strings.ToUpper(stmt)
		if isTransactionControlStatement(stmt) {
			t.Errorf("statement %d is transaction control: %s", i+1, stmt)
		}
		for _, banned := range []string{"CONCURRENTLY", "NOT VALID", "BEGIN", "COMMIT", "ROLLBACK", "ADD CONSTRAINT", "REFERENCES"} {
			if strings.Contains(upper, banned) {
				t.Errorf("statement %d contains %s:\n%s", i+1, banned, stmt)
			}
		}
	}
	const previous = "20261113120000_drop_legacy_blacklist_action_check.sql"
	if staffForwardBlockMigration <= previous {
		t.Errorf("%s must sort after %s", staffForwardBlockMigration, previous)
	}
}

// TestStaffForwardBlockMigrationDefaults proves, on PostgreSQL only, that after the migration chain
// both tables exist, that the migration's statements run a second time without error, that a row
// inserted with only its required columns reads the defaults, and that the named constraints refuse
// a duplicate source (23505), a non-negative source (23514) and an action outside the five (23514).
func TestStaffForwardBlockMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the staff forward block migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the staff forward block migration test on PostgreSQL")
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

	for _, table := range []string{"staff_forward_blocks", "staff_forward_block_settings"} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("table %s is missing: the migration chain must run before this test", table)
		}
	}

	run := staffForwardBlockStatements(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below vanish, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the staff forward block transaction: %v", err)
		}
	}()

	for i, stmt := range run {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d/%d of %s failed: %v", i+1, len(run), staffForwardBlockMigration, err)
		}
	}

	staffChat := captchaModeChatID(t)
	if err := tx.Exec(`INSERT INTO staff_forward_blocks (staff_chat_id, source_chat_id) VALUES (?, -1001)`, staffChat).Error; err != nil {
		t.Fatalf("insert a block with only its required columns: %v", err)
	}
	if err := tx.Exec(`INSERT INTO staff_forward_block_settings (staff_chat_id) VALUES (?)`, staffChat).Error; err != nil {
		t.Fatalf("insert a settings row with only its required column: %v", err)
	}
	var block struct {
		SourceTitle string
		CreatedBy   int64
		HasCreated  bool
	}
	if err := tx.Raw(`SELECT source_title, created_by, created_at IS NOT NULL AS has_created
		FROM staff_forward_blocks WHERE staff_chat_id = ?`, staffChat).Scan(&block).Error; err != nil {
		t.Fatalf("read the block defaults: %v", err)
	}
	if block.SourceTitle != "" || block.CreatedBy != 0 || !block.HasCreated {
		t.Errorf("block defaults = %+v, want an empty title, created_by 0 and a creation time", block)
	}
	var action string
	if err := tx.Raw(`SELECT action FROM staff_forward_block_settings WHERE staff_chat_id = ?`, staffChat).Scan(&action).Error; err != nil {
		t.Fatalf("read the action default: %v", err)
	}
	if action != "none" {
		t.Errorf("action default = %q, want none", action)
	}

	refused := []struct {
		name, statement, sqlstate, constraint string
		args                                  []any
	}{
		{
			"duplicate source", `INSERT INTO staff_forward_blocks (staff_chat_id, source_chat_id) VALUES (?, -1001)`,
			"SQLSTATE 23505", "uk_staff_forward_blocks_source", []any{staffChat},
		},
		{
			"non-negative source", `INSERT INTO staff_forward_blocks (staff_chat_id, source_chat_id) VALUES (?, 5)`,
			"SQLSTATE 23514", "chk_staff_forward_blocks_source", []any{staffChat},
		},
		{
			"action x", `INSERT INTO staff_forward_block_settings (staff_chat_id, action) VALUES (?, 'x')`,
			"SQLSTATE 23514", "chk_staff_forward_block_action", []any{staffChat - 1},
		},
	}
	for _, tc := range refused {
		if err := tx.Exec("SAVEPOINT refused").Error; err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		err := tx.Exec(tc.statement, tc.args...).Error
		switch {
		case err == nil:
			t.Errorf("%s was accepted by the database", tc.name)
		case !strings.Contains(err.Error(), tc.sqlstate) || !strings.Contains(err.Error(), tc.constraint):
			t.Errorf("%s failed with %v, want %s naming %s", tc.name, err, tc.sqlstate, tc.constraint)
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT refused").Error; err != nil {
			t.Fatalf("roll back to savepoint: %v", err)
		}
	}
	for i, v := range []string{"none", "warn", "mute", "kick", "ban"} {
		if err := tx.Exec(`INSERT INTO staff_forward_block_settings (staff_chat_id, action) VALUES (?, ?)`, staffChat-100-int64(i), v).Error; err != nil {
			t.Errorf("action %s is refused: %v", v, err)
		}
	}
}
