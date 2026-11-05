//go:build testtools

package modules

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsOncePage opens the Welcome page of the env's chat, in the group menu or in the owner's
// private chat, and returns the message ID plus a press function that sends the given fields
// of the page in the same place.
func settingsOncePage(env *settingsEnv, owner gotgbot.User, private bool) (int64, func(msgID int64, kv ...string)) {
	env.t.Helper()
	if private {
		msgID := openEditPagePrivate(env, owner, settingsPageWelcome)
		return msgID, func(id int64, kv ...string) {
			env.pressPrivate(owner, id, env.privateFields(env.chat.Id, kv...))
		}
	}
	msgID := openEditPage(env, owner, settingsPageWelcome)
	return msgID, func(id int64, kv ...string) {
		env.press(owner, id, env.groupFields(owner, kv...))
	}
}

// settingsOnceChat is the supergroup the env's chat stands for, as the join handlers see it.
func settingsOnceChat(env *settingsEnv) gotgbot.Chat {
	return gotgbot.Chat{Id: env.chat.Id, Type: "supergroup", Title: env.chat.Title}
}

// settingsOnceStored reads welcome_once of the chat straight from the database.
func settingsOnceStored(t *testing.T, chatID int64) bool {
	t.Helper()
	row, ok := settingsEditStored(t, chatID)
	return ok && row.WelcomeSettings != nil && row.WelcomeSettings.Once
}

func TestSettingsEditOnceSwitch(t *testing.T) {
	for _, private := range []bool{true, false} {
		name := "group menu"
		if private {
			name = "private menu"
		}
		t.Run(name, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			owner := env.newOwner(env.chat)
			chat := settingsOnceChat(env)
			greetOnceCleanup(t, chat.Id)
			// The member is welcomed by this text; the page itself never reads it.
			if err := greetings.SetWelcomeText(chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
				t.Fatalf("SetWelcomeText: %v", err)
			}
			msgID, press := settingsOncePage(env, owner, private)

			text, keyboard := env.lastEdit(chatForPage(env, owner, private), msgID)
			if want := staffMarker("settings_edit_line_once") + " ❌ " + staffMarker("common_status_off"); !strings.Contains(text, want) {
				t.Errorf("the Welcome page %q lacks the off line %q", text, want)
			}
			once, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
			if !strings.Contains(once.Text, "❌") || fields["a"] != settingsActToggle || fields["p"] != settingsPageWelcome || fields["v"] != "1" {
				t.Fatalf("the Greet once cell is %q with %v, want the off icon and tg on page w with v=1", once.Text, fields)
			}

			// Pressing it stores welcome_once and redraws the line and the cell.
			answers := env.answerCount()
			press(msgID, "a", "tg", "p", "w", "t", "o", "v", "1")
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("the press was answered %d times, want once", got)
			}
			if !settingsOnceStored(t, chat.Id) {
				t.Fatal("the press did not store welcome_once true")
			}
			text, keyboard = env.lastEdit(chatForPage(env, owner, private), msgID)
			if want := staffMarker("settings_edit_line_once") + " ✅ " + staffMarker("common_status_on"); !strings.Contains(text, want) {
				t.Errorf("after the press the page %q lacks the on line %q", text, want)
			}
			if settingsEditHasNoticeLine(text, settingsNoticeWarnIcon) || settingsEditHasNoticeLine(text, settingsNoticeOKIcon) {
				t.Errorf("a stored write redrew with a notice: %q", text)
			}
			once, fields = settingsEditControl(t, keyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
			if !strings.Contains(once.Text, "✅") || fields["v"] != "0" {
				t.Errorf("after the press the cell is %q with %v, want the on icon and v=0", once.Text, fields)
			}

			// A double tap of the same absolute value stores the same row.
			press(msgID, "a", "tg", "p", "w", "t", "o", "v", "1")
			if !settingsOnceStored(t, chat.Id) {
				t.Error("a repeated v=1 press turned Greet once off")
			}

			// The switch is the runtime's: a member who joins, leaves and joins again is welcomed once.
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			member := gotgbot.User{Id: 5151, FirstName: "Alice"}
			greetOnceJoin(t, bot, chat, member)
			greetOnceLeave(t, bot, chat, member)
			greetOnceJoin(t, bot, chat, member)
			if got := greetOnceWelcomes(client); got != 1 {
				t.Errorf("a member who joined, left and joined again was welcomed %d times, want once", got)
			}

			// /resetwelcome leaves the switch as it is (UI-SPEC assumption A3).
			env.sendIn(env.chat, owner, "/resetwelcome")
			if !settingsOnceStored(t, chat.Id) {
				t.Error("/resetwelcome turned Greet once off")
			}

			// A failed write redraws the freshly loaded page with the old state.
			previous := greetingWrites.WelcomeOnce
			greetingWrites.WelcomeOnce = func(int64, bool) error { return errors.New("write failed") }
			t.Cleanup(func() { greetingWrites.WelcomeOnce = previous })
			press(msgID, "a", "tg", "p", "w", "t", "o", "v", "0")
			text, keyboard = env.lastEdit(chatForPage(env, owner, private), msgID)
			if !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
				t.Errorf("a failed write drew %q, want the save-failed notice", text)
			}
			once, fields = settingsEditControl(t, keyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
			if !strings.Contains(once.Text, "✅") || fields["v"] != "0" || !settingsOnceStored(t, chat.Id) {
				t.Errorf("after a failed write the cell is %q with %v, want the old state on", once.Text, fields)
			}
		})
	}
}

