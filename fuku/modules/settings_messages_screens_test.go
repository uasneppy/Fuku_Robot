//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsMsgScreenFields are the only callback fields a button of the Messages page may carry: the action, the owner
// field of either menu, the page, the family or part, the event, the destination (d=p on the private copy's buttons,
// Phase 22) and the absolute switch target.
var settingsMsgScreenFields = map[string]bool{"a": true, "c": true, "u": true, "p": true, "t": true, "k": true, "d": true, "v": true}

// settingsMsgAllCustom is the stored messages of a chat whose every event of every family has its own text, so the hub and
// the family list draw every line Custom. The map key is the repository's own (event|dest); a Lookup of each event proves
// the set was built right, so a change of that key fails here and not silently.
func settingsMsgAllCustom(t *testing.T, silent bool) *eventmsg.Messages {
	t.Helper()
	msgs := &eventmsg.Messages{ChatID: -1001234567890, Rows: map[string]eventmsg.Message{}}
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			msgs.Rows[ev.Key+"|"+eventmsg.DestGroup] = eventmsg.Message{
				Event: ev.Key, Dest: eventmsg.DestGroup, Text: "custom", Silent: silent,
			}
		}
	}
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			if _, ok := msgs.Lookup(ev.Key, eventmsg.DestGroup); !ok {
				t.Fatalf("the hand-built messages have no row for %s", ev.Key)
			}
		}
	}
	return msgs
}

// settingsMsgFullState is an event message with every part stored (a long text, the media kind given, one button) and
// Silent on: the event screen's worst case, eight rows (the private copy screen's worst case too, seven rows).
func settingsMsgFullState(mediaType int) settingsEditState {
	return settingsEditState{
		Kind: settingsEditEvent, Text: strings.Repeat("x", 3000), MediaType: mediaType, FileID: "f",
		Buttons: models.ButtonArray{{Name: "B", Url: "https://example.com"}}, Silent: true,
	}
}

// settingsMsgEmptyState is an event message with nothing stored: the default text, no media, no buttons, Silent off.
func settingsMsgEmptyState() settingsEditState {
	return settingsEditState{Kind: settingsEditEvent, TextIsDefault: true}
}

// settingsMsgPMOffState is a private copy with nothing stored, as the page reads a missing pm row: the default text,
// no media, no buttons and the switch off (Silent true).
func settingsMsgPMOffState() settingsEditState {
	return settingsEditState{Kind: settingsEditEvent, TextIsDefault: true, Silent: true}
}

// settingsMsgNamedScreen is one rendered screen of the page with the row widths its keyboard must have ("" for a screen
// whose shape a test does not fix, such as the plain reply of a refusal, which always has its one Cancel).
type settingsMsgNamedScreen struct {
	screen settingsScreen
	widths string
}

