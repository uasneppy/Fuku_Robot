package modules

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The Messages page of the settings menu (Phase 20, D-18, D-19): the group's custom event messages, the
// texts the bot posts in the group when someone is punished. It exists only in the bot's private chat
// (PrivateOnly, so a menu opened in the group shows the settings_msg_group_note line and runs no page code)
// and not in a Staff Group (Hidden: nothing is set at Staff Group level, D-01). Everything it draws comes
// from one table of families and events: the hub lists every family that has events with each event's
// state (Default, Custom or Silent), a family's list opens an event, and an event's screen shows the state
// of its parts. Each event is one message editor definition (registerSettingsEditDef), whose wait key is
// h + the event code + the destination code and whose buttons carry the event in the field k, so plans
// 20-07 to 20-09 make the event screen act through the editor's hooks. Every read is fresh and creates
// nothing.

const (
	// settingsPageMessages is the p value of the Messages page.
	settingsPageMessages = "h"
	// settingsActMsgFamily is the action code that opens a family's list (field t, a family code).
	settingsActMsgFamily = "hl"
	// settingsActMsgEvent is the action code that opens an event's screen (field k, an event code).
	settingsActMsgEvent = "he"
	// settingsActMsgSet, settingsActMsgRemove, settingsActMsgCancel, settingsActMsgPreview and
	// settingsActMsgFull are the editor codes every event definition carries: Set, Remove, the prompt's
	// Cancel, Preview of one part and Full preview. Plans 20-08 and 20-09 register them as page actions.
	settingsActMsgSet     = "hs"
	settingsActMsgRemove  = "hr"
	settingsActMsgCancel  = "hc"
	settingsActMsgPreview = "hv"
	settingsActMsgFull    = "hf"
	// settingsMsgEventField is the callback field that names the event of a press (not e, which is the
	// confirm-card expiry everywhere else in the namespace).
	settingsMsgEventField = "k"
	// settingsMsgDestGroupCode is the one-letter code of the group destination in a wait key.
	settingsMsgDestGroupCode = "g"
)

// settingsMsgEvent is one event of a family: the eventmsg event it edits, its two-letter code (the k
// value), its icon, its name and description keys, and the buttons the bot itself adds to its message (the
// Unban or Unmute row), counted against Telegram's keyboard limit.
type settingsMsgEvent struct {
	Key             string
	Code            string
	Icon            string
	NameKey         string
	DescKey         string
	ReservedButtons int
}

// settingsMsgFamily is one family of events (D-19): its code (the t value of hl), icon, name, the intro of
// its list and its events in page order. A family with no events is not drawn anywhere.
type settingsMsgFamily struct {
	Code     string
	Icon     string
	NameKey  string
	IntroKey string
	Events   []settingsMsgEvent
}

// settingsMsgManualEvents are the seven manual punishment events, in the order of eventmsg.Events.
var settingsMsgManualEvents = []settingsMsgEvent{
	{Key: eventmsg.EventBan, Code: "bn", Icon: "🚫", NameKey: "settings_msg_ev_ban", DescKey: "settings_msg_desc_ban", ReservedButtons: 1},
	{Key: eventmsg.EventTimedBan, Code: "tb", Icon: "⏳", NameKey: "settings_msg_ev_tban", DescKey: "settings_msg_desc_tban"},
	{Key: eventmsg.EventKick, Code: "kk", Icon: "👢", NameKey: "settings_msg_ev_kick", DescKey: "settings_msg_desc_kick"},
	{Key: eventmsg.EventMute, Code: "mu", Icon: "🔇", NameKey: "settings_msg_ev_mute", DescKey: "settings_msg_desc_mute", ReservedButtons: 1},
	{Key: eventmsg.EventTimedMute, Code: "tm", Icon: "🔕", NameKey: "settings_msg_ev_tmute", DescKey: "settings_msg_desc_tmute"},
	{Key: eventmsg.EventUnban, Code: "ub", Icon: "🔓", NameKey: "settings_msg_ev_unban", DescKey: "settings_msg_desc_unban"},
	{Key: eventmsg.EventUnmute, Code: "um", Icon: "🔊", NameKey: "settings_msg_ev_unmute", DescKey: "settings_msg_desc_unmute"},
}

