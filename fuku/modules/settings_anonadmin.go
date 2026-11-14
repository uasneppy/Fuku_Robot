package modules

import (
	"context"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/admin"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Anonymous admins page (page y) shows whether anonymous-admin mode is on and lets the group's
// owner switch it. Every administrator the owner gate admits can open it, because /anonadmin's
// status view shows the mode to anyone and viewing writes nothing. The toggle declares
// settingsRightOwnerBotAdmin, because /anonadmin changes the mode only for the live creator and
// only while the bot is an administrator: the page refuses exactly what the command refuses. The
// page is read through admin.LoadAnonAdminContext, which never creates the admin row that the
// command's own read creates.

const (
	// settingsPageAnon is the p value of the Anonymous admins page.
	settingsPageAnon = "y"
	// settingsAnonControl is the t value of the toggle.
	settingsAnonControl = "m"
)

// settingsAnonRead reads whether anonymous-admin mode is on, with the error returned and no row
// created. It is a variable only so a test can make the read fail.
var settingsAnonRead = admin.LoadAnonAdminContext

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageAnon,
		Order:     89,
		Icon:      "🎭",
		TitleKey:  "settings_page_anon",
		MainState: settingsAnonMainState,
		Open:      settingsAnonOpen,
		Controls: map[string]settingsControl{
			settingsAnonControl: {
				Right:    settingsRightOwnerBotAdmin,
				Set:      settingsAnonSet,
				Actions:  []string{settingsActToggle},
				Validate: settingsEditSwitchValue,
			},
		},
	})
}

// settingsAnonMainState is the Anonymous admins line of the Main page: On or Off. A failed read
// answers unknown.
func settingsAnonMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	on, err := settingsAnonRead(ctx, group.Id)
	if err != nil {
		return "", false
	}
	if on {
		return settingsText(tr, "common_status_on"), true
	}
	return settingsText(tr, "common_status_off"), true
}

// settingsAnonOpen reads the mode and draws the page. A read error is returned, which the
// dispatcher turns into the cannot-open edit.
func settingsAnonOpen(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	on, err := settingsAnonRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsAnonAdmin(p.view(), on, notice), nil
}

// renderSettingsAnonAdmin draws the Anonymous admins page: /anonadmin's own sentence for the
// mode (with the group title escaped and capped at 64 runes, spliced into the translated text the
// way the group line is), the hint that only the owner can change it, and the full-width toggle
// that sends the opposite value. It is pure.
func renderSettingsAnonAdmin(v settingsView, on bool, notice string) settingsScreen {
	key := "admin_anon_admin_disabled"
	if on {
		key = "admin_anon_admin_enabled"
	}
	body := []string{
		fmt.Sprintf(settingsText(v.Tr, key), staffDisplayTitle(v.GroupTitle)),
		"",
		settingsText(v.Tr, "settings_anon_hint"),
	}
	k := newSettingsKeyboard(v)
	label := settingsText(v.Tr, "settings_btn_anon_toggle", i18n.TranslationParams{"state": settingsStateWord(v.Tr, on)})
	k.row(settingsToggleButton(k, settingsPageAnon, settingsAnonControl, label, on)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_anon", body, notice, false))
}

// settingsAnonSet is the press of the toggle: the shared boolean press with the page's read and
// draw and applyAnonAdmin, the function /anonadmin calls. A press of the value already stored
// writes nothing and redraws with no notice (the command would say "already"); a failed save
// redraws the stored state with the command's own failure text.
func settingsAnonSet(p *settingsPress, value string) *settingsScreen {
	return settingsToggleSet(p, value,
		func(ctx context.Context) (bool, error) { return settingsAnonRead(ctx, p.group.Id) },
		func(ctx context.Context, before bool, on bool) settingsToggleResult[bool] {
			result := applyAnonAdmin(ctx, p.group.Id, before, on)
			if result.Outcome == anonAdminSaveFailed {
				return settingsToggleResult[bool]{Key: result.Key}
			}
			return settingsToggleResult[bool]{State: result.On, Stored: true}
		},
		func(on bool, notice string) settingsScreen {
			return renderSettingsAnonAdmin(p.view(), on, notice)
		})
}
