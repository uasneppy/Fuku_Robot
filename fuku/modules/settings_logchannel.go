package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/actionlog"
)

// The Log channel page (page o) turns each of the six log categories of a group on or off, as /log
// and /nolog do: every cell writes exactly one category through applyLogCategory and then posts the
// same #SETTINGS audit entry the commands post. Linking and unlinking a channel stay /setlog and
// /unsetlog. The page shows only whether a channel is linked and the six flags: it is read by every
// member of a group menu while the log channel is private to the administrators, so the renderer
// never receives the row and no screen, notice or alert can name the channel.

const (
	// settingsPageLog is the p value of the Log channel page.
	settingsPageLog = "o"
	// settingsLogCategoryCount is the number of log categories.
	settingsLogCategoryCount = 6
)

// settingsLogCategory is one row of the page's category table: the t value of its control, the
// category's name in the repository (the word /log and /nolog take) and the locale key of the
// name shown on the page.
type settingsLogCategory struct {
	Code    string
	Name    string
	NameKey string
}

// settingsLogCategories is the one table the controls, the body lines and the cells are built
// from, in the order of logchannels.AllCategories.
var settingsLogCategories = [settingsLogCategoryCount]settingsLogCategory{
	{"s", logchannels.CategorySettings, "settings_log_cat_settings"},
	{"a", logchannels.CategoryAdmin, "settings_log_cat_admin"},
	{"u", logchannels.CategoryUser, "settings_log_cat_user"},
	{"m", logchannels.CategoryAutomated, "settings_log_cat_automated"},
	{"r", logchannels.CategoryReports, "settings_log_cat_reports"},
	{"o", logchannels.CategoryOther, "settings_log_cat_other"},
}

// settingsLogRead reads the chat's log channel row the way the commands' loader does, with the
// error returned and a chat with no channel read as (nil, nil). It is a variable only so a test can
// make the read fail.
var settingsLogRead = logchannels.LoadContext

// settingsLogState is all the page knows: whether a channel is linked and the state of each
// category in table order. It carries no part of the row beyond those, so nothing that is drawn can
// name the channel.
type settingsLogState struct {
	Linked bool
	On     [settingsLogCategoryCount]bool
}

// settingsLogStateOf reduces a row (nil for a chat with no channel) to the page's state. A chat
// with no channel logs nothing, so every category reads off.
func settingsLogStateOf(row *models.LogChannel) settingsLogState {
	state := settingsLogState{Linked: row != nil}
	for i, cat := range settingsLogCategories {
		state.On[i] = logchannels.CategoryEnabled(row, cat.Name)
	}
	return state
}

// settingsLogReadState reads the page's state. A read error is returned, never a guess.
func settingsLogReadState(ctx context.Context, chatID int64) (settingsLogState, error) {
	row, err := settingsLogRead(ctx, chatID)
	if err != nil {
		return settingsLogState{}, err
	}
	return settingsLogStateOf(row), nil
}

func init() {
	controls := make(map[string]settingsControl, len(settingsLogCategories))
	for _, cat := range settingsLogCategories {
		// /log and /nolog ask for RequireUserAdmin and nothing more, so the live admin check every
		// press makes is the whole right.
		controls[cat.Code] = settingsControl{
			Right:    settingsRightAdmin,
			Set:      settingsLogControl(cat),
			Actions:  []string{settingsActToggle},
			Validate: settingsEditSwitchValue,
		}
	}
	registerSettingsPage(settingsPage{
		Key:       settingsPageLog,
		Order:     88,
		Icon:      "🧾",
		TitleKey:  "settings_page_log",
		MainState: settingsLogMainState,
		Open:      settingsLogOpen,
		Controls:  controls,
	})
}

// settingsLogLinkedWord is Linked or Not linked in the view's language.
func settingsLogLinkedWord(tr *i18n.Translator, linked bool) string {
	if linked {
		return settingsText(tr, "settings_state_linked")
	}
	return settingsText(tr, "settings_state_not_linked")
}

