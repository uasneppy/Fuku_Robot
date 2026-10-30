package migrations

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const raidDetectionMigration = "20261018130000_add_chat_raid_detection.sql"

// raidCarryOverChatID returns a random large negative chat ID, so the rows the test
// inserts cannot collide with anything already in the CI database.
func raidCarryOverChatID(t *testing.T) int64 {
	t.Helper()
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return -2000000000000 - int64(binary.BigEndian.Uint64(buf[:])&0x3fffffffffffffff)
}

// raidDetectionRow is the part of a chat_raid_detection row the test compares.
type raidDetectionRow struct {
	JoinEnabled    bool
	JoinThreshold  int
	FloodEnabled   bool
	FloodThreshold int
}

// TestRaidDetectionCarryOver proves, on PostgreSQL only, that the settings migration
// carries each chat's old antiraid auto threshold over as its join surge threshold:
// clamped into 3..500 in both directions (D-18, D-22), a zero threshold getting no row so
// the defaults apply (D-03), an existing row kept, and the whole copy idempotent.
//
// It needs ALITA_TEST_DATABASE=true and DATABASE_URL, and a database the migration chain
// already ran against (the CI integrity step). It executes the migration file's statements
// exactly as the runner does, inside one transaction that is always rolled back, with
// fresh chat IDs, so it leaves nothing behind.
func TestRaidDetectionCarryOver(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ALITA_TEST_DATABASE")), "true") {
		t.Skip("set ALITA_TEST_DATABASE=true and DATABASE_URL to run the raid detection carry-over test on PostgreSQL")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required to run the raid detection carry-over test on PostgreSQL")
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

	for _, table := range []string{"chat_raid_detection", "antiraid_settings", "chats"} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("table %s is missing: the migration chain must run before this test", table)
		}
	}

	migrationPath, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations", raidDetectionMigration))
	if err != nil {
		t.Fatalf("resolve the migration path: %v", err)
	}
	content, err := os.ReadFile(migrationPath) // #nosec G304 - fixed repository path
	if err != nil {
		t.Fatalf("read %s: %v", raidDetectionMigration, err)
	}
	runner := &MigrationRunner{}
	statements := runner.splitSQLStatements(cleanSupabaseSQL(string(content)))
	var run []string
	for _, stmt := range statements {
		if strings.TrimSpace(stmt) == "" || isTransactionControlStatement(stmt) {
			continue
		}
		run = append(run, stmt)
	}
	if len(run) == 0 {
		t.Fatalf("%s holds no statements", raidDetectionMigration)
	}

	tx := database.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	// Never committed: the rows below and the migration's copy of every real chat vanish.
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back the carry-over transaction: %v", err)
		}
	}()

	cases := []struct {
		name      string
		chatID    int64
		threshold int
	}{
		{name: "zero is off", chatID: raidCarryOverChatID(t), threshold: 0},
		{name: "below the floor", chatID: raidCarryOverChatID(t), threshold: 1},
		{name: "in range", chatID: raidCarryOverChatID(t), threshold: 20},
		{name: "above the ceiling", chatID: raidCarryOverChatID(t), threshold: 900},
	}
	keptChat := raidCarryOverChatID(t)

	now := time.Now().UTC()
	seedChat := func(chatID int64) {
		t.Helper()
		if err := tx.Exec(
			`INSERT INTO chats (chat_id, chat_name, language, is_inactive, created_at, updated_at)
			 VALUES (?, ?, ?, FALSE, ?, ?)`,
			chatID, "carry over test", "en", now, now,
		).Error; err != nil {
			t.Fatalf("insert chat %d: %v", chatID, err)
		}
	}
	seedAntiRaid := func(chatID int64, threshold int) {
		t.Helper()
		if err := tx.Exec(
			`INSERT INTO antiraid_settings (chat_id, auto_antiraid_threshold) VALUES (?, ?)`,
			chatID, threshold,
		).Error; err != nil {
			t.Fatalf("insert antiraid_settings for chat %d: %v", chatID, err)
		}
	}
	for _, tc := range cases {
		seedChat(tc.chatID)
		seedAntiRaid(tc.chatID, tc.threshold)
	}
	// A chat that already has a raid detection row keeps it, whatever the old threshold.
	seedChat(keptChat)
	seedAntiRaid(keptChat, 40)
	if err := tx.Exec(
		`INSERT INTO chat_raid_detection (chat_id, join_enabled, join_threshold) VALUES (?, FALSE, 25)`,
		keptChat,
	).Error; err != nil {
		t.Fatalf("insert the existing chat_raid_detection row: %v", err)
	}

	readRows := func(chatID int64) []raidDetectionRow {
		t.Helper()
		var rows []raidDetectionRow
		if err := tx.Raw(
			`SELECT join_enabled, join_threshold, flood_enabled, flood_threshold
			 FROM chat_raid_detection WHERE chat_id = ?`, chatID,
		).Scan(&rows).Error; err != nil {
			t.Fatalf("read chat_raid_detection for chat %d: %v", chatID, err)
		}
		return rows
	}

	check := func(pass string) {
		t.Helper()
		if rows := readRows(cases[0].chatID); len(rows) != 0 {
			t.Errorf("%s: a zero threshold got %d row(s), want none so the defaults apply", pass, len(rows))
		}
		wants := map[int64]int{
			cases[1].chatID: 3,   // D-18: below the floor becomes 3
			cases[2].chatID: 20,  // in range is kept
			cases[3].chatID: 500, // D-22: above the ceiling becomes 500
		}
		for _, tc := range cases[1:] {
			rows := readRows(tc.chatID)
			if len(rows) != 1 {
				t.Errorf("%s: %s: got %d rows, want 1", pass, tc.name, len(rows))
				continue
			}
			got := rows[0]
			want := raidDetectionRow{JoinEnabled: true, JoinThreshold: wants[tc.chatID], FloodEnabled: true, FloodThreshold: 30}
			if got != want {
				t.Errorf("%s: %s (old threshold %d): row = %+v, want %+v", pass, tc.name, tc.threshold, got, want)
			}
		}
		rows := readRows(keptChat)
		if len(rows) != 1 {
			t.Errorf("%s: the chat with an existing row has %d rows, want 1", pass, len(rows))
		} else if rows[0].JoinEnabled || rows[0].JoinThreshold != 25 {
			t.Errorf("%s: the existing row was changed: %+v, want join_enabled false and join_threshold 25", pass, rows[0])
		}
	}

	execMigration := func(pass string) {
		t.Helper()
		for i, stmt := range run {
			if err := tx.Exec(stmt).Error; err != nil {
				t.Fatalf("%s: statement %d/%d of %s failed: %v", pass, i+1, len(run), raidDetectionMigration, err)
			}
		}
	}

	execMigration("first run")
	check("first run")
	execMigration("second run")
	check("second run (idempotent)")
}
