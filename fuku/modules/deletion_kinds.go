package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// deletionRow is one row of the scheduled deletion tables: the code the settings page puts in a
// button, the kind a rule and a queue row carry, the locale keys of the page, the delay presets
// the choice rows offer and, for a service row, the test that decides whether a message belongs
// to it. One type serves both tables so the page draws a service row and a bot row with one
// renderer, and nothing branches on a row's code or kind: the differences are fields.
type deletionRow struct {
	// Code is two letters. The first is the section ('s' service, 'b' bot).
	Code string
	// Kind is the deletion.Kind constant a rule and a queue row carry.
	Kind    string
	NameKey string
	DescKey string
	// HintKey is the locale key of a hint the row's screen shows, empty for none.
	HintKey string
	// Presets are the delays in seconds the choice rows offer, ascending. The "no rule" choice is
	// implicit and not in the list.
	Presets []int
	// Match reports whether a message is one this row deletes. It is nil on a bot row, which
	// matches no incoming update: the bot's own messages are scheduled where they are sent.
	Match func(*gotgbot.Message) bool
}

// deletionServicePresets are the delays a service row offers: 0 deletes at once (D-04).
var deletionServicePresets = []int{0, 10, 60, 600, 3600, 21600, 86400}

// deletionBotPresets are the delays a bot row offers. A bot message is deleted after a wait,
// never at once, so 0 and 10 seconds are not offered (D-04).
var deletionBotPresets = []int{60, 600, 3600, 21600, 86400}

// deletionServiceRows are the service message rows in page order. The creation and migrate
// types, the creation of a forum topic and the giveaway post itself are in no predicate: they
// are never deleted and never offered (D-21).
var deletionServiceRows = []deletionRow{
	{
		Code: "sn", Kind: deletion.KindPinned,
		NameKey: "settings_del_svc_pinned", DescKey: "settings_del_desc_pinned",
		Presets: deletionServicePresets,
		Match:   func(m *gotgbot.Message) bool { return m.PinnedMessage != nil },
	},
}

// deletionBotRows are the five kinds of the bot's own messages that can be scheduled, in page
// order (D-11). A row here is the whole definition of a kind: there is no row for a staff,
// lockdown, captcha, log or raid message, so none can be scheduled by choosing a kind (D-18).
var deletionBotRows = []deletionRow{
	{
		Code: "bw", Kind: deletion.KindBotWelcome,
		NameKey: "settings_del_bot_welcome", DescKey: "settings_del_desc_welcome",
		Presets: deletionBotPresets,
	},
	{
		Code: "bg", Kind: deletion.KindBotGoodbye,
		NameKey: "settings_del_bot_goodbye", DescKey: "settings_del_desc_goodbye",
		Presets: deletionBotPresets,
	},
	{
		Code: "br", Kind: deletion.KindBotRules,
		NameKey: "settings_del_bot_rules", DescKey: "settings_del_desc_rules",
		Presets: deletionBotPresets,
	},
	{
		Code: "ba", Kind: deletion.KindBotAuto,
		NameKey: "settings_del_bot_auto", DescKey: "settings_del_desc_auto",
		HintKey: "settings_del_hint_punish_auto",
		Presets: deletionBotPresets,
	},
	{
		Code: "bm", Kind: deletion.KindBotManual,
		NameKey: "settings_del_bot_manual", DescKey: "settings_del_desc_manual",
		Presets: deletionBotPresets,
	},
}

// deletionRowOf returns the row with the given code, from either table.
func deletionRowOf(code string) (deletionRow, bool) {
	return deletionFindRow(func(r deletionRow) bool { return r.Code == code })
}

// deletionRowOfKind returns the row of the given kind, from either table.
func deletionRowOfKind(kind string) (deletionRow, bool) {
	return deletionFindRow(func(r deletionRow) bool { return r.Kind == kind })
}

// deletionFindRow returns the first row, service rows before bot rows, that satisfies match.
func deletionFindRow(match func(deletionRow) bool) (deletionRow, bool) {
	for _, table := range [][]deletionRow{deletionServiceRows, deletionBotRows} {
		for _, row := range table {
			if match(row) {
				return row, true
			}
		}
	}
	return deletionRow{}, false
}

// deletionServiceKindOf returns the first service row, in table order, whose predicate matches
// the message. ok is false for a nil message, for content, and for the types that are never
// deleted. It answers for the joined and left rows too: who handles those is the watcher's
// business, not the table's.
func deletionServiceKindOf(msg *gotgbot.Message) (deletionRow, bool) {
	if msg == nil {
		return deletionRow{}, false
	}
	for _, row := range deletionServiceRows {
		if row.Match != nil && row.Match(msg) {
			return row, true
		}
	}
	return deletionRow{}, false
}

// deletionEffectiveDelay is the effective rule of one kind: whether a rule is on and the delay
// in seconds (0 deletes at once). For every kind but the two that share the /cleanservice flag
// it is the stored row alone: on when the rules hold the kind, with the delay as stored.
func deletionEffectiveDelay(kind string, rules *deletion.Rules, cleanServiceOn bool) (delay int, on bool) {
	_ = cleanServiceOn
	if rules == nil {
		return 0, false
	}
	delay, on = rules.Delays[kind]
	return delay, on
}