// settingsLogMainState is the Log channel line of the Main page: Linked or Not linked. A failed
// read answers unknown.
func settingsLogMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsLogReadState(ctx, group.Id)
	if err != nil {
		return "", false
	}
	return settingsLogLinkedWord(tr, state.Linked), true
}

// settingsLogOpen reads the page's state and draws it. A read error is returned, which the
// dispatcher turns into the cannot-open edit.
func settingsLogOpen(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	state, err := settingsLogReadState(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsLogChannel(p.view(), state, notice), nil
}

// renderSettingsLogChannel draws the Log channel page: the linked state, one line per category
// with its state word, the hint that links and unlinks are commands, and the toggle grid of the
// six categories (every cell drawn, linked or not: a press without a channel is refused with the
// notice of the command). It is pure and receives the state only, never the row.
func renderSettingsLogChannel(v settingsView, state settingsLogState, notice string) settingsScreen {
	body := []string{
		settingsText(v.Tr, "settings_log_status", i18n.TranslationParams{"state": settingsLogLinkedWord(v.Tr, state.Linked)}),
	}
	cells := make([]settingsToggleCell, 0, len(settingsLogCategories))
	for i, cat := range settingsLogCategories {
		name := settingsText(v.Tr, cat.NameKey)
		on := state.On[i]
		word := settingsText(v.Tr, "common_status_off")
		if on {
			word = settingsText(v.Tr, "common_status_on")
		}
		body = append(body, settingsText(v.Tr, "settings_main_state_line", i18n.TranslationParams{
			"icon": settingsStateIcon(on), "page": name, "state": word,
		}))
		cells = append(cells, settingsToggleCell{Control: cat.Code, Icon: settingsStateIcon(on), Name: name, On: on})
	}
	body = append(body, "", settingsText(v.Tr, "settings_log_hint"))

	k := newSettingsKeyboard(v)
	settingsToggleGrid(k, settingsPageLog, cells)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_log", body, notice, false))
}

// settingsLogControl builds the Set of one category's cell: the shared boolean press with the
// page's read and draw and an apply step that writes exactly that category through
// applyLogCategory. After a stored change the page is redrawn first and the same #SETTINGS audit
// entry /log and /nolog post is sent to the log channel; a refusal or a failed save posts nothing.
func settingsLogControl(cat settingsLogCategory) func(*settingsPress, string) *settingsScreen {
	return func(p *settingsPress, value string) *settingsScreen {
		var auditKey string
		screen := settingsToggleSet(p, value,
			func(ctx context.Context) (settingsLogState, error) { return settingsLogReadState(ctx, p.group.Id) },
			func(ctx context.Context, before settingsLogState, on bool) settingsToggleResult[settingsLogState] {
				result := applyLogCategory(ctx, p.group.Id, cat.Name, on)
				if result.Outcome != logApplied {
					return settingsToggleResult[settingsLogState]{Key: result.Key}
				}
				auditKey = result.Key
				if result.Row == nil {
					// The write went through and the read-back failed: the helper reads the page again.
					return settingsToggleResult[settingsLogState]{}
				}
				return settingsToggleResult[settingsLogState]{State: settingsLogStateOf(result.Row), Stored: true}
			},
			func(state settingsLogState, notice string) settingsScreen {
				return renderSettingsLogChannel(p.view(), state, notice)
			})
		if screen == nil || auditKey == "" {
			return screen
		}
		// The change is stored: answer and redraw before the audit post, so a slow send never keeps
		// the menu waiting. Destination gates the post on the categories exactly as for the commands
		// (a change that switched the settings category off posts nothing), and a failed post is only
		// logged by the action log.
		p.answer("", false)
		p.edit(*screen)
		actionlog.Settings(p.b, p.group, &p.query.From, auditKey+" "+cat.Name)
		return nil
	}
}
