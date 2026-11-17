package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const usersUsernameLowerIndexMigration = "20261117120000_users_username_lower_index.sql"

// TestUsersUsernameLowerIndexMigrationStatements proves, without a database, what the Go
// runner hands to PostgreSQL for the users username index migration: one idempotent
// expression index on LOWER(username), the exact form user.FindUsersByUsername queries, so
// the lookup no longer scans the table; no transaction control, no CONCURRENTLY (the runner
// applies each file in one transaction), and a file name that sorts after the migration that
// was newest when it was written.
func TestUsersUsernameLowerIndexMigrationStatements(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", usersUsernameLowerIndexMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", usersUsernameLowerIndexMigration, err)
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
		t.Fatalf("%s gives %d statements, want 1:\n%s", usersUsernameLowerIndexMigration, len(run), strings.Join(run, "\n---\n"))
	}
	if want := "CREATE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username))"; strings.TrimSuffix(run[0], ";") != want {
		t.Errorf("statement = %q, want %q", run[0], want)
	}
	upper := strings.ToUpper(string(content))
	for _, banned := range []string{"BEGIN", "COMMIT", "ROLLBACK", "CONCURRENTLY"} {
		if strings.Contains(upper, banned) {
			t.Errorf("the migration contains %s", banned)
		}
	}
	if previous := warnModeCheckMigration; usersUsernameLowerIndexMigration <= previous {
		t.Errorf("%s must sort after %s", usersUsernameLowerIndexMigration, previous)
	}
}
