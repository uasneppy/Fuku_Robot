//go:build testtools

package modules

import (
	"strings"
	"testing"
)

// TestSettingsPageActionValidate drives a test page whose action refuses every part code but
// "x" through the real dispatcher: a forged field set is answered as an invalid request before
// any getChatMember, runs nothing and edits nothing, and an action without Validate keeps its
// old behaviour.
func TestSettingsPageActionValidate(t *testing.T) {
	const strictCode, plainCode, pageKey = "zv", "zw", "zv"
	ran := map[string]int{}
	registerSettingsPage(settingsPage{
		Key:      pageKey,
		Order:    9999,
		TitleKey: "settings_page_captcha",
		Actions: map[string]settingsPageAction{
			strictCode: {
				Fields:   []string{"t"},
				Right:    settingsRightAdmin,
				Validate: func(fields map[string]string) bool { return fields["t"] == "x" },
				Run: func(*settingsPress) *settingsScreen {
					ran[strictCode]++
					return nil
				},
			},
			plainCode: {
				Fields: []string{"t"},
				Right:  settingsRightAdmin,
				Run: func(*settingsPress) *settingsScreen {
					ran[plainCode]++
					return nil
				},
			},
		},
	})
	t.Cleanup(func() {
		delete(settingsPages, pageKey)
		delete(settingsActions, strictCode)
		delete(settingsActions, plainCode)
	})

	env := newSettingsEnv(t)
	msgID := env.openMenu(env.admin)

	t.Run("a field set the action accepts runs it and answers once", func(t *testing.T) {
		answers := env.answerCount()
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", strictCode, "t", "x"))
		if ran[strictCode] != 1 {
			t.Errorf("the action ran %d times, want once", ran[strictCode])
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want once", got)
		}
	})

	forged := []struct {
		name string
		kv   []string
	}{
		{"a missing part", []string{"a", strictCode}},
		{"a part the page does not have", []string{"a", strictCode, "t", "m"}},
		{"a non-ASCII part", []string{"a", strictCode, "t", "é"}},
		{"an extra key", []string{"a", strictCode, "t", "x", "z", "1"}},
	}
	for _, tc := range forged {
		t.Run("refuses "+tc.name+" before any lookup", func(t *testing.T) {
			ranBefore := ran[strictCode]
			answers := env.answerCount()
			edits := len(env.edits(env.chat.Id, msgID))
			lookups := env.memberLookupsOf(env.chat.Id, env.admin.Id)

			env.press(env.admin, msgID, env.groupFields(env.admin, tc.kv...))

			if got := env.answerCount() - answers; got != 1 {
				t.Fatalf("the press was answered %d times, want once", got)
			}
			if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("common_callback_invalid_request")) {
				t.Errorf("answer = %q (alert %v), want the alert common_callback_invalid_request", text, alert)
			}
			if ran[strictCode] != ranBefore {
				t.Error("a forged press ran the action")
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("a forged press edited the menu %d times, want none", got-edits)
			}
			if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("a forged press made %d getChatMember calls, want none", got-lookups)
			}
		})
	}

	t.Run("an action with no Validate allows its fields and requires none", func(t *testing.T) {
		for _, kv := range [][]string{{"a", plainCode}, {"a", plainCode, "t", "anything"}} {
			before := ran[plainCode]
			env.press(env.admin, msgID, env.groupFields(env.admin, kv...))
			if ran[plainCode] != before+1 {
				t.Errorf("fields %v did not run the action", kv)
			}
		}
	})
}