// chatForPage is the chat the page message sits in: the group for a group menu, the owner's
// private chat for a private one.
func chatForPage(env *settingsEnv, owner gotgbot.User, private bool) int64 {
	if private {
		return owner.Id
	}
	return env.chat.Id
}

func TestSettingsEditSwitchGrid(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	welcome := settingsEditDef(t, settingsPageWelcome)
	goodbye := settingsEditDef(t, settingsPageGoodbye)
	buttons := models.ButtonArray{{Name: "B", Url: "https://example.com"}}

	rowsOf := func(def settingsEditPageDef, st settingsEditState, private bool) []string {
		screen := renderSettingsEditor(settingsEditView(tr, private), def, st, settingsEditScreenOpts{BotUsername: "FukuTestBot"})
		if screen.Omitted != 0 {
			t.Errorf("%s omitted %d buttons", def.Key, screen.Omitted)
		}
		return settingsEditRowNames(screen)
	}

	t.Run("Welcome with nothing set is seven rows", func(t *testing.T) {
		st := settingsEditState{Kind: settingsEditWelcome, TextIsDefault: true, Enabled: true}
		want := []string{
			"set_text", "set_media", "set_buttons", "preview_text | full_preview",
			"switch_welcome | switch_clean", "switch_once", "back | close",
		}
		if got := rowsOf(welcome, st, true); !slices.Equal(got, want) {
			t.Errorf("private rows = %q, want %q", got, want)
		}
	})

	t.Run("Welcome with every part set is eight rows private and nine in a group", func(t *testing.T) {
		st := settingsEditState{
			Kind: settingsEditWelcome, Text: "custom", MediaType: db.PHOTO, FileID: "f", Buttons: buttons,
			Enabled: true, Clean: true, Once: true,
		}
		private := rowsOf(welcome, st, true)
		if len(private) != 8 {
			t.Errorf("private page has %d rows %q, want 8", len(private), private)
		}
		if group := rowsOf(welcome, st, false); len(group) != 9 || group[len(group)-2] != "open_private" {
			t.Errorf("group page has %d rows %q, want 9 with Open in private above the navigation row", len(group), group)
		}
	})

	t.Run("Goodbye draws its two switches as one row", func(t *testing.T) {
		st := settingsEditState{Kind: settingsEditGoodbye, TextIsDefault: true, Enabled: true}
		want := []string{
			"set_text", "set_media", "set_buttons", "preview_text | full_preview",
			"switch_goodbye | switch_clean", "back | close",
		}
		if got := rowsOf(goodbye, st, true); !slices.Equal(got, want) {
			t.Errorf("private rows = %q, want %q", got, want)
		}
	})

	t.Run("an odd last cell sits alone and an undrawn cell closes the grid up", func(t *testing.T) {
		def := welcome
		def.Switches = slices.Clone(welcome.Switches)
		hidden := def.Switches[1]
		hidden.Drawn = func(settingsEditState) bool { return false }
		def.Switches[1] = hidden
		st := settingsEditState{Kind: settingsEditWelcome, TextIsDefault: true, Enabled: true}
		rows := rowsOf(def, st, true)
		if got := rows[4 : len(rows)-1]; !slices.Equal(got, []string{"switch_welcome | switch_once"}) {
			t.Errorf("with Delete last undrawn the switch rows are %q, want one row of the two left", got)
		}
		body := settingsEditBody(settingsEditView(tr, true), def, st)
		for _, line := range body {
			if strings.Contains(line, staffMarker("settings_edit_line_clean_welcome")) || strings.Contains(line, staffMarker("settings_edit_clean_hint_welcome")) {
				t.Errorf("an undrawn switch left %q in the body", line)
			}
		}
		none := def
		none.Switches = nil
		if body := settingsEditBody(settingsEditView(tr, true), none, st); strings.Count(strings.Join(body, "\n")+"\n", "\n\n") != 1 {
			t.Errorf("a page with no switch has no blank line before the status lines, only the one before the hint: %q", body)
		}
	})

	t.Run("the body says each drawn switch in words, in grid order, between the parts and the hints", func(t *testing.T) {
		st := settingsEditState{Kind: settingsEditWelcome, TextIsDefault: true, Enabled: true, Once: true}
		text := renderSettingsEditor(settingsEditView(tr, true), welcome, st, settingsEditScreenOpts{}).Text
		last := -1
		for _, key := range []string{
			"settings_edit_line_buttons", "settings_edit_line_welcome", "settings_edit_line_clean_welcome", "settings_edit_line_once",
			"settings_edit_hint_greeting", "settings_edit_clean_hint_welcome", "settings_edit_hint_once",
		} {
			at := strings.Index(text, staffMarker(key))
			if at <= last {
				t.Errorf("%s is at %d, want after %d in %q", key, at, last, text)
			}
			last = at
		}
		for _, want := range []string{
			staffMarker("settings_edit_line_welcome") + " ✅ " + staffMarker("common_status_on"),
			staffMarker("settings_edit_line_clean_welcome") + " ❌ " + staffMarker("common_status_off"),
			staffMarker("settings_edit_line_once") + " ✅ " + staffMarker("common_status_on"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the Welcome body lacks %q in %q", want, text)
			}
		}
		goodbyeText := renderSettingsEditor(settingsEditView(tr, true), goodbye, settingsEditState{Kind: settingsEditGoodbye, Enabled: true}, settingsEditScreenOpts{}).Text
		for _, absent := range []string{"settings_edit_line_once", "settings_edit_hint_once"} {
			if strings.Contains(goodbyeText, staffMarker(absent)) {
				t.Errorf("the Goodbye page carries %s", absent)
			}
		}
	})

	t.Run("every cell label is the icon, one space and the name in every language", func(t *testing.T) {
		withRealLocales(t)
		for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
			real := i18n.MustNewTranslator(code)
			for _, def := range []settingsEditPageDef{welcome, goodbye} {
				for _, on := range []bool{true, false} {
					st := settingsEditState{Kind: def.Kind, Enabled: on, Clean: on, Once: on}
					screen := renderSettingsEditor(settingsEditView(real, true), def, st, settingsEditScreenOpts{})
					icon := settingsStateIcon(on)
					for _, cell := range settingsEditSwitchCells(def, st) {
						want := settingsText(real, cell.LabelKey, i18n.TranslationParams{"state": icon})
						button, found := settingsButtonByText(screen.Keyboard.InlineKeyboard, want)
						if !found {
							t.Errorf("%s %s: no button labelled %q", code, def.Key, want)
							continue
						}
						name, ok := strings.CutPrefix(button.Text, icon+" ")
						if !ok || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "{}") {
							t.Errorf("%s %s: label %q is not the icon, one space and a name", code, def.Key, button.Text)
						}
						if n := utf8.RuneCountInString(button.Text); n > 20 {
							t.Errorf("%s %s: label %q has %d code points, want at most 20", code, def.Key, button.Text, n)
						}
					}
				}
			}
		}
	})
}

