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

const memberRequirementAIMigration = "20261109120000_add_member_requirement_ai_rule.sql"

// memberRequirementAIStatements reads the AI rule migration and returns the statements the
// runner would execute, dropping empty and transaction-control statements exactly as
// applyMigration does.
func memberRequirementAIStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", memberRequirementAIMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", memberRequirementAIMigration, err)
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

// TestMemberRequirementAIMigrationStatements proves, without a database, what the Go runner
// hands to PostgreSQL for the AI rule migration: one idempotent column addition to
// chat_requirements, SMALLINT NOT NULL DEFAULT 0 with its named CHECK 0..4 inline, no
// transaction control, no CONCURRENTLY, no NOT VALID and no top-level constraint addition
// (both appliers swallow duplicate_object on a bare ADD CONSTRAINT), and a file name that
// sorts after the lookup rules migration.
func TestMemberRequirementAIMigrationStatements(t *testing.T) {
	run := memberRequirementAIStatements(t)
	for i, stmt := range run {
		run[i] = stripSQLComments(stmt)
	}
	if len(run) != 1 {
		t.Fatalf("%s gives %d statements, want 1 (one column):\n%s",
			memberRequirementAIMigration, len(run), strings.Join(run, "\n---\n"))
	}

	stmt := run[0]
	if !strings.Contains(stmt, "ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS ai_action ") {
		t.Errorf("the statement must add ai_action to chat_requirements idempotently, got:\n%s", stmt)
	}
	if !strings.Contains(stmt, "ai_action SMALLINT NOT NULL DEFAULT 0 ") {
		t.Errorf("ai_action must be SMALLINT NOT NULL DEFAULT 0, got:\n%s", stmt)
	}
	if !strings.Contains(stmt, "CONSTRAINT chk_chat_requirements_ai CHECK (ai_action BETWEEN 0 AND 4)") {
		t.Errorf("the statement must carry constraint chk_chat_requirements_ai inline, got:\n%s", stmt)
	}

	upper := strings.ToUpper(stmt)
	if isTransactionControlStatement(stmt) {
		t.Errorf("the statement is transaction control: %s", stmt)
	}
	for _, banned := range []string{"CONCURRENTLY", "NOT VALID", "BEGIN", "COMMIT", "ROLLBACK"} {
		if strings.Contains(upper, banned) {
			t.Errorf("the statement contains %s:\n%s", banned, stmt)
		}
	}
	// A top-level ADD CONSTRAINT is the one form both appliers rewrite; the inline
	// CONSTRAINT of ADD COLUMN is not.
	if strings.Contains(upper, "ADD CONSTRAINT") {
		t.Errorf("the statement contains ADD CONSTRAINT:\n%s", stmt)
	}

	if memberRequirementAIMigration <= memberRequirementLookupMigration {
		t.Errorf("%s must sort after %s", memberRequirementAIMigration, memberRequirementLookupMigration)
	}
}

// TestMemberRequirementAIMigrationDefaults proves, on PostgreSQL only, that after the
// migration chain chat_requirements has ai_action, that the migration's statement runs a
// second time without error, that a row inserted with only its chat_id reads the column 0
// (off), that the named constraint exists, and that a value of 5 is refused with SQLSTATE
// 23514 naming the constraint.
func TestMemberRequirementAIMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the AI rule migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the AI rule migration test on PostgreSQL")
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

	run := memberRequirementAIStatements(t)
	if len(run) == 0 {
		t.Fatalf("%s holds no statements", memberRequirementAIMigration)
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the row below vanishes, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the AI rule transaction: %v", err)
		}
	}()

	for i, stmt := range run {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d/%d of %s failed: %v", i+1, len(run), memberRequirementAIMigration, err)
		}
	}

	chatID := captchaModeChatID(t)
	if err := tx.Exec(`INSERT INTO chat_requirements (chat_id) VALUES (?)`, chatID).Error; err != nil {
		t.Fatalf("insert a chat_requirements row with only its chat_id: %v", err)
	}
	var stored struct {
		AIAction int64
	}
	if err := tx.Raw(
		`SELECT ai_action FROM chat_requirements WHERE chat_id = ?`, chatID,
	).Scan(&stored).Error; err != nil {
		t.Fatalf("read the new column's default: %v", err)
	}
	if stored.AIAction != 0 {
		t.Errorf("ai_action defaults to %d, want 0 (off)", stored.AIAction)
	}

	var names []string
	if err := tx.Raw(
		`SELECT conname FROM pg_constraint WHERE conrelid = 'chat_requirements'::regclass`,
	).Scan(&names).Error; err != nil {
		t.Fatalf("list the constraints of chat_requirements: %v", err)
	}
	found := false
	for _, name := range names {
		if name == "chk_chat_requirements_ai" {
			found = true
		}
	}
	if !found {
		t.Errorf("constraint chk_chat_requirements_ai is missing from chat_requirements (found %v)", names)
	}

	// A refused statement aborts the transaction in PostgreSQL, so it runs behind a savepoint.
	if err := tx.Exec("SAVEPOINT bad_action").Error; err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	err = tx.Exec("UPDATE chat_requirements SET ai_action = 5 WHERE chat_id = ?", chatID).Error
	if err == nil {
		t.Error("ai_action = 5 was accepted by the database")
	} else if !strings.Contains(err.Error(), "SQLSTATE 23514") || !strings.Contains(err.Error(), "chk_chat_requirements_ai") {
		t.Errorf("ai_action = 5 failed with %v, want SQLSTATE 23514 naming chk_chat_requirements_ai", err)
	}
	if err := tx.Exec("ROLLBACK TO SAVEPOINT bad_action").Error; err != nil {
		t.Fatalf("roll back to savepoint: %v", err)
	}
}