// settingsMsgScreens renders every screen of the Messages page for view through the functions the code draws and sends
// them with: the hub (every event Custom, every event Silent, nothing stored), the family list of every family, every
// event screen and every private copy screen with nothing stored and with every part stored and Silent on (a photo, and a
// sticker, which adds the no-caption line), each under every notice given, the three waiting prompts of each (replaced, at the largest limit the prompt
// can name), the refusal reply of every editor refusal reason and, in a group view, the group note with and without a bot
// username. The keys are descriptive and unique. The page exists only in a private menu, so a group view's screens other
// than the note are what no press can reach; they are drawn all the same, because the privacy sweep searches them too.
func settingsMsgScreens(t *testing.T, view settingsView, notices []string) map[string]settingsMsgNamedScreen {
	t.Helper()
	tr := view.Tr
	screens := map[string]settingsMsgNamedScreen{}
	add := func(name string, screen settingsScreen, widths string) {
		screens[name] = settingsMsgNamedScreen{screen: screen, widths: widths}
	}
	stored := map[string]*eventmsg.Messages{
		"all custom": settingsMsgAllCustom(t, false), "all silent": settingsMsgAllCustom(t, true),
		"nothing": {ChatID: -1001234567890, Rows: map[string]eventmsg.Message{}},
	}
	// Every private copy option is on with every event custom, so the hub's block is drawn with its longest state words.
	stored["all custom"].Options = eventmsg.Options{PMDetails: true, PMLog: true}
	for i, notice := range notices {
		for name, msgs := range stored {
			add(fmt.Sprintf("hub %s notice %d", name, i), renderSettingsMsgHub(view, msgs, notice), "[1 1 2]")
			for _, fam := range settingsMsgFamilies {
				if len(fam.Events) == 0 {
					continue
				}
				add(fmt.Sprintf("family %s %s notice %d", fam.Code, name, i), renderSettingsMsgFamily(view, fam, msgs, notice), "[2 2 2 1 2]")
			}
		}
	}
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			def, ok := settingsEditPageByKey(settingsMsgWaitPage(ev))
			if !ok {
				t.Fatalf("no editor definition for the %s event", ev.Key)
			}
			pmDef, ok := settingsEditPageByKey(settingsMsgPMWaitPage(ev))
			if !ok {
				t.Fatalf("no private copy definition for the %s event", ev.Key)
			}
			for i, notice := range notices {
				opts := settingsEditScreenOpts{Notice: notice, BotUsername: "FukuTestBot"}
				add(fmt.Sprintf("event %s empty notice %d", ev.Code, i), renderSettingsMsgEvent(view, fam, ev, def, settingsMsgEmptyState(), opts), "[1 1 1 1 1 1 2]")
				add(fmt.Sprintf("pm %s empty notice %d", ev.Code, i), renderSettingsMsgPM(view, fam, ev, pmDef, settingsMsgPMOffState(), opts), "[1 1 1 1 1 2]")
				for kind, mediaType := range map[string]int{"photo": db.PHOTO, "sticker": db.STICKER} {
					add(fmt.Sprintf("event %s full %s notice %d", ev.Code, kind, i),
						renderSettingsMsgEvent(view, fam, ev, def, settingsMsgFullState(mediaType), opts), "[2 2 2 2 2 1 1 2]")
					add(fmt.Sprintf("pm %s full %s notice %d", ev.Code, kind, i),
						renderSettingsMsgPM(view, fam, ev, pmDef, settingsMsgFullState(mediaType), opts), "[2 2 2 2 2 1 2]")
				}
			}
			for name, d := range map[string]settingsEditPageDef{"": def, "pm ": pmDef} {
				for _, part := range d.Parts {
					add(fmt.Sprintf("%sprompt %s %s", name, ev.Code, part.Code),
						renderSettingsEditPrompt(view, d, part.Code, time.Now(), settingsEditTextMax, true), "[1]")
					for key, reason := range settingsEditRefusalReasons(tr) {
						add(fmt.Sprintf("%srefusal %s %s %s", name, ev.Code, part.Code, key), renderSettingsInputRefusal(view, d, part.Code, reason), "[1]")
					}
				}
			}
		}
	}
	// The note is what a menu opened in the group draws in place of the page (its Open in private row is a group menu's).
	if !view.Private {
		add("group note", renderSettingsPrivateOnlyNoteWith(view, "settings_page_msgs", "settings_msg_group_note", "FukuTestBot"), "[1 2]")
		add("group note without a bot username", renderSettingsPrivateOnlyNoteWith(view, "settings_page_msgs", "settings_msg_group_note", ""), "[2]")
	}
	return screens
}

