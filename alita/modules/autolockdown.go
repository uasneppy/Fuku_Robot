package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/raiddetection"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// The three triggers /autolockdown can change.
const (
	autoTriggerJoins = "joins"
	autoTriggerFlood = "flood"
	autoTriggerAI    = "ai"
)

var (
	// errAutoLockdownUsage is returned for any argument list outside the grammar.
	errAutoLockdownUsage = errors.New("autolockdown: usage")
	// errAutoLockdownRange is returned for a number outside the stored threshold range.
	errAutoLockdownRange = errors.New("autolockdown: threshold out of range")
)

// autolockdownDesc is deliberately not Disableable: a chat cannot switch off its raid
// tool. Its authority is a live getChatMember of the sender, never the admin cache.
var autolockdownDesc = helpers.CommandDescriptor{
	Name:           "autolockdown",
	RequiredChecks: []helpers.CheckFunc{requireLockdownGroup(), requireAutoLockdownAuthority()},
}

// requireAutoLockdownAuthority is one command with two authorities (D-10). With no
// arguments the command only shows the settings, so any live creator or administrator
// may run it. With arguments it changes them, so it needs the creator or an
// administrator with can_restrict_members. Both are the live check of
// requireLockdownAuthority, which also sends an anonymous admin to the proof first;
// after the proof the arguments are the cached command's, so the same choice is made
// for the tapper.
func requireAutoLockdownAuthority() helpers.CheckFunc {
	view := requireLockdownAuthority(false)
	change := requireLockdownAuthority(true)
	return func(c *helpers.CommandContext) bool {
		if c.Ctx != nil && len(lockdownCommandArgs(c.Ctx)) == 0 {
			return view(c)
		}
		return change(c)
	}
}

// autoLockdownChange is one parsed /autolockdown change. A zero Trigger means no change
// was asked for: the command shows the settings.
type autoLockdownChange struct {
	// Trigger is autoTriggerJoins, autoTriggerFlood or autoTriggerAI.
	Trigger string
	// Enable is the new switch when SetThreshold is false.
	Enable bool
	// SetThreshold says the change sets Threshold and turns the trigger on.
	SetThreshold bool
	// Threshold is the new threshold when SetThreshold is true.
	Threshold int
}

// parseAutoLockdownArgs reads the words after /autolockdown. No words is the status
// (a zero change). Otherwise exactly two words: joins, flood or ai in any case, then on, off
// or a whole number, which sets the threshold and turns the trigger on (D-21). Anything
// else is errAutoLockdownUsage, and a number outside MinThreshold..MaxThreshold is
// errAutoLockdownRange (D-18).
func parseAutoLockdownArgs(args []string) (autoLockdownChange, error) {
	if len(args) == 0 {
		return autoLockdownChange{}, nil
	}
	if len(args) != 2 {
		return autoLockdownChange{}, errAutoLockdownUsage
	}
	var change autoLockdownChange
	switch strings.ToLower(args[0]) {
	case autoTriggerJoins:
		change.Trigger = autoTriggerJoins
	case autoTriggerFlood:
		change.Trigger = autoTriggerFlood
	case autoTriggerAI:
		change.Trigger = autoTriggerAI
	default:
		return autoLockdownChange{}, errAutoLockdownUsage
	}
	switch word := strings.ToLower(args[1]); word {
	case "on":
		change.Enable = true
	case "off":
		change.Enable = false
	default:
		if !autoIsWholeNumber(word) {
			return autoLockdownChange{}, errAutoLockdownUsage
		}
		n, err := strconv.ParseInt(word, 10, 64)
		if err != nil || n < raiddetection.MinThreshold || n > raiddetection.MaxThreshold {
			// A number too long for 64 bits is a number, and out of range.
			return autoLockdownChange{}, errAutoLockdownRange
		}
		change.SetThreshold = true
		change.Threshold = int(n)
		change.Enable = true
	}
	return change, nil
}

