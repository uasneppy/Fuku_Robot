//go:build testtools

package lockdown

import (
	"os"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &models.ChatLockdown{}, &models.LockdownJoiner{}, &models.ChatLockdownAlert{}, &models.ChatLockdownBurst{}, &models.ChatLockdownBanRun{}))
}
