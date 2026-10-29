package modules

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/shutdown"
)

// shutdownDrainGrace is the time the manager allows a drain on top of its own wait,
// so the drain's own timer ends it, not the manager's.
const shutdownDrainGrace = time.Second

// RegisterStaffActionsDrain registers StopStaffActions with an allowance of
// staffActionStopWait plus shutdownDrainGrace. Register it after the DB-close handler
// so LIFO runs it first: the manager then waits out the full staff wait before closing
// the database, because workers still write links and records while they wind down.
func RegisterStaffActionsDrain(m *shutdown.Manager) {
	m.RegisterHandlerWithTimeout(func() error {
		log.Info("[Shutdown] Stopping staff actions...")
		StopStaffActions()
		return nil
	}, staffActionStopWait+shutdownDrainGrace)
}

// RegisterLockdownWorkerDrain registers StopLockdownWorker with an allowance of
// lockdownWorkerStopWait plus shutdownDrainGrace. Register it after the DB-close handler
// so LIFO runs it first. Its allowance follows the worker's own wait, so a future change
// of that wait cannot be cut short by the manager's default.
func RegisterLockdownWorkerDrain(m *shutdown.Manager) {
	m.RegisterHandlerWithTimeout(func() error {
		log.Info("[Shutdown] Stopping lockdown worker...")
		StopLockdownWorker()
		return nil
	}, lockdownWorkerStopWait+shutdownDrainGrace)
}

// RegisterRaidPostsDrain registers StopRaidPosts with an allowance of raidPostStopWait plus
// shutdownDrainGrace. Register it after the DB-close handler so LIFO runs it first: a post
// reads the Staff Group link, the log channel and the language from the database. The wait
// is short on purpose, because a post is best effort and the lockdown it announces is
// already in place.
func RegisterRaidPostsDrain(m *shutdown.Manager) {
	m.RegisterHandlerWithTimeout(func() error {
		log.Info("[Shutdown] Draining raid detection posts...")
		StopRaidPosts()
		return nil
	}, raidPostStopWait+shutdownDrainGrace)
}

// RegisterRaidAIDrain registers StopRaidAI with an allowance of raidAIStopWait plus
// shutdownDrainGrace. Register it after the DB-close handler so LIFO runs it first, and
// after RegisterRaidPostsDrain, so it runs before the posts drain: a lockdown start an AI
// verdict asked for reads and writes the database and can post a notice. The AI spam drain
// is registered after it, so it runs earlier still, because an /aispam verdict is what asks
// for the start. The wait is short on purpose: the drain cancels the AI calls first, and a
// start cut off leaves at worst an unconfirmed row the lockdown worker settles.
func RegisterRaidAIDrain(m *shutdown.Manager) {
	m.RegisterHandlerWithTimeout(func() error {
		log.Info("[Shutdown] Draining raid AI checks...")
		StopRaidAI()
		return nil
	}, raidAIStopWait+shutdownDrainGrace)
}
