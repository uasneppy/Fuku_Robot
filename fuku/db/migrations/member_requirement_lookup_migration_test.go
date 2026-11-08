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

const memberRequirementLookupMigration = "20261108120000_add_member_requirement_lookup_rules.sql"

// memberRequirementLookupStatements reads the lookup rules migration and returns the
// statements the runner would execute, dropping empty and transaction-control statements
// exactly as applyMigration does.
func memberRequirementLookupStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", memberRequirementLookupMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", memberRequirementLookupMigration, err)
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

// TestMemberRequirementLookupMigrationStatements proves, without a database, what the Go
// runner hands to PostgreSQL for the lookup rules migration: two idempotent column
// additions to chat_requirements, each SMALLINT NOT NULL DEFAULT 0 with its named CHECK
// 0..4 inline, no transaction control, no CONCURRENTLY, no NOT VALID and no top-level
// constraint addition (both appliers swallow duplicate_object on a bare ADD CONSTRAINT),
// and a file name that sorts after the member requirements migration.
func TestMemberRequirementLookupMigrationStatements(t *testing.T) {
	run := memberRequirementLookupStatements(t)
	if len(run) != 2 {
		t.Fatalf("%s gives %d statements, want 2 (two columns):\n%s",
			memberRequirementLookupMigration, len(run), strings.Join(run, "\n---\n"))
	}

	columns := []struct{ column, constraint string }{
		{"photo_action", "chk_chat_requirements_photo"},
		{"channel_action", "chk_chat_requirements_channel"},
	}
	for i, want := range columns {
		stmt := run[i]
		if !strings.Contains(stmt, "ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS "+want.column+" ") {
			t.Errorf("statement %d must add %s to chat_requirements idempotently, got:\n%s", i+1, want.column, stmt)
		}
		if !strings.Contains(stmt, want.column+" SMALLINT NOT NULL DEFAULT 0 ") {
			t.Errorf("statement %d: %s must be SMALLINT NOT NULL DEFAULT 0, got:\n%s", i+1, want.column, stmt)
		}
		if !strings.Contains(stmt, "CONSTRAINT "+want.constraint+" CHECK ("+want.column+" BETWEEN 0 AND 4)") {
			t.Errorf("statement %d must carry constraint %s inline, got:\n%s", i+1, want.constraint, stmt)
		}
	}

	for i, stmt := range run {
		upper := strings.ToUpper(stmt)
		if isTransactionControlStatement(stmt) {
			t.Errorf("statement %d is transaction control: %s", i+1, stmt)
		}
		for _, banned := range []string{"CONCURRENTLY", "NOT VALID", "BEGIN", "COMMIT", "ROLLBACK"} {
			if strings.Contains(upper, banned) {
				t.Errorf("statement %d contains %s:\n%s", i+1, banned, stmt)
			}
		}
		// A top-level ADD CONSTRAINT is the one form both appliers rewrite; the inline
		// CONSTRAINT of ADD COLUMN is not, so the statement must start with ALTER TABLE ... ADD COLUMN.
		if strings.Contains(upper, "ADD CONSTRAINT") {
			t.Errorf("statement %d contains ADD CONSTRAINT:\n%s", i+1, stmt)
		}
	}

	if memberRequirementLookupMigration <= memberRequirementsMigration {
		t.Errorf("%s must sort after %s", memberRequirementLookupMigration, memberRequirementsMigration)
	}
}

// TestMemberRequirementLookupMigrationDefaults proves, on PostgreSQL only, that after the
// migration chain chat_requirements has photo_action and channel_action, that the
// migration's statements run a second time without error, that a row inserted with only
// its chat_id reads both columns 0 (off), that both named constraints exist, and that a
// value of 5 is refused with SQLSTATE 23514 naming the constraint.
func TestMemberRequirementLookupMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the lookup rules migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the lookup rules migration test on PostgreSQL")
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

	if !database.Migrator().HasTable("chat_requirements") {
		t.Fatal("table chat_requirements is missing: the migration chain must run before this test")
	}

	run := memberRequirementLookupStatements(t)
	if len(run) == 0 {
		t.Fatalf("%s holds no statements", memberRequirementLookupMigration)
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the row below vanishes, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the lookup rules transaction: %v", err)
		}
	}()

	for i, stmt := range run {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d/%d of %s failed: %v", i+1, len(run), memberRequirementLookupMigration, err)
		}
	}

	chatID := captchaModeChatID(t)
	if err := tx.Exec(`INSERT INTO chat_requirements (chat_id) VALUES (?)`, chatID).Error; err != nil {
		t.Fatalf("insert a chat_requirements row with only its chat_id: %v", err)
	}
	var stored struct {
		PhotoAction   int64
		ChannelAction int64
	}
	if err := tx.Raw(
		`SELECT photo_action, channel_action FROM chat_requirements WHERE chat_id = ?`, chatID,
	).Scan(&stored).Error; err != nil {
		t.Fatalf("read the new columns' defaults: %v", err)
	}
	if stored.PhotoAction != 0 || stored.ChannelAction != 0 {
		t.Errorf("photo_action and channel_action default to %+v, want both 0 (off)", stored)
	}

	var names []string
	if err := tx.Raw(
		`SELECT conname FROM pg_constraint WHERE conrelid = 'chat_requirements'::regclass`,
	).Scan(&names).Error; err != nil {
		t.Fatalf("list the constraints of chat_requirements: %v", err)
	}
	have := map[string]bool{}
	for _, name := range names {
		have[name] = true
	}
	for _, want := range []string{"chk_chat_requirements_photo", "chk_chat_requirements_channel"} {
		if !have[want] {
			t.Errorf("constraint %s is missing from chat_requirements (found %v)", want, names)
		}
	}

	for _, check := range []struct{ column, constraint string }{
		{"photo_action", "chk_chat_requirements_photo"},
		{"channel_action", "chk_chat_requirements_channel"},
	} {
		// A refused statement aborts the transaction in PostgreSQL, so it runs behind a savepoint.
		if err := tx.Exec("SAVEPOINT bad_action").Error; err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		err := tx.Exec("UPDATE chat_requirements SET "+check.column+" = 5 WHERE chat_id = ?", chatID).Error
		if err == nil {
			t.Errorf("%s = 5 was accepted by the database", check.column)
		} else if !strings.Contains(err.Error(), "SQLSTATE 23514") || !strings.Contains(err.Error(), check.constraint) {
			t.Errorf("%s = 5 failed with %v, want SQLSTATE 23514 naming %s", check.column, err, check.constraint)
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT bad_action").Error; err != nil {
			t.Fatalf("roll back to savepoint: %v", err)
		}
	}
}
