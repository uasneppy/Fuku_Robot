package lockdown

// Lockdown alert state (chat_lockdowns.alert_started_at, chat_lockdown_alerts) is
// operational state shared by every replica's worker. It is always read fresh from the
// database and never cached, so no cache key is read here and none needs
// invalidation. Writes use db.DB without a request context, so a cancelled context
// never loses a record write, and every claim or move is one conditional update whose
// RowsAffected == 1 says which caller won.

import (
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)

// AlertPlan is what one lockdown's alert decision creates: one copy row per
// destination. Slots must carry LockdownID, ChatID, Kind, Seq, Dest and DestChatID;
// BeginAlert makes them pending.
type AlertPlan struct {
	Slots []models.ChatLockdownAlert
}

// BeginAlert decides the alert of a lockdown exactly once. One conditional update sets
// alert_started_at while the lockdown is active, its lock confirmed and no alert was
// decided yet; the caller whose update matched (RowsAffected == 1) creates every slot
// of the plan in the same transaction, so a lockdown is never decided without its
// copies and never decided twice, on any replica. false with no error means another
// caller decided it first (or the lockdown is not eligible) and nothing was written. A
// plan with no slots still stamps the lockdown, so a destination that appears later
// never gets a stale alert.
func BeginAlert(lockdownID uint, plan AlertPlan) (bool, error) {
	won := false
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		won = false
		stamp := now()
		result := tx.Model(&models.ChatLockdown{}).
			Where("id = ? AND state = ? AND locked_at IS NOT NULL AND alert_started_at IS NULL",
				lockdownID, models.LockdownStateActive).
			Updates(map[string]any{"alert_started_at": stamp, "updated_at": stamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		for i := range plan.Slots {
			slot := plan.Slots[i]
			slot.ID = 0
			slot.LockdownID = lockdownID
			slot.State = models.AlertStatePending
			slot.CreatedAt = stamp
			slot.UpdatedAt = stamp
			if err := tx.Create(&slot).Error; err != nil {
				return err
			}
		}
		won = true
		return nil
	})
	if err != nil {
		log.Errorf("[Lockdown] BeginAlert: %v", err)
		return false, alitaerrors.Wrapf(err, "begin alert of lockdown %d", lockdownID)
	}
	return won, nil
}

// ListAlertsToPlanFresh reads up to limit active lockdowns whose lock Telegram
// confirmed and whose alert was not decided yet, oldest ID first, straight from the
// database.
func ListAlertsToPlanFresh(limit int) ([]models.ChatLockdown, error) {
	var rows []models.ChatLockdown
	err := db.DB.
		Where("state = ? AND locked_at IS NOT NULL AND alert_started_at IS NULL", models.LockdownStateActive).
		Order("id").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListAlertsToPlanFresh: %v", err)
		return nil, alitaerrors.Wrap(err, "list lockdowns whose alert is not decided")
	}
	return rows, nil
}

// ListPendingAlertSlotsFresh reads up to limit pending copy rows of active lockdowns,
// oldest ID first, straight from the database. A lockdown that is lifting or lifted
// has no pending copy listed, so a copy planned before a lift is never sent after it.
func ListPendingAlertSlotsFresh(limit int) ([]models.ChatLockdownAlert, error) {
	var rows []models.ChatLockdownAlert
	err := db.DB.
		Where("state = ? AND lockdown_id IN (SELECT id FROM chat_lockdowns WHERE state = ?)",
			models.AlertStatePending, models.LockdownStateActive).
		Order("id").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListPendingAlertSlotsFresh: %v", err)
		return nil, alitaerrors.Wrap(err, "list pending alert copies")
	}
	return rows, nil
}

// GetAlertSlotFresh reads one copy row by its ID straight from the database. It
// returns (nil, nil) when there is no such row.
func GetAlertSlotFresh(id uint) (*models.ChatLockdownAlert, error) {
	var row models.ChatLockdownAlert
	err := db.DB.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Lockdown] GetAlertSlotFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get alert copy %d", id)
	}
	return &row, nil
}

// ClaimAlertSlot moves a copy from pending to posting and stamps claimed_at, in one
// conditional update, and reports whether this call won the claim.
func ClaimAlertSlot(id uint) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("id = ? AND state = ?", id, models.AlertStatePending).
		Updates(map[string]any{"state": models.AlertStatePosting, "claimed_at": stamp, "updated_at": stamp})
	if result.Error != nil {
		log.Errorf("[Lockdown] ClaimAlertSlot: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "claim alert copy %d", id)
	}
	return result.RowsAffected == 1, nil
}

