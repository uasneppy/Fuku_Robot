//go:build testtools

package shutdown

import "time"

// RunForTest runs every registered handler the way a shutdown does and returns the
// exit code instead of exiting. It has no signal handling and no once guard.
func (m *Manager) RunForTest(budget, defaultTimeout time.Duration) int {
	return m.runHandlers(budget, defaultTimeout)
}
