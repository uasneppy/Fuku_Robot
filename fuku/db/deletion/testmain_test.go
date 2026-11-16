//go:build testtools

package deletion

import (
	"os"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m,
		&models.DeleteRule{},
		&models.ScheduledDeletion{},
		&models.StaffGroup{},
		&models.StaffGroupLink{},
		&models.GreetingSettings{},
	))
}