// settingsMsgFamilies are the families of the hub in page order. Only the manual family has events now;
// later phases add the automatic, lockdown and requirement families. It is a variable only so a test can
// draw more families; production code never reassigns it.
var settingsMsgFamilies = []settingsMsgFamily{
	{Code: "m", Icon: "🔨", NameKey: "settings_msg_fam_manual", IntroKey: "settings_msg_fam_manual_intro", Events: settingsMsgManualEvents},
}

// settingsMsgParts are the parts of an event message, in the order the event screen lists them. The text
// part is the Welcome text part as a stand-in until plan 20-08 gives the events their own wait text.
var settingsMsgParts = []settingsEditPart{settingsEditTextPart, settingsEditMediaPart, settingsEditButtonsPart}

// settingsMsgRead reads a group's event messages fresh: the page never reads through the cache, so it
// never shows a value another replica has just changed as stale, and it creates nothing. It is a variable
// only so a test can make it fail or count its calls.
var settingsMsgRead = eventmsg.GetFresh

// settingsMsgStaffLookup reads a chat's Staff Group row fresh for the page's Hidden. It is the page's own seam
// (not the Deletion page's deletionStaffLookup, whose reads that page's tests count) and a variable only so a test
// can make it fail.
var settingsMsgStaffLookup = staff.GetStaffGroupFresh

func init() {
	registerSettingsPage(settingsPage{
		Key:            settingsPageMessages,
		Order:          74,
		Icon:           "📨",
		TitleKey:       "settings_page_msgs",
		MainState:      settingsMsgMainState,
		Open:           settingsMsgOpen,
		Hidden:         settingsMsgHidden,
		PrivateOnly:    true,
		PrivateNoteKey: "settings_msg_group_note",
		Actions: map[string]settingsPageAction{
			settingsActMsgFamily: {
				Fields: []string{"t"}, Right: settingsRightAdmin, Validate: settingsMsgFamilyFields, Run: settingsMsgRunFamily,
			},
			settingsActMsgEvent: {
				Fields: []string{settingsMsgEventField}, Right: settingsRightAdmin, Validate: settingsMsgEventFields, Run: settingsMsgRunEvent,
			},
		},
	})
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			registerSettingsEditDef(settingsMsgEditDef(fam, ev))
		}
	}
}

// settingsMsgFamilyByCode finds a family that has events.
func settingsMsgFamilyByCode(code string) (settingsMsgFamily, bool) {
	for _, fam := range settingsMsgFamilies {
		if fam.Code == code && len(fam.Events) > 0 {
			return fam, true
		}
	}
	return settingsMsgFamily{}, false
}

// settingsMsgEventByCode finds an event and the family that lists it.
func settingsMsgEventByCode(code string) (settingsMsgFamily, settingsMsgEvent, bool) {
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			if ev.Code == code {
				return fam, ev, true
			}
		}
	}
	return settingsMsgFamily{}, settingsMsgEvent{}, false
}

// settingsMsgWaitPage is the key of an event's editor definition, which is also the p of its wait: h, the
// event code and the destination code (hbng for the group's ban message).
func settingsMsgWaitPage(ev settingsMsgEvent) string {
	return settingsPageMessages + ev.Code + settingsMsgDestGroupCode
}