func TestSettingsMessagesCallbackBytes(t *testing.T) {
	withRealLocales(t)
	tr := i18n.MustNewTranslator("en")
	views := settingsBlacklistWorstViews(tr)
	private := views["private"]

	// sizes records the largest button of each action code over every private screen; Back to Main and Close have codes too.
	sizes := make(map[string]int)
	for name, named := range settingsMsgScreens(t, private, []string{""}) {
		screen := named.screen
		if screen.Omitted != 0 {
			t.Errorf("%s: %d buttons were omitted for their size, want none", name, screen.Omitted)
		}
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if button.CallbackData == "" {
				continue
			}
			if size := len(button.CallbackData); size > 64 {
				t.Errorf("%s: button %q carries %d bytes, over the 64-byte cap", name, button.Text, size)
			}
			fields := settingsButtonFields(t, button)
			for key := range fields {
				if !settingsMsgScreenFields[key] {
					t.Errorf("%s: button %q carries the field %q, outside a c u p t k d v", name, button.Text, key)
				}
			}
			sizes[fields["a"]] = max(sizes[fields["a"]], len(button.CallbackData))
		}
	}
	for code, want := range map[string]int{
		settingsActMsgFamily: 37, settingsActMsgEvent: 42,
		settingsActMsgSet: 46, settingsActMsgRemove: 46, settingsActMsgPreview: 46, settingsActMsgCancel: 46, settingsActMsgSilent: 42,
		settingsActMsgFull: 42, settingsActMsgPM: 42, settingsActMsgPMOption: 41, settingsActOpen: 37, settingsActBack: 33, settingsActClose: 33,
	} {
		if got := sizes[code]; got != want {
			t.Errorf("the largest %s button is %d bytes, the plans fixed %d", code, got, want)
		}
	}

	// Main's cell of the page in both menus, and the group note's buttons.
	cell := func(v settingsView) int {
		screen := renderSettingsMain(v, []settingsMainEntry{{Key: settingsPageMessages, Icon: "📨", Title: "Messages", State: "7", Known: true}}, "")
		for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
			if f := settingsButtonFields(t, button); f["a"] == settingsActOpen && f["p"] == settingsPageMessages {
				return len(button.CallbackData)
			}
		}
		return 0
	}
	if got := cell(private); got != 37 {
		t.Errorf("the Main cell is %d bytes in a private menu, want 37", got)
	}
	if got := cell(views["group"]); got != 33 {
		t.Errorf("the Main cell is %d bytes in a group menu, want 33", got)
	}
	note := renderSettingsPrivateOnlyNoteWith(views["group"], "settings_page_msgs", "settings_msg_group_note", "FukuTestBot")
	if note.Omitted != 0 {
		t.Errorf("the group note omitted %d buttons", note.Omitted)
	}
	for _, button := range settingsAllButtons(note.Keyboard.InlineKeyboard) {
		if button.CallbackData != "" && len(button.CallbackData) != 29 {
			t.Errorf("group note button %q is %d bytes, want 29", button.Text, len(button.CallbackData))
		}
	}
}

func TestSettingsMessagesPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	title := strings.Repeat("Ж", 64)
	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			views := settingsBlacklistWorstViews(tr)
			// A notice adds only its own length, so the two longest of the notices the page's screens can carry stand for
			// every combination: a failed save, the saved notice of each part with the caption and album notes, the
			// removed notices and the editor's other page notices.
			candidates := []string{
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
				settingsNoticeWarn(settingsText(tr, "settings_edit_nothing_to_remove")),
				settingsNoticeWarn(settingsText(tr, "settings_edit_group_note")),
				settingsNoticeWarn(settingsText(tr, "settings_edit_cancelled")),
				settingsNoticeWarn(settingsText(tr, "settings_edit_removed_text_default")),
				settingsNoticeWarn(settingsText(tr, "settings_edit_removed_media")),
				settingsNoticeWarn(settingsText(tr, "settings_edit_removed_buttons")),
			}
			for _, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
				candidates = append(candidates, settingsInputSavedNotice(tr, part, true, true))
			}
			notices := append([]string{""}, settingsBlacklistLongest(candidates, 2)...)

			for viewName, view := range views {
				view.GroupTitle, view.OpenerName = title, title
				screens := settingsMsgScreens(t, view, notices)
				for name, named := range screens {
					label := fmt.Sprintf("%s %s", viewName, name)
					screen := named.screen
					if units := panelUTF16Len(screen.Text); units == 0 || units > 3800 {
						t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
					}
					for _, placeholder := range []string{"{n}", "{max}", "{time}", "{part}", "{state}", "{page}", "{icon}", "%!"} {
						if strings.Contains(screen.Text, placeholder) {
							t.Errorf("%s keeps the unfilled %s", label, placeholder)
						}
					}
					if screen.Omitted != 0 {
						t.Errorf("%s omitted %d buttons", label, screen.Omitted)
					}
					if got := fmt.Sprint(settingsListRowWidths(screen.Keyboard.InlineKeyboard)); got != named.widths {
						t.Errorf("%s has row widths %s, want %s", label, got, named.widths)
					}
					// A half-width cell fits 20 code points and a full-width button 40; the Open in private URL row is
					// a URL button of its own.
					for _, row := range screen.Keyboard.InlineKeyboard {
						limit := 20
						if len(row) == 1 {
							limit = 40
						}
						for _, button := range row {
							if strings.TrimSpace(button.Text) == "" {
								t.Errorf("%s has a button with an empty label", label)
							}
							if n := len([]rune(button.Text)); n > limit && button.Url == "" {
								t.Errorf("%s: button %q has %d code points, want at most %d", label, button.Text, n, limit)
							}
						}
					}
				}
			}
		})
	}
}
