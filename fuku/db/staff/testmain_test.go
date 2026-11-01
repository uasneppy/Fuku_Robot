//go:build testtools

package staff

import (
	"os"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &models.StaffGroup{}, &models.StaffGroupLink{}, &models.StaffAction{}, &models.StaffActionGroup{}))
}
