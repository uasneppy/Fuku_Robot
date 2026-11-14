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

const scheduledDeletionMigration = "20261110120000_add_scheduled_deletion.sql"

// scheduledDeletionStatements reads the scheduled deletion migration and returns the
// statements the runner would execute, dropping empty and transaction-control statements
// exactly as applyMigration does.
func scheduledDeletionStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", scheduledDeletionMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", scheduledDeletionMigration, err)
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

// TestScheduledDeletionMigrationStatements proves, without a database, what the Go runner
// hands to PostgreSQL for the scheduled deletion migration: two idempotent CREATE TABLE
// statements and two idempotent CREATE INDEX statements whose partial predicates survive
// the splitter, the four named constraints inline, no transaction control, no CONCURRENTLY,
// no NOT VALID and no top-level ADD CONSTRAINT (both appliers swallow duplicate_object on a
// bare one), and a file name that sorts after the AI rule migration.
func TestScheduledDeletionMigrationStatements(t *testing.T) {
	run := scheduledDeletionStatements(t)
	if len(run) != 4 {
		t.Fatalf("%s gives %d statements, want 4:\n%s", scheduledDeletionMigration, len(run), strings.Join(run, "\n---\n"))
	}

	wantPrefix := []string{
		"CREATE TABLE IF NOT EXISTS chat_delete_rules ",
		"CREATE TABLE IF NOT EXISTS scheduled_deletions ",
		"CREATE INDEX IF NOT EXISTS idx_scheduled_deletions_due ",
		"CREATE INDEX IF NOT EXISTS idx_scheduled_deletions_acting ",
	}
	for i, prefix := range wantPrefix {
		if !strings.HasPrefix(run[i], prefix) {
			t.Errorf("statement %d must start with %q, got:\n%s", i+1, prefix, run[i])
		}
	}

	all := strings.Join(run, "\n")
	for _, name := range []string{
		"chk_chat_delete_rules_delay",
		"uk_scheduled_deletions_message",
		"chk_scheduled_deletions_state",
		"chk_scheduled_deletions_attempts",
	} {
		if !strings.Contains(all, "CONSTRAINT "+name+" ") {
			t.Errorf("the statements must carry constraint %s inline", name)
		}
	}
	if !strings.Contains(run[2], "WHERE state = 'pending'") {
		t.Errorf("idx_scheduled_deletions_due must keep its pending predicate, got:\n%s", run[2])
	}
	if !strings.Contains(run[3], "WHERE state = 'acting'") {
		t.Errorf("idx_scheduled_deletions_acting must keep its acting predicate, got:\n%s", run[3])
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
		// A top-level ADD CONSTRAINT is the one form both appliers rewrite.
		if strings.Contains(upper, "ADD CONSTRAINT") {
			t.Errorf("statement %d contains ADD CONSTRAINT:\n%s", i+1, stmt)
		}
	}

	if scheduledDeletionMigration <= memberRequirementAIMigration {
		t.Errorf("%s must sort after %s", scheduledDeletionMigration, memberRequirementAIMigration)
	}
}

// TestScheduledDeletionMigrationDefaults proves, on PostgreSQL only, that after the
// migration chain both tables exist, that the migration's statements run a second time
// without error, that a queue row inserted with only its required columns reads state
// pending, an empty token, 0 attempts and no claim time, that the named constraints refuse
// a delay of 86401 (23514), a second row for one (chat, message) (23505) and a state of x
// (23514), and that the due index carries its pending predicate.
func TestScheduledDeletionMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the scheduled deletion migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the scheduled deletion migration test on PostgreSQL")
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

	for _, table := range []string{"chat_delete_rules", "scheduled_deletions"} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("table %s is missing: the migration chain must run before this test", table)
		}
	}

	run := scheduledDeletionStatements(t)
	if len(run) == 0 {
		t.Fatalf("%s holds no statements", scheduledDeletionMigration)
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below vanish, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the scheduled deletion transaction: %v", err)
		}
	}()

	for i, stmt := range run {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d/%d of %s failed: %v", i+1, len(run), scheduledDeletionMigration, err)
		}
	}

	chatID := captchaModeChatID(t)
	if err := tx.Exec(
		`INSERT INTO scheduled_deletions (chat_id, message_id, kind, sent_at, delete_at)
		 VALUES (?, 1, 'bot:welcome', NOW(), NOW())`, chatID,
	).Error; err != nil {
		t.Fatalf("insert a scheduled_deletions row with only its required columns: %v", err)
	}
	var stored struct {
		State      string
		ClaimToken string
		Attempts   int64
		Claimed    bool
	}
	if err := tx.Raw(
		`SELECT state, claim_token, attempts, claimed_at IS NOT NULL AS claimed
		 FROM scheduled_deletions WHERE chat_id = ?`, chatID,
	).Scan(&stored).Error; err != nil {
		t.Fatalf("read the defaults: %v", err)
	}
	if stored.State != "pending" || stored.ClaimToken != "" || stored.Attempts != 0 || stored.Claimed {
		t.Errorf("defaults = %+v, want pending, an empty token, 0 attempts and no claim time", stored)
	}

	// A refused statement aborts the transaction in PostgreSQL, so each runs behind a savepoint.
	refused := []struct {
		name, statement, sqlstate, constraint string
		args                                  []any
	}{
		{
			"delay 86401", `INSERT INTO chat_delete_rules (chat_id, kind, delay_secs) VALUES (?, 'service:chat_photo', 86401)`,
			"SQLSTATE 23514", "chk_chat_delete_rules_delay", []any{chatID},
		},
		{
			"duplicate message", `INSERT INTO scheduled_deletions (chat_id, message_id, kind, sent_at, delete_at)
			 VALUES (?, 1, 'bot:rules', NOW(), NOW())`,
			"SQLSTATE 23505", "uk_scheduled_deletions_message", []any{chatID},
		},
		{
			"state x", `INSERT INTO scheduled_deletions (chat_id, message_id, kind, sent_at, delete_at, state)
			 VALUES (?, 2, 'bot:rules', NOW(), NOW(), 'x')`,
			"SQLSTATE 23514", "chk_scheduled_deletions_state", []any{chatID},
		},
		{
			"negative attempts", `INSERT INTO scheduled_deletions (chat_id, message_id, kind, sent_at, delete_at, attempts)
			 VALUES (?, 3, 'bot:rules', NOW(), NOW(), -1)`,
			"SQLSTATE 23514", "chk_scheduled_deletions_attempts", []any{chatID},
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
	if err := tx.Exec(`INSERT INTO chat_delete_rules (chat_id, kind, delay_secs) VALUES (?, 'service:chat_photo', 86400)`, chatID).Error; err != nil {
		t.Errorf("the largest delay 86400 is refused: %v", err)
	}

	var indexdef string
	if err := tx.Raw(
		`SELECT indexdef FROM pg_indexes WHERE tablename = 'scheduled_deletions' AND indexname = 'idx_scheduled_deletions_due'`,
	).Scan(&indexdef).Error; err != nil {
		t.Fatalf("read the due index: %v", err)
	}
	if !strings.Contains(indexdef, "state") || !strings.Contains(indexdef, "pending") || !strings.Contains(strings.ToUpper(indexdef), "WHERE") {
		t.Errorf("idx_scheduled_deletions_due = %q, want a partial index with its pending predicate", indexdef)
	}
}
