//go:build testtools

package captcha

import (
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbmodels "github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestCreateCaptchaAttemptExpiresAtMicroseconds pins C1: the attempt is created with the
// microsecond-precision expiry PostgreSQL keeps, so the in-memory value equals the stored one.
func TestCreateCaptchaAttemptExpiresAtMicroseconds(t *testing.T) {
	skipIfNoDb(t)

	base := time.Now().UnixNano()
	userID := base
	chatID := base + 1
	ensureCaptchaParents(t, userID, chatID)
	t.Cleanup(func() {
		_ = db.DB.Where("user_id = ? AND chat_id = ?", userID, chatID).Delete(&dbmodels.CaptchaAttempts{}).Error
	})

	for i := 0; i < 5; i++ {
		attempt, err := CreateCaptchaAttemptPreMessage(userID, chatID, "42", 2)
		if err != nil || attempt == nil {
			t.Fatalf("CreateCaptchaAttemptPreMessage() = %v, %v", attempt, err)
		}
		if ns := attempt.ExpiresAt.Nanosecond() % 1000; ns != 0 {
			t.Fatalf("ExpiresAt = %v carries %d ns below the microsecond", attempt.ExpiresAt, ns)
		}
		stored, err := GetCaptchaAttemptByID(attempt.ID)
		if err != nil || stored == nil {
			t.Fatalf("GetCaptchaAttemptByID() = %v, %v", stored, err)
		}
		if !stored.ExpiresAt.Equal(attempt.ExpiresAt) {
			t.Fatalf("stored ExpiresAt = %v, in memory %v; want equal", stored.ExpiresAt, attempt.ExpiresAt)
		}
	}
}
