package modules

import (
	"context"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/aispam"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/reports"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Switches page (page t) turns three boolean settings on or off: Reports, the AI spam
// filter and Auto-approve of join requests. Each control writes through the apply function its
// command uses and each is read through an error-returning reader that creates nothing, so a
// press leaves the same row and cache the command leaves and a first read leaves no row behind.

const (
	// settingsPageSwitches is the p value of the Switches page.
	settingsPageSwitches = "t"
	// settingsSwReportsKey, settingsSwAIKey and settingsSwApproveKey are the t values of the
	// three toggles.
	settingsSwReportsKey = "r"
	settingsSwAIKey      = "a"
	settingsSwApproveKey = "j"
)

// The page's readers are variables only so a test can make one fail. Reports and the AI filter
// read the command's own getters' tables through their error-returning readers; Auto-approve
// reads the greetings row the way the editor pages do, with no row created.
var (
	settingsSwReportsRead = reports.LoadChatStatusContext
	settingsSwAIRead      = aispam.LoadEnabledContext
	settingsSwApproveRead = func(ctx context.Context, chatID int64) (bool, error) {
		settings, err := greetings.LoadSettingsContext(ctx, chatID)
		if err != nil {
			return false, err
		}
		return settings.ShouldAutoApprove, nil
	}
)

// settingsSwState is what the page shows: the stored value of each of the three switches, and
// whether the AI spam filter can run on this bot (config only, never stored).
type settingsSwState struct {
	Reports bool
	AI      bool
	Approve bool
	// AIAvailable is aispamConfigured() at the time the state was read.
	AIAvailable bool
}

// count is how many of the three switches are on.
func (s settingsSwState) count() int {
	n := 0
	for _, on := range []bool{s.Reports, s.AI, s.Approve} {
		if on {
			n++
		}
	}
	return n
}

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageSwitches,
		Order:     86,
		Icon:      "🔘",
		TitleKey:  "settings_page_switches",
		MainState: settingsSwMainState,
		Open:      settingsSwOpen,
		Controls: map[string]settingsControl{
			settingsSwReportsKey: {
				Right:    settingsRightAdminFloodBot,
				Set:      settingsSwControl(settingsSwApplyReports),
				Actions:  []string{settingsActToggle},
				Validate: settingsEditSwitchValue,
			},
			// /aispam has no bot check (RequireGroup and RequireUserAdmin only), so the AI switch
			// needs only the live admin check every press makes.
			settingsSwAIKey: {
				Right:    settingsRightAdmin,
				Set:      settingsSwControl(settingsSwApplyAI),
				Actions:  []string{settingsActToggle},
				Validate: settingsEditSwitchValue,
			},
			// /autoapprove asks for the bot as an administrator first, then can_change_info.
			settingsSwApproveKey: {
				Right:    settingsRightChangeInfoBotAdmin,
				Set:      settingsSwControl(settingsSwApplyApprove),
				Actions:  []string{settingsActToggle},
				Validate: settingsEditSwitchValue,
			},
		},
	})
}

// settingsSwReadState reads the three switches. Any read error is returned, never a guess.
func settingsSwReadState(ctx context.Context, chatID int64) (settingsSwState, error) {
	s := settingsSwState{AIAvailable: aispamConfigured()}
	var err error
	if s.Reports, err = settingsSwReportsRead(ctx, chatID); err != nil {
		return settingsSwState{}, err
	}
	if s.AI, err = settingsSwAIRead(ctx, chatID); err != nil {
		return settingsSwState{}, err
	}
	if s.Approve, err = settingsSwApproveRead(ctx, chatID); err != nil {
		return settingsSwState{}, err
	}
	return s, nil
}

// settingsSwMainState is the Switches line of the Main page: how many of the three are on, or
// Off when none is. A failed read answers unknown.
func settingsSwMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsSwReadState(ctx, group.Id)
	if err != nil {
		return "", false
	}
	if n := state.count(); n > 0 {
		return settingsText(tr, "settings_state_sw_on", i18n.TranslationParams{"n": n}), true
	}
	return settingsText(tr, "settings_state_off_plain"), true
}

