package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

// TestRulesMediaMigrationDefaults proves, on PostgreSQL only, that after the migration chain
// a greetings row and a rules row inserted with only their required columns read every new
// column as its zero value (the meaning of a row saved before this phase), that the
// migration's statements can run again without error, and that greeted_members holds one
// row per (chat, user): a conflicting insert does nothing and a plain second insert fails
// with unique_violation (D-11, D-14).
//
// It needs FUKU_TEST_DATABASE=true and DATABASE_URL, and a database the migration chain
// already ran against (the CI integrity step). It runs inside one transaction that is always
// rolled back, with fresh chat IDs, so it leaves nothing behind. It is never run locally.
func TestRulesMediaMigrationDefaults(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("FUKU_TEST_DATABASE")), "true") {
		t.Skip("set FUKU_TEST_DATABASE=true and DATABASE_URL to run the rules media migration test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the rules media migration test on PostgreSQL")
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

	for _, table := range []string{"greetings", "rules", "chats", "greeted_members"} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("table %s is missing: the migration chain must run before this test", table)
		}
	}

	run := rulesMediaStatements(t)
	if len(run) == 0 {
		t.Fatalf("%s holds no statements", rulesMediaMigration)
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below vanish, and so does the second run of the migration.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the rules media transaction: %v", err)
		}
	}()

	for i, stmt := range run {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatalf("second run: statement %d/%d of %s failed: %v", i+1, len(run), rulesMediaMigration, err)
		}
	}

	chatID := captchaModeChatID(t)
	now := time.Now().UTC()
	if err := tx.Exec(
		`INSERT INTO chats (chat_id, chat_name, language, is_inactive, created_at, updated_at)
		 VALUES (?, ?, ?, FALSE, ?, ?)`,
		chatID, "rules media test", "en", now, now,
	).Error; err != nil {
		t.Fatalf("insert chat %d: %v", chatID, err)
	}
	if err := tx.Exec(`INSERT INTO greetings (chat_id) VALUES (?)`, chatID).Error; err != nil {
		t.Fatalf("insert a greetings row with only its required columns: %v", err)
	}
	if err := tx.Exec(`INSERT INTO rules (chat_id) VALUES (?)`, chatID).Error; err != nil {
		t.Fatalf("insert a rules row with only its required columns: %v", err)
	}

	var greeting struct {
		WelcomeMediaBelow bool
		GoodbyeMediaBelow bool
		WelcomeOnce       bool
	}
	if err := tx.Raw(
		`SELECT welcome_media_below, goodbye_media_below, welcome_once FROM greetings WHERE chat_id = ?`, chatID,
	).Scan(&greeting).Error; err != nil {
		t.Fatalf("read the greetings defaults: %v", err)
	}
	if greeting.WelcomeMediaBelow || greeting.GoodbyeMediaBelow || greeting.WelcomeOnce {
		t.Errorf("greetings switches default to %+v, want all false", greeting)
	}

	var rules struct {
		FileID     string
		MediaType  int64
		Btns       string
		MediaBelow bool
	}
	if err := tx.Raw(
		`SELECT file_id, media_type, btns::text AS btns, media_below FROM rules WHERE chat_id = ?`, chatID,
	).Scan(&rules).Error; err != nil {
		t.Fatalf("read the rules defaults: %v", err)
	}
	if rules.FileID != "" || rules.MediaType != 0 || rules.Btns != "[]" || rules.MediaBelow {
		t.Errorf("rules columns default to %+v, want file_id '', media_type 0, btns '[]', media_below false", rules)
	}

	const claim = `INSERT INTO greeted_members (chat_id, user_id) VALUES (?, ?)`
	for i := 0; i < 2; i++ {
		if err := tx.Exec(claim+` ON CONFLICT DO NOTHING`, chatID, 4242).Error; err != nil {
			t.Fatalf("conflicting insert %d into greeted_members: %v", i+1, err)
		}
	}
	var rows int64
	if err := tx.Raw(`SELECT COUNT(*) FROM greeted_members WHERE chat_id = ? AND user_id = ?`, chatID, 4242).Scan(&rows).Error; err != nil {
		t.Fatalf("count greeted_members: %v", err)
	}
	if rows != 1 {
		t.Errorf("greeted_members holds %d rows for one (chat, user) after two conflicting inserts, want 1", rows)
	}

	// A refused insert aborts the transaction in PostgreSQL, so it runs behind a savepoint.
	if err := tx.Exec("SAVEPOINT duplicate_greeted").Error; err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	err = tx.Exec(claim, chatID, 4242).Error
	if err == nil {
		t.Fatalf("a plain second insert of one (chat, user) was accepted, want a unique violation")
	}
	if !strings.Contains(err.Error(), "SQLSTATE 23505") {
		t.Errorf("a plain second insert failed with %v, want SQLSTATE 23505 (unique_violation)", err)
	}
	if err := tx.Exec("ROLLBACK TO SAVEPOINT duplicate_greeted").Error; err != nil {
		t.Fatalf("roll back to savepoint: %v", err)
	}
}
