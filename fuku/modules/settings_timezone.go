package modules

import (
	"context"
	"errors"
	"html"
	"slices"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Timezone page (page z) chooses the zone a group's times are shown in: a short grid of common
// zones, Use default, and Type another, which waits for a typed zone in the bot's private chat (the
// settings wait and its door). Every write goes through applyTimezone, the function /timezone writes
// through, and needs can_change_info, as the command does.

const (
	// settingsPageTimezone is the p value of the Timezone page.
	settingsPageTimezone = "z"
	// settingsTimezoneControlKey is the t value of the zone choice; its v is a preset code or
	// settingsTimezoneDefaultCode, never a zone name, so every button fits 64 bytes.
	settingsTimezoneControlKey = "z"
	// settingsTimezoneDefaultCode is the v of Use default.
	settingsTimezoneDefaultCode = "d"
	// settingsTimezonePart is the part of the typed zone's wait.
	settingsTimezonePart = "z"
	// settingsActTimezoneType is Type another, settingsActTimezoneCancel the Cancel of its prompt.
	settingsActTimezoneType   = "zo"
	settingsActTimezoneCancel = "zk"
)

// settingsTimezonePreset is one cell of the page's grid: the code a button carries, the zone it stores
// and the key of its label.
type settingsTimezonePreset struct {
	Code, Zone, LabelKey string
}

// settingsTimezonePresets are the page's common zones, three to a row.
var settingsTimezonePresets = []settingsTimezonePreset{
	{"ky", "Europe/Kyiv", "settings_tz_city_kyiv"},
	{"wa", "Europe/Warsaw", "settings_tz_city_warsaw"},
	{"be", "Europe/Berlin", "settings_tz_city_berlin"},
	{"lo", "Europe/London", "settings_tz_city_london"},
	{"li", "Europe/Lisbon", "settings_tz_city_lisbon"},
	{"is", "Europe/Istanbul", "settings_tz_city_istanbul"},
	{"tb", "Asia/Tbilisi", "settings_tz_city_tbilisi"},
	{"ny", "America/New_York", "settings_tz_city_newyork"},
	{"ut", "UTC", "settings_tz_city_utc"},
}

// settingsTimezoneRead reads the chat's stored zone ("" for the default), a variable only so a test can fail it.
var settingsTimezoneRead = func(ctx context.Context, chatID int64) (string, error) {
	return chatTimezoneLookup(ctx, chatID)
}

func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageTimezone,
		Order:     87,
		Icon:      "🕒",
		TitleKey:  "settings_page_tz",
		MainState: settingsTimezoneMainState,
		Open:      settingsTimezoneOpen,
		Controls: map[string]settingsControl{
			settingsTimezoneControlKey: {
				Right:    settingsRightChangeInfo,
				Set:      settingsTimezoneSet,
				Actions:  []string{settingsActSet},
				Validate: settingsTimezoneValueShape,
			},
		},
		Actions: map[string]settingsPageAction{
			settingsActTimezoneType: {
				Right: settingsRightChangeInfo,
				Run:   settingsTimezoneRunType,
			},
			settingsActTimezoneCancel: {
				Fields:   []string{"t"},
				Right:    settingsRightAdmin,
				Validate: func(fields map[string]string) bool { return fields["t"] == settingsTimezonePart },
				Run:      settingsTimezoneRunCancel,
			},
		},
	})
	settingsTypedPages[settingsPageTimezone] = settingsTypedPage{
		Part:       settingsTimezonePart,
		CancelCode: settingsActTimezoneCancel,
		Right:      settingsRightChangeInfo,
		Typed:      settingsTimezoneTyped,
	}
}

// settingsTimezoneValueShape is the strict check of a press's v: a preset code or the default code.
func settingsTimezoneValueShape(v string) bool {
	if v == settingsTimezoneDefaultCode {
		return true
	}
	return slices.ContainsFunc(settingsTimezonePresets, func(p settingsTimezonePreset) bool { return p.Code == v })
}

// settingsTimezoneLocation is the location of a stored value, the default for "".
func settingsTimezoneLocation(stored string) *time.Location {
	if loc, ok := parseStoredTimezone(stored); ok {
		return loc
	}
	return defaultChatLocation()
}

// settingsTimezoneMainState is the page's Main line: the zone's short label now. A failed read is unknown.
func settingsTimezoneMainState(_ *gotgbot.Bot, _ *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	stored, err := settingsTimezoneRead(ctx, group.Id)
	if err != nil {
		return "", false
	}
	return timezoneLabelAt(time.Now(), settingsTimezoneLocation(stored)), true
}

