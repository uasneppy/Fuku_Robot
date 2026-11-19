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

const chatConversationMigration = "20261119120000_add_chat_conversations.sql"

// chatConversationStatements reads the chat conversation migration and returns the statements the
// runner would execute, dropping empty and transaction-control statements as applyMigration does.
func chatConversationStatements(t *testing.T) []string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", chatConversationMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(path) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", chatConversationMigration, err)
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

// TestChatConversationMigrationStatements proves, without a database, what the Go runner hands to
// PostgreSQL for the conversation migration: one idempotent CREATE TABLE with its two named checks
// inline, no transaction control, no CONCURRENTLY, no NOT VALID, no top-level ADD CONSTRAINT and no
// foreign key, and a file name that sorts after the previous migration.
func TestChatConversationMigrationStatements(t *testing.T) {
	run := chatConversationStatements(t)
	if len(run) != 1 {
		t.Fatalf("%s gives %d statements, want 1:\n%s", chatConversationMigration, len(run), strings.Join(run, "\n---\n"))
	}
	if !strings.HasPrefix(run[0], "CREATE TABLE IF NOT EXISTS chat_conversations ") {
		t.Errorf("unexpected statement:\n%s", run[0])
	}
	for _, name := range []string{"chk_chat_conversations_provider", "chk_chat_conversations_audience"} {
		if !strings.Contains(run[0], "CONSTRAINT "+name+" ") {
			t.Errorf("the statement must carry constraint %s inline", name)
		}
	}
	upper := strings.ToUpper(run[0])
	for _, banned := range []string{"CONCURRENTLY", "NOT VALID", "BEGIN", "COMMIT", "ROLLBACK", "ADD CONSTRAINT", "REFERENCES"} {
		if strings.Contains(upper, banned) {
			t.Errorf("the statement contains %s", banned)
		}
	}
	const previous = "20261118120000_add_chat_timezones.sql"
	if chatConversationMigration <= previous {
		t.Errorf("%s must sort after %s", chatConversationMigration, previous)
	}
}

// TestChatConversationMigrationDefaults proves, on PostgreSQL only, that the table exists after the
// chain, that the statement runs a second time without error, that a row with only chat_id reads off,
// anthropic, everyone, and that the named checks refuse a provider and an audience outside the stored
// values (23514).
func TestChatConversationMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the chat conversation migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the chat conversation migration test on PostgreSQL")
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if !database.Migrator().HasTable("chat_conversations") {
		t.Fatal("table chat_conversations is missing: the migration chain must run before this test")
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()

	for i, stmt := range chatConversationStatements(t) {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d failed: %v", i+1, err)
		}
	}
	chat := captchaModeChatID(t)
	if err := tx.Exec(`INSERT INTO chat_conversations (chat_id) VALUES (?)`, chat).Error; err != nil {
		t.Fatalf("insert with only chat_id: %v", err)
	}
	var row struct {
		Enabled  bool
		Provider string
		Audience int
	}
	if err := tx.Raw(`SELECT enabled, provider, audience FROM chat_conversations WHERE chat_id = ?`, chat).Scan(&row).Error; err != nil {
		t.Fatalf("read the defaults: %v", err)
	}
	if row.Enabled || row.Provider != "anthropic" || row.Audience != 0 {
		t.Errorf("defaults = %+v, want off, anthropic, 0", row)
	}
	for _, tc := range []struct{ name, stmt, constraint string }{
		{"provider openai", `UPDATE chat_conversations SET provider = 'openai' WHERE chat_id = ?`, "chk_chat_conversations_provider"},
		{"audience 2", `UPDATE chat_conversations SET audience = 2 WHERE chat_id = ?`, "chk_chat_conversations_audience"},
	} {
		if err := tx.Exec("SAVEPOINT refused").Error; err != nil {
			t.Fatal(err)
		}
		err := tx.Exec(tc.stmt, chat).Error
		if err == nil || !strings.Contains(err.Error(), "SQLSTATE 23514") || !strings.Contains(err.Error(), tc.constraint) {
			t.Errorf("%s: err %v, want 23514 naming %s", tc.name, err, tc.constraint)
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT refused").Error; err != nil {
			t.Fatal(err)
		}
	}
}
