package modules

import (
	"context"
	stderrors "errors"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// deletionRulesLookup is the one read of a group's deletion rules, shared by the service
// watcher, the send sites, the settings page and the apply functions. It is the cached loader
// (the key is in skipLocal, so no replica serves it from process memory); it is a variable only
// so a test can make it fail.
var deletionRulesLookup = deletion.GetRules

// deletionFlagLookup reads the shared /cleanservice flag of a chat's greetings row. Its default
// is the reader that creates nothing and returns its error; it is a variable only so a test can
// make it fail.
var deletionFlagLookup = greetings.LoadSettingsContext

// deletionRuleWriter holds the repository writes of one rule. Each drops the rules cache key.
type deletionRuleWriter struct {
	Set    func(ctx context.Context, chatID int64, kind string, delay int) error
	Remove func(ctx context.Context, chatID int64, kind string) error
}

// deletionRuleWrites is the writer the apply functions call. It is a variable only so a test
// can make a write fail; no test alters a table to do it.
var deletionRuleWrites = deletionRuleWriter{
	Set:    deletion.SetRule,
	Remove: deletion.RemoveRule,
}

// errDeletionSharedKind is the refusal of applyDeleteRule for joined and left, which share the
// /cleanservice flag and are written only by applyCleanServiceRule.
var errDeletionSharedKind = stderrors.New("kind shares the clean service flag")

// deletionApplyTimeout bounds the read-back an apply function makes after its write.
const deletionApplyTimeout = 5 * time.Second

// deletionApplyOutcome says what an apply function did.
type deletionApplyOutcome int

const (
	// deletionApplied means the rule was stored.
	deletionApplied deletionApplyOutcome = iota
	// deletionUnchanged means the stored state already equals the request and nothing was written.
	deletionUnchanged
	// deletionRefused means the request named a kind or a delay the function does not write,
	// refused before any read or write.
	deletionRefused
	// deletionSaveFailed means the state could not be read or the write failed; the stored rule
	// is as it was.
	deletionSaveFailed
)

// deletionApplyResult is what an apply function returns: the outcome, the rules read back
// through the cached loader (nil when the call failed or the read-back did, in which case the
// caller reloads) and the error behind a refusal or a failure. applyCleanServiceRule also fills
// CleanService (the shared flag read back) and the effective state of the kind it was called
// for (Delay and On, as deletionEffectiveDelay reads it); all three are zero when Rules is nil.
type deletionApplyResult struct {
	Outcome      deletionApplyOutcome
	Rules        *deletion.Rules
	CleanService bool
	Delay        int
	On           bool
	Err          error
}

// deletionTarget is the absolute value an apply function stores: a delay, or no rule. It is
// never a delta, so a repeated press stores the same state (D-16).
type deletionTarget struct {
	Remove    bool
	DelaySecs int
}

// deletionRemove is the target that removes a rule.
var deletionRemove = deletionTarget{Remove: true}

// deletionSet is the target that stores a delay: 0 deletes at once, 1 to 86400 after that many seconds.
func deletionSet(delaySecs int) deletionTarget {
	return deletionTarget{DelaySecs: delaySecs}
}

// equalsStored reports whether the rules already hold the target for the kind.
func (t deletionTarget) equalsStored(kind string, rules *deletion.Rules) bool {
	stored, present := rules.Delays[kind]
	if t.Remove {
		return !present
	}
	return present && stored == t.DelaySecs
}

// applyDeleteRule is the one write path of every deletion rule except joined and left (those
// share the /cleanservice flag and have applyCleanServiceRule). It refuses an unknown kind, a
// delay the kind does not accept (deletion.ValidateRuleDelay: 0..86400 for a service or bot kind,
// the kind's own presets for a member kind) and the two shared kinds before any read or write, reads the stored
// state through the cached loader (a failed read is a save failure with no write), writes
// nothing when the stored state already equals the target, and otherwise writes through the
// repository, which drops the cache key, and reads the rules back under a timeout that outlives
// a cancelled request. It logs the chat ID and the kind class only.
func applyDeleteRule(ctx context.Context, chatID int64, kind string, target deletionTarget) deletionApplyResult {
	if err := deletionValidate(kind, target); err != nil {
		return deletionApplyResult{Outcome: deletionRefused,
			Err: errors.Wrapf(err, "refuse deletion rule of chat %d", chatID)}
	}
	stored, err := deletionRulesLookup(ctx, chatID)
	if err != nil {
		log.Errorf("[Deletion] read the rules of chat %d before a write: %v", chatID, err)
		return deletionApplyResult{Outcome: deletionSaveFailed, Err: err}
	}
	if target.equalsStored(kind, stored) {
		return deletionApplyResult{Outcome: deletionUnchanged, Rules: stored}
	}
	if target.Remove {
		err = deletionRuleWrites.Remove(ctx, chatID, kind)
	} else {
		err = deletionRuleWrites.Set(ctx, chatID, kind, target.DelaySecs)
	}
	if err != nil {
		log.Errorf("[Deletion] write a rule of chat %d: %v", chatID, err)
		return deletionApplyResult{Outcome: deletionSaveFailed, Err: err}
	}
	return deletionApplyResult{Outcome: deletionApplied, Rules: deletionReadBack(ctx, chatID)}
}

// deletionValidate refuses what applyDeleteRule never writes: an unknown kind, a kind that shares
// the /cleanservice flag and a delay the kind's rule does not accept (the order is the kind, the
// shared kind, the delay).
func deletionValidate(kind string, target deletionTarget) error {
	switch {
	case !deletion.ValidKind(kind):
		return deletion.ErrUnknownKind
	case deletion.SharesCleanServiceFlag(kind):
		return errDeletionSharedKind
	case target.Remove:
		return nil
	}
	return deletion.ValidateRuleDelay(kind, target.DelaySecs)
}

// deletionReadBack reads the rules after a write, through the cached loader, on a context that
// survives a cancelled request. A failed read is only logged, because the write succeeded: the
// caller gets nil and reloads.
func deletionReadBack(ctx context.Context, chatID int64) *deletion.Rules {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deletionApplyTimeout)
	defer cancel()
	rules, err := deletionRulesLookup(readCtx, chatID)
	if err != nil {
		log.Warnf("[Deletion] read back the rules of chat %d: %v", chatID, err)
		return nil
	}
	return rules
}

