package modules

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// The Messages page of the settings menu (Phase 21, D-18, D-19): the group's custom event messages, the
// texts the bot posts in the group when someone is punished. It exists only in the bot's private chat
// (PrivateOnly, so a menu opened in the group shows the settings_msg_group_note line and runs no page code)
// and not in a Staff Group (Hidden: nothing is set at Staff Group level, D-01). Everything it draws comes
// from one table of families and events: the hub lists every family that has events with each event's
// state (Default, Custom or Silent), a family's list opens an event, and an event's screen shows the state
// of its parts. Each event is one message editor definition (registerSettingsEditDef), whose wait key is
// h + the event code + the destination code and whose buttons carry the event in the field k, so plans
// 21-07 to 21-09 make the event screen act through the editor's hooks. Every read is fresh and creates
// nothing, and a preview only ever goes to the presser's own private chat.

const (
	// settingsPageMessages is the p value of the Messages page.
	settingsPageMessages = "h"
	// settingsActMsgFamily is the action code that opens a family's list (field t, a family code).
	settingsActMsgFamily = "hl"
	// settingsActMsgEvent is the action code that opens an event's screen (field k, an event code).
	settingsActMsgEvent = "he"
	// settingsActMsgSet, settingsActMsgRemove, settingsActMsgCancel, settingsActMsgPreview and
	// settingsActMsgFull are the editor codes every event definition carries: Set, Remove, the prompt's
	// Cancel, Preview of one part and Full preview, all five page actions resolved by the field k.
	settingsActMsgSet     = "hs"
	settingsActMsgRemove  = "hr"
	settingsActMsgCancel  = "hc"
	settingsActMsgPreview = "hv"
	settingsActMsgFull    = "hf"
	// settingsActMsgSilent is the action code of an event screen's Silent switch (fields k, the event code,
	// and v, the absolute target "0" or "1").
	settingsActMsgSilent = "hq"
	// settingsMsgEventField is the callback field that names the event of a press (not e, which is the
	// confirm-card expiry everywhere else in the namespace).
	settingsMsgEventField = "k"
	// settingsMsgDestGroupCode is the one-letter code of the group destination in a wait key.
	settingsMsgDestGroupCode = "g"
	// settingsMsgDestField is the callback field that names the destination of an editor press: absent for the
	// group message (so every Phase 21 button keeps its bytes) and settingsMsgDestPMCode for the private copy.
	settingsMsgDestField = "d"
	// settingsMsgDestPMCode is the one-letter code of the private copy: the only value of the field d and the
	// destination code in its wait key (hbnp for the ban's private copy).
	settingsMsgDestPMCode = "p"
	// settingsActMsgPM is the action code of the private copy screen's switch (fields k, the event code, and v,
	// the absolute target "0" or "1").
	settingsActMsgPM = "hp"
	// settingsMsgPMIcon is the icon of the private copy.
	settingsMsgPMIcon = "📩"
	// settingsActMsgPMOption is the action code of a toggle of the hub's private copies block (fields t, an option code
	// of settingsMsgPMOptions, and v, the absolute target "0" or "1").
	settingsActMsgPMOption = "ho"
	// settingsMsgSampleDuration is the duration a preview of a timed event fills in: the form the duration
	// takes in a timed reply (the command's own duration text, which is not translated).
	settingsMsgSampleDuration = "2 hours"
	// settingsMsgSampleSpan is how far ahead of the preview the sample end of a timed event lies.
	settingsMsgSampleSpan = 2 * time.Hour
)

// settingsMsgEvent is one event of a family: the eventmsg event it edits, its two-letter code (the k
// value), its icon, its name and description keys, the buttons the bot itself adds to its message (the
// Unban or Unmute row), counted against Telegram's keyboard limit, the send site's own default reply
// builder (which a preview fills in) and whether the event is timed (its preview fills {duration} and
// {until}).
type settingsMsgEvent struct {
	Key             string
	Code            string
	Icon            string
	NameKey         string
	DescKey         string
	ReservedButtons int
	Default         eventDefaultReply
	Timed           bool
}

// settingsMsgFamily is one family of events (D-19): its code (the t value of hl), icon, name, the intro of
// its list, the prompt of its events' text part (which lists the placeholders the family's events fill), the
// hint of its event screens and its events in page order. A family with no events is not drawn anywhere.
type settingsMsgFamily struct {
	Code        string
	Icon        string
	NameKey     string
	IntroKey    string
	WaitTextKey string
	HintKey     string
	Events      []settingsMsgEvent
}

// settingsMsgManualEvents are the seven manual punishment events, in the order of eventmsg.Events.
var settingsMsgManualEvents = []settingsMsgEvent{
	{
		Key: eventmsg.EventBan, Code: "bn", Icon: "🚫", NameKey: "settings_msg_ev_ban", DescKey: "settings_msg_desc_ban",
		ReservedButtons: 1, Default: banDefaultReply,
	},
	{
		Key: eventmsg.EventTimedBan, Code: "tb", Icon: "⏳", NameKey: "settings_msg_ev_tban", DescKey: "settings_msg_desc_tban",
		Default: tbanDefaultReply, Timed: true,
	},
	{
		Key: eventmsg.EventKick, Code: "kk", Icon: "👢", NameKey: "settings_msg_ev_kick", DescKey: "settings_msg_desc_kick",
		Default: kickDefaultReply,
	},
	{
		Key: eventmsg.EventMute, Code: "mu", Icon: "🔇", NameKey: "settings_msg_ev_mute", DescKey: "settings_msg_desc_mute",
		ReservedButtons: 1, Default: muteDefaultReply,
	},
	{
		Key: eventmsg.EventTimedMute, Code: "tm", Icon: "🔕", NameKey: "settings_msg_ev_tmute", DescKey: "settings_msg_desc_tmute",
		Default: tmuteDefaultReply, Timed: true,
	},
	{
		Key: eventmsg.EventUnban, Code: "ub", Icon: "🔓", NameKey: "settings_msg_ev_unban", DescKey: "settings_msg_desc_unban",
		Default: unbanDefaultReply,
	},
	{
		Key: eventmsg.EventUnmute, Code: "um", Icon: "🔊", NameKey: "settings_msg_ev_unmute", DescKey: "settings_msg_desc_unmute",
		Default: unmuteDefaultReply,
	},
}