// renderSettingsTimezone draws the page: the zone with its label, the time there now, whether it is the
// bot's default, the hint, the preset grid with the stored one marked, Use default and Type another, the
// Open in private row in a group menu, and the navigation row. It is pure.
func renderSettingsTimezone(v settingsView, stored string, now time.Time, botUsername, notice string) settingsScreen {
	loc := settingsTimezoneLocation(stored)
	body := []string{
		settingsText(v.Tr, "settings_tz_current", i18n.TranslationParams{
			"zone": html.EscapeString(timezoneDisplayName(stored)), "label": timezoneLabelAt(now, loc),
		}),
		settingsText(v.Tr, "settings_tz_now", i18n.TranslationParams{"time": now.In(loc).Format("15:04")}),
	}
	if stored == "" {
		body = append(body, settingsText(v.Tr, "settings_tz_is_default"))
	} else {
		body = append(body, settingsText(v.Tr, "settings_tz_default_is", i18n.TranslationParams{"zone": html.EscapeString(chatTimezoneDefaultName())}))
	}
	body = append(body, "", settingsText(v.Tr, "settings_tz_hint"))

	k := newSettingsKeyboard(v)
	var cells []gotgbot.InlineKeyboardButton
	for _, preset := range settingsTimezonePresets {
		label := settingsText(v.Tr, preset.LabelKey)
		if preset.Zone == stored {
			label = settingsSelectedPrefix + label
		}
		fields := map[string]string{"a": settingsActSet, "p": settingsPageTimezone, "t": settingsTimezoneControlKey, "v": preset.Code}
		if button, ok := k.button(label, fields); ok {
			cells = append(cells, button)
		}
	}
	for _, row := range slices.Collect(slices.Chunk(cells, 3)) {
		k.row(row...)
	}
	var actions []gotgbot.InlineKeyboardButton
	defaultLabel := settingsText(v.Tr, "settings_btn_tz_default")
	if stored == "" {
		defaultLabel = settingsSelectedPrefix + defaultLabel
	}
	if button, ok := k.button(defaultLabel, map[string]string{"a": settingsActSet, "p": settingsPageTimezone, "t": settingsTimezoneControlKey, "v": settingsTimezoneDefaultCode}); ok {
		actions = append(actions, button)
	}
	if button, ok := k.button(settingsText(v.Tr, "settings_btn_tz_other"), map[string]string{"a": settingsActTimezoneType}); ok {
		actions = append(actions, button)
	}
	k.row(actions...)
	settingsEditOpenPrivateRow(k, settingsEditScreenOpts{BotUsername: botUsername})
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_tz", body, notice, false))
}

