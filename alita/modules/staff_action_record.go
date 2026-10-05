package modules

import (
	"encoding/json"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// staffPriorState is the target's state in one group before a staff action, as one
// live getChatMember read reported it. It is what undo puts back (D-02). Status ""
// means the state was never captured, which is never undoable. Perms is filled only
// for a restricted target: restrictChatMember replaces the whole permission set, so
// only a stored set lets undo restore a partial restriction exactly.
type staffPriorState struct {
	Status   string
	IsMember bool
	// Until is the end of the ban or restriction as a Unix time; 0 is permanent.
	Until int64
	Perms *gotgbot.ChatPermissions
}

// staffPriorFromMember captures the state a live getChatMember answer reports.
func staffPriorFromMember(m gotgbot.MergedChatMember) staffPriorState {
	prior := staffPriorState{
		Status:   m.Status,
		IsMember: staffTargetStateFrom(m).IsMember,
		Until:    m.UntilDate,
	}
	if m.Status == gotgbot.ChatMemberStatusRestricted {
		prior.Perms = &gotgbot.ChatPermissions{
			CanSendMessages:       m.CanSendMessages,
			CanSendAudios:         m.CanSendAudios,
			CanSendDocuments:      m.CanSendDocuments,
			CanSendPhotos:         m.CanSendPhotos,
			CanSendVideos:         m.CanSendVideos,
			CanSendVideoNotes:     m.CanSendVideoNotes,
			CanSendVoiceNotes:     m.CanSendVoiceNotes,
			CanSendPolls:          m.CanSendPolls,
			CanSendOtherMessages:  m.CanSendOtherMessages,
			CanAddWebPagePreviews: m.CanAddWebPagePreviews,
			CanReactToMessages:    helpers.Ptr(m.CanReactToMessages),
			CanEditTag:            helpers.Ptr(m.CanEditTag),
			CanChangeInfo:         m.CanChangeInfo,
			CanInviteUsers:        m.CanInviteUsers,
			CanPinMessages:        m.CanPinMessages,
			CanManageTopics:       helpers.Ptr(m.CanManageTopics),
		}
	}
	return prior
}

// row converts the state to the columns the repository stores. The permission set
// is marshalled to JSON text, "" when there is none.
func (p staffPriorState) row() (staff.ActionPrior, error) {
	row := staff.ActionPrior{Status: p.Status, IsMember: p.IsMember, Until: p.Until}
	if p.Perms != nil {
		raw, err := json.Marshal(p.Perms)
		if err != nil {
			return staff.ActionPrior{}, fmt.Errorf("marshal prior permissions: %w", err)
		}
		row.Permissions = string(raw)
	}
	return row, nil
}

// staffPriorFromRow reads the state back from a group row.
func staffPriorFromRow(g models.StaffActionGroup) (staffPriorState, error) {
	prior := staffPriorState{Status: g.PriorStatus, IsMember: g.PriorIsMember, Until: g.PriorUntil}
	if g.PriorPermissions != "" {
		var perms gotgbot.ChatPermissions
		if err := json.Unmarshal([]byte(g.PriorPermissions), &perms); err != nil {
			return staffPriorState{}, fmt.Errorf("decode prior permissions of group %d: %w", g.GroupChatID, err)
		}
		prior.Perms = &perms
	}
	return prior, nil
}

// staffOutcomeName is the outcome as the database stores it.
func staffOutcomeName(o staffOutcome) string {
	switch o {
	case staffOutcomeDone:
		return models.StaffActionOutcomeDone
	case staffOutcomeSkipped:
		return models.StaffActionOutcomeSkipped
	case staffOutcomeFailed:
		return models.StaffActionOutcomeFailed
	}
	return models.StaffActionOutcomePending
}

// staffOutcomeFromName is staffOutcomeName's inverse. An unknown name reads as
// failed, like staffReasonOutcome's default, so a row is never shown as a success
// it did not earn.
func staffOutcomeFromName(s string) staffOutcome {
	switch s {
	case models.StaffActionOutcomePending:
		return staffOutcomePending
	case models.StaffActionOutcomeDone:
		return staffOutcomeDone
	case models.StaffActionOutcomeSkipped:
		return staffOutcomeSkipped
	}
	return staffOutcomeFailed
}

// staffResultRow converts a group's result to the repository's shape.
func staffResultRow(res staffGroupResult) staff.ActionGroupResult {
	return staff.ActionGroupResult{
		GroupChatID: res.Link.GroupChatID,
		Outcome:     staffOutcomeName(res.Outcome),
		Reason:      string(res.Reason),
		Detail:      res.Detail,
	}
}

// newStaffActionRecord builds the audit record of a confirmed card: the parent row
// from the card and one pending row per linked group, in link order. untilDate is
// the one end date sent to every group, and msgID the card message that becomes the
// summary.
func newStaffActionRecord(
	card *staffActionCard,
	staffChatID, msgID, untilDate int64,
	links []models.StaffGroupLink,
) (*models.StaffAction, []models.StaffActionGroup) {
	action := &models.StaffAction{
		StaffChatID:    staffChatID,
		IssuerUserID:   card.Issuer,
		IssuerName:     card.IssuerName,
		TargetUserID:   card.Target,
		TargetName:     card.TargetName,
		Action:         string(card.Kind),
		Reason:         card.Reason,
		DurationSec:    card.DurationSec,
		DurationAmount: card.DurationAmount,
		DurationUnit:   card.DurationUnit,
		OverLimit:      card.OverLimit,
		UntilDate:      untilDate,
		GroupCount:     len(links),
		SummaryChatID:  staffChatID,
		SummaryMsgID:   msgID,
	}
	groups := make([]models.StaffActionGroup, len(links))
	for i, link := range links {
		groups[i] = models.StaffActionGroup{
			Seq:         i,
			GroupChatID: link.GroupChatID,
			GroupTitle:  link.GroupTitle,
			Outcome:     models.StaffActionOutcomePending,
		}
	}
	return action, groups
}

// Test seams: tests replace these to prove the fail-closed paths with a real
// database. Production never reassigns them.
var (
	// staffCreateActionRecord stores the audit record at Confirm.
	staffCreateActionRecord = staff.CreateAction
	// staffSavePrior stores a group's prior state before its Telegram write.
	staffSavePrior = staff.SavePrior
)

// saveStaffGroupResult stores one group's result as the group finishes. It is best
// effort: the finalize write at the end of the run is authoritative, so a failure is
// logged and the run goes on.
func saveStaffGroupResult(card *staffActionCard, res staffGroupResult) {
	if card.ActionID == 0 {
		return
	}
	if err := staff.SaveGroupResult(card.ActionID, staffResultRow(res)); err != nil {
		log.Errorf("[StaffActions] save result of group %d for action %d: %v", res.Link.GroupChatID, card.ActionID, err)
	}
}

// finalizeStaffActionRecord writes every group's final result and the finish time in
// one transaction, retrying once after an error. It reports whether the record was
// finalized.
func finalizeStaffActionRecord(card *staffActionCard, results []staffGroupResult) bool {
	if card.ActionID == 0 {
		return false
	}
	rows := make([]staff.ActionGroupResult, len(results))
	for i, res := range results {
		rows[i] = staffResultRow(res)
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if err = staff.FinalizeAction(card.ActionID, rows); err == nil {
			return true
		}
	}
	log.Errorf("[StaffActions] finalize record of action %d: %v", card.ActionID, err)
	return false
}