// applyCleanServiceRule is the one write path of the joined and left rows, which share the
// /cleanservice flag (D-02): Keep turns the flag off and removes both delay rows, Now turns it
// on and removes this kind's row, After N turns it on and stores this kind's row, all in one
// transaction (deletion.ApplyJoinLeftRule). It refuses a kind that is not joined or left, a mode
// that is not one of the three and a delay outside 1..86400 before any read or write, reads the
// effective state (the flag fresh, the rules through the cached loader; a failed read is a save
// failure with no write), writes nothing when the state already equals the request, and reads
// the effective state back after a write. Because both kinds share one flag, turning one on
// makes the other read Delete now unless it has its own row: that coupling is D-02, not a choice.
func applyCleanServiceRule(ctx context.Context, chatID int64, kind string, state deletion.JoinLeftState) deletionApplyResult {
	if err := deletionValidateJoinLeft(kind, state); err != nil {
		return deletionApplyResult{Outcome: deletionRefused,
			Err: errors.Wrapf(err, "refuse joined or left rule of chat %d", chatID)}
	}
	flag, rules, err := deletionJoinLeftRead(ctx, chatID)
	if err != nil {
		log.Errorf("[Deletion] read the joined and left state of chat %d before a write: %v", chatID, err)
		return deletionApplyResult{Outcome: deletionSaveFailed, Err: err}
	}
	if deletionJoinLeftHolds(kind, state, flag, rules) {
		return deletionJoinLeftResult(deletionUnchanged, kind, flag, rules)
	}
	if err := deletion.ApplyJoinLeftRule(ctx, chatID, kind, state); err != nil {
		log.Errorf("[Deletion] write the joined or left rule of chat %d: %v", chatID, err)
		return deletionApplyResult{Outcome: deletionSaveFailed, Err: err}
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deletionApplyTimeout)
	defer cancel()
	flag, rules, err = deletionJoinLeftRead(readCtx, chatID)
	if err != nil {
		log.Warnf("[Deletion] read back the joined and left state of chat %d: %v", chatID, err)
		return deletionApplyResult{Outcome: deletionApplied}
	}
	return deletionJoinLeftResult(deletionApplied, kind, flag, rules)
}

// deletionValidateJoinLeft refuses what applyCleanServiceRule never writes.
func deletionValidateJoinLeft(kind string, state deletion.JoinLeftState) error {
	switch {
	case !deletion.SharesCleanServiceFlag(kind):
		return deletion.ErrNotJoinLeft
	case state.Mode != deletion.JoinLeftKeep && state.Mode != deletion.JoinLeftNow && state.Mode != deletion.JoinLeftAfter:
		return deletion.ErrInvalidJoinLeftState
	case state.Mode == deletion.JoinLeftAfter && (state.DelaySecs < 1 || state.DelaySecs > deletion.MaxDelaySecs):
		return deletion.ErrDelayOutOfRange
	}
	return nil
}

// deletionJoinLeftRead reads the shared flag and the rules of a chat.
func deletionJoinLeftRead(ctx context.Context, chatID int64) (flag bool, rules *deletion.Rules, err error) {
	settings, err := deletionFlagLookup(ctx, chatID)
	if err != nil {
		return false, nil, err
	}
	rules, err = deletionRulesLookup(ctx, chatID)
	if err != nil {
		return false, nil, err
	}
	return settings.ShouldCleanService, rules, nil
}

// deletionJoinLeftHolds reports whether the stored state already is the requested one. Keep also
// needs both delay rows gone: with the flag off they mean nothing, but a stale one must not be
// left to come back at a later Now (D-26).
func deletionJoinLeftHolds(kind string, state deletion.JoinLeftState, flag bool, rules *deletion.Rules) bool {
	delay, on := deletionEffectiveDelay(kind, rules, flag)
	switch state.Mode {
	case deletion.JoinLeftKeep:
		_, joined := rules.Delays[deletion.KindJoined]
		_, left := rules.Delays[deletion.KindLeft]
		return !on && !joined && !left
	case deletion.JoinLeftNow:
		return on && delay == 0
	default:
		return on && delay == state.DelaySecs
	}
}

// deletionJoinLeftResult builds the result of applyCleanServiceRule from the state read.
func deletionJoinLeftResult(outcome deletionApplyOutcome, kind string, flag bool, rules *deletion.Rules) deletionApplyResult {
	delay, on := deletionEffectiveDelay(kind, rules, flag)
	return deletionApplyResult{Outcome: outcome, Rules: rules, CleanService: flag, Delay: delay, On: on}
}
