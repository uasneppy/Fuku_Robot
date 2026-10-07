package lockdown

import "github.com/divkix/Alita_Robot/alita/db/models"

// RED scaffold: the signatures compile and do nothing, so the tests written first
// fail on their assertions. The next commit replaces this file.

// Start is a stub.
func Start(row *models.ChatLockdown) (bool, error) { return false, nil }

// GetActiveFresh is a stub.
func GetActiveFresh(chatID int64) (*models.ChatLockdown, error) { return nil, nil }

// GetFresh is a stub.
func GetFresh(id uint) (*models.ChatLockdown, error) { return nil, nil }

// ConfirmLocked is a stub.
func ConfirmLocked(id uint) (bool, error) { return false, nil }

// DeleteUnconfirmed is a stub.
func DeleteUnconfirmed(id uint) (bool, error) { return false, nil }

// BeginLift is a stub.
func BeginLift(id uint, by int64, byName string, manualChange bool) (bool, error) {
	return false, nil
}

// FinishLift is a stub.
func FinishLift(id uint) (bool, error) { return false, nil }
