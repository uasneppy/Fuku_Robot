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

// The two triggers /autolockdown can change.
const (
	autoTriggerJoins = "joins"
	autoTriggerFlood = "flood"
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
	// Trigger is autoTriggerJoins or autoTriggerFlood.
	Trigger string
	// Enable is the new switch when SetThreshold is false.
	Enable bool
	// SetThreshold says the change sets Threshold and turns the trigger on.
	SetThreshold bool
	// Threshold is the new threshold when SetThreshold is true.
	Threshold int
}

// parseAutoLockdownArgs reads the words after /autolockdown. No words is the status
// (a zero change). Otherwise exactly two words: joins or flood in any case, then on, off
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
// (on or off and its threshold) and the hint on how to change them.
func autolockdownStatusText(tr *i18n.Translator, s *models.ChatRaidDetection) string {
	return lockdownJoinLines([]string{
		lockdownText(tr, "lockdown_auto_title"),
		autolockdownTriggerLine(tr, s.JoinEnabled, s.JoinThreshold, "lockdown_auto_joins_on", "lockdown_auto_joins_off"),
		autolockdownTriggerLine(tr, s.FloodEnabled, s.FloodThreshold, "lockdown_auto_flood_on", "lockdown_auto_flood_off"),
		lockdownText(tr, "lockdown_auto_hint"),
	})
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

	// A basic group cannot be locked, so there is nothing to tune.
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
		settings, readErr := raiddetection.GetSettings(context.Background(), chat.Id)
		if readErr != nil {
			log.Errorf("[Lockdown] automatic lockdown settings of chat %d: %v", chat.Id, readErr)
			return reply(lockdownText(tr, "lockdown_auto_read_failed"))
		}
		return reply(autolockdownStatusText(tr, settings))
	}

	if writeErr := autolockdownWrite(chat.Id, change); writeErr != nil {
		if errors.Is(writeErr, raiddetection.ErrThresholdOutOfRange) {
			return reply(lockdownText(tr, "lockdown_auto_range", i18n.TranslationParams{
				"min": raiddetection.MinThreshold,
				"max": raiddetection.MaxThreshold,
			}))
		}
		log.Errorf("[Lockdown] saving automatic lockdown change for chat %d: %v", chat.Id, writeErr)
		return reply(lockdownText(tr, "lockdown_auto_save_failed"))
	}

	saved := lockdownText(tr, "lockdown_auto_saved")
	settings, readErr := raiddetection.GetSettings(context.Background(), chat.Id)
	if readErr != nil {
		// The change is saved; only the readback failed.
		log.Warnf("[Lockdown] automatic lockdown settings of chat %d after the save: %v", chat.Id, readErr)
		return reply(saved)
	}
	return reply(lockdownJoinLines([]string{saved, autolockdownStatusText(tr, settings)}))
}
