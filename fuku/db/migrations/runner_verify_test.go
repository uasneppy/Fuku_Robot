package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/config"
)

// writeMigrationFile writes one migration file into dir and returns its path.
func writeMigrationFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

// TestRunMigrationFilesAlwaysVerifies holds I5: the runner compared the number of recorded rows
// with the number of files and returned at once when they were equal, so a file modified after
// it was applied passed unseen, and a recorded row whose file is gone hid a new pending file,
// which was then never applied. Every file is now checked on every run.
func TestRunMigrationFilesAlwaysVerifies(t *testing.T) {
	previousConfig := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previousConfig })
	config.AppConfig = &config.Config{AutoMigrateSilentFail: false}

	t.Run("a modified file is refused", func(t *testing.T) {
		database := newMetadataTestDB(t)
		dir := t.TempDir()
		file := writeMigrationFile(t, dir, "001_one.sql", "CREATE TABLE verify_one (id INTEGER);")
		if err := database.Create(&SchemaMigration{Version: "001_one.sql", Checksum: strings.Repeat("0", 64)}).Error; err != nil {
			t.Fatalf("record the migration: %v", err)
		}
		runner := &MigrationRunner{db: database, migrationsPath: dir}
		err := runner.runMigrationFiles([]string{file})
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("runMigrationFiles() = %v, want a checksum mismatch", err)
		}
	})

	t.Run("a row with no file does not hide a pending file", func(t *testing.T) {
		database := newMetadataTestDB(t)
		dir := t.TempDir()
		file := writeMigrationFile(t, dir, "002_two.sql", "CREATE TABLE verify_two (id INTEGER);")
		if err := database.Create(&SchemaMigration{Version: "001_gone.sql", Checksum: strings.Repeat("0", 64)}).Error; err != nil {
			t.Fatalf("record the orphan row: %v", err)
		}
		runner := &MigrationRunner{db: database, migrationsPath: dir}
		if err := runner.runMigrationFiles([]string{file}); err != nil {
			t.Fatalf("runMigrationFiles() = %v", err)
		}
		if !migrationApplied(t, runner, "002_two.sql") {
			t.Fatal("the pending file was not applied")
		}
		if !database.Migrator().HasTable("verify_two") {
			t.Fatal("the pending file's table does not exist")
		}
		orphans, err := runner.recordedWithoutFile([]string{file})
		if err != nil {
			t.Fatalf("recordedWithoutFile() = %v", err)
		}
		if len(orphans) != 1 || orphans[0] != "001_gone.sql" {
			t.Fatalf("recordedWithoutFile() = %v, want [001_gone.sql]", orphans)
		}
	})
}