// settingsMsgFamilies are the families of the hub in page order. Only the manual family has events now;
// later phases add the automatic, lockdown and requirement families. It is a variable only so a test can
// draw more families; production code never reassigns it.
var settingsMsgFamilies = []settingsMsgFamily{
	{
		Code: "m", Icon: "🔨", NameKey: "settings_msg_fam_manual", IntroKey: "settings_msg_fam_manual_intro",
		WaitTextKey: "settings_msg_wait_text_manual", HintKey: "settings_msg_hint_manual", Events: settingsMsgManualEvents,
	},
}

// settingsMsgTextPart is the text part of a family's event messages: the editor's text part with the
// family's own prompt, which lists the Welcome placeholders and the family's event placeholders and says
// that the default's reason line is not added to a custom text (D-11).
func settingsMsgTextPart(fam settingsMsgFamily) settingsEditPart {
	part := settingsEditTextPart
	part.WaitKey = fam.WaitTextKey
	part.RemovedKey = "settings_edit_removed_text_default"
	return part
}

// settingsMsgPartsOf are the parts of a family's event messages, in the order the event screen lists them:
// the family's text part, then the editor's media and buttons parts.
func settingsMsgPartsOf(fam settingsMsgFamily) []settingsEditPart {
	return []settingsEditPart{settingsMsgTextPart(fam), settingsEditMediaPart, settingsEditButtonsPart}
}

// settingsMsgEditCodes are the editor codes every event definition carries and the right Set and Remove
// need (can_change_info, D-22). The page's Set, Remove, Cancel, Preview and Full preview actions are built
// from them, one set of codes for every event, the event named by the field k.
var settingsMsgEditCodes = settingsEditPageDef{
	SetCode: settingsActMsgSet, RemoveCode: settingsActMsgRemove, CancelCode: settingsActMsgCancel,
	PreviewCode: settingsActMsgPreview, FullCode: settingsActMsgFull, EditRight: settingsRightChangeInfo,
}

// settingsMsgEditActions are the page's Set (hs), Remove (hr), Cancel (hc), Preview (hv) and Full preview
// (hf) actions: the editor's own, resolved by k and d to the definition of the event's group message or private
// copy (settingsMsgDefByFields, which requires the private owner field c, so a group menu's press is refused in
// step 1). Set and Remove need can_change_info; Cancel and the two previews need only the admin check every
// press makes.
func settingsMsgEditActions() map[string]settingsPageAction {
	all := settingsEditResolvedActions(settingsMsgEditCodes, []string{settingsMsgEventField, settingsMsgDestField}, settingsMsgDefByFields)
	codes := []string{settingsActMsgSet, settingsActMsgRemove, settingsActMsgCancel, settingsActMsgPreview, settingsActMsgFull}
	actions := make(map[string]settingsPageAction, len(codes))
	for _, code := range codes {
		actions[code] = all[code]
	}
	return actions
}

// settingsMsgRead reads a group's event messages fresh: the page never reads through the cache, so it
// never shows a value another replica has just changed as stale, and it creates nothing. It is a variable
// only so a test can make it fail or count its calls.
var settingsMsgRead = eventmsg.GetFresh

// settingsMsgStaffLookup reads a chat's Staff Group row fresh for the page's Hidden. It is the page's own seam
// (not the Deletion page's deletionStaffLookup, whose reads that page's tests count) and a variable only so a test
// can make it fail.
var settingsMsgStaffLookup = staff.GetStaffGroupFresh

func init() {
	actions := map[string]settingsPageAction{
		settingsActMsgFamily: {
			Fields: []string{"t"}, Right: settingsRightAdmin, Validate: settingsMsgFamilyFields, Run: settingsMsgRunFamily,
		},
		settingsActMsgEvent: {
			Fields: []string{settingsMsgEventField, settingsMsgDestField}, Right: settingsRightAdmin, Validate: settingsMsgEventFields,
			Run: settingsMsgRunEvent,
		},
		settingsActMsgSilent: {
			Fields: []string{settingsMsgEventField, "v"}, Right: settingsRightChangeInfo, Validate: settingsMsgSilentFields, Run: settingsMsgRunSilent,
		},
		settingsActMsgPM: {
			Fields: []string{settingsMsgEventField, "v"}, Right: settingsRightChangeInfo, Validate: settingsMsgPMFields, Run: settingsMsgRunPM,
		},
		settingsActMsgPMOption: {
			Fields: []string{"t", "v"}, Right: settingsRightChangeInfo, Validate: settingsMsgPMOptionFields, Run: settingsMsgRunPMOption,
		},
	}
	maps.Copy(actions, settingsMsgEditActions())
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
		Actions:        actions,
	})
	// Every event also gets the definition of its private copy. It is registered whether or not the event has a
	// private copy spec (the spec table is filled by another file's init, whose order is not relied on); a press,
	// a screen and a wait reach it only through settingsMsgDefByFields and settingsMsgPMDefOf, which require the
	// spec at the time of the press.
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			registerSettingsEditDef(settingsMsgEditDef(fam, ev))
			registerSettingsEditDef(settingsMsgPMEditDef(fam, ev))
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