// settingsMsgEditDef is the message editor definition of one event: the hooked kind, the page's title,
// the five editor codes, can_change_info to change it (D-22), the three parts, the field k that names the
// event on every editor button, the bot's own reserved buttons and the hooks that read and draw it.
func settingsMsgEditDef(fam settingsMsgFamily, ev settingsMsgEvent) settingsEditPageDef {
	return settingsEditPageDef{
		Key: settingsMsgWaitPage(ev), Kind: settingsEditEvent, Icon: ev.Icon, TitleKey: "settings_page_msgs",
		SetCode: settingsActMsgSet, RemoveCode: settingsActMsgRemove, CancelCode: settingsActMsgCancel,
		PreviewCode: settingsActMsgPreview, FullCode: settingsActMsgFull, EditRight: settingsRightChangeInfo,
		Parts:           append([]settingsEditPart(nil), settingsMsgParts...),
		Fields:          map[string]string{settingsMsgEventField: ev.Code},
		ReservedButtons: ev.ReservedButtons,
		Hooks:           &settingsEditHooks{Load: settingsMsgLoad(ev), Draw: settingsMsgDraw(fam, ev)},
	}
}

// settingsMsgDefByFields is the definition of the event a press names by k. It is false for a press
// without the private owner field c (a group menu's press, refused in step 1 before any lookup) and for an
// unknown event.
func settingsMsgDefByFields(fields map[string]string) (settingsEditPageDef, bool) {
	if !settingsListPrivateField(fields) {
		return settingsEditPageDef{}, false
	}
	_, ev, ok := settingsMsgEventByCode(fields[settingsMsgEventField])
	if !ok {
		return settingsEditPageDef{}, false
	}
	return settingsEditPageByKey(settingsMsgWaitPage(ev))
}

// settingsMsgFamilyFields is the Validate of hl: the private owner field and a family code with events.
func settingsMsgFamilyFields(fields map[string]string) bool {
	if !settingsListPrivateField(fields) {
		return false
	}
	_, ok := settingsMsgFamilyByCode(fields["t"])
	return ok
}

// settingsMsgEventFields is the Validate of he: the private owner field and a known event code.
func settingsMsgEventFields(fields map[string]string) bool {
	_, ok := settingsMsgDefByFields(fields)
	return ok
}

// settingsMsgState is how one event's message is drawn.
type settingsMsgState int

const (
	// settingsMsgDefault means nothing is stored for the event: the bot posts its default text.
	settingsMsgDefault settingsMsgState = iota
	// settingsMsgCustom means the event has its own text, media or buttons.
	settingsMsgCustom
	// settingsMsgSilent means the event's Silent switch is on: the bot posts nothing.
	settingsMsgSilent
)

// settingsMsgStateOf is the state of one event's group message. Silent wins over a stored text, because
// the bot then posts nothing; a row whose parts are all empty with Silent off is Default, as the send path
// reads it.
func settingsMsgStateOf(msgs *eventmsg.Messages, ev settingsMsgEvent) settingsMsgState {
	msg, ok := msgs.Lookup(ev.Key, eventmsg.DestGroup)
	switch {
	case !ok:
		return settingsMsgDefault
	case msg.Silent:
		return settingsMsgSilent
	case msg.HasContent():
		return settingsMsgCustom
	}
	return settingsMsgDefault
}

// settingsMsgStateWord is the word of a state.
func settingsMsgStateWord(tr *i18n.Translator, state settingsMsgState) string {
	switch state {
	case settingsMsgCustom:
		return settingsText(tr, "settings_msg_state_custom")
	case settingsMsgSilent:
		return settingsText(tr, "settings_msg_state_silent")
	case settingsMsgDefault:
	}
	return settingsText(tr, "settings_state_default")
}

// settingsMsgCountChanged counts the events of every family whose state is not Default (Custom or Silent).
func settingsMsgCountChanged(msgs *eventmsg.Messages) int {
	n := 0
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			if settingsMsgStateOf(msgs, ev) != settingsMsgDefault {
				n++
			}
		}
	}
	return n
}

// settingsMsgReadNow reads a group's messages under the settings read timeout.
func settingsMsgReadNow(chatID int64) (*eventmsg.Messages, error) {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	return settingsMsgRead(ctx, chatID)
}

