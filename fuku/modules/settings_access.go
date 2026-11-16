package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/menuaccess"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Access page is the owner's switch "Admins can change settings" (MENU-12). It is drawn and
// opened only for the live creator of the group (OwnerOnly), turning the switch ON takes two
// taps through the shared confirm card and turning it OFF takes one, and nothing is announced
// anywhere (D-11). The table is read fresh, so a flip is seen by the very next press on any
// replica and there is no cache key to invalidate (D-12, D-19).

const (
	// settingsPageAccess is the p value of the Access page.
	settingsPageAccess = "a"
	// settingsAccessControlSwitch is the t value of the switch.
	settingsAccessControlSwitch = "s"
	// settingsActAccessYes is the action code of the ON card's Yes (fields: e).
	settingsActAccessYes = "ay"
)

// settingsAccessWrite stores the switch. It is a variable only so a test can make the write fail.
var settingsAccessWrite = menuaccess.SetAdminsAllowed

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageAccess,
		Order:     90,
		Icon:      "🔑",
		TitleKey:  "settings_page_access",
		OwnerOnly: true,
		MainState: settingsAccessMainState,
		Open:      settingsAccessOpen,
		Controls: map[string]settingsControl{
			settingsAccessControlSwitch: {Right: settingsRightOwner, Set: settingsAccessSet},
		},
		Actions: map[string]settingsPageAction{
			settingsActAccessYes: {Fields: []string{"e"}, Right: settingsRightOwner, Run: settingsAccessYes},
		},
	})
}

// accessApplyOutcome is what applyAdminsAllowed did.
type accessApplyOutcome int

const (
	// accessApplied means the switch holds the requested value.
	accessApplied accessApplyOutcome = iota
	// accessRefusedNotOwner means the presser is not the live creator; nothing was written.
	accessRefusedNotOwner
	// accessWriteFailed means the write failed; the stored value is unchanged.
	accessWriteFailed
)

// accessApplyResult is the typed result of applyAdminsAllowed. Allowed is the value the page
// must show: the requested one after a write, the unchanged one after a refusal or a failure.
type accessApplyResult struct {
	Outcome accessApplyOutcome
	Allowed bool
	Err     error
}

// applyAdminsAllowed is the one code path of the owner switch: the non-creator refusal, the
// write and the value to draw. status is the presser's live member status. It has no cache to
// invalidate because the switch is read fresh from PostgreSQL (D-19, research R-02). A request
// is the absolute target, so repeating it writes the same value again.
func applyAdminsAllowed(ctx context.Context, status string, chatID int64, allowed bool, by int64) accessApplyResult {
	if status != gotgbot.ChatMemberStatusCreator {
		return accessApplyResult{Outcome: accessRefusedNotOwner, Allowed: !allowed}
	}
	if err := settingsAccessWrite(ctx, chatID, allowed, by); err != nil {
		return accessApplyResult{Outcome: accessWriteFailed, Allowed: !allowed, Err: err}
	}
	return accessApplyResult{Outcome: accessApplied, Allowed: allowed}
}

// settingsAccessRead reads the switch fresh, through the seam the gate reads.
func settingsAccessRead(chatID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	return settingsAccessLookup(ctx, chatID)
}

// settingsAccessMainState is the Access line of the Main page: Admins allowed or Owner only.
func settingsAccessMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	allowed, err := settingsAccessRead(group.Id)
	if err != nil {
		return "", false
	}
	if allowed {
		return settingsText(tr, "settings_state_access_on"), true
	}
	return settingsText(tr, "settings_state_access_off"), true
}

// settingsAccessOpen reads the switch and draws the page. A read error is returned, which the
// dispatcher turns into the cannot-open edit.
func settingsAccessOpen(p *settingsPress, notice string) (settingsScreen, error) {
	allowed, err := settingsAccessRead(p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsAccessPage(p.view(), allowed, notice), nil
}

// renderSettingsAccessPage draws the Access page: the status line, what the state means and
// the one toggle, whose label shows the current state.
func renderSettingsAccessPage(v settingsView, allowed bool, notice string) settingsScreen {
	state := settingsStateWord(v.Tr, allowed)
	desc := "settings_access_off_desc"
	if allowed {
		desc = "settings_access_on_desc"
	}
	body := []string{
		settingsText(v.Tr, "settings_access_status", i18n.TranslationParams{"state": state}),
		settingsText(v.Tr, desc),
	}

	k := newSettingsKeyboard(v)
	label := settingsText(v.Tr, "settings_btn_access_toggle", i18n.TranslationParams{"state": state})
	k.row(settingsToggleButton(k, settingsPageAccess, settingsAccessControlSwitch, label, allowed)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_access", body, notice, false))
}

// renderSettingsAccessCard draws the ON card: the question and [Yes | No].
func renderSettingsAccessCard(v settingsView) settingsScreen {
	k := newSettingsKeyboard(v)
	settingsCardRows(k, map[string]string{"a": settingsActAccessYes}, settingsPageAccess)
	body := []string{settingsText(v.Tr, "settings_access_confirm")}
	return k.screen(settingsCompose(v, "settings_page_access", body, "", false))
}

// settingsAccessRedraw draws the page from an apply result, with the save-failed notice when
// the write failed.
func settingsAccessRedraw(p *settingsPress, result accessApplyResult) *settingsScreen {
	notice := ""
	if result.Outcome == accessWriteFailed {
		log.Errorf("[Settings] write of the access switch of chat %d failed: %v", p.group.Id, result.Err)
		notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
	}
	screen := renderSettingsAccessPage(p.view(), result.Allowed, notice)
	return &screen
}

// settingsAccessApply runs the apply function for the press and draws its result. A refusal
// (the right already judged the presser, so this is only the defence behind it) is the owner
// alert with nothing edited.
func settingsAccessApply(p *settingsPress, allowed bool) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	result := applyAdminsAllowed(ctx, p.member.Status, p.group.Id, allowed, p.query.From.Id)
	if result.Outcome == accessRefusedNotOwner {
		p.answer(settingsText(p.tr, "settings_access_owner_only"), true)
		return nil
	}
	return settingsAccessRedraw(p, result)
}

// settingsAccessSet serves tg and st alike. The target 1 only draws the card and writes
// nothing: turning admin access on takes a second tap (D-09). The target 0 narrows access and
// writes at once.
func settingsAccessSet(p *settingsPress, value string) *settingsScreen {
	switch value {
	case "1":
		screen := renderSettingsAccessCard(p.view())
		return &screen
	case "0":
		return settingsAccessApply(p, false)
	}
	p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
	return nil
}

// settingsAccessYes writes ON after the card's expiry check. The press already ran the whole
// pipeline again, so a presser who is no longer the live creator never gets here.
func settingsAccessYes(p *settingsPress) *settingsScreen {
	expired, ok := settingsCardExpired(p.fields)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	if expired {
		allowed, err := settingsAccessRead(p.group.Id)
		if err != nil {
			p.cannotOpen()
			return nil
		}
		screen := renderSettingsAccessPage(p.view(), allowed, settingsNoticeWarn(settingsText(p.tr, "settings_card_expired")))
		return &screen
	}
	return settingsAccessApply(p, true)
}
