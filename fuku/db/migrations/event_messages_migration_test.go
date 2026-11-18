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

const chatEventMessagesMigration = "20261119120000_add_chat_event_messages.sql"

// chatEventMessagesStatements reads the event messages migration and returns the statements the
// runner would execute, dropping empty and transaction-control statements as applyMigration does.
func chatEventMessagesStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", chatEventMessagesMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", chatEventMessagesMigration, err)
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

// TestChatEventMessagesMigrationStatements proves, without a database, what the Go runner hands to
// PostgreSQL for the event messages migration: one idempotent CREATE TABLE carrying its primary key
// inline, no transaction control, no CONCURRENTLY, no NOT VALID, no top-level ADD CONSTRAINT, no
// foreign key and no CHECK (event and dest are validated in Go), and a file name that sorts after
// the previous migration.
func TestChatEventMessagesMigrationStatements(t *testing.T) {
	run := chatEventMessagesStatements(t)
	if len(run) != 1 {
		t.Fatalf("%s gives %d statements, want 1:\n%s", chatEventMessagesMigration, len(run), strings.Join(run, "\n---\n"))
	}
	stmt := run[0]
	if !strings.HasPrefix(stmt, "CREATE TABLE IF NOT EXISTS chat_event_messages ") {
		t.Errorf("the statement must start with CREATE TABLE IF NOT EXISTS chat_event_messages, got:\n%s", stmt)
	}
	if !strings.Contains(stmt, "CONSTRAINT pk_chat_event_messages PRIMARY KEY (chat_id, event, dest)") {
		t.Errorf("the statement must carry the primary key inline:\n%s", stmt)
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
	const previous = "20261118120000_add_chat_timezones.sql"
	if chatEventMessagesMigration <= previous {
		t.Errorf("%s must sort after %s", chatEventMessagesMigration, previous)
	}
}

// TestChatEventMessagesMigrationDefaults proves, on PostgreSQL only, that after the migration chain
// the table exists, that the migration's statement runs a second time without error, that a row
// inserted with only its key reads every column's zero default, and that the primary key refuses
// a second row of the same (chat_id, event, dest) with SQLSTATE 23505.
func TestChatEventMessagesMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the event messages migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the event messages migration test on PostgreSQL")
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

	if !database.Migrator().HasTable("chat_event_messages") {
		t.Fatalf("table chat_event_messages is missing: the migration chain must run before this test")
	}

	run := chatEventMessagesStatements(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below vanish, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the event messages transaction: %v", err)
		}
	}()

	for i, stmt := range run {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d/%d of %s failed: %v", i+1, len(run), chatEventMessagesMigration, err)
		}
	}

	chat := captchaModeChatID(t)
	if err := tx.Exec(`INSERT INTO chat_event_messages (chat_id, event, dest) VALUES (?, 'ban', 'group')`, chat).Error; err != nil {
		t.Fatalf("insert a row with only its key: %v", err)
	}
	var row struct {
		Text      string
		FileID    string
		MediaType int
		Buttons   string
		Silent    bool
		UpdatedBy int64
		HasTime   bool
	}
	if err := tx.Raw(`SELECT text, file_id, media_type, buttons::text AS buttons, silent, updated_by,
		updated_at IS NOT NULL AS has_time
		FROM chat_event_messages WHERE chat_id = ? AND event = 'ban' AND dest = 'group'`, chat).Scan(&row).Error; err != nil {
		t.Fatalf("read the defaults: %v", err)
	}
	if row.Text != "" || row.FileID != "" || row.MediaType != 0 || row.Buttons != "[]" || row.Silent || row.UpdatedBy != 0 || !row.HasTime {
		t.Errorf("defaults = %+v, want empty text and file, media 0, buttons [], silent false, updated_by 0 and a time", row)
	}

	if err := tx.Exec("SAVEPOINT duplicate").Error; err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	err = tx.Exec(`INSERT INTO chat_event_messages (chat_id, event, dest) VALUES (?, 'ban', 'group')`, chat).Error
	switch {
	case err == nil:
		t.Errorf("a second row of the same key was accepted by the database")
	case !strings.Contains(err.Error(), "SQLSTATE 23505") || !strings.Contains(err.Error(), "pk_chat_event_messages"):
		t.Errorf("a second row of the same key failed with %v, want SQLSTATE 23505 naming pk_chat_event_messages", err)
	}
	if err := tx.Exec("ROLLBACK TO SAVEPOINT duplicate").Error; err != nil {
		t.Fatalf("roll back to savepoint: %v", err)
	}
	if err := tx.Exec(`INSERT INTO chat_event_messages (chat_id, event, dest) VALUES (?, 'kick', 'group')`, chat).Error; err != nil {
		t.Errorf("another event of the same chat is refused: %v", err)
	}
}
