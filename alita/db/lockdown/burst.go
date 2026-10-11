package lockdown

// The burst of a lockdown (chat_lockdown_burst plus the joiner rows the lockdown
// recorded) is operational state shared by every replica. It is always read fresh from
// the database and never cached, so no cache key is read here and none needs
// invalidation. The pre-lockdown rows are written once, by BeginAlert, in the
// transaction that decides the alert; later changes to a row are conditional updates.

import (
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// BurstAccount is one account of a lockdown's burst, as an alert lists it. Pre is true
// for an account that joined before the lock (from the saved tracker snapshot) and
// false for one the lockdown itself recorded.
type BurstAccount struct {
	UserID     int64
	FirstName  string
	Username   string
	InviteLink string
	IsBot      bool
	Pre        bool
}

// BurstCounts are the numbers behind a lockdown's trigger: Before is how many accounts
// joined in the minutes before the lock, Since how many other accounts the lockdown
// recorded after it.
type BurstCounts struct {
	Before int64
	Since  int64
}

// BurstSampleFresh reads the burst of a lockdown straight from the database: the first
// limit accounts and the counts of the whole burst. The pre-lockdown accounts come
// first, by join time and then by ID, followed by the joiner rows of the lockdown whose
// user has no pre-lockdown row (so someone who joined, left and joined again is listed
// once, as a pre-lockdown account), in the order they were recorded. Before is the
// number of pre-lockdown rows and Since the number of those joiner rows, whatever their
// state and whatever limit is. A limit of zero or less returns the counts alone.
func BurstSampleFresh(lockdownID uint, limit int) ([]BurstAccount, BurstCounts, error) {
	var counts BurstCounts
	fail := func(err error) ([]BurstAccount, BurstCounts, error) {
		log.Errorf("[Lockdown] BurstSampleFresh: %v", err)
		return nil, BurstCounts{}, alitaerrors.Wrapf(err, "read the burst of lockdown %d", lockdownID)
	}

	preUsers := db.DB.Model(&models.ChatLockdownBurst{}).
		Select("user_id").
		Where("lockdown_id = ? AND source = ?", lockdownID, models.BurstSourcePre)

	if err := db.DB.Model(&models.ChatLockdownBurst{}).
		Where("lockdown_id = ? AND source = ?", lockdownID, models.BurstSourcePre).
		Count(&counts.Before).Error; err != nil {
		return fail(err)
	}
	if err := db.DB.Model(&models.LockdownJoiner{}).
		Where("lockdown_id = ? AND user_id NOT IN (?)", lockdownID, preUsers).
		Count(&counts.Since).Error; err != nil {
		return fail(err)
	}
	if limit <= 0 {
		return nil, counts, nil
	}

	var pre []models.ChatLockdownBurst
	if err := db.DB.
		Where("lockdown_id = ? AND source = ?", lockdownID, models.BurstSourcePre).
		Order("joined_at, id").
		Limit(limit).
		Find(&pre).Error; err != nil {
		return fail(err)
	}
	accounts := make([]BurstAccount, 0, len(pre))
	for _, row := range pre {
		accounts = append(accounts, BurstAccount{
			UserID:     row.UserID,
			FirstName:  row.FirstName,
			Username:   row.Username,
			InviteLink: row.InviteLink,
			IsBot:      row.IsBot,
			Pre:        true,
		})
	}

	if rest := limit - len(accounts); rest > 0 && counts.Since > 0 {
		var joiners []models.LockdownJoiner
		if err := db.DB.
			Where("lockdown_id = ? AND user_id NOT IN (?)", lockdownID, preUsers).
			Order("id").
			Limit(rest).
			Find(&joiners).Error; err != nil {
			return fail(err)
		}
		for _, row := range joiners {
			accounts = append(accounts, BurstAccount{
				UserID:     row.UserID,
				FirstName:  row.FirstName,
				Username:   row.Username,
				InviteLink: row.InviteLink,
				IsBot:      row.IsBot,
			})
		}
	}
	return accounts, counts, nil
}
