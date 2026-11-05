package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rulesMediaMigration = "20261105120000_add_rules_media_and_greet_once.sql"

// rulesMediaStatements reads the rules media migration and returns the statements the
// runner would execute, dropping empty and transaction-control statements exactly as
// applyMigration does.
func rulesMediaStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", rulesMediaMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", rulesMediaMigration, err)
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

// TestRulesMediaMigrationStatements proves, without a database, what the Go runner hands
// to PostgreSQL for the rules media migration: seven idempotent column additions with a
// NOT NULL zero-value default (three on greetings, four on rules) and one idempotent
// greeted_members creation with its two-column primary key, no transaction control, no
// CONCURRENTLY and no constraint addition (both appliers swallow duplicate_object on a bare
// ADD CONSTRAINT), and a file name that sorts after the previous newest migration.
func TestRulesMediaMigrationStatements(t *testing.T) {
	run := rulesMediaStatements(t)
	if len(run) != 8 {
		t.Fatalf("%s gives %d statements, want 8 (seven columns, one table):\n%s",
			rulesMediaMigration, len(run), strings.Join(run, "\n---\n"))
	}

	columns := []struct{ table, column string }{
		{"greetings", "welcome_media_below"},
		{"greetings", "goodbye_media_below"},
		{"greetings", "welcome_once"},
		{"rules", "file_id"},
		{"rules", "media_type"},
		{"rules", "btns"},
		{"rules", "media_below"},
	}
	for i, want := range columns {
		stmt := run[i]
		if !strings.Contains(stmt, "ALTER TABLE "+want.table+" ADD COLUMN IF NOT EXISTS "+want.column+" ") {
			t.Errorf("statement %d must add %s.%s idempotently, got:\n%s", i+1, want.table, want.column, stmt)
		}
		if !strings.Contains(stmt, "NOT NULL DEFAULT") {
			t.Errorf("statement %d must be NOT NULL with a default, got:\n%s", i+1, stmt)
		}
	}

	table := run[7]
	if !strings.Contains(table, "CREATE TABLE IF NOT EXISTS greeted_members") {
		t.Errorf("statement 8 must create greeted_members idempotently, got:\n%s", table)
	}
	if !strings.Contains(table, "PRIMARY KEY (chat_id, user_id)") {
		t.Errorf("statement 8 must declare PRIMARY KEY (chat_id, user_id) inline, got:\n%s", table)
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

	// The newest file before this one; a later migration may sort after it, so the test pins
	// only that nothing older than it was shadowed.
	if rulesMediaMigration <= settingsHubMigration {
		t.Errorf("%s must sort after %s", rulesMediaMigration, settingsHubMigration)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "..", "migrations", settingsHubMigration)); err != nil {
		t.Errorf("the migration this one must follow is missing: %v", err)
	}
}
