package modules

import (
	"context"

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

// settingsSwState is what the page shows: the stored value of each of the three switches.
type settingsSwState struct {
	Reports bool
	AI      bool
	Approve bool
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
		},
	})
}

// settingsSwReadState reads the three switches. Any read error is returned, never a guess.
func settingsSwReadState(ctx context.Context, chatID int64) (settingsSwState, error) {
	var s settingsSwState
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

// renderSettingsSwitches draws the Switches page: one state block per switch and one full-width
// toggle each, then the navigation row.
func renderSettingsSwitches(v settingsView, s settingsSwState, notice string) settingsScreen {
	reportsKey := "reports_status_disabled_group"
	if s.Reports {
		reportsKey = "reports_status_enabled_group"
	}
	body := []string{settingsText(v.Tr, reportsKey)}

	k := newSettingsKeyboard(v)
	settingsSwToggleRow(k, settingsSwReportsKey, "settings_btn_sw_reports", s.Reports)
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