// settingsMsgPMWaitPage is the key of an event's private copy definition, which is also the p of its wait: h,
// the event code and the private copy's code (hbnp for the ban's private copy).
func settingsMsgPMWaitPage(ev settingsMsgEvent) string {
	return settingsPageMessages + ev.Code + settingsMsgDestPMCode
}

// settingsMsgEditDef is the message editor definition of one event: the hooked kind, the page's title,
// the five editor codes, can_change_info to change it (D-22), the family's three parts, the field k that
// names the event on every editor button, the bot's own reserved buttons and the hooks that read, draw,
// remove, write and preview it.
func settingsMsgEditDef(fam settingsMsgFamily, ev settingsMsgEvent) settingsEditPageDef {
	def := settingsMsgEditCodes
	def.Key, def.Kind, def.Icon, def.TitleKey = settingsMsgWaitPage(ev), settingsEditEvent, ev.Icon, "settings_page_msgs"
	def.Parts = settingsMsgPartsOf(fam)
	def.Fields = map[string]string{settingsMsgEventField: ev.Code}
	def.ReservedButtons = ev.ReservedButtons
	def.Hooks = &settingsEditHooks{
		Load: settingsMsgLoad(ev, eventmsg.DestGroup), Draw: settingsMsgDraw(fam, ev),
		Remove: settingsMsgRemove(ev, eventmsg.DestGroup), Write: settingsMsgWrite(ev, eventmsg.DestGroup),
		Preview: settingsMsgPreview(ev), PreviewHeaderKey: "settings_msg_preview_header",
	}
	return def
}

// settingsMsgPMEditDef is the message editor definition of one event's private copy (D-07, D-23): the hooked
// kind, the private copy's title (so the prompt says it is for the private copy), its icon, the same five editor
// codes and can_change_info as the group message, the family's three parts, the fields k and d=p on every editor
// button (Set, Remove, the previews and the Cancel of the prompt and of a refusal), no reserved buttons (a
// private copy never carries the bot's Unban or Unmute row) and the hooks that read, draw, remove, write and preview
// the pm row (the previews through composeEventPM, the send path's composer, under their own header line).
func settingsMsgPMEditDef(fam settingsMsgFamily, ev settingsMsgEvent) settingsEditPageDef {
	def := settingsMsgEditCodes
	def.Key, def.Kind, def.Icon, def.TitleKey = settingsMsgPMWaitPage(ev), settingsEditEvent, settingsMsgPMIcon, "settings_msg_pm_title"
	def.Parts = settingsMsgPartsOf(fam)
	def.Fields = map[string]string{settingsMsgEventField: ev.Code, settingsMsgDestField: settingsMsgDestPMCode}
	def.Hooks = &settingsEditHooks{
		Load: settingsMsgLoad(ev, eventmsg.DestPM), Draw: settingsMsgPMDraw(fam, ev),
		Remove: settingsMsgRemove(ev, eventmsg.DestPM), Write: settingsMsgWrite(ev, eventmsg.DestPM),
		Preview: settingsMsgPMPreview(ev), PreviewHeaderKey: "settings_msg_preview_header_pm",
	}
	return def
}

// settingsMsgPMDefOf is the private copy definition of the event named by code and the event: false for an
// unknown event and for an event with no private copy spec (eventPMSpecFor).
func settingsMsgPMDefOf(code string) (settingsEditPageDef, settingsMsgEvent, bool) {
	_, ev, ok := settingsMsgEventByCode(code)
	if !ok {
		return settingsEditPageDef{}, settingsMsgEvent{}, false
	}
	if _, spec := eventPMSpecFor(ev.Key); !spec {
		return settingsEditPageDef{}, settingsMsgEvent{}, false
	}
	def, ok := settingsEditPageByKey(settingsMsgPMWaitPage(ev))
	return def, ev, ok
}

