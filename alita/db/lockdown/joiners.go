package lockdown

import (
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// JoinRecord is what the join guard stores about one user who joined a locked group.
type JoinRecord struct {
	LockdownID     uint
	ChatID         int64
	UserID         int64
	FirstName      string
	Username       string
	IsBot          bool
	Path           string
	InviteLink     string
	ViaJoinRequest bool
	PerformerID    int64
	State          string
	BanUntil       int64
	JoinMsgID      int64
}

// RecordJoin is not implemented yet.
func RecordJoin(rec JoinRecord) (*models.LockdownJoiner, bool, error) {
	return nil, false, nil
}

// ClaimJoiner is not implemented yet.
func ClaimJoiner(id uint, from, to string) (bool, error) {
	return false, nil
}

// MoveJoiner is not implemented yet.
func MoveJoiner(id uint, from, to, detail string, countAttempt bool) (bool, error) {
	return false, nil
}

// ListPendingFresh is not implemented yet.
func ListPendingFresh(limit int) ([]models.LockdownJoiner, error) {
	return nil, nil
}

// DeletePendingJoin is not implemented yet.
func DeletePendingJoin(id uint) (bool, error) {
	return false, nil
}
