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

// The Messages page of the settings menu (Phase 20, D-18, D-19): the group's custom event messages, the
// texts the bot posts in the group when someone is punished. It exists only in the bot's private chat
// (PrivateOnly, so a menu opened in the group shows the settings_msg_group_note line and runs no page code)
// and not in a Staff Group (Hidden: nothing is set at Staff Group level, D-01). Everything it draws comes
// from one table of families and events: the hub lists every family that has events with each event's
// state (Default, Custom or Silent), a family's list opens an event, and an event's screen shows the state
// of its parts. Each event is one message editor definition (registerSettingsEditDef), whose wait key is
// h + the event code + the destination code and whose buttons carry the event in the field k, so plans
// 20-07 to 20-09 make the event screen act through the editor's hooks. Every read is fresh and creates
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
// (hf) actions: the editor's own, resolved by k to the event's definition (settingsMsgDefByFields, which
// requires the private owner field c, so a group menu's press is refused in step 1). Set and Remove need
// can_change_info; Cancel and the two previews need only the admin check every press makes.
func settingsMsgEditActions() map[string]settingsPageAction {
	all := settingsEditResolvedActions(settingsMsgEditCodes, []string{settingsMsgEventField}, settingsMsgDefByFields)
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
			Fields: []string{settingsMsgEventField}, Right: settingsRightAdmin, Validate: settingsMsgEventFields, Run: settingsMsgRunEvent,
		},
		settingsActMsgSilent: {
			Fields: []string{settingsMsgEventField, "v"}, Right: settingsRightChangeInfo, Validate: settingsMsgSilentFields, Run: settingsMsgRunSilent,
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
		Load: settingsMsgLoad(ev), Draw: settingsMsgDraw(fam, ev),
		Remove: settingsMsgRemove(ev), Write: settingsMsgWrite(ev),
		Preview: settingsMsgPreview(ev), PreviewHeaderKey: "settings_msg_preview_header",
	}
	return def
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