// settingsButtonByText finds the button whose label is exactly text.
func settingsButtonByText(keyboard [][]gotgbot.InlineKeyboardButton, text string) (gotgbot.InlineKeyboardButton, bool) {
	for _, button := range settingsAllButtons(keyboard) {
		if button.Text == text {
			return button, true
		}
	}
	return gotgbot.InlineKeyboardButton{}, false
}

func TestSettingsDriftOnce(t *testing.T) {
	withStaffLocale(t)
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	chat := settingsOnceChat(env)
	greetOnceCleanup(t, chat.Id)
	if err := greetings.SetWelcomeText(chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText: %v", err)
	}
	// The switch is written straight through the repository, not through the menu.
	if err := greetings.SetWelcomeOnce(chat.Id, true); err != nil {
		t.Fatalf("SetWelcomeOnce: %v", err)
	}

	msgID := openEditPage(env, owner, settingsPageWelcome)
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if want := staffMarker("settings_edit_line_once") + " ✅ " + staffMarker("common_status_on"); !strings.Contains(text, want) {
		t.Errorf("the page %q lacks %q for a stored welcome_once", text, want)
	}
	once, fields := settingsEditControl(t, keyboard, "settings_btn_edit_switch_once", settingsEditControlOnce)
	if !strings.Contains(once.Text, "✅") || fields["v"] != "0" {
		t.Errorf("the cell is %q with %v, want the on icon and v=0", once.Text, fields)
	}

	// The join reads the same switch.
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	member := gotgbot.User{Id: 6262, FirstName: "Bea"}
	greetOnceJoin(t, bot, chat, member)
	greetOnceLeave(t, bot, chat, member)
	greetOnceJoin(t, bot, chat, member)
	if got := greetOnceWelcomes(client); got != 1 {
		t.Errorf("the join welcomed %d times, want once", got)
	}
	if greetedRows(t, chat.Id) == 0 {
		t.Fatal("the join left no greeted_members row to test against")
	}

	// Rows of the record change nothing on the page, which never reads them.
	for _, id := range []int64{7001, 7002, 7003} {
		if err := db.DB.Create(&models.GreetedMember{ChatID: chat.Id, UserID: id}).Error; err != nil {
			t.Fatalf("seed greeted_members: %v", err)
		}
	}
	reopened := openEditPage(env, owner, settingsPageWelcome)
	again, _ := env.lastEdit(env.chat.Id, reopened)
	if again != text {
		t.Errorf("the page changed with the record's rows:\n%q\nwant\n%q", again, text)
	}
}

