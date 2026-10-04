//go:build testtools

package modules

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// sweepNoPace removes the pause between link checks for the test's duration.
func sweepNoPace(t *testing.T) {
	t.Helper()
	previous := staffSweepPace
	staffSweepPace = 0
	t.Cleanup(func() { staffSweepPace = previous })
}

func TestStaffSweepTracer(t *testing.T) {
	sweepNoPace(t)
	env := newOwnershipEnv(t)
	g1 := env.addGroup(t, env.ownerID, "Group One")
	g2 := env.addGroup(t, env.ownerID, "Group Two")

	// Nobody ran a command and no watcher fired: G2's creator changed while the
	// bot is only a plain member there, and G1's bot lost the restrict right.
	env.client.setCreator(g2.GroupChatID, env.ownerID+1)
	env.client.smu.Lock()
	env.client.botRole[g2.GroupChatID] = staffRoleMember
	env.client.botRole[g1.GroupChatID] = staffRoleAdminNoRestrict
	env.client.smu.Unlock()

	report := runStaffSweep(context.Background(), env.bot)

	if !report.Ran || report.Removed != 1 || report.HealthChanged != 1 {
		t.Fatalf("report = %+v, want Ran, Removed 1, HealthChanged 1", report)
	}
	wantLinkGone(t, g2.GroupChatID)
	wantHealth(t, g1.GroupChatID, models.StaffHealthBotCannotRestrict)

	notices := textsToChat(env.client, env.staffID)
	if len(notices) != 2 {
		t.Fatalf("notices to the Staff Group = %q, want 2", notices)
	}
	var unlinked, health int
	for _, text := range notices {
		switch {
		case strings.Contains(text, staffMarker("staff_notice_unlinked_group_owner_changed")):
			unlinked++
		case strings.Contains(text, staffMarker("staff_notice_health_bot_cannot_restrict")):
			health++
		}
	}
	if unlinked != 1 || health != 1 {
		t.Fatalf("notices = %q, want one owner-changed and one cannot-restrict", notices)
	}

	before := len(env.client.callsFor("sendMessage"))
	again := runStaffSweep(context.Background(), env.bot)
	if again.Removed != 0 || again.HealthChanged != 0 {
		t.Fatalf("second report = %+v, want no changes", again)
	}
	if after := len(env.client.callsFor("sendMessage")); after != before {
		t.Fatalf("second sweep sent %d more message(s), want none", after-before)
	}
}

// sweepFlood is a scripted 429 answer.
func sweepFlood(method string) error {
	return &gotgbot.TelegramError{
		Method:         method,
		Code:           429,
		Description:    "Too Many Requests: retry after 5",
		ResponseParams: &gotgbot.ResponseParameters{RetryAfter: 5},
	}
}

// sweepCallCount is the number of Telegram calls the fake has recorded.
func sweepCallCount(c *staffBotClient) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// sweepStaleGroup links a group to env's Staff Group whose live creator is someone
// else, so a sweep that gets a real answer removes it.
func sweepStaleGroup(t *testing.T, env *ownershipEnv, title string) models.StaffGroupLink {
	t.Helper()
	link := env.addGroup(t, env.ownerID, title)
	env.client.setCreator(link.GroupChatID, env.ownerID+1)
	return link
}

func TestStaffSweepLockOneRunner(t *testing.T) {
	sweepNoPace(t)
	withMiniredis(t)
	env := newOwnershipEnv(t)
	env.addGroup(t, env.ownerID, "Group One")

	reports := make([]staffSweepReport, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range reports {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reports[i] = runStaffSweep(context.Background(), env.bot)
		}()
	}
	close(start)
	wg.Wait()

	ran := 0
	for _, r := range reports {
		if r.Ran {
			ran++
		}
	}
	if ran != 1 {
		t.Fatalf("reports = %+v, want exactly one with Ran=true", reports)
	}
	if n := len(callsToChat(env.client, "getChatAdministrators", env.staffID)); n != 1 {
		t.Fatalf("getChatAdministrators for the Staff Group was called %d times, want 1 (one runner)", n)
	}
	ttl := cache.GetRedisClient().TTL(cache.Context, staffSweepLockKey).Val()
	if ttl <= 0 || ttl > staffSweepInterval-time.Minute {
		t.Fatalf("lock TTL = %v, want in (0, %v]", ttl, staffSweepInterval-time.Minute)
	}
}

func TestStaffSweepSkipsWhenLockHeld(t *testing.T) {
	sweepNoPace(t)
	withMiniredis(t)
	env := newOwnershipEnv(t)
	stale := sweepStaleGroup(t, env, "Group One")
	if err := cache.GetRedisClient().Set(cache.Context, staffSweepLockKey, "another-replica", time.Hour).Err(); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	report := runStaffSweep(context.Background(), env.bot)

	if report.Ran {
		t.Fatalf("report = %+v, want Ran=false while another replica holds the lock", report)
	}
	if n := sweepCallCount(env.client); n != 0 {
		t.Fatalf("%d Telegram calls were made, want none", n)
	}
	if link, err := staff.GetLinkOfGroupFresh(stale.GroupChatID); err != nil || link == nil {
		t.Fatalf("link = (%+v, %v), want it untouched", link, err)
	}
}

func TestStaffSweepRunsWithoutRedis(t *testing.T) {
	sweepNoPace(t)
	t.Cleanup(cache.DisableRedisForTest())
	env := newOwnershipEnv(t)
	stale := sweepStaleGroup(t, env, "Group One")

	report := runStaffSweep(context.Background(), env.bot)

	if !report.Ran || report.Removed != 1 {
		t.Fatalf("report = %+v, want Ran with one link removed", report)
	}
	wantLinkGone(t, stale.GroupChatID)
}