// settingsMsgMainState is the Messages line of Main: one fresh database read and no Telegram call. Default
// when no event is customised, "{n} custom" otherwise, the unknown mark when the read failed. It is called
// only in a private menu (PrivateOnly).
func settingsMsgMainState(_ *gotgbot.Bot, tr *i18n.Translator, group *gotgbot.Chat) (string, bool) {
	msgs, err := settingsMsgReadNow(group.Id)
	if err != nil {
		return "", false
	}
	if n := settingsMsgCountChanged(msgs); n > 0 {
		return settingsText(tr, "settings_state_msg_n", i18n.TranslationParams{"n": n}), true
	}
	return settingsText(tr, "settings_state_default"), true
}

// settingsMsgHidden is the page's Hidden: true only for a definite Staff Group row, read fresh. An unreadable
// row shows the page, which only edits messages a Staff Group never posts.
func settingsMsgHidden(group *gotgbot.Chat) bool {
	staffRow, err := settingsMsgStaffLookup(group.Id)
	return err == nil && staffRow != nil
}

// settingsMsgOpen draws the hub. A read error is returned, which the dispatcher turns into the cannot-open
// edit.
func settingsMsgOpen(p *settingsPress, notice string) (settingsScreen, error) {
	msgs, err := settingsMsgReadNow(p.group.Id)
	if err != nil {
		return settingsScreen{}, err
	}
	return renderSettingsMsgHub(p.view(), msgs, notice), nil
}