// autoIsWholeNumber reports whether word is ASCII digits with an optional leading minus.
func autoIsWholeNumber(word string) bool {
	digits := strings.TrimPrefix(word, "-")
	if digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// autolockdownWrite saves one change through the matching repository function. It is a
// variable only so a test can make the save fail; production code never reassigns it.
var autolockdownWrite = writeAutoLockdown

// writeAutoLockdown calls the repository write for the change's trigger and kind.
func writeAutoLockdown(chatID int64, change autoLockdownChange) error {
	switch change.Trigger {
	case autoTriggerJoins:
		if change.SetThreshold {
			return raiddetection.SetJoinThreshold(chatID, change.Threshold)
		}
		return raiddetection.SetJoinEnabled(chatID, change.Enable)
	case autoTriggerFlood:
		if change.SetThreshold {
			return raiddetection.SetFloodThreshold(chatID, change.Threshold)
		}
		return raiddetection.SetFloodEnabled(chatID, change.Enable)
	case autoTriggerAI:
		if change.SetThreshold {
			return raiddetection.SetAIThreshold(chatID, change.Threshold)
		}
		return raiddetection.SetAIEnabled(chatID, change.Enable)
	}
	return fmt.Errorf("autolockdown: unknown trigger %q", change.Trigger)
}

// autolockdownTriggerLine is the status line of one trigger: the on text when it is on,
// the off text when it is not, each with the threshold.
func autolockdownTriggerLine(tr *i18n.Translator, enabled bool, threshold int, onKey, offKey string) string {
	key := offKey
	if enabled {
		key = onKey
	}
	return lockdownText(tr, key, i18n.TranslationParams{"threshold": threshold})
}

// autolockdownStatusText is the settings a group has: a title, one line per trigger
// (join surge, flood and AI spam, each on or off with its threshold), the notice that AI
// checks are not set up when the server has neither the TypeSafe nor the Gemini key
// (raidAIConfigured, D-14), and the hint on how to change them.
func autolockdownStatusText(tr *i18n.Translator, s *models.ChatRaidDetection) string {
	lines := []string{
		lockdownText(tr, "lockdown_auto_title"),
		autolockdownTriggerLine(tr, s.JoinEnabled, s.JoinThreshold, "lockdown_auto_joins_on", "lockdown_auto_joins_off"),
		autolockdownTriggerLine(tr, s.FloodEnabled, s.FloodThreshold, "lockdown_auto_flood_on", "lockdown_auto_flood_off"),
		autolockdownTriggerLine(tr, s.AIEnabled, s.AIThreshold, "lockdown_auto_ai_on", "lockdown_auto_ai_off"),
	}
	if !raidAIConfigured() {
		lines = append(lines, lockdownText(tr, "lockdown_auto_ai_unavailable"))
	}
	return lockdownJoinLines(append(lines, lockdownText(tr, "lockdown_auto_hint")))
}

// autolockdown shows or changes the automatic lockdown triggers of the group. The
// authority was judged by requireAutoLockdownAuthority before it runs.
func (m moduleStruct) autolockdown(b *gotgbot.Bot, ctx *ext.Context) error {
	return autolockdownApply(b, ctx, lockdownCommandArgs(ctx))
}

// autolockdownApply is the whole command body for the words that followed the command,
// so another command can reuse it with words of its own (the /autoantiraid alias
// prepends joins). It saves nothing for a chat that is not a supergroup (D-21), for
// words outside the grammar, or when the save fails, and says so. It always ends the
// group.
func autolockdownApply(b *gotgbot.Bot, ctx *ext.Context, args []string) error {
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	if chat == nil || msg == nil {
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	reply := func(text string) error {
		replyLockdown(b, msg, text)
		return ext.EndGroups
	}

	// A basic group cannot be locked, so there is nothing to tune. applyAutoLockdown
	// refuses it too; the check stays here so the parse errors and the status of a
	// basic group keep the one reply they always had.
	if chat.Type != "supergroup" {
		return reply(lockdownText(tr, "lockdown_basic_group"))
	}

	change, err := parseAutoLockdownArgs(args)
	switch {
	case errors.Is(err, errAutoLockdownRange):
		return reply(lockdownText(tr, "lockdown_auto_range", i18n.TranslationParams{
			"min": raiddetection.MinThreshold,
			"max": raiddetection.MaxThreshold,
		}))
	case err != nil:
		return reply(lockdownText(tr, "lockdown_auto_usage"))
	}

	if change.Trigger == "" {
		settings, readErr := autolockdownReadSettings(chat.Id)
		if readErr != nil {
			log.Errorf("[Lockdown] automatic lockdown settings of chat %d: %v", chat.Id, readErr)
			return reply(lockdownText(tr, "lockdown_auto_read_failed"))
		}
		return reply(autolockdownStatusText(tr, settings))
	}

	result := applyAutoLockdown(chat, change)
	if refusal := autolockdownApplyText(tr, result); refusal != "" {
		return reply(refusal)
	}

	saved := lockdownText(tr, "lockdown_auto_saved")
	if result.ReadErr != nil {
		// The change is saved; only the readback failed.
		return reply(saved)
	}
	return reply(lockdownJoinLines([]string{saved, autolockdownStatusText(tr, result.Settings)}))
}

// autoLockdownApplyOutcome is how one trigger change ended.
type autoLockdownApplyOutcome int

const (
	// autoApplySaved means the change was written. Settings holds the row read back, or
	// ReadErr says why it could not be read.
	autoApplySaved autoLockdownApplyOutcome = iota
	// autoApplyBasicGroup means the chat is not a supergroup, so nothing was written.
	autoApplyBasicGroup
	// autoApplyRange means a threshold outside raiddetection.MinThreshold..MaxThreshold,
	// so nothing was written.
	autoApplyRange
	// autoApplySaveFailed means the write failed; Err holds the cause and nothing is
	// known to have changed.
	autoApplySaveFailed
)

// autoLockdownApplyResult is what applyAutoLockdown reports.
type autoLockdownApplyResult struct {
	Outcome autoLockdownApplyOutcome
	// Settings is the chat's settings read back after a save, nil when ReadErr is set
	// or the outcome is not autoApplySaved.
	Settings *models.ChatRaidDetection
	// Err is the write error of autoApplySaveFailed.
	Err error
	// ReadErr is the error of the readback after a successful save.
	ReadErr error
}

// applyAutoLockdown is the one write path of the automatic lockdown triggers, shared by
// /autolockdown, /autoantiraid and the settings menu. It refuses a chat that is not a
// supergroup (D-21), writes through the autolockdownWrite seam (a threshold also turns
// its trigger on) and reads the settings back under raidSettingsTimeout. The caller has
// already judged the actor's authority.
func applyAutoLockdown(chat *gotgbot.Chat, change autoLockdownChange) autoLockdownApplyResult {
	if chat == nil || chat.Type != "supergroup" {
		return autoLockdownApplyResult{Outcome: autoApplyBasicGroup}
	}
	if writeErr := autolockdownWrite(chat.Id, change); writeErr != nil {
		if errors.Is(writeErr, raiddetection.ErrThresholdOutOfRange) {
			return autoLockdownApplyResult{Outcome: autoApplyRange}
		}
		log.Errorf("[Lockdown] saving automatic lockdown change for chat %d: %v", chat.Id, writeErr)
		return autoLockdownApplyResult{Outcome: autoApplySaveFailed, Err: writeErr}
	}
	settings, readErr := autolockdownReadSettings(chat.Id)
	if readErr != nil {
		log.Warnf("[Lockdown] automatic lockdown settings of chat %d after the save: %v", chat.Id, readErr)
		return autoLockdownApplyResult{Outcome: autoApplySaved, ReadErr: readErr}
	}
	return autoLockdownApplyResult{Outcome: autoApplySaved, Settings: settings}
}

// autolockdownApplyText is the reply for a refused or failed change, exactly as
// /autolockdown has always sent it, and "" for a saved one.
func autolockdownApplyText(tr *i18n.Translator, r autoLockdownApplyResult) string {
	switch r.Outcome {
	case autoApplyBasicGroup:
		return lockdownText(tr, "lockdown_basic_group")
	case autoApplyRange:
		return lockdownText(tr, "lockdown_auto_range", i18n.TranslationParams{
			"min": raiddetection.MinThreshold,
			"max": raiddetection.MaxThreshold,
		})
	case autoApplySaveFailed:
		return lockdownText(tr, "lockdown_auto_save_failed")
	}
	return ""
}

// autolockdownReadSettings reads the chat's raid detection settings under
// raidSettingsTimeout, so a stuck cache or database ends the command with its "could not
// read" reply instead of holding the handler.
func autolockdownReadSettings(chatID int64) (*models.ChatRaidDetection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), raidSettingsTimeout)
	defer cancel()
	return raidSettingsLookup(ctx, chatID)
}
