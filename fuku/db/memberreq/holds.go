package memberreq

import (
	"context"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// StaleClaim is how old a pending hold (claimed, never confirmed punished) must be before
// ReclaimStaleHold may take it over. It is a variable only so tests can shorten it.
var StaleClaim = 2 * time.Minute

// ClaimHold stub.
func ClaimHold(ctx context.Context, hold *models.ChatRequirementHold) (bool, error) {
	return true, nil
}

// ReclaimStaleHold stub.
func ReclaimStaleHold(ctx context.Context, fresh *models.ChatRequirementHold) (bool, int64, error) {
	return false, 0, nil
}