// settingsMsgSilentFields is the Validate of hq: the private owner field, a known event code and a target
// value of exactly "0" or "1".
func settingsMsgSilentFields(fields map[string]string) bool {
	return settingsMsgEventFields(fields) && settingsEditSwitchValue(fields["v"])
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

// settingsMsgRunSilent serves hq, the Silent switch of an event screen: it writes the absolute target
// through applyEventSilent, the switch's one write path, with the presser as the writer, and redraws the
// screen from the stored message it read back, with no notice (a stale screen's press of the stored value
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
	switch {
	case res.Outcome == greetingSaveFailed:
		return settingsEditRedraw(p, def, settingsNoticeWarn(settingsText(p.tr, "common_settings_save_failed")))
	case res.Row == nil:
		return settingsEditRedraw(p, def, "")
	}
	screen := settingsEditDraw(p.view(), def, settingsMsgStateFromMessage(*res.Row), settingsEditScreenOpts{
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

// settingsMsgLoad is an event definition's Load hook: the state of the event's group message from a fresh
// read, creating nothing. No row reads as the default text with no media, no buttons and Silent off.
func settingsMsgLoad(ev settingsMsgEvent) func(context.Context, int64) (settingsEditState, error) {
	return func(ctx context.Context, chatID int64) (settingsEditState, error) {
		msgs, err := settingsMsgRead(ctx, chatID)
		if err != nil {
			return settingsEditState{}, err
		}
		msg, _ := msgs.Lookup(ev.Key, eventmsg.DestGroup)
		return settingsMsgStateFromMessage(msg), nil
	}
}

// settingsMsgDraw is an event definition's Draw hook.
func settingsMsgDraw(fam settingsMsgFamily, ev settingsMsgEvent) func(settingsView, settingsEditPageDef, settingsEditState, settingsEditScreenOpts) settingsScreen {
	return func(v settingsView, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen {
		return renderSettingsMsgEvent(v, fam, ev, def, st, opts)
	}
}

// settingsMsgApplied is the outcome and the editor state of an event message apply function's result:
// nil when the write failed or its read-back did (the caller reloads). A write that left no row is drawn
// as the default message.
func settingsMsgApplied(res eventMsgApplyResult) (greetingApplyOutcome, *settingsEditState) {
	if res.Outcome == greetingSaveFailed || res.Row == nil {
		return res.Outcome, nil
	}
	st := settingsMsgStateFromMessage(*res.Row)
	return res.Outcome, &st
}

// settingsMsgRemove is an event definition's Remove hook: it clears one part through its apply function
// (an empty text, no file and type 0, no buttons) for the presser by and returns the state read back. A
// row with nothing left and Silent off reads as Default, so the send path sends today's reply.
func settingsMsgRemove(ev settingsMsgEvent) func(context.Context, int64, string, int64) (greetingApplyOutcome, *settingsEditState) {
	return func(ctx context.Context, chatID int64, part string, by int64) (greetingApplyOutcome, *settingsEditState) {
		switch part {
		case settingsEditPartText:
			return settingsMsgApplied(applyEventText(ctx, chatID, ev.Key, "", by))
		case settingsEditPartMedia:
			return settingsMsgApplied(applyEventMedia(ctx, chatID, ev.Key, "", 0, by))
		case settingsEditPartButtons:
			return settingsMsgApplied(applyEventButtons(ctx, chatID, ev.Key, nil, by))
		}
		return greetingSaveFailed, nil
	}
}

// settingsMsgWrite is an event definition's Write hook: it stores the part the editor's check accepted
// (the converted text, the media's file ID and type, or the parsed buttons) through the part's apply
// function, with the person who typed it as the writer, and reports whether it was stored.
func settingsMsgWrite(ev settingsMsgEvent) func(context.Context, int64, string, settingsEditVerdict, int64) bool {
	return func(ctx context.Context, chatID int64, part string, verdict settingsEditVerdict, by int64) bool {
		var res eventMsgApplyResult
		switch part {
		case settingsEditPartText:
			res = applyEventText(ctx, chatID, ev.Key, verdict.Text, by)
		case settingsEditPartMedia:
			res = applyEventMedia(ctx, chatID, ev.Key, verdict.FileID, verdict.DataType, by)
		case settingsEditPartButtons:
			res = applyEventButtons(ctx, chatID, ev.Key, verdict.Buttons, by)
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
		ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
		groupTr := i18n.MustNewTranslator(lang.GetGroupLanguage(ctx, p.group.Id))
		cancel()
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
// name or a link), the editor's note for a media kind that carries no caption, the Silent line and, after a
// blank line each, the family's hint and the Silent hint; the keyboard opens with the editor's part rows
// ([Set] while a part is not set, [Set | Remove] while it is, each button carrying k), then the editor's
// preview rows (Preview of each part that is set, then Full preview, two to a row), then the full-width
// Silent toggle (the state icon, sending the opposite of the stored state as an absolute target), then Back
// (the family) and Close: at most seven rows, [2 2 2 2 2 1 2]. It is pure.
func renderSettingsMsgEvent(
	v settingsView, fam settingsMsgFamily, ev settingsMsgEvent, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts,
) settingsScreen {
	tr := v.Tr
	body := []string{ev.Icon + " <b>" + settingsText(tr, ev.NameKey) + "</b>", settingsText(tr, ev.DescKey), ""}
	for _, part := range def.Parts {
		body = append(body, settingsText(tr, part.LineKey, i18n.TranslationParams{
			"state": settingsEditStateWord(tr, st, part.Code),
		}))
	}
	if settingsEditPartIsSet(st, settingsEditPartMedia) && (st.MediaType == db.STICKER || st.MediaType == db.VIDEO_NOTE) {
		body = append(body, settingsText(tr, "settings_edit_note_no_caption"))
	}
	body = append(body,
		settingsText(tr, "settings_msg_line_silent", i18n.TranslationParams{"state": settingsStateWord(tr, st.Silent)}),
		"", settingsText(tr, fam.HintKey),
		"", settingsText(tr, "settings_msg_hint_silent"))
	k := newSettingsKeyboard(v)
	settingsEditPartRows(k, def, st)
	settingsEditPreviewRows(k, def, st)
	target := "1"
	if st.Silent {
		target = "0"
	}
	label := settingsText(tr, "settings_btn_msg_silent", i18n.TranslationParams{"state": settingsStateIcon(st.Silent)})
	if button, ok := k.button(label, map[string]string{"a": settingsActMsgSilent, settingsMsgEventField: ev.Code, "v": target}); ok {
		k.row(button)
	}
	settingsDelBackRow(k, map[string]string{"a": settingsActMsgFamily, "t": fam.Code})
	return k.screen(settingsCompose(v, "settings_page_msgs", body, opts.Notice, false))
}
