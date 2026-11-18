//go:build testtools

package timezone

import (
	"context"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/cache"
)

func TestChatTimezoneRoundTrip(t *testing.T) {
	ctx := context.Background()
	chatID := -time.Now().UnixNano()
	t.Cleanup(func() { _ = Remove(ctx, chatID) })

	if tz, err := Get(ctx, chatID); err != nil || tz != "" {
		t.Fatalf("Get of a chat with no row = %q, %v, want empty", tz, err)
	}
	if err := Set(ctx, chatID, "Europe/Kyiv", 7); err != nil {
		t.Fatal(err)
	}
	cache.ResetLocalForTest()
	if tz, err := Get(ctx, chatID); err != nil || tz != "Europe/Kyiv" {
		t.Fatalf("Get after Set = %q, %v", tz, err)
	}
	if err := Set(ctx, chatID, "UTC+03:00", 8); err != nil {
		t.Fatal(err)
	}
	cache.ResetLocalForTest()
	if tz, err := Get(ctx, chatID); err != nil || tz != "UTC+03:00" {
		t.Fatalf("Get after a second Set = %q, %v, want the cache dropped", tz, err)
	}
	if err := Remove(ctx, chatID); err != nil {
		t.Fatal(err)
	}
	cache.ResetLocalForTest()
	if tz, err := GetFresh(ctx, chatID); err != nil || tz != "" {
		t.Fatalf("GetFresh after Remove = %q, %v", tz, err)
	}
	if tz, err := Get(ctx, chatID); err != nil || tz != "" {
		t.Fatalf("Get after Remove = %q, %v", tz, err)
	}
}