// settingsMsgDefByFields is the definition a press names by k and d: the event's group message without d, its
// private copy with d=p when the event has a private copy spec. It is false for a press without the private owner
// field c (a group menu's press, refused in step 1 before any lookup), for an unknown event, for any other d and
// for d=p on an event with no private copy.
func settingsMsgDefByFields(fields map[string]string) (settingsEditPageDef, bool) {
	if !settingsListPrivateField(fields) {
		return settingsEditPageDef{}, false
	}
	code := fields[settingsMsgEventField]
	dest, hasDest := fields[settingsMsgDestField]
	switch {
	case hasDest && dest == settingsMsgDestPMCode:
		def, _, ok := settingsMsgPMDefOf(code)
		return def, ok
	case hasDest:
		return settingsEditPageDef{}, false
	}
	_, ev, ok := settingsMsgEventByCode(code)
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

// settingsMsgSilentFields is the Validate of hq: the private owner field, a known event code and a target
// value of exactly "0" or "1".
func settingsMsgSilentFields(fields map[string]string) bool {
	return settingsMsgEventFields(fields) && settingsEditSwitchValue(fields["v"])
}

// settingsMsgPMFields is the Validate of hp: the private owner field, an event with a private copy and a target
// value of exactly "0" or "1".
func settingsMsgPMFields(fields map[string]string) bool {
	if !settingsListPrivateField(fields) || !settingsEditSwitchValue(fields["v"]) {
		return false
	}
	_, _, ok := settingsMsgPMDefOf(fields[settingsMsgEventField])
	return ok
}

// settingsMsgPMOption is one of the group's private copy options on the hub (D-24): its code (the t value of ho), the
// key of its line and of its toggle's label (both take {state}), how it reads from the stored options and its one write
// path. A later option is one more row of settingsMsgPMOptions and needs no renderer change.
type settingsMsgPMOption struct {
	Code     string
	LineKey  string
	LabelKey string
	Value    func(eventmsg.Options) bool
	Apply    func(ctx context.Context, chatID int64, on bool, by int64) eventMsgOptionsResult
}

// settingsMsgPMOptions are the options of the hub's private copies block, in their order: the Details button, then the
// log line.
var settingsMsgPMOptions = []settingsMsgPMOption{
	{
		Code: "d", LineKey: "settings_msg_line_pm_details", LabelKey: "settings_btn_msg_pm_details",
		Value: func(o eventmsg.Options) bool { return o.PMDetails }, Apply: applyEventPMDetails,
	},
	{
		Code: "l", LineKey: "settings_msg_line_pm_log", LabelKey: "settings_btn_msg_pm_log",
		Value: func(o eventmsg.Options) bool { return o.PMLog }, Apply: applyEventPMLog,
	},
}

// settingsMsgPMOptionByCode finds an option of the private copies block.
func settingsMsgPMOptionByCode(code string) (settingsMsgPMOption, bool) {
	for _, opt := range settingsMsgPMOptions {
		if opt.Code == code {
			return opt, true
		}
	}
	return settingsMsgPMOption{}, false
}

// settingsMsgPMOptionFields is the Validate of ho: the private owner field, a known option code and a target value of
// exactly "0" or "1".
func settingsMsgPMOptionFields(fields map[string]string) bool {
	if !settingsListPrivateField(fields) || !settingsEditSwitchValue(fields["v"]) {
		return false
	}
	_, ok := settingsMsgPMOptionByCode(fields["t"])
	return ok
}

// settingsMsgAnyPM reports whether some event of the page has a private copy spec, so the hub's private copies block
// has something to set.
func settingsMsgAnyPM() bool {
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			if _, ok := eventPMSpecFor(ev.Key); ok {
				return true
			}
		}
	}
	return false
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

// settingsMsgPMOn reports whether ev's private copy is switched on: the event has a private copy spec and its pm row,
// resolved as the send path resolves it (no row is a copy that is off), is not Silent.
func settingsMsgPMOn(msgs *eventmsg.Messages, ev settingsMsgEvent) bool {
	if _, spec := eventPMSpecFor(ev.Key); !spec {
		return false
	}
	return !msgs.Resolve(ev.Key, eventmsg.DestPM).Silent
}

