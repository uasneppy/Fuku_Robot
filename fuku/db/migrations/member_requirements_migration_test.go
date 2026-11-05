package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const memberRequirementsMigration = "20261106120000_add_member_requirements.sql"

// memberRequirementsStatements reads the member requirements migration and returns the
// statements the runner would execute, dropping empty and transaction-control statements
// exactly as applyMigration does.
func memberRequirementsStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", memberRequirementsMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", memberRequirementsMigration, err)
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

// TestMemberRequirementsMigrationStatements proves, without a database, what the Go runner
// hands to PostgreSQL for the member requirements migration: two idempotent table
// creations with every constraint inline, both switches of chat_requirements NOT NULL
// DEFAULT FALSE, no transaction control, no CONCURRENTLY and no constraint addition (both
// appliers swallow duplicate_object on a bare ADD CONSTRAINT), and a file name that sorts
// after the previous migration.
func TestMemberRequirementsMigrationStatements(t *testing.T) {
	run := memberRequirementsStatements(t)
	if len(run) != 2 {
		t.Fatalf("%s gives %d statements, want 2 (two tables):\n%s",
			memberRequirementsMigration, len(run), strings.Join(run, "\n---\n"))
	}
	settings, holds := run[0], run[1]
	if !strings.Contains(settings, "CREATE TABLE IF NOT EXISTS chat_requirements ") {
		t.Errorf("statement 1 must create chat_requirements idempotently, got:\n%s", settings)
	}
	if !strings.Contains(holds, "CREATE TABLE IF NOT EXISTS chat_requirement_holds ") {
		t.Errorf("statement 2 must create chat_requirement_holds idempotently, got:\n%s", holds)
	}

	for _, name := range []string{
		"uk_chat_requirements_chat",
		"chk_chat_requirements_last_name",
		"chk_chat_requirements_username",
		"chk_chat_requirements_russian",
		"chk_chat_requirements_arabic",
		"chk_chat_requirements_chinese",
	} {
		if !strings.Contains(settings, "CONSTRAINT "+name+" ") {
			t.Errorf("chat_requirements must declare constraint %s inline", name)
		}
	}
	for _, name := range []string{
		"uk_chat_requirement_holds_chat_user",
		"chk_chat_requirement_holds_rules",
		"chk_chat_requirement_holds_action",
	} {
		if !strings.Contains(holds, "CONSTRAINT "+name+" ") {
			t.Errorf("chat_requirement_holds must declare constraint %s inline", name)
		}
	}
	for _, column := range []string{"check_on_join", "delete_messages"} {
		if !strings.Contains(settings, column+" BOOLEAN NOT NULL DEFAULT FALSE") {
			t.Errorf("chat_requirements.%s must be BOOLEAN NOT NULL DEFAULT FALSE", column)
		}
	}

	for i, stmt := range run {
		upper := strings.ToUpper(stmt)
		if isTransactionControlStatement(stmt) {
			t.Errorf("statement %d is transaction control: %s", i+1, stmt)
		}
		for _, banned := range []string{"CONCURRENTLY", "ADD CONSTRAINT", "BEGIN", "COMMIT", "ROLLBACK"} {
			if strings.Contains(upper, banned) {
				t.Errorf("statement %d contains %s:\n%s", i+1, banned, stmt)
			}
		}
	}

	if memberRequirementsMigration <= rulesMediaMigration {
		t.Errorf("%s must sort after %s", memberRequirementsMigration, rulesMediaMigration)
	}
}
