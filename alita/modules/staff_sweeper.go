package modules

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// staffSweepLockKey is the operational Redis key one replica holds per cycle. It
// sits outside the alita:cache: prefix, so CLEAR_CACHE_ON_STARTUP leaves it alone.
const staffSweepLockKey = "alita:staff:sweep:lock"

// staffSweepOwner identifies this process as the holder of the sweep lock.
var staffSweepOwner = uuid.NewString()

// Variables rather than constants so tests can shorten them.
var (
	// staffSweepInterval is how often the hourly sweep runs.
	staffSweepInterval = time.Hour
	// staffSweepPace is the pause before each Telegram link check, so a sweep over
	// many groups stays well under the Bot API rate limits.
	staffSweepPace = 250 * time.Millisecond
)

// staffSweepReport is what one sweep cycle did.
type staffSweepReport struct {
	// Ran is false when another replica holds the sweep lock for this cycle.
	Ran bool
	// StaffGroups is the number of Staff Groups walked.
	StaffGroups int
	// Removed is the number of links this sweep unlinked.
	Removed int
	// HealthChanged is the number of links whose bot health this sweep changed.
	HealthChanged int
	// Orphans is the number of links deleted because their Staff Group row is gone.
	Orphans int64
}

// staffSweepPaceFunc waits staffSweepPace before the next Telegram call. It
// returns false when ctx is cancelled first, so the caller stops promptly.
func staffSweepPaceFunc(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	if staffSweepPace <= 0 {
		return true
	}
	timer := time.NewTimer(staffSweepPace)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// staffSweepFirstDelay returns how long to wait before the first cycle: one to
// five minutes after startup, which catches ownership updates the Bot API dropped
// while the bot was down for over 24 hours. A variable so tests can shorten it.
var staffSweepFirstDelay = func() time.Duration {
	return time.Minute + time.Duration(rand.IntN(240))*time.Second
}

// staffSweepCycleHook, when not nil, runs at the start of every cycle. Production
// leaves it nil; tests use it to count cycles or inject a panic.
var staffSweepCycleHook func()

// StartStaffSweeper is a placeholder until the lifecycle is implemented.
func StartStaffSweeper(b *gotgbot.Bot) {}

// StopStaffSweeper is a placeholder until the lifecycle is implemented.
func StopStaffSweeper() {}

// staffSweepLockTTL is how long a replica holds the sweep lock: one minute short
// of the interval, so the next hourly cycle can always take it.
func staffSweepLockTTL() time.Duration {
	ttl := staffSweepInterval - time.Minute
	if ttl <= 0 {
		ttl = staffSweepInterval / 2
	}
	return ttl
}

// acquireStaffSweepLock reports whether this replica may run the current cycle.
// Without Redis, or on a Redis error, it returns true: the sweep stays correct
// unguarded because every write it makes is one conditional statement, so
// correctness never depends on the lock. The lock is never released early; its
// TTL is what limits the fleet to one sweep per interval.
func acquireStaffSweepLock() bool {
	client := cache.GetRedisClient()
	if client == nil {
		return true
	}
	ok, err := client.SetNX(cache.Context, staffSweepLockKey, staffSweepOwner, staffSweepLockTTL()).Result()
	if err != nil {
		log.Warnf("[StaffSweeper] lock unavailable, sweeping unguarded: %v", err)
		return true
	}
	return ok
}

// runStaffSweep rechecks every Staff Group and every link live, with no command
// run, so a link whose owner changed in a group where the bot cannot see
// ownership updates still disappears (D-15.2). It reuses recheckStaffGroup and
// applyLinkHealth, so the notices are the ones the watchers and the panel post,
// and the conditional writes behind them keep each change to one notice.
//
// Only one replica runs a given cycle (acquireStaffSweepLock); the others get
// Ran=false and make no Telegram call. Only a successful Telegram answer removes
// a link or a Staff Group is ever touched: an error leaves everything as it was,
// and a 400 carrying migrate_to_chat_id re-keys the chat instead. Links whose
// Staff Group row is gone are deleted silently. It stops as soon as ctx is
// cancelled.
func runStaffSweep(ctx context.Context, b *gotgbot.Bot) staffSweepReport {
	if !acquireStaffSweepLock() {
		return staffSweepReport{}
	}
	report := staffSweepReport{Ran: true}

	orphans, err := staff.DeleteOrphanLinks()
	if err != nil {
		log.Errorf("[StaffSweeper] orphan cleanup failed: %v", err)
	}
	report.Orphans = orphans

	groups, err := staff.ListStaffGroupsFresh()
	if err != nil {
		return report
	}
	report.StaffGroups = len(groups)

	for _, sg := range groups {
		if ctx.Err() != nil {
			break
		}
		summary := recheckStaffGroup(ctx, b, sg.ChatID, staffSweepPaceFunc)
		report.Removed += len(summary.RemovedGroupIDs)

		links, err := staff.ListLinksByStaffFresh(sg.ChatID)
		if err != nil {
			continue
		}
		for _, link := range links {
			if !staffSweepPaceFunc(ctx) {
				break
			}
			member, res, err := chat_status.FetchBotMember(b, link.GroupChatID)
			if res == chat_status.BotMemberUnknown {
				// Never delete on an error (P4). A migrated chat is re-keyed so
				// the next cycle finds it under its new ID.
				if newID, rekeyed := rekeyFromTelegramError(link.GroupChatID, err); rekeyed {
					log.Infof("[StaffSweeper] group %d migrated to %d, link re-keyed", link.GroupChatID, newID)
					continue
				}
				log.Warnf("[StaffSweeper] bot lookup for group %d failed: %v", link.GroupChatID, err)
				continue
			}
			if applyLinkHealth(b, link, staffHealthFromBot(member, res)) {
				report.HealthChanged++
			}
		}
	}

	log.Infof("[StaffSweeper] cycle done: %d Staff Group(s), %d link(s) removed, %d health change(s), %d orphan(s)",
		report.StaffGroups, report.Removed, report.HealthChanged, report.Orphans)
	return report
}
