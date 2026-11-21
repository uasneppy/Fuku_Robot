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

const eventMessageOptionsMigration = "20261120120000_add_event_message_options.sql"

// eventMessageOptionsStatements reads the event message options migration and returns the
// statements the runner would execute, dropping empty and transaction-control statements as
// applyMigration does.
func eventMessageOptionsStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", eventMessageOptionsMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", eventMessageOptionsMigration, err)
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

// TestEventMessageOptionsMigrationStatements proves, without a database, what the Go runner hands
// to PostgreSQL for the event message options migration: one idempotent CREATE TABLE with the five
// columns, each NOT NULL, carrying its primary key inline, no transaction control, no CONCURRENTLY,
// no NOT VALID, no top-level ADD CONSTRAINT, no foreign key and no CHECK, and a file name that
// sorts after the previous migration.
func TestEventMessageOptionsMigrationStatements(t *testing.T) {
	run := eventMessageOptionsStatements(t)
	if len(run) != 1 {
		t.Fatalf("%s gives %d statements, want 1:\n%s", eventMessageOptionsMigration, len(run), strings.Join(run, "\n---\n"))
	}
	stmt := run[0]
	if !strings.HasPrefix(stmt, "CREATE TABLE IF NOT EXISTS chat_event_message_options ") {
		t.Errorf("the statement must start with CREATE TABLE IF NOT EXISTS chat_event_message_options, got:\n%s", stmt)
	}
	if !strings.Contains(stmt, "CONSTRAINT pk_chat_event_message_options PRIMARY KEY (chat_id)") {
		t.Errorf("the statement must carry the primary key inline:\n%s", stmt)
	}
	for _, column := range []string{
		"chat_id BIGINT NOT NULL",
		"pm_details BOOLEAN NOT NULL DEFAULT FALSE",
		"pm_log BOOLEAN NOT NULL DEFAULT FALSE",
		"updated_by BIGINT NOT NULL DEFAULT 0",
		"updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()",
	} {
		if !strings.Contains(stmt, column) {
			t.Errorf("the statement must define %q:\n%s", column, stmt)
		}
	}
	upper := strings.ToUpper(stmt)
	if isTransactionControlStatement(stmt) {
		t.Errorf("the statement is transaction control: %s", stmt)
	}
	for _, banned := range []string{"CONCURRENTLY", "NOT VALID", "BEGIN", "COMMIT", "ROLLBACK", "ADD CONSTRAINT", "REFERENCES", "CHECK"} {
		if strings.Contains(upper, banned) {
			t.Errorf("the statement contains %s:\n%s", banned, stmt)
		}
	}
	if eventMessageOptionsMigration <= chatEventMessagesMigration {
		t.Errorf("%s must sort after %s", eventMessageOptionsMigration, chatEventMessagesMigration)
	}
}

// TestEventMessageOptionsMigrationDefaults proves, on PostgreSQL only, that after the migration
// chain the table exists, that the migration's statement runs a second time without error, that a
// row inserted with only its chat reads both options off, updated_by 0 and a set time, and that the
// primary key refuses a second row of the same chat with SQLSTATE 23505.
func TestEventMessageOptionsMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the event message options migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the event message options migration test on PostgreSQL")
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

	if !database.Migrator().HasTable("chat_event_message_options") {
		t.Fatalf("table chat_event_message_options is missing: the migration chain must run before this test")
	}

	run := eventMessageOptionsStatements(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below vanish, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the event message options transaction: %v", err)
		}
	}()

	for i, stmt := range run {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d/%d of %s failed: %v", i+1, len(run), eventMessageOptionsMigration, err)
		}
	}

	chat := captchaModeChatID(t)
	if err := tx.Exec(`INSERT INTO chat_event_message_options (chat_id) VALUES (?)`, chat).Error; err != nil {
		t.Fatalf("insert a row with only its key: %v", err)
	}
	var row struct {
		PMDetails bool `gorm:"column:pm_details"`
		PMLog     bool `gorm:"column:pm_log"`
		UpdatedBy int64
		HasTime   bool
	}
	if err := tx.Raw(`SELECT pm_details, pm_log, updated_by, updated_at IS NOT NULL AS has_time
		FROM chat_event_message_options WHERE chat_id = ?`, chat).Scan(&row).Error; err != nil {
		t.Fatalf("read the defaults: %v", err)
	}
	if row.PMDetails || row.PMLog || row.UpdatedBy != 0 || !row.HasTime {
		t.Errorf("defaults = %+v, want both options false, updated_by 0 and a time", row)
	}

	if err := tx.Exec("SAVEPOINT duplicate").Error; err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	err = tx.Exec(`INSERT INTO chat_event_message_options (chat_id) VALUES (?)`, chat).Error
	switch {
	case err == nil:
		t.Errorf("a second row of the same chat was accepted by the database")
	case !strings.Contains(err.Error(), "SQLSTATE 23505") || !strings.Contains(err.Error(), "pk_chat_event_message_options"):
		t.Errorf("a second row of the same chat failed with %v, want SQLSTATE 23505 naming pk_chat_event_message_options", err)
	}
	if err := tx.Exec("ROLLBACK TO SAVEPOINT duplicate").Error; err != nil {
		t.Fatalf("roll back to savepoint: %v", err)
	}
}
