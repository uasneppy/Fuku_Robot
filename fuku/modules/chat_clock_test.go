//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// timezoneCleanup removes the chats' timezone rows and cache keys before the test and when it ends.
func timezoneCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatTimezone{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("chat_tz", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestChatClockFollowsDST holds the formatter over Kyiv's change to summer time on 29 March 2026 at 01:00
// UTC: the hour and the label move together, and a fixed offset never moves.
func TestChatClockFollowsDST(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2026, 3, 29, 0, 30, 0, 0, time.UTC)
	after := time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC)
	if got := formatChatTime(before, kyiv, "15:04"); got != "02:30 (UTC+2)" {
		t.Errorf("before the change = %q", got)
	}
	if got := formatChatTime(after, kyiv, "15:04"); got != "04:30 (UTC+3)" {
		t.Errorf("after the change = %q", got)
	}
	fixed, ok := parseStoredTimezone("UTC+05:30")
	if !ok {
		t.Fatal("UTC+05:30 does not parse")
	}
	if got := formatChatTime(after, fixed, "15:04"); got != "07:00 (UTC+5:30)" {
		t.Errorf("a fixed offset = %q", got)
	}
	if got := formatChatTime(after, time.UTC, "15:04"); got != "01:30 (UTC)" {
		t.Errorf("UTC = %q", got)
	}
}

func TestChatClockReadsTheGroup(t *testing.T) {
	chatID := uniqueModuleChatID()
	timezoneCleanup(t, chatID)
	if got := chatClockOf(chatID); got != defaultChatLocation() {
		t.Errorf("a group with no row = %v, want the default", got)
	}
	if got := defaultChatLocation().String(); got != "Europe/Kyiv" {
		t.Errorf("the default = %q, want Europe/Kyiv", got)
	}
	if result := applyTimezone(context.Background(), chatID, 1, "europe/warsaw"); result.Outcome != timezoneApplied || result.Value != "Europe/Warsaw" {
		t.Fatalf("apply = %+v", result)
	}
	if got := chatClockOf(chatID).String(); got != "Europe/Warsaw" {
		t.Errorf("after the write = %q", got)
	}
	failed := errors.New("database is down")
	mreqSwap(t, &chatTimezoneLookup, func(context.Context, int64) (string, error) { return "", failed })
	if got := chatClockOf(chatID); got != defaultChatLocation() {
		t.Errorf("a failed read = %v, want the default", got)
	}
}

func TestApplyTimezone(t *testing.T) {
	chatID := uniqueModuleChatID()
	timezoneCleanup(t, chatID)
	ctx := context.Background()
	for _, tc := range []struct {
		in      string
		outcome timezoneOutcome
		value   string
	}{
		{"Europe/Kyiv", timezoneApplied, "Europe/Kyiv"},
		{"Europe/Kiev", timezoneApplied, "Europe/Kiev"},
		{"america/new_york", timezoneApplied, "America/New_York"},
		{"+3", timezoneApplied, "UTC+03:00"},
		{"-5", timezoneApplied, "UTC-05:00"},
		{"+5:30", timezoneApplied, "UTC+05:30"},
		{"UTC+2", timezoneApplied, "UTC+02:00"},
		{"utc", timezoneApplied, "UTC"},
		{"default", timezoneReset, ""},
		{"", timezoneInvalid, ""},
		{"Mars/Olympus", timezoneInvalid, ""},
		{"+15", timezoneInvalid, ""},
		{"+5:20", timezoneInvalid, ""},
		{"Local", timezoneInvalid, ""},
		{"EST", timezoneInvalid, ""},
	} {
		got := applyTimezone(ctx, chatID, 1, tc.in)
		if got.Outcome != tc.outcome || got.Value != tc.value {
			t.Errorf("applyTimezone(%q) = %+v, want %d %q", tc.in, got, tc.outcome, tc.value)
		}
	}
	mreqSwap(t, &timezoneWrites.Set, func(context.Context, int64, string, int64) error { return errors.New("down") })
	if got := applyTimezone(ctx, chatID, 1, "UTC"); got.Outcome != timezoneSaveFailed {
		t.Errorf("a failed write = %+v", got)
	}
}
