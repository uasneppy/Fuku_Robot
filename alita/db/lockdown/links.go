package lockdown

// The invite links a burst came through are read from the same rows as the burst
// itself (chat_lockdown_burst plus the joiner rows the lockdown recorded). They are
// operational state shared by every replica, always read fresh from the database and
// never cached, so no cache key is read here and none needs invalidation.

import (
	"sort"

	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// InviteLinkCount is one invite link and how many accounts of the burst joined
// through it.
type InviteLinkCount struct {
	Link  string
	Count int64
}

// TopInviteLinksFresh reads, straight from the database, the invite links the burst of
// a lockdown came through, most used first and ties by link. The burst is the same one
// BurstSampleFresh counts: the pre-lockdown rows and the lockdown's joiner rows whose
// user has no pre-lockdown row, so someone who joined, left and joined again counts
// once, by the pre-lockdown row. Join requests count like joins, and an account whose
// join Telegram reported no link for is left out. A burst with no link returns an empty
// list.
func TopInviteLinksFresh(lockdownID uint) ([]InviteLinkCount, error) {
	fail := func(err error) ([]InviteLinkCount, error) {
		log.Errorf("[Lockdown] TopInviteLinksFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "read the invite links of lockdown %d", lockdownID)
	}

	preUsers := db.DB.Model(&models.ChatLockdownBurst{}).
		Select("user_id").
		Where("lockdown_id = ? AND source = ?", lockdownID, models.BurstSourcePre)

	var pre []InviteLinkCount
	if err := db.DB.Model(&models.ChatLockdownBurst{}).
		Select("invite_link AS link, COUNT(*) AS count").
		Where("lockdown_id = ? AND source = ? AND invite_link <> ?", lockdownID, models.BurstSourcePre, "").
		Group("invite_link").
		Scan(&pre).Error; err != nil {
		return fail(err)
	}
	var joined []InviteLinkCount
	if err := db.DB.Model(&models.LockdownJoiner{}).
		Select("invite_link AS link, COUNT(*) AS count").
		Where("lockdown_id = ? AND invite_link <> ? AND user_id NOT IN (?)", lockdownID, "", preUsers).
		Group("invite_link").
		Scan(&joined).Error; err != nil {
		return fail(err)
	}

	totals := make(map[string]int64, len(pre)+len(joined))
	for _, group := range [][]InviteLinkCount{pre, joined} {
		for _, row := range group {
			totals[row.Link] += row.Count
		}
	}
	links := make([]InviteLinkCount, 0, len(totals))
	for link, count := range totals {
		links = append(links, InviteLinkCount{Link: link, Count: count})
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].Count != links[j].Count {
			return links[i].Count > links[j].Count
		}
		return links[i].Link < links[j].Link
	})
	return links, nil
}
