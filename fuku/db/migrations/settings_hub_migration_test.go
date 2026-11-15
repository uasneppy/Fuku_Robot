package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const settingsHubMigration = "20261104120000_add_settings_hub_tables.sql"

// settingsHubStatements reads the settings hub migration and returns the statements the
// runner would execute, dropping empty and transaction-control statements exactly as
// applyMigration does.
func settingsHubStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", settingsHubMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", settingsHubMigration, err)
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

// TestSettingsHubMigrationStatements proves, without a database, what the Go runner hands
// to PostgreSQL for the settings hub migration: two idempotent table creations with every
// constraint inline, one idempotent index, no transaction control, no CONCURRENTLY and no
// ALTER TABLE (both appliers swallow duplicate_object on a bare constraint addition), and a
// file name that sorts after every other migration.
func TestSettingsHubMigrationStatements(t *testing.T) {
	run := settingsHubStatements(t)
	if len(run) != 3 {
		t.Fatalf("%s gives %d statements, want 3 (two tables, one index):\n%s",
			settingsHubMigration, len(run), strings.Join(run, "\n---\n"))
	}

	access, index, idx := run[0], run[1], run[2]
	if !strings.Contains(access, "CREATE TABLE IF NOT EXISTS chat_settings_access") {
		t.Errorf("statement 1 must create chat_settings_access idempotently, got:\n%s", access)
	}
	if !strings.Contains(index, "CREATE TABLE IF NOT EXISTS chat_admin_index") {
		t.Errorf("statement 2 must create chat_admin_index idempotently, got:\n%s", index)
	}
	if !strings.Contains(idx, "CREATE INDEX IF NOT EXISTS idx_chat_admin_index_user_id") {
		t.Errorf("statement 3 must create idx_chat_admin_index_user_id idempotently, got:\n%s", idx)
	}

	// Each constraint is declared inside its own CREATE TABLE.
	inline := map[string]string{
		"uk_chat_settings_access_chat":  access,
		"uk_chat_admin_index_chat_user": index,
		"chk_chat_admin_index_role":     index,
	}
	for name, stmt := range inline {
		if !strings.Contains(stmt, "CONSTRAINT "+name) {
			t.Errorf("constraint %s must be inline in its CREATE TABLE statement", name)
		}
	}

	for i, stmt := range run {
		upper := strings.ToUpper(stmt)
		if isTransactionControlStatement(stmt) {
			t.Errorf("statement %d is transaction control: %s", i+1, stmt)
		}
		if strings.Contains(upper, "CONCURRENTLY") {
			t.Errorf("statement %d uses CONCURRENTLY, which cannot run in a transaction", i+1)
		}
		if strings.Contains(upper, "ALTER TABLE") {
			t.Errorf("statement %d uses ALTER TABLE, constraints must be inline", i+1)
		}
	}

	// The newest file at the time of writing; a later migration may sort after this one, so
	// the test pins only that nothing older than it was shadowed.
	const previousNewest = "20261101120000_default_language_ukrainian.sql"
	if settingsHubMigration <= previousNewest {
		t.Errorf("%s must sort after %s", settingsHubMigration, previousNewest)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "..", "migrations", previousNewest)); err != nil {
		t.Errorf("the migration this one must follow is missing: %v", err)
	}
}