// settingsSwOpen reads the switches and draws the page. A read error is returned, which the
// dispatcher turns into the cannot-open edit.
func settingsSwOpen(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsSwReadState(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsSwitches(p.view(), state, notice), nil
}

// settingsSwToggleRow is the full-width toggle of one switch: its label with the state icon and
// word, sending the absolute target.
func settingsSwToggleRow(k *settingsKeyboard, control, labelKey string, on bool) {
	label := settingsText(k.view.Tr, labelKey, i18n.TranslationParams{"state": settingsStateWord(k.view.Tr, on)})
	k.row(settingsToggleButton(k, settingsPageSwitches, control, label, on)...)
}

// settingsSwAIBlock is the AI block of the page: the state sentence, then the data notice while
// the filter can run on this bot, else the unavailable line (a code-side 🚫 and the key, which
// names no cause) in the notice's place, whatever the stored value is.
func settingsSwAIBlock(v settingsView, s settingsSwState) []string {
	sentence := "settings_sw_ai_off"
	if s.AI {
		sentence = "aispam_status_enabled"
	}
	block := []string{settingsText(v.Tr, sentence)}
	if s.AIAvailable {
		return append(block, strings.TrimRight(settingsText(v.Tr, "aispam_data_notice"), "\n"))
	}
	return append(block, "🚫 "+settingsText(v.Tr, "settings_sw_ai_unavailable"))
}

// renderSettingsSwitches draws the Switches page: one state block per switch and one full-width
// toggle each, then the navigation row. The AI toggle is drawn while the filter is available or
// stored on (so it can be switched off) and left out when it is unavailable and off.
func renderSettingsSwitches(v settingsView, s settingsSwState, notice string) settingsScreen {
	reportsKey := "reports_status_disabled_group"
	if s.Reports {
		reportsKey = "reports_status_enabled_group"
	}
	approveKey := "greetings_auto_approve_disabled"
	if s.Approve {
		approveKey = "greetings_auto_approve_enabled"
	}
	body := []string{settingsText(v.Tr, reportsKey), ""}
	body = append(body, settingsSwAIBlock(v, s)...)
	body = append(body, "", settingsText(v.Tr, approveKey))

	k := newSettingsKeyboard(v)
	settingsSwToggleRow(k, settingsSwReportsKey, "settings_btn_sw_reports", s.Reports)
	if s.AIAvailable || s.AI {
		settingsSwToggleRow(k, settingsSwAIKey, "settings_btn_sw_aispam", s.AI)
	}
	settingsSwToggleRow(k, settingsSwApproveKey, "settings_btn_sw_approve", s.Approve)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_switches", body, notice, false))
}

// settingsSwControl builds the Set of one toggle of the page: the shared boolean press with the
// page's read and draw and the control's own apply step.
func settingsSwControl(
	apply func(ctx context.Context, p *settingsPress, before settingsSwState, on bool) settingsToggleResult[settingsSwState],
) func(*settingsPress, string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		return settingsToggleSet(p, value,
			func(ctx context.Context) (settingsSwState, error) { return settingsSwReadState(ctx, p.group.Id) },
			func(ctx context.Context, before settingsSwState, on bool) settingsToggleResult[settingsSwState] {
				return apply(ctx, p, before, on)
			},
			func(state settingsSwState, notice string) settingsScreen {
				return renderSettingsSwitches(p.view(), state, notice)
			})
	}
}

// settingsSwApplyReports stores Reports through applyReportsEnabled, the function /reports
// calls for a group.
func settingsSwApplyReports(ctx context.Context, p *settingsPress, before settingsSwState, on bool) settingsToggleResult[settingsSwState] {
	result := applyReportsEnabled(ctx, p.group.Id, on)
	if result.Outcome != reportsApplied {
		return settingsToggleResult[settingsSwState]{Key: result.Key}
	}
	before.Reports = result.Status
	return settingsToggleResult[settingsSwState]{State: before, Stored: true}
}

// settingsSwApplyAI stores the AI spam filter through applyAISpamEnabled, the function /aispam
// calls. A press that turns it on while the filter cannot run on this bot (a stale or forged
// one: the Off-only form draws no toggle) is the press's one answer, the alert, and writes
// nothing; the page is then redrawn from the stored rows. Turning it off always works.
func settingsSwApplyAI(ctx context.Context, p *settingsPress, before settingsSwState, on bool) settingsToggleResult[settingsSwState] {
	unavailable := settingsToggleResult[settingsSwState]{State: before, Stored: true}
	if on && !aispamConfigured() {
		p.answer(settingsText(p.tr, "settings_sw_ai_unavailable"), true)
		return unavailable
	}
	result := applyAISpamEnabled(ctx, p.group.Id, on)
	switch result.Outcome {
	case aispamApplied:
		before.AI = result.Enabled
		return settingsToggleResult[settingsSwState]{State: before, Stored: true}
	case aispamNoKey, aispamGloballyDisabled:
		// The gate above makes this unreachable; the same alert if it is ever met.
		p.answer(settingsText(p.tr, "settings_sw_ai_unavailable"), true)
		return unavailable
	}
	return settingsToggleResult[settingsSwState]{Key: result.Key}
}

// settingsSwApplyApprove stores Auto-approve of join requests through applyAutoApprove, the
// function /autoapprove calls.
func settingsSwApplyApprove(ctx context.Context, p *settingsPress, before settingsSwState, on bool) settingsToggleResult[settingsSwState] {
	result := applyAutoApprove(ctx, p.group.Id, on)
	if result.Outcome != greetingApplied {
		return settingsToggleResult[settingsSwState]{Key: "common_settings_save_failed"}
	}
	if result.Settings == nil {
		// The write went through and the read-back failed: the helper reads the page again.
		return settingsToggleResult[settingsSwState]{}
	}
	before.Approve = result.Settings.ShouldAutoApprove
	return settingsToggleResult[settingsSwState]{State: before, Stored: true}
}