// settingsTimezoneDraw reads the stored zone and draws the page with the notice.
func settingsTimezoneDraw(p *settingsPress, notice string) (settingsScreen, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	stored, err := settingsTimezoneRead(ctx, p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsTimezone(p.view(), stored, time.Now(), getBotUsername(p.b), notice), nil
}

// settingsTimezoneOpen is the page's Open. A read error is the cannot-open edit.
func settingsTimezoneOpen(p *settingsPress, notice string) (settingsScreen, error) {
	return settingsTimezoneDraw(p, notice)
}

// settingsTimezoneShow turns a draw into the screen of a press: a read error is the cannot-open edit.
func settingsTimezoneShow(p *settingsPress, screen settingsScreen, err error) *settingsScreen {
	if err != nil {
		log.Warnf("[Settings] the timezone of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsTimezoneSet stores a preset or the default through applyTimezone and redraws. A press of the
// stored value writes nothing and shows no notice; a failed save redraws the stored state with the
// save-failed notice.
func settingsTimezoneSet(p *settingsPress, value string) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	stored, err := settingsTimezoneRead(ctx, p.group.Id)
	if err != nil {
		return settingsTimezoneShow(p, settingsScreen{}, err)
	}
	input := "default"
	for _, preset := range settingsTimezonePresets {
		if preset.Code == value {
			input = preset.Zone
		}
	}
	if (input == "default" && stored == "") || input == stored {
		screen, err := settingsTimezoneDraw(p, "")
		return settingsTimezoneShow(p, screen, err)
	}
	notice := ""
	if result := applyTimezone(ctx, p.group.Id, p.query.From.Id, input); result.Outcome == timezoneSaveFailed || result.Outcome == timezoneInvalid {
		notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
	}
	screen, err := settingsTimezoneDraw(p, notice)
	return settingsTimezoneShow(p, screen, err)
}

// renderSettingsTimezonePrompt draws the prompt of Type another: what to send, the wait's end in the
// group's zone, the replaced note, and one Cancel button. It is pure.
func renderSettingsTimezonePrompt(v settingsView, until time.Time, replaced bool) settingsScreen {
	body := []string{
		settingsText(v.Tr, "settings_tz_wait"),
		"",
		settingsText(v.Tr, "settings_edit_wait_footer", i18n.TranslationParams{"time": formatChatTime(until, chatClockOf(v.GroupID), "15:04")}),
	}
	if replaced {
		body = append(body, "", settingsText(v.Tr, "settings_edit_replaced_note"))
	}
	k := newSettingsKeyboard(v)
	if cancel, ok := k.button(settingsText(v.Tr, "settings_btn_edit_cancel"), map[string]string{"a": settingsActTimezoneCancel, "t": settingsTimezonePart}); ok {
		k.row(cancel)
	}
	return k.screen(settingsCompose(v, "settings_page_tz", body, "", false))
}

// settingsTimezoneRunType serves Type another: in a group menu it redraws the page with the note that typing
// works in private (no wait); in a private menu it writes the one wait of the user and edits the message
// into the prompt. Without Redis, or when the wait cannot be written, it answers settings_edit_unavailable.
func settingsTimezoneRunType(p *settingsPress) *settingsScreen {
	if !p.private {
		screen, err := settingsTimezoneDraw(p, settingsNoticeWarn(settingsText(p.tr, "settings_tz_group_note")))
		return settingsTimezoneShow(p, screen, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	nonce, err := newSettingsInputNonce()
	if err != nil {
		log.Errorf("[Settings] no nonce for a wait: %v", err)
		p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
		return nil
	}
	until := time.Now().Add(settingsInputTTL)
	previous, err := settingsInputSet(ctx, p.query.From.Id, settingsInputWait{
		GroupID:     p.group.Id,
		Page:        settingsPageTimezone,
		Part:        settingsTimezonePart,
		PromptMsgID: p.query.Message.GetMessageId(),
		Nonce:       nonce,
		ExpiresAt:   until,
	})
	if err != nil {
		if !errors.Is(err, errSettingsInputUnavailable) {
			log.Warnf("[Settings] the wait of user %d could not be written: %v", p.query.From.Id, err)
		}
		p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
		return nil
	}
	screen := renderSettingsTimezonePrompt(p.view(), until, settingsEditWaitReplaced(previous, p.group.Id, settingsPageTimezone, settingsTimezonePart))
	return &screen
}

// settingsTimezoneRunCancel serves the prompt's Cancel, as the list pages' Cancel does, and redraws the page.
func settingsTimezoneRunCancel(p *settingsPress) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	outcome, err := settingsInputCancel(ctx, p.query.From.Id, p.group.Id, settingsPageTimezone, p.fields["t"], p.query.Message.GetMessageId())
	notice := ""
	switch {
	case errors.Is(err, errSettingsInputUnavailable):
		p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
	case err != nil:
		log.Warnf("[Settings] the wait of user %d could not be cancelled: %v", p.query.From.Id, err)
		p.answer(settingsText(p.tr, "settings_edit_unavailable"), true)
		return nil
	case outcome == settingsInputCancelDeleted:
		notice = settingsNoticeOK(settingsText(p.tr, "settings_edit_cancelled"))
	case outcome == settingsInputCancelExpired:
		p.answer(settingsText(p.tr, "settings_edit_wait_expired"), true)
	default:
		p.answer(settingsText(p.tr, "settings_edit_prompt_stale"), true)
	}
	screen, err := settingsTimezoneDraw(p, notice)
	return settingsTimezoneShow(p, screen, err)
}

// settingsTimezoneTyped is the door's handler of the typed zone: claimed and stored through applyTimezone.
// A text that names no zone is refused with the wait kept; a failed save releases the claim and keeps
// it. A saved zone finishes the wait, sends the page as a new message and closes the prompt.
func settingsTimezoneTyped(r *settingsInputReply) {
	text := r.msg.GetText()
	if _, _, ok := normaliseTimezoneInput(text); !ok {
		r.refuse(r.text("timezone_invalid"))
		return
	}
	claim, claimed := r.claimWait()
	if !claimed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	result := applyTimezone(ctx, r.wait.GroupID, r.user.Id, text)
	if result.Outcome == timezoneSaveFailed || result.Outcome == timezoneInvalid {
		r.releaseWait(claim)
		r.refuse(r.text("common_settings_save_failed"))
		return
	}
	r.finishWait(claim)
	log.Infof("[Settings] user %d typed a timezone for chat %d", r.user.Id, r.wait.GroupID)
	screen := renderSettingsTimezone(r.view(), result.Value, time.Now(), "", settingsNoticeOK(settingsText(r.tr, "settings_tz_saved")))
	r.send(screen.Text, &screen.Keyboard, false)
	r.closePrompt()
}
