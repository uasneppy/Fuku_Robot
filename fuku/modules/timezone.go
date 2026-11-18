package modules

import (
	"context"
	"html"
	"strings"
	"time"
	"unicode"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/timezone"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

// /timezone shows or changes the zone a group's times are shown in. Any administrator may view it;
// changing it needs can_change_info, as the Timezone settings page does. Both write through
// applyTimezone, the one write path of the setting.

// timezoneValueMax is the longest stored zone value, the column's size.
const timezoneValueMax = 64

// timezoneWrites is the write seam of applyTimezone, a variable only so a test can fail it.
var timezoneWrites = struct {
	Set    func(ctx context.Context, chatID int64, tz string, by int64) error
	Remove func(ctx context.Context, chatID int64) error
}{
	Set:    timezone.Set,
	Remove: timezone.Remove,
}

// timezoneOutcome is what applyTimezone did; its zero value is a stored zone.
type timezoneOutcome int

const (
	// timezoneApplied means the zone was stored.
	timezoneApplied timezoneOutcome = iota
	// timezoneReset means the chat's row was removed: the bot's default applies again.
	timezoneReset
	// timezoneInvalid means the input names no zone or offset; nothing was written.
	timezoneInvalid
	// timezoneSaveFailed means the write failed.
	timezoneSaveFailed
)

// timezoneApplyResult is the typed result of applyTimezone: the outcome, the stored value ("" after a
// reset) and the error of a failed write.
type timezoneApplyResult struct {
	Outcome timezoneOutcome
	Value   string
	Err     error
}

// timezoneTitleSegments capitalises every word of a zone name the way the IANA database writes it
// ("america/new_york" to "America/New_York").
func timezoneTitleSegments(raw string) string {
	out := []rune(strings.ToLower(raw))
	start := true
	for i, r := range out {
		if start {
			out[i] = unicode.ToUpper(r)
		}
		start = r == '/' || r == '_' || r == '-'
	}
	return string(out)
}

// normaliseTimezoneInput reads what a user typed: "default" (the bot's default), an offset such as +3,
// -5, +5:30 or UTC+2 (stored as UTC+hh:mm), or an IANA name in any letter case (stored as the database
// spells it; an old alias such as Europe/Kiev resolves too). ok is false for anything else.
func normaliseTimezoneInput(raw string) (value string, isDefault, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > timezoneValueMax {
		return "", false, false
	}
	if strings.EqualFold(s, "default") {
		return "", true, true
	}
	if offset, isOffset := parseUTCOffset(s); isOffset {
		return storedOffsetName(offset), false, true
	}
	if strings.EqualFold(s, "utc") || strings.EqualFold(s, "gmt") {
		return "UTC", false, true
	}
	for _, candidate := range []string{s, timezoneTitleSegments(s)} {
		if candidate == "Local" || !strings.Contains(candidate, "/") && candidate != "UTC" {
			continue
		}
		if _, err := time.LoadLocation(candidate); err == nil {
			return candidate, false, true
		}
	}
	return "", false, false
}

// applyTimezone stores a chat's zone from what a user typed or pressed: the one write path of
// /timezone and of the Timezone settings page. It judges no person's rights and sends nothing.
func applyTimezone(ctx context.Context, chatID, by int64, raw string) timezoneApplyResult {
	value, isDefault, ok := normaliseTimezoneInput(raw)
	if !ok {
		return timezoneApplyResult{Outcome: timezoneInvalid}
	}
	if isDefault {
		if err := timezoneWrites.Remove(ctx, chatID); err != nil {
			log.Errorf("[Timezone] the timezone of chat %d could not be reset: %v", chatID, err)
			return timezoneApplyResult{Outcome: timezoneSaveFailed, Err: err}
		}
		return timezoneApplyResult{Outcome: timezoneReset}
	}
	if err := timezoneWrites.Set(ctx, chatID, value, by); err != nil {
		log.Errorf("[Timezone] the timezone of chat %d could not be saved: %v", chatID, err)
		return timezoneApplyResult{Outcome: timezoneSaveFailed, Value: value, Err: err}
	}
	return timezoneApplyResult{Outcome: timezoneApplied, Value: value}
}

// timezoneDisplayName names a chat's zone for a reply: the stored value, or the default's name.
func timezoneDisplayName(stored string) string {
	if stored == "" {
		return chatTimezoneDefaultName()
	}
	return stored
}

// timezoneStatusText is /timezone's view: the zone, its label and the time now, whether it is the bot's
// default, then how to change it.
func timezoneStatusText(tr *i18n.Translator, stored string, loc *time.Location, now time.Time) string {
	params := i18n.TranslationParams{
		"zone":  html.EscapeString(timezoneDisplayName(stored)),
		"label": timezoneLabelAt(now, loc),
		"time":  now.In(loc).Format("15:04"),
	}
	lines := []string{settingsText(tr, "timezone_current", params)}
	if stored == "" {
		lines = append(lines, settingsText(tr, "timezone_is_default"))
	}
	lines = append(lines, "", settingsText(tr, "timezone_usage"))
	return strings.Join(lines, "\n")
}

var timezoneDesc = helpers.CommandDescriptor{
	Name:           "timezone",
	RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup(), helpers.RequireUserAdmin()},
}

// timezoneCommand is /timezone: with no argument it shows the zone; with one it changes it, which
// needs can_change_info.
func timezoneCommand(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	user := chat_status.RequireUser(b, ctx)
	if user == nil || chat == nil || msg == nil {
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	reqCtx := tracing.UpdateContext(ctx)
	args := ctx.Args()[1:]
	if len(args) == 0 {
		stored, err := chatTimezoneLookup(reqCtx, chat.Id)
		if err != nil {
			_, _ = msg.Reply(b, settingsText(tr, "settings_check_failed"), formatting.Shtml())
			return ext.EndGroups
		}
		loc := chatClock(reqCtx, chat.Id)
		_, err = msg.Reply(b, timezoneStatusText(tr, stored, loc, time.Now()), formatting.Shtml())
		if err != nil {
			log.Error(err)
		}
		return ext.EndGroups
	}
	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}
	result := applyTimezone(reqCtx, chat.Id, user.Id, strings.Join(args, " "))
	var text string
	switch result.Outcome {
	case timezoneInvalid:
		text = settingsText(tr, "timezone_invalid")
	case timezoneSaveFailed:
		text = settingsText(tr, "common_settings_save_failed")
	default:
		now := time.Now()
		loc, _ := parseStoredTimezone(result.Value)
		if loc == nil {
			loc = defaultChatLocation()
		}
		key := "timezone_set"
		if result.Outcome == timezoneReset {
			key = "timezone_reset"
		}
		text = settingsText(tr, key, i18n.TranslationParams{
			"zone":  html.EscapeString(timezoneDisplayName(result.Value)),
			"label": timezoneLabelAt(now, loc),
			"time":  now.In(loc).Format("15:04"),
		})
	}
	if _, err := msg.Reply(b, text, formatting.Shtml()); err != nil {
		log.Error(err)
	}
	return ext.EndGroups
}

// LoadTimezone registers /timezone. Its help is part of the Settings help.
func LoadTimezone(dispatcher *ext.Dispatcher) {
	helpers.WrapCommand(dispatcher, timezoneDesc, pipelineHandler(timezoneCommand))
}

func init() {
	RegisterLegacyModule("Timezone", 236, LoadTimezone)
	RegisterAnonymousAdminHandler("timezone", anonPipelineHandler(timezoneDesc, timezoneCommand))
}
