//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// TestStaffActionBlockAboveMaxWaitFailsFast sets a shared Telegram block longer
// than the pacer's MaxWait and checks that a staff /ban does not sleep through it:
// every group is reported as rate limited at once, no request reaches a linked
// group, and the next run works as soon as the block is gone (a refused caller
// must not have taken a slot).
func TestStaffActionBlockAboveMaxWaitFailsFast(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	client := cache.GetRedisClient()
	if client == nil {
		t.Fatal("no redis client")
	}
	const blockKey = "alita:staff:pace:block"
	if err := client.Set(cache.Context, blockKey, "1", 2*time.Minute).Err(); err != nil {
		t.Fatalf("set shared block: %v", err)
	}

	start := time.Now()
	summary, _ := env.runStaffCommand("/ban 4242")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("run under a long block took %s, want a fast failure", elapsed)
	}

	wantFailedLine(t, summary, "Group A", "staff_act_fail_rate_limited")
	wantFailedLine(t, summary, "Group B", "staff_act_fail_rate_limited")
	for _, group := range env.groups {
		if calls := env.writes(group); len(calls) != 0 {
			t.Fatalf("group %d got %d write calls under a refused block, want none", group, len(calls))
		}
		if calls := env.callsTo("getChatAdministrators", group); len(calls) != 0 {
			t.Fatalf("group %d got %d getChatAdministrators calls under a refused block, want none", group, len(calls))
		}
		if calls := env.callsTo("getChatMember", group); len(calls) != 0 {
			t.Fatalf("group %d got %d getChatMember calls under a refused block, want none", group, len(calls))
		}
	}

	if err := client.Del(cache.Context, blockKey).Err(); err != nil {
		t.Fatalf("clear shared block: %v", err)
	}
	summary, _ = env.runStaffCommand("/ban 4242")
	wantDoneLine(t, summary, "Group A")
	wantDoneLine(t, summary, "Group B")
	for _, group := range env.groups {
		wantBanned(t, env.fake, group, staffTestTarget)
	}
}