// settingsMsgRunFamily serves hl: a family's list, read fresh. A read error is the cannot-open edit.
func settingsMsgRunFamily(p *settingsPress) *settingsScreen {
	fam, ok := settingsMsgFamilyByCode(p.fields["t"])
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	msgs, err := settingsMsgReadNow(p.group.Id)
	if err != nil {
		log.Warnf("[Settings] the event messages of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	screen := renderSettingsMsgFamily(p.view(), fam, msgs, "")
	return &screen
}

// settingsMsgRunEvent serves he: the event's screen, drawn by its editor definition (load, then the Draw
// hook). A read error is the cannot-open edit.
func settingsMsgRunEvent(p *settingsPress) *settingsScreen {
	def, ok := settingsMsgDefByFields(p.fields)
	if !ok {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	return settingsEditRedraw(p, def, "")
}

// settingsMsgLoad is an event definition's Load hook: the state of the event's group message from a fresh
// read, creating nothing. No row reads as the default text with no media, no buttons and Silent off.
func settingsMsgLoad(ev settingsMsgEvent) func(context.Context, int64) (settingsEditState, error) {
	return func(ctx context.Context, chatID int64) (settingsEditState, error) {
		msgs, err := settingsMsgRead(ctx, chatID)
		if err != nil {
			return settingsEditState{}, err
		}
		msg, _ := msgs.Lookup(ev.Key, eventmsg.DestGroup)
		return settingsEditState{
			Kind: settingsEditEvent, Text: msg.Text, TextIsDefault: msg.Text == "",
			MediaType: msg.MediaType, FileID: msg.FileID, Buttons: models.ButtonArray(msg.Buttons),
			Silent: msg.Silent,
		}, nil
	}
}

// settingsMsgDraw is an event definition's Draw hook.
func settingsMsgDraw(fam settingsMsgFamily, ev settingsMsgEvent) func(settingsView, settingsEditPageDef, settingsEditState, settingsEditScreenOpts) settingsScreen {
	return func(v settingsView, _ settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen {
		return renderSettingsMsgEvent(v, fam, ev, st, opts)
	}
}

// settingsMsgEventLines is the state lines of a family's events: event icon, name and state word.
func settingsMsgEventLines(tr *i18n.Translator, fam settingsMsgFamily, msgs *eventmsg.Messages) []string {
	lines := make([]string, 0, len(fam.Events))
	for _, ev := range fam.Events {
		lines = append(lines, settingsText(tr, "settings_main_state_line", i18n.TranslationParams{
			"icon": ev.Icon, "page": settingsText(tr, ev.NameKey), "state": settingsMsgStateWord(tr, settingsMsgStateOf(msgs, ev)),
		}))
	}
	return lines
}

// renderSettingsMsgHub draws the hub: the intro, then for each family with events a blank line, its icon
// and bold name and one state line per event; one cell per family with events, two to a row, then Back
// (Main) and Close. It is pure.
func renderSettingsMsgHub(v settingsView, msgs *eventmsg.Messages, notice string) settingsScreen {
	tr := v.Tr
	body := []string{settingsText(tr, "settings_msg_intro")}
	k := newSettingsKeyboard(v)
	var cells []gotgbot.InlineKeyboardButton
	for _, fam := range settingsMsgFamilies {
		if len(fam.Events) == 0 {
			continue
		}
		name := settingsText(tr, fam.NameKey)
		body = append(body, "", fam.Icon+" <b>"+name+"</b>")
		body = append(body, settingsMsgEventLines(tr, fam, msgs)...)
		if button, ok := k.button(fam.Icon+" "+name, map[string]string{"a": settingsActMsgFamily, "t": fam.Code}); ok {
			cells = append(cells, button)
		}
	}
	settingsDelGrid(k, cells)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_msgs", body, notice, false))
}

// renderSettingsMsgFamily draws a family's list: its intro, the state line of each event and the hint; one
// cell per event, two to a row, then Back (the hub) and Close. It is pure.
func renderSettingsMsgFamily(v settingsView, fam settingsMsgFamily, msgs *eventmsg.Messages, notice string) settingsScreen {
	tr := v.Tr
	body := []string{settingsText(tr, fam.IntroKey), ""}
	body = append(body, settingsMsgEventLines(tr, fam, msgs)...)
	body = append(body, "", settingsText(tr, "settings_msg_list_hint"))
	k := newSettingsKeyboard(v)
	var cells []gotgbot.InlineKeyboardButton
	for _, ev := range fam.Events {
		if button, ok := k.button(ev.Icon+" "+settingsText(tr, ev.NameKey), map[string]string{"a": settingsActMsgEvent, settingsMsgEventField: ev.Code}); ok {
			cells = append(cells, button)
		}
	}
	settingsDelGrid(k, cells)
	settingsDelBackRow(k, map[string]string{"a": settingsActOpen, "p": settingsPageMessages})
	return k.screen(settingsCompose(v, "settings_page_msgs", body, notice, false))
}

// renderSettingsMsgEvent draws an event's screen (the Draw hook of its editor definition): the event's icon
// and bold name, its description, then the state of each part in words (never the stored text, a button
// name or a link) and the editor's note for a media kind that carries no caption; Back (the family) and
// Close. It is pure. Plans 20-07 to 20-09 add the Silent switch, the part rows and the previews.
func renderSettingsMsgEvent(v settingsView, fam settingsMsgFamily, ev settingsMsgEvent, st settingsEditState, opts settingsEditScreenOpts) settingsScreen {
	tr := v.Tr
	body := []string{ev.Icon + " <b>" + settingsText(tr, ev.NameKey) + "</b>", settingsText(tr, ev.DescKey), ""}
	for _, part := range settingsMsgParts {
		body = append(body, settingsText(tr, part.LineKey, i18n.TranslationParams{
			"state": settingsEditStateWord(tr, st, part.Code),
		}))
	}
	if settingsEditPartIsSet(st, settingsEditPartMedia) && (st.MediaType == db.STICKER || st.MediaType == db.VIDEO_NOTE) {
		body = append(body, settingsText(tr, "settings_edit_note_no_caption"))
	}
	k := newSettingsKeyboard(v)
	settingsDelBackRow(k, map[string]string{"a": settingsActMsgFamily, "t": fam.Code})
	return k.screen(settingsCompose(v, "settings_page_msgs", body, opts.Notice, false))
}
