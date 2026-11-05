package modules

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// memberReqSettingsLookup is the one read of a group's requirement settings, shared by the
// settings page, the Main status line and, later, the message watcher (D-06). It is the cached
// loader, so every reader sees the same value; it is a variable only so a test can make it fail.
var memberReqSettingsLookup = memberreq.GetSettings

// memberReqWrites holds the repository writes of the two switches. It is a variable only so a test
// can make a write fail.
var memberReqWrites = struct {
	CheckOnJoin    func(ctx context.Context, chatID int64, on bool) error
	DeleteMessages func(ctx context.Context, chatID int64, on bool) error
	Action         func(ctx context.Context, chatID int64, rule memberreq.Rule, action int) error
}{
	CheckOnJoin:    memberreq.SetCheckOnJoin,
	DeleteMessages: memberreq.SetDeleteMessages,
	Action:         memberreq.SetRuleAction,
}

// memberReqApplyOutcome is what an apply function did.
type memberReqApplyOutcome int

const (
	// memberReqApplied means the write succeeded.
	memberReqApplied memberReqApplyOutcome = iota
	// memberReqSaveFailed means the write failed; the stored value is unchanged.
	memberReqSaveFailed
)

// memberReqApplyResult is the typed result of an apply function: the outcome, the locale key of
// the notice a caller shows (empty when none), the row read back past every cache (nil when the
// read-back failed, in which case the caller reloads) and the write's error.
type memberReqApplyResult struct {
	Outcome memberReqApplyOutcome
	Key     string
	Row     *models.ChatRequirements
	Err     error
}

// finishMemberReqApply turns the error of a repository write into the result: it reads the row
// back fresh, after a failure too, so the page draws what is really stored.
func finishMemberReqApply(ctx context.Context, chatID int64, what string, writeErr error) memberReqApplyResult {
	result := memberReqApplyResult{Outcome: memberReqApplied}
	if writeErr != nil {
		result.Outcome = memberReqSaveFailed
		result.Key = "common_settings_save_failed"
		result.Err = errors.Wrapf(writeErr, "write member requirement %s of chat %d", what, chatID)
	}
	row, err := memberreq.GetSettingsFresh(ctx, chatID)
	if err != nil {
		log.Warnf("[MemberReq] could not read back %s of chat %d: %v", what, chatID, err)
		return result
	}
	result.Row = row
	return result
}

// applyMemberReqJoin is the one write path of the Check on join switch: the column-targeted
// write (a Go false is written), the dropped cache key (the repository does it) and the row read
// back. There is no command for it (D-17), so a drift test and an authority test stand in for a
// parity test. The value is the absolute target, so repeating it writes the same value again.
func applyMemberReqJoin(ctx context.Context, chatID int64, on bool) memberReqApplyResult {
	return finishMemberReqApply(ctx, chatID, "check on join", memberReqWrites.CheckOnJoin(ctx, chatID, on))
}

// applyMemberReqDelete is the one write path of the Delete messages switch, as applyMemberReqJoin
// is of Check on join.
func applyMemberReqDelete(ctx context.Context, chatID int64, on bool) memberReqApplyResult {
	return finishMemberReqApply(ctx, chatID, "delete messages", memberReqWrites.DeleteMessages(ctx, chatID, on))
}
