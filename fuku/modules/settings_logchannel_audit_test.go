//go:build testtools

package modules

import (
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
)

// TestSettingsLogChannelAuditSurvivesFailedReadBack proves a stored category change always leaves its
// #SETTINGS audit entry, as /log and /nolog always do: the write lands and the two reads that follow
// it (the apply function's read-back and the page's second read) hit a short database fault, so the
// page cannot be drawn, and the entry must still reach the log channel. Without it an administrator
// could change a category during such a fault and leave no trace in the trail the page keeps.
func TestSettingsLogChannelAuditSurvivesFailedReadBack(t *testing.T) {
	env := newSettingsLogEnv(t)
	channelID := settingsLogLink(t, env.chat)
	menu := settingsLogOpenGroupMenu(t, env)

	var failNext atomic.Int32
	realWrite := logCategoryWrites
	mreqSwap(t, &logCategoryWrites, func(chatID int64, name string, on bool) error {
		err := realWrite(chatID, name, on)
		failNext.Store(2) // exactly the read-back and the page's re-read
		return err
	})
	fault := "test:fail_log_channels_read_" + t.Name()
	err := db.DB.Callback().Query().Before("gorm:query").Register(fault, func(tx *gorm.DB) {
		if tx.Statement.Table == "log_channels" && failNext.Load() > 0 {
			failNext.Add(-1)
			_ = tx.AddError(errors.New("database is down"))
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Query().Remove(fault); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})

	answers := env.answerCount()
	menu.pressCell(logchannels.CategoryUser)

	if left := failNext.Load(); left != 0 {
		t.Fatalf("%d of the two reads after the write were not made, so the test did not reach its scenario", left)
	}
	wantFlags := []byte(strings.Repeat("1", len(logchannels.AllCategories)))
	wantFlags[slices.Index(logchannels.AllCategories, logchannels.CategoryUser)] = '0'
	if got := logGoldenFlags(t, env.chat.Id); got != string(wantFlags) {
		t.Errorf("the flags are %s, want %s (only the user category stored off)", got, wantFlags)
	}
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("the press answered %d times, want once", got)
	}
	posts := settingsLogPosts(env, channelID)
	if len(posts) != 1 || !strings.Contains(posts[0], "#SETTINGS") {
		t.Errorf("a stored change whose page could not be drawn posted %v, want the one #SETTINGS audit entry", posts)
	}
}