// settingsMsgCountChanged counts the events of every family whose group message is not Default (Custom or Silent) or
// whose private copy is on (D-24), each event once.
func settingsMsgCountChanged(msgs *eventmsg.Messages) int {
	n := 0
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			if settingsMsgStateOf(msgs, ev) != settingsMsgDefault || settingsMsgPMOn(msgs, ev) {
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
// when no event is customised and no private copy is on, "{n} custom" otherwise (settingsMsgCountChanged), the
// unknown mark when the read failed. It is called only in a private menu (PrivateOnly).
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

// settingsMsgRunSilent serves hq, the Silent switch of an event screen: it writes the absolute target
// through applyEventSilent, the switch's one write path, with the presser as the writer, and redraws the
// screen from the stored messages it read back, with no notice (a stale screen's press of the stored value
// writes the same value again). A failed write redraws the stored state with the save-failed notice; a
// write whose read-back failed reloads the screen.
func settingsMsgRunSilent(p *settingsPress) *settingsScreen {
	def, ok := settingsMsgDefByFields(p.fields)
	_, ev, evOK := settingsMsgEventByCode(p.fields[settingsMsgEventField])
	if !ok || !evOK || !settingsEditSwitchValue(p.fields["v"]) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	res := applyEventSilent(ctx, p.group.Id, ev.Key, p.fields["v"] == "1", p.query.From.Id)
	return settingsMsgRedrawApplied(p, def, ev, eventmsg.DestGroup, res)
}

// settingsMsgRunPM serves hp, the switch of a private copy screen: it writes the absolute target through
// applyEventPM, the switch's one write path (the pm row's Silent), with the presser as the writer, and redraws
// the private copy screen from the stored messages it read back, with no notice (a stale screen's press of the
// stored value writes the same value again). A failed write redraws the stored state with the save-failed
// notice; a write whose read-back failed reloads the screen.
func settingsMsgRunPM(p *settingsPress) *settingsScreen {
	def, ev, ok := settingsMsgPMDefOf(p.fields[settingsMsgEventField])
	if !ok || !settingsEditSwitchValue(p.fields["v"]) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	res := applyEventPM(ctx, p.group.Id, ev.Key, p.fields["v"] == "1", p.query.From.Id)
	return settingsMsgRedrawApplied(p, def, ev, eventmsg.DestPM, res)
}

// settingsMsgRunPMOption serves ho, a toggle of the hub's private copies block: it writes the absolute target through the
// option's Apply, its one write path, with the presser as the writer (no same-value shortcut: the value is absolute, so a
// stale hub's press stores the same value again), and redraws the hub from the messages the write read back, with no
// notice. A failed write redraws the hub from a fresh read with the save-failed notice; a write whose read-back failed
// reloads the hub. A read that fails is the cannot-open edit.
func settingsMsgRunPMOption(p *settingsPress) *settingsScreen {
	opt, ok := settingsMsgPMOptionByCode(p.fields["t"])
	if !ok || !settingsEditSwitchValue(p.fields["v"]) {
		p.answer(settingsText(p.tr, "common_callback_invalid_request"), true)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	res := opt.Apply(ctx, p.group.Id, p.fields["v"] == "1", p.query.From.Id)
	if res.Messages != nil {
		screen := renderSettingsMsgHub(p.view(), res.Messages, "")
		return &screen
	}
	notice := ""
	if res.Outcome == greetingSaveFailed {
		notice = settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed"))
	}
	screen, err := settingsMsgOpen(p, notice)
	if err != nil {
		log.Warnf("[Settings] the event messages of chat %d could not be read: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	return &screen
}

// settingsMsgRedrawApplied redraws def's screen after a switch write: from the messages the write read back,
// reloaded when the read-back is missing, or the stored state with the save-failed notice when the write failed.
func settingsMsgRedrawApplied(p *settingsPress, def settingsEditPageDef, ev settingsMsgEvent, dest string, res eventMsgApplyResult) *settingsScreen {
	switch {
	case res.Outcome == greetingSaveFailed:
		return settingsEditRedraw(p, def, settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed")))
	case res.Messages == nil:
		return settingsEditRedraw(p, def, "")
	}
	screen := settingsEditDraw(p.view(), def, settingsMsgStateFor(res.Messages, ev, dest), settingsEditScreenOpts{
		BotUsername: getBotUsername(p.b),
	})
	return &screen
}

// settingsMsgStateFromMessage is the editor state of one stored event message; the zero Message (no row)
// reads as the default text with no media, no buttons and Silent off.
func settingsMsgStateFromMessage(msg eventmsg.Message) settingsEditState {
	return settingsEditState{
		Kind: settingsEditEvent, Text: msg.Text, TextIsDefault: msg.Text == "",
		MediaType: msg.MediaType, FileID: msg.FileID, Buttons: models.ButtonArray(msg.Buttons),
		Silent: msg.Silent,
	}
}

// settingsMsgStateFor is the editor state of one event's message for a destination, as the send path reads it
// (Resolve: no group row is the default message with Silent off, no pm row a private copy that is off). The
// group message's state also carries PMOn, whether the event's private copy is switched on, for an event with a
// private copy spec, so the event screen's line can say it after any redraw.
func settingsMsgStateFor(msgs *eventmsg.Messages, ev settingsMsgEvent, dest string) settingsEditState {
	st := settingsMsgStateFromMessage(msgs.Resolve(ev.Key, dest))
	if dest == eventmsg.DestGroup {
		st.PMOn = settingsMsgPMOn(msgs, ev)
	}
	return st
}

// settingsMsgLoad is an event definition's Load hook for a destination: the state of the event's message from a
// fresh read, creating nothing (settingsMsgStateFor).
func settingsMsgLoad(ev settingsMsgEvent, dest string) func(context.Context, int64) (settingsEditState, error) {
	return func(ctx context.Context, chatID int64) (settingsEditState, error) {
		msgs, err := settingsMsgRead(ctx, chatID)
		if err != nil {
			return settingsEditState{}, err
		}
		return settingsMsgStateFor(msgs, ev, dest), nil
	}
}

// settingsMsgDraw is an event definition's Draw hook.
func settingsMsgDraw(fam settingsMsgFamily, ev settingsMsgEvent) func(settingsView, settingsEditPageDef, settingsEditState, settingsEditScreenOpts) settingsScreen {
	return func(v settingsView, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen {
		return renderSettingsMsgEvent(v, fam, ev, def, st, opts)
	}
}

// settingsMsgPMDraw is a private copy definition's Draw hook.
func settingsMsgPMDraw(fam settingsMsgFamily, ev settingsMsgEvent) func(settingsView, settingsEditPageDef, settingsEditState, settingsEditScreenOpts) settingsScreen {
	return func(v settingsView, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen {
		return renderSettingsMsgPM(v, fam, ev, def, st, opts)
	}
}

// settingsMsgApplied is the outcome and the editor state of an event message apply function's result for a
// destination: nil when the write failed or its read-back did (the caller reloads). The state is built from the
// whole fresh read (settingsMsgStateFor), so the group message's private copy line survives a Remove.
func settingsMsgApplied(res eventMsgApplyResult, ev settingsMsgEvent, dest string) (greetingApplyOutcome, *settingsEditState) {
	if res.Outcome == greetingSaveFailed || res.Messages == nil {
		return res.Outcome, nil
	}
	st := settingsMsgStateFor(res.Messages, ev, dest)
	return res.Outcome, &st
}

// settingsMsgRemove is an event definition's Remove hook for a destination: it clears one part through its apply
// function (an empty text, no file and type 0, no buttons) for the presser by and returns the state read back. A
// group row with nothing left and Silent off reads as Default, so the send path sends today's reply; a private
// copy keeps its switch, so with it on and nothing left the default private text is sent.
func settingsMsgRemove(ev settingsMsgEvent, dest string) func(context.Context, int64, string, int64) (greetingApplyOutcome, *settingsEditState) {
	return func(ctx context.Context, chatID int64, part string, by int64) (greetingApplyOutcome, *settingsEditState) {
		switch part {
		case settingsEditPartText:
			return settingsMsgApplied(applyEventText(ctx, chatID, ev.Key, dest, "", by), ev, dest)
		case settingsEditPartMedia:
			return settingsMsgApplied(applyEventMedia(ctx, chatID, ev.Key, dest, "", 0, by), ev, dest)
		case settingsEditPartButtons:
			return settingsMsgApplied(applyEventButtons(ctx, chatID, ev.Key, dest, nil, by), ev, dest)
		}
		return greetingSaveFailed, nil
	}
}

// settingsMsgWrite is an event definition's Write hook for a destination: it stores the part the editor's check
// accepted (the converted text, the media's file ID and type, or the parsed buttons) through the part's apply
// function, with the person who typed it as the writer, and reports whether it was stored. Storing a part of a
// private copy never turns it on.
func settingsMsgWrite(ev settingsMsgEvent, dest string) func(context.Context, int64, string, settingsEditVerdict, int64) bool {
	return func(ctx context.Context, chatID int64, part string, verdict settingsEditVerdict, by int64) bool {
		var res eventMsgApplyResult
		switch part {
		case settingsEditPartText:
			res = applyEventText(ctx, chatID, ev.Key, dest, verdict.Text, by)
		case settingsEditPartMedia:
			res = applyEventMedia(ctx, chatID, ev.Key, dest, verdict.FileID, verdict.DataType, by)
		case settingsEditPartButtons:
			res = applyEventButtons(ctx, chatID, ev.Key, dest, verdict.Buttons, by)
		default:
			return false
		}
		return res.Outcome != greetingSaveFailed
	}
}

// settingsMsgPreviewReply is the reply a preview of ev builds, as the send site builds it for a real command:
// the presser is the punished member, the reason is the sample reason in the group's language and, for a timed
// event, the duration is settingsMsgSampleDuration and the end settingsMsgSampleSpan from now; the default text
// is the event's own builder in the group's language. The reply carries no options, so composeEventMessage adds
// no bot row: the Unban and Unmute buttons are live callbacks that must never reach a private chat.
func settingsMsgPreviewReply(p *settingsPress, groupTr *i18n.Translator, ev settingsMsgEvent) eventReply {
	target := p.query.From
	values := eventValues{Reason: settingsText(groupTr, "settings_msg_sample_reason")}
	if ev.Timed {
		values.Duration, values.Until = settingsMsgSampleDuration, time.Now().Add(settingsMsgSampleSpan)
	}
	text, _ := ev.Default(groupTr, eventDefaultInput{Target: target, Reason: values.Reason, TimeVal: values.Duration})
	return eventReply{Event: ev.Key, Target: target, Values: values, Default: text}
}

// settingsMsgPreview is an event definition's Preview hook (D-20). It sends to the presser's private chat only,
// writes nothing, schedules nothing and ignores Silent: it shows the message the event would post. Full
// preview is built by composeEventMessage, the send path's own composer, from settingsMsgPreviewReply and the
// screen's stored parts, filled with the presser as the member and the admin, in the group's language, and
// sent by media.SendGreetingMessage; Preview text is the stored text filled the same way with no keyboard;
// Preview media and Preview buttons are the editor's own (the media alone, the stored buttons alone). A
// {rules} token reads the group's rules as the send path does.
func settingsMsgPreview(ev settingsMsgEvent) func(*settingsPress, settingsEditPageDef, settingsEditState, string) error {
	return func(p *settingsPress, _ settingsEditPageDef, st settingsEditState, part string) error {
		switch part {
		case settingsEditPartMedia:
			return settingsEditPreviewMediaAlone(p, st)
		case settingsEditPartButtons:
			return settingsEditPreviewButtonsAlone(p, &gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard.BuildKeyboard(st.Buttons)})
		case "", settingsEditPartText:
		default:
			return errors.New("settings messages: no such part to preview")
		}
		groupTr := settingsMsgGroupTranslator(p)
		r := settingsMsgPreviewReply(p, groupTr, ev)
		presser := p.query.From
		actor := eventActor{User: &presser}
		if part == settingsEditPartText {
			values := eventPlaceholderValues(groupTr, p.group.Id, actor, r.Values)
			text, _ := renderEventText(p.b, p.group, &r.Target, st.Text, nil, values)
			_, err := media.SendGreeting(p.b, p.query.From.Id, text, "", db.TEXT, &gotgbot.InlineKeyboardMarkup{}, 0)
			return err
		}
		row := eventmsg.Message{
			Event: ev.Key, Dest: eventmsg.DestGroup, Text: st.Text, FileID: st.FileID, MediaType: st.MediaType, Buttons: st.Buttons,
		}
		_, err := media.SendGreetingMessage(p.b, p.query.From.Id, composeEventMessage(p.b, p.group, groupTr, r, row, actor))
		return err
	}
}

// settingsMsgGroupTranslator is the translator of the group's language, read by chat ID under the settings read
// timeout as the send path reads it: a preview is in the language the group's members read, while its header line
// stays in the presser's.
func settingsMsgGroupTranslator(p *settingsPress) *i18n.Translator {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()
	return i18n.MustNewTranslator(lang.GetGroupLanguage(ctx, p.group.Id))
}

// errSettingsMsgNoPMSpec is the error of a private copy preview of an event that has no private copy spec, which
// settingsMsgDefByFields already refuses in step 1, so no press reaches it.
var errSettingsMsgNoPMSpec = errors.New("settings messages: the event has no private copy")

// settingsMsgPMPreview is a private copy definition's Preview hook (D-27). It sends to the presser's private chat
// only, ignores the copy's switch, writes nothing, schedules nothing and never goes through offerEventPM: it shows
// what the punished person would receive. Full preview is built by composeEventPM, the send path's own composer,
// from settingsMsgPreviewReply (the presser as the member and the admin, the sample reason and, for a timed event,
// the sample duration and end, in the group's language) and the screen's stored parts, with no lockdown note, and
// sent by media.SendGreetingMessage; with no text stored it is the event's default private text. Preview text is the
// stored text composed the same way and sent alone with no keyboard; Preview media and Preview buttons are the
// editor's own (the media alone, the stored buttons alone). The keyboard is the stored buttons and the {rules}
// button only, never the group's Unban or Unmute row.
func settingsMsgPMPreview(ev settingsMsgEvent) func(*settingsPress, settingsEditPageDef, settingsEditState, string) error {
	return func(p *settingsPress, _ settingsEditPageDef, st settingsEditState, part string) error {
		spec, ok := eventPMSpecFor(ev.Key)
		if !ok {
			return errSettingsMsgNoPMSpec
		}
		row := eventmsg.Message{Event: ev.Key, Dest: eventmsg.DestPM, Text: st.Text}
		switch part {
		case settingsEditPartMedia:
			return settingsEditPreviewMediaAlone(p, st)
		case settingsEditPartButtons:
			return settingsEditPreviewButtonsAlone(p, &gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard.BuildKeyboard(st.Buttons)})
		case "":
			row.FileID, row.MediaType, row.Buttons = st.FileID, st.MediaType, st.Buttons
		case settingsEditPartText:
		default:
			return errors.New("settings messages: no such part to preview")
		}
		groupTr := settingsMsgGroupTranslator(p)
		presser := p.query.From
		msg := composeEventPM(p.b, p.group, groupTr, spec, settingsMsgPreviewReply(p, groupTr, ev), row, eventActor{User: &presser})
		if part == settingsEditPartText {
			_, err := media.SendGreeting(p.b, presser.Id, msg.Text, "", db.TEXT, &gotgbot.InlineKeyboardMarkup{}, 0)
			return err
		}
		_, err := media.SendGreetingMessage(p.b, presser.Id, msg)
		return err
	}
}

// settingsMsgPMSuffix is the code-side mark an event line carries while its private copy is on (D-24): an icon and no
// words, so it needs no string.
const settingsMsgPMSuffix = " · " + settingsMsgPMIcon

// settingsMsgEventLines is the state lines of a family's events: event icon, name and the group message's state word,
// then settingsMsgPMSuffix while the event's private copy is on.
func settingsMsgEventLines(tr *i18n.Translator, fam settingsMsgFamily, msgs *eventmsg.Messages) []string {
	lines := make([]string, 0, len(fam.Events))
	for _, ev := range fam.Events {
		line := settingsText(tr, "settings_main_state_line", i18n.TranslationParams{
			"icon": ev.Icon, "page": settingsText(tr, ev.NameKey), "state": settingsMsgStateWord(tr, settingsMsgStateOf(msgs, ev)),
		})
		if settingsMsgPMOn(msgs, ev) {
			line += settingsMsgPMSuffix
		}
		lines = append(lines, line)
	}
	return lines
}

// renderSettingsMsgHub draws the hub: the intro, then for each family with events a blank line, its icon
// and bold name and one state line per event, then, while some event has a private copy, the private copies block
// (D-24): a blank line, the bold private copy title with its icon, one line per option of settingsMsgPMOptions with
// its state word, a blank line and the options hints (the Details button's, then the log line's). The keyboard is one
// cell per family with events, two to a row, then one full-width toggle per option (the state icon, sending the
// opposite of the stored value as an absolute target, a=ho), then Back (Main) and Close: [1 1 1 2] with the one family
// and the two options. It is pure.
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
	if settingsMsgAnyPM() {
		body = append(body, "", settingsMsgPMIcon+" <b>"+settingsText(tr, "settings_msg_pm_title")+"</b>")
		for _, opt := range settingsMsgPMOptions {
			on := opt.Value(msgs.Options)
			body = append(body, settingsText(tr, opt.LineKey, i18n.TranslationParams{"state": settingsStateWord(tr, on)}))
			target := "1"
			if on {
				target = "0"
			}
			label := settingsText(tr, opt.LabelKey, i18n.TranslationParams{"state": settingsStateIcon(on)})
			if button, ok := k.button(label, map[string]string{"a": settingsActMsgPMOption, "t": opt.Code, "v": target}); ok {
				k.row(button)
			}
		}
		body = append(body, "", settingsText(tr, "settings_msg_hint_pm_options"), settingsText(tr, "settings_msg_hint_pm_log"))
	}
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

// settingsMsgPartLines is the state of each part of def in words (never the stored text, a button name or a
// link), then the editor's note for a media kind that carries no caption.
func settingsMsgPartLines(tr *i18n.Translator, def settingsEditPageDef, st settingsEditState) []string {
	lines := make([]string, 0, len(def.Parts)+1)
	for _, part := range def.Parts {
		lines = append(lines, settingsText(tr, part.LineKey, i18n.TranslationParams{
			"state": settingsEditStateWord(tr, st, part.Code),
		}))
	}
	if settingsEditPartIsSet(st, settingsEditPartMedia) && (st.MediaType == db.STICKER || st.MediaType == db.VIDEO_NOTE) {
		lines = append(lines, settingsText(tr, "settings_edit_note_no_caption"))
	}
	return lines
}

// settingsMsgSwitchRow adds a full-width switch of an event's screen: the label key with the state icon of on,
// pressing action with the event code and the opposite of on as the absolute target.
func settingsMsgSwitchRow(k *settingsKeyboard, labelKey, action string, ev settingsMsgEvent, on bool) {
	target := "1"
	if on {
		target = "0"
	}
	label := settingsText(k.view.Tr, labelKey, i18n.TranslationParams{"state": settingsStateIcon(on)})
	if button, ok := k.button(label, map[string]string{"a": action, settingsMsgEventField: ev.Code, "v": target}); ok {
		k.row(button)
	}
}

// renderSettingsMsgEvent draws an event's screen (the Draw hook of its editor definition): the event's icon
// and bold name, its description, then the state of each part in words (never the stored text, a button
// name or a link), the editor's note for a media kind that carries no caption, the Silent line, for an event
// with a private copy the private copy line (On or Off, D-22), and, after a blank line each, the family's hint
// and the Silent hint; the keyboard opens with the editor's part rows ([Set] while a part is not set,
// [Set | Remove] while it is, each button carrying k), then the editor's preview rows (Preview of each part
// that is set, then Full preview, two to a row), then the full-width Silent toggle (the state icon, sending the
// opposite of the stored state as an absolute target), then, for an event with a private copy, the full-width
// row that opens the private copy's screen (a=he, k and d=p), then Back (the family) and Close: at most eight
// rows, [2 2 2 2 2 1 1 2]. It is pure.
func renderSettingsMsgEvent(
	v settingsView, fam settingsMsgFamily, ev settingsMsgEvent, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts,
) settingsScreen {
	tr := v.Tr
	_, hasPM := eventPMSpecFor(ev.Key)
	body := []string{ev.Icon + " <b>" + settingsText(tr, ev.NameKey) + "</b>", settingsText(tr, ev.DescKey), ""}
	body = append(body, settingsMsgPartLines(tr, def, st)...)
	body = append(body, settingsText(tr, "settings_msg_line_silent", i18n.TranslationParams{"state": settingsStateWord(tr, st.Silent)}))
	if hasPM {
		body = append(body, settingsText(tr, "settings_msg_line_pm", i18n.TranslationParams{"state": settingsStateWord(tr, st.PMOn)}))
	}
	body = append(body,
		"", settingsText(tr, fam.HintKey),
		"", settingsText(tr, "settings_msg_hint_silent"))
	k := newSettingsKeyboard(v)
	settingsEditPartRows(k, def, st)
	settingsEditPreviewRows(k, def, st)
	settingsMsgSwitchRow(k, "settings_btn_msg_silent", settingsActMsgSilent, ev, st.Silent)
	if hasPM {
		open := map[string]string{"a": settingsActMsgEvent, settingsMsgEventField: ev.Code, settingsMsgDestField: settingsMsgDestPMCode}
		if button, ok := k.button(settingsText(tr, "settings_btn_msg_pm_open"), open); ok {
			k.row(button)
		}
	}
	settingsDelBackRow(k, map[string]string{"a": settingsActMsgFamily, "t": fam.Code})
	return k.screen(settingsCompose(v, "settings_page_msgs", body, opts.Notice, false))
}

// renderSettingsMsgPM draws an event's private copy screen (the Draw hook of its private copy definition, D-23):
// the event's icon and bold name, the private copy's intro, a blank line, the state of each part in words (the
// text reads Default while no text is stored: the default private text is then sent), the no-caption note, the
// private copy line (On while the pm row's Silent is false) and, after a blank line each, the family's hint and
// the private copy hint; the keyboard is the editor's part rows (each button carrying k and d=p through the
// definition's Fields), the editor's preview rows (Preview of each part that is set, then Full preview, two to a
// row, each carrying k and d=p), the full-width switch (the state icon, sending the opposite of the stored state as
// an absolute target, a=hp), then Back (the event's screen) and Close: [1 1 1 1 1 2] with nothing stored and at most
// [2 2 2 2 2 1 2]. It is pure.
func renderSettingsMsgPM(
	v settingsView, fam settingsMsgFamily, ev settingsMsgEvent, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts,
) settingsScreen {
	tr := v.Tr
	on := !st.Silent
	body := []string{ev.Icon + " <b>" + settingsText(tr, ev.NameKey) + "</b>", settingsText(tr, "settings_msg_pm_intro"), ""}
	body = append(body, settingsMsgPartLines(tr, def, st)...)
	body = append(body,
		settingsText(tr, "settings_msg_line_pm", i18n.TranslationParams{"state": settingsStateWord(tr, on)}),
		"", settingsText(tr, fam.HintKey),
		"", settingsText(tr, "settings_msg_hint_pm"))
	k := newSettingsKeyboard(v)
	settingsEditPartRows(k, def, st)
	settingsEditPreviewRows(k, def, st)
	settingsMsgSwitchRow(k, "settings_btn_msg_pm", settingsActMsgPM, ev, on)
	settingsDelBackRow(k, map[string]string{"a": settingsActMsgEvent, settingsMsgEventField: ev.Code})
	return k.screen(settingsCompose(v, def.TitleKey, body, opts.Notice, false))
}