// RecordAlertPosted records a sent copy: it moves a posting row to posted and stores
// the message ID, the exact text sent and the link shown in it, with the attempt
// counter back at zero and the claim and detail cleared. It reports whether this call
// moved the row.
func RecordAlertPosted(id uint, messageID int64, body, link string) (bool, error) {
	result := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("id = ? AND state = ?", id, models.AlertStatePosting).
		Updates(map[string]any{
			"state":      models.AlertStatePosted,
			"message_id": messageID,
			"body":       body,
			"link":       link,
			"attempts":   0,
			"claimed_at": nil,
			"detail":     "",
			"updated_at": now(),
		})
	if result.Error != nil {
		log.Errorf("[Lockdown] RecordAlertPosted: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "record posted alert copy %d", id)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseAlertSlot gives a claimed copy back: posting to pending, claim cleared, detail
// replaced. countAttempt adds one to the attempt counter. It reports whether this call
// moved the row.
func ReleaseAlertSlot(id uint, detail string, countAttempt bool) (bool, error) {
	updates := map[string]any{
		"state":      models.AlertStatePending,
		"claimed_at": nil,
		"detail":     detail,
		"updated_at": now(),
	}
	if countAttempt {
		updates["attempts"] = gorm.Expr("attempts + 1")
	}
	result := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("id = ? AND state = ?", id, models.AlertStatePosting).
		Updates(updates)
	if result.Error != nil {
		log.Errorf("[Lockdown] ReleaseAlertSlot: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "release alert copy %d", id)
	}
	return result.RowsAffected == 1, nil
}

// FailAlertSlot ends a claimed copy for good: posting to failed, claim cleared, detail
// replaced, one attempt counted. It reports whether this call moved the row.
func FailAlertSlot(id uint, detail string) (bool, error) {
	result := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("id = ? AND state = ?", id, models.AlertStatePosting).
		Updates(map[string]any{
			"state":      models.AlertStateFailed,
			"claimed_at": nil,
			"detail":     detail,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": now(),
		})
	if result.Error != nil {
		log.Errorf("[Lockdown] FailAlertSlot: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "fail alert copy %d", id)
	}
	return result.RowsAffected == 1, nil
}

// ListAlertSlotsToCloseFresh reads up to limit posted copies that still need closing:
// not closed, not claimed, and belonging to a lockdown that is no longer active (lifting
// or lifted). Newest ID first, so the alerts staff see last are fixed first. It reads
// straight from the database.
func ListAlertSlotsToCloseFresh(limit int) ([]models.ChatLockdownAlert, error) {
	var rows []models.ChatLockdownAlert
	err := db.DB.
		Where("state = ? AND closed_at IS NULL AND claimed_at IS NULL AND lockdown_id IN (SELECT id FROM chat_lockdowns WHERE state <> ?)",
			models.AlertStatePosted, models.LockdownStateActive).
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		log.Errorf("[Lockdown] ListAlertSlotsToCloseFresh: %v", err)
		return nil, alitaerrors.Wrap(err, "list alert copies to close")
	}
	return rows, nil
}

// ClaimAlertClose claims a posted, open, unclaimed copy for closing by stamping
// claimed_at, in one conditional update, and reports whether this call won the claim. A
// posted copy has no other claim, so the column is reused.
func ClaimAlertClose(id uint) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("id = ? AND state = ? AND closed_at IS NULL AND claimed_at IS NULL", id, models.AlertStatePosted).
		Updates(map[string]any{"claimed_at": stamp, "updated_at": stamp})
	if result.Error != nil {
		log.Errorf("[Lockdown] ClaimAlertClose: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "claim alert copy %d for closing", id)
	}
	return result.RowsAffected == 1, nil
}

// RecordAlertClosed records a closed copy: closed_at is stamped, the claim and the live
// buttons are cleared and detail is stored. It reports whether this call closed the row;
// a copy closed already is left alone.
func RecordAlertClosed(id uint, detail string) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("id = ? AND closed_at IS NULL", id).
		Updates(map[string]any{
			"closed_at":    stamp,
			"claimed_at":   nil,
			"buttons_live": false,
			"edit_pending": false,
			"detail":       detail,
			"updated_at":   stamp,
		})
	if result.Error != nil {
		log.Errorf("[Lockdown] RecordAlertClosed: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "record closed alert copy %d", id)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseStaleAlertClaims gives back every alert copy claim a worker made before the
// cut-off and never finished, because the worker stopped (a restart, a crash, a call
// that never came back). No attempt is counted, so any replica tries the copy again:
//
//   - a posting copy goes back to pending (its send is repeated, which can post a second
//     copy when the first send reached Telegram; a duplicate alert is accepted over a
//     lost one);
//   - a posted, open copy with a close claim loses the claim and is closed again.
//
// The two updates run in one transaction and match disjoint rows. It returns how many
// rows it released.
func ReleaseStaleAlertClaims(before time.Time) (int64, error) {
	cutoff := before.UTC()
	var released int64
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		released = 0
		sending := tx.Model(&models.ChatLockdownAlert{}).
			Where("state = ? AND claimed_at < ?", models.AlertStatePosting, cutoff).
			Updates(map[string]any{"state": models.AlertStatePending, "claimed_at": nil, "updated_at": now()})
		if sending.Error != nil {
			return sending.Error
		}
		closing := tx.Model(&models.ChatLockdownAlert{}).
			Where("state = ? AND closed_at IS NULL AND claimed_at < ?", models.AlertStatePosted, cutoff).
			Updates(map[string]any{"claimed_at": nil, "updated_at": now()})
		if closing.Error != nil {
			return closing.Error
		}
		released = sending.RowsAffected + closing.RowsAffected
		return nil
	})
	if err != nil {
		log.Errorf("[Lockdown] ReleaseStaleAlertClaims: %v", err)
		return 0, alitaerrors.Wrap(err, "release stale alert copy claims")
	}
	return released, nil
}

// ReleaseAlertClose gives a claimed, still open copy back for another try: the claim is
// cleared, and countAttempt adds one to the attempt counter. It reports whether this
// call moved the row.
func ReleaseAlertClose(id uint, countAttempt bool) (bool, error) {
	updates := map[string]any{
		"claimed_at": nil,
		"updated_at": now(),
	}
	if countAttempt {
		updates["attempts"] = gorm.Expr("attempts + 1")
	}
	result := db.DB.Model(&models.ChatLockdownAlert{}).
		Where("id = ? AND state = ? AND closed_at IS NULL AND claimed_at IS NOT NULL", id, models.AlertStatePosted).
		Updates(updates)
	if result.Error != nil {
		log.Errorf("[Lockdown] ReleaseAlertClose: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "release alert copy %d from closing", id)
	}
	return result.RowsAffected == 1, nil
}