func TestStaffSweepRunsOnRedisError(t *testing.T) {
	sweepNoPace(t)
	withMiniredis(t)
	env := newOwnershipEnv(t)
	stale := sweepStaleGroup(t, env, "Group One")
	// A closed client makes SetNX fail; correctness must not depend on the lock.
	if err := cache.GetRedisClient().Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}

	report := runStaffSweep(context.Background(), env.bot)

	if !report.Ran || report.Removed != 1 {
		t.Fatalf("report = %+v, want Ran with one link removed despite the Redis error", report)
	}
	wantLinkGone(t, stale.GroupChatID)
}

func TestStaffSweepErrorsNeverDelete(t *testing.T) {
	sweepNoPace(t)
	env := newOwnershipEnv(t)
	// S cannot be asked who its creator is.
	env.client.setFailure("getChatAdministrators", env.staffID, sweepFlood("getChatAdministrators"))
	g1 := env.addGroup(t, env.ownerID, "Group One")

	// S2 is healthy, but one linked group cannot be asked who its creator is and
	// another cannot be asked about the bot.
	s2 := uniqueModuleChatID()
	staffCleanup(t, s2)
	env.client.setCreator(s2, env.ownerID)
	if _, err := staff.CreateStaffGroup(s2, env.ownerID, "Second HQ"); err != nil {
		t.Fatalf("create second Staff Group: %v", err)
	}
	var second []models.StaffGroupLink
	for _, title := range []string{"Group Two", "Group Three"} {
		groupID := uniqueModuleChatID()
		staffCleanup(t, groupID)
		env.client.setCreator(groupID, env.ownerID)
		link := models.StaffGroupLink{GroupChatID: groupID, StaffChatID: s2, OwnerUserID: env.ownerID, GroupTitle: title}
		if err := db.DB.Create(&link).Error; err != nil {
			t.Fatalf("seed link %q: %v", title, err)
		}
		second = append(second, link)
	}
	env.client.setFailure("getChatAdministrators", second[0].GroupChatID, sweepFlood("getChatAdministrators"))
	env.client.setFailure("getChatMember", second[1].GroupChatID, sweepFlood("getChatMember"))

	report := runStaffSweep(context.Background(), env.bot)

	if !report.Ran || report.Removed != 0 || report.HealthChanged != 0 {
		t.Fatalf("report = %+v, want nothing removed or changed", report)
	}
	for _, link := range append([]models.StaffGroupLink{g1}, second...) {
		wantHealth(t, link.GroupChatID, models.StaffHealthOK)
	}
	for _, chatID := range []int64{env.staffID, s2} {
		if row, err := staff.GetStaffGroupFresh(chatID); err != nil || row == nil {
			t.Fatalf("Staff Group %d = (%+v, %v), want it kept", chatID, row, err)
		}
	}
	if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
		t.Fatalf("%d notice(s) were sent (%+v), want none", len(sent), sent)
	}
}

func TestStaffSweepRekeysOnMigrateError(t *testing.T) {
	sweepNoPace(t)
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")
	newID := uniqueModuleChatID()
	staffCleanup(t, newID)
	env.client.setFailure("getChatMember", link.GroupChatID, &gotgbot.TelegramError{
		Method:         "getChatMember",
		Code:           400,
		Description:    "Bad Request: group chat was upgraded to a supergroup chat",
		ResponseParams: &gotgbot.ResponseParameters{MigrateToChatId: newID},
	})

	runStaffSweep(context.Background(), env.bot)

	moved, err := staff.GetLinkOfGroupFresh(newID)
	if err != nil || moved == nil || moved.StaffChatID != env.staffID {
		t.Fatalf("link of the new chat = (%+v, %v), want the re-keyed link", moved, err)
	}
	if old, err := staff.GetLinkOfGroupFresh(link.GroupChatID); err != nil || old != nil {
		t.Fatalf("link of the old chat = (%+v, %v), want none", old, err)
	}
}

func TestStaffSweepDeletesOrphanLinks(t *testing.T) {
	sweepNoPace(t)
	env := newOwnershipEnv(t)
	kept := env.addGroup(t, env.ownerID, "Group One")

	orphanStaff, orphanGroup := uniqueModuleChatID(), uniqueModuleChatID()
	staffCleanup(t, orphanStaff, orphanGroup)
	orphan := models.StaffGroupLink{GroupChatID: orphanGroup, StaffChatID: orphanStaff, OwnerUserID: env.ownerID, GroupTitle: "Orphan"}
	if err := db.DB.Create(&orphan).Error; err != nil {
		t.Fatalf("seed orphan link: %v", err)
	}

	report := runStaffSweep(context.Background(), env.bot)

	if report.Orphans != 1 {
		t.Fatalf("report = %+v, want Orphans 1", report)
	}
	wantLinkGone(t, orphanGroup)
	wantHealth(t, kept.GroupChatID, models.StaffHealthOK)
	if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
		t.Fatalf("%d notice(s) were sent (%+v), want none for an orphan", len(sent), sent)
	}
}

func TestStaffSweepStopsOnCancel(t *testing.T) {
	env := newOwnershipEnv(t)
	for i := 0; i < 10; i++ {
		env.addGroup(t, env.ownerID, "Group")
	}
	previous := staffSweepPace
	staffSweepPace = 50 * time.Millisecond
	t.Cleanup(func() { staffSweepPace = previous })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		runStaffSweep(ctx, env.bot)
	}()

	// The Staff Group's own check is call one; the second is the first link.
	deadline := time.After(5 * time.Second)
	for len(env.client.callsFor("getChatAdministrators")) < 2 {
		select {
		case <-deadline:
			t.Fatal("the sweep never reached its first link")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runStaffSweep did not return within 500ms of the cancel")
	}
}
