package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

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

// runStaffSweep rechecks every Staff Group and every link live, with no command
// run, so a link whose owner changed in a group where the bot cannot see
// ownership updates still disappears (D-15.2). It reuses recheckStaffGroup and
// applyLinkHealth, so the notices are the ones the watchers and the panel post,
// and the conditional writes behind them keep each change to one notice.
//
// Only a successful Telegram answer removes anything; an error leaves the link
// untouched. It stops as soon as ctx is cancelled.
func runStaffSweep(ctx context.Context, b *gotgbot.Bot) staffSweepReport {
	report := staffSweepReport{Ran: true}

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
				log.Warnf("[StaffSweeper] bot lookup for group %d failed: %v", link.GroupChatID, err)
				continue
			}
			if applyLinkHealth(b, link, staffHealthFromBot(member, res)) {
				report.HealthChanged++
			}
		}
	}

	log.Infof("[StaffSweeper] cycle done: %d Staff Group(s), %d link(s) removed, %d health change(s)",
		report.StaffGroups, report.Removed, report.HealthChanged)
	return report
}
