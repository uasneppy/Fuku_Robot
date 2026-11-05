package modules

import (
	"strings"
	"testing"
)

// settingsControlRefusal is one forged tg or st press and the reason it must be refused.
type settingsControlRefusal struct {
	name string
	// kv are the fields besides the action and the owner field.
	kv []string
}

// settingsControlUnknownCases are presses naming a page or a control that does not exist, or
// carrying a field set the press does not take.
var settingsControlUnknownCases = []settingsControlRefusal{
	{"unknown control z on Goodbye", []string{"p", "g", "t", "z", "v", "1"}},
	{"switch e on Rules", []string{"p", "r", "t", "e", "v", "1"}},
	{"switch d on Rules", []string{"p", "r", "t", "d", "v", "1"}},
	{"page q is not registered", []string{"p", "q", "t", "e", "v", "1"}},
	{"t missing", []string{"p", "w", "v", "1"}},
	{"extra key k", []string{"p", "w", "t", "e", "v", "1", "k", "1"}},
}

// settingsControlValueCases are presses of a real switch with a value that is not 0 or 1.
var settingsControlValueCases = []settingsControlRefusal{
	{"Welcome switch with v=2", []string{"p", "w", "t", "e", "v", "2"}},
	{"Goodbye clean with v=-1", []string{"p", "g", "t", "d", "v", "-1"}},
	{"Welcome switch with v=+1", []string{"p", "w", "t", "e", "v", "+1"}},
	{"Welcome switch with an empty v", []string{"p", "w", "t", "e", "v", ""}},
}

// TestSettingsControlValidate proves that a forged tg or st press is refused in step 1: answered
// once with the alert common_callback_invalid_request, with no getChatMember, no getChat, no
// edit and no write, in a group menu and in a private one. A valid press still runs.
func TestSettingsControlValidate(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	group := env.chat.Id
	groupMenu := openEditPage(env, owner, settingsPageWelcome)
	privateMenu := openEditPagePrivate(env, owner, settingsPageWelcome)

	press := func(private bool, fields map[string]string) {
		if private {
			env.pressPrivate(owner, privateMenu, fields)
			return
		}
		env.press(owner, groupMenu, fields)
	}
	fieldsFor := func(private bool, action string, kv []string) map[string]string {
		kv = append([]string{"a", action}, kv...)
		if private {
			return env.privateFields(group, kv...)
		}
		return env.groupFields(owner, kv...)
	}

	cases := append(append([]settingsControlRefusal{}, settingsControlUnknownCases...), settingsControlValueCases...)
	for _, private := range []bool{false, true} {
		kind := "group menu"
		menuChat, menuID := group, groupMenu
		if private {
			kind, menuChat, menuID = "private menu", owner.Id, privateMenu
		}
		for _, action := range []string{settingsActToggle, settingsActSet} {
			for _, tc := range cases {
				t.Run(kind+"/"+action+"/"+tc.name, func(t *testing.T) {
					members := env.memberLookupsOf(group, owner.Id)
					chats := len(callsToChat(env.fake.staffBotClient, "getChat", group))
					answers := env.answerCount()
					edits := len(env.edits(menuChat, menuID))
					press(private, fieldsFor(private, action, tc.kv))

					if got := env.answerCount() - answers; got != 1 {
						t.Errorf("the press was answered %d times, want once", got)
					}
					if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
						t.Errorf("answer = %q (alert %v), want the alert common_callback_invalid_request", answer, alert)
					}
					if got := env.memberLookupsOf(group, owner.Id); got != members {
						t.Errorf("the refused press made %d getChatMember calls", got-members)
					}
					if got := len(callsToChat(env.fake.staffBotClient, "getChat", group)); got != chats {
						t.Errorf("the refused press made %d getChat calls", got-chats)
					}
					if got := len(env.edits(menuChat, menuID)); got != edits {
						t.Errorf("the refused press edited the message %d times", got-edits)
					}
					if _, stored := settingsEditStored(t, group); stored {
						t.Error("the refused press wrote a greetings row")
					}
				})
			}
		}
	}

	t.Run("a valid switch press still runs", func(t *testing.T) {
		env.press(owner, groupMenu, fieldsFor(false, settingsActToggle, []string{"p", "w", "t", "e", "v", "0"}))
		row, ok := settingsEditStored(t, group)
		if !ok || row.WelcomeSettings == nil || row.WelcomeSettings.ShouldWelcome {
			t.Fatalf("stored greetings = %+v (found %v), want welcome off", row.WelcomeSettings, ok)
		}
	})
}