// settingsOnceDoor is the press door of TestSettingsGateAllDoors with the Greet once press in
// place of the Back press: the same opener, the same world change, one answer.
func settingsOnceDoor(t *testing.T, c settingsGateCase) string {
	t.Helper()
	var msgID int64
	if !c.ownerPerson {
		msgID = c.env.openMenu(c.env.admin)
	}
	c.apply()
	person := c.person()
	if c.ownerPerson {
		msgID = c.env.openMenu(person)
	}
	answers := c.env.answerCount()

	c.env.press(person, msgID, c.env.groupFields(person, "a", settingsActToggle, "p", settingsPageWelcome, "t", settingsEditControlOnce, "v", "1"))

	if got := c.env.answerCount() - answers; got != 1 {
		t.Fatalf("the Greet once press was answered %d times, want once", got)
	}
	if answer, alert := c.env.lastAnswer(); alert {
		return settingsGateClassify([]string{answer})
	}
	if settingsOnceStored(t, c.env.chat.Id) {
		return "in"
	}
	return "none"
}

func TestSettingsAuthorityOnce(t *testing.T) {
	const menuMsgID = 7272
	nextAdmin := int64(8_000_000)
	newAdmin := func(env *settingsEnv, chat gotgbot.Chat, canChangeInfo bool) gotgbot.User {
		nextAdmin++
		admin := gotgbot.User{Id: uniqueLinkOwnerID() + nextAdmin, FirstName: "Plain"}
		listEditAdmin(env, chat, admin, canChangeInfo)
		return admin
	}

	t.Run("any live administrator may press it when the owner let admins in", func(t *testing.T) {
		for _, canChangeInfo := range []bool{false, true} {
			env := newSettingsEditEnv(t)
			chat := env.addChat()
			editCleanup(t, chat.Id)
			admin := newAdmin(env, chat, canChangeInfo)
			settingsEditPressAs(env, chat, admin, menuMsgID, "w", "o", "1")
			if answer, alert := env.lastAnswer(); alert {
				t.Errorf("can_change_info %v: the press was refused with %q", canChangeInfo, answer)
			}
			if !settingsOnceStored(t, chat.Id) {
				t.Errorf("can_change_info %v: the press did not write welcome_once", canChangeInfo)
			}
		}
	})

	t.Run("a plain member is refused and nothing is written", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 9_000_000, FirstName: "Member"}
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		settingsEditPressAs(env, env.chat, member, menuMsgID, "w", "o", "1")
		answer, alert := env.lastAnswer()
		if !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_not_admin", answer, alert)
		}
		if _, ok := settingsEditStored(t, env.chat.Id); ok {
			t.Error("a plain member's press wrote a row")
		}
	})

	t.Run("over the four owner worlds it decides as the press door does", func(t *testing.T) {
		worlds := []struct {
			name, want string
			owner      bool
			setup      func(env *settingsEnv)
		}{
			{"owner exists, switch off: the administrator", "owner_only", false, func(env *settingsEnv) {
				env.newOwner(env.chat)
				env.setAdminsAllowed(env.chat.Id, false)
			}},
			{"owner exists, switch off: the owner", "in", true, func(env *settingsEnv) {
				env.setAdminsAllowed(env.chat.Id, false)
			}},
			{"switch on", "in", false, func(env *settingsEnv) {
				env.newOwner(env.chat)
				env.setAdminsAllowed(env.chat.Id, true)
			}},
			{"no owner", "in", false, func(env *settingsEnv) {
				env.fake.clearCreator(env.chat.Id)
				env.listAdmin(env.chat.Id, env.admin.Id)
				env.setAdminsAllowed(env.chat.Id, false)
			}},
			{"owner state unknown", "check_failed", false, func(env *settingsEnv) {
				env.newOwner(env.chat)
				env.setAdminsAllowed(env.chat.Id, false)
				env.fake.setFailure("getChatAdministrators", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			}},
		}
		for _, world := range worlds {
			t.Run(world.name, func(t *testing.T) {
				decide := func(door func(*testing.T, settingsGateCase) string) string {
					resetSettingsOwnerMemo(t)
					env := newSettingsEditEnv(t)
					var owner gotgbot.User
					return door(t, settingsGateCase{
						env:         env,
						ownerPerson: world.owner,
						apply: func() {
							if world.owner {
								owner = env.newOwner(env.chat)
							}
							world.setup(env)
						},
						person: func() gotgbot.User {
							if world.owner {
								return owner
							}
							return env.admin
						},
					})
				}
				once, press := decide(settingsOnceDoor), decide(settingsGateDoorPress)
				if once != world.want || press != world.want {
					t.Errorf("Greet once decided %q and the press door %q, want both %q", once, press, world.want)
				}
			})
		}
	})
}
