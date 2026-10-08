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
