//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// settingsEditReservedCheck runs the check of the buttons part of def for text over the stored state.
func settingsEditReservedCheck(def settingsEditPageDef, part, text string, st settingsEditState) settingsEditVerdict {
	st.Kind = def.Kind
	return settingsEditCheck(settingsEditCheckInput{
		Def: def, Part: part, Msg: settingsEditTypedMessage(text), State: st,
		Owner: gotgbot.User{Id: 1, FirstName: "Al"}, GroupTitle: "Group", Language: "en",
	})
}

// TestSettingsEditReservedButtons proves that the buttons the bot adds itself count against Telegram's
// limit of 100 buttons, on the buttons part and on the text part, and that the Welcome page is unchanged.
func TestSettingsEditReservedButtons(t *testing.T) {
	welcome, _ := settingsEditPageByKey(settingsPageWelcome)
	event := settingsEditPageDef{Key: "qtr", Kind: settingsEditEvent, Parts: welcome.Parts, ReservedButtons: 1}
	noReserve := event
	noReserve.ReservedButtons = 0
	refused := func(t *testing.T, got settingsEditVerdict) {
		t.Helper()
		if got.OK || got.RefusalKey != "settings_edit_refuse_buttons_limit" {
			t.Fatalf("OK=%v key=%q, want settings_edit_refuse_buttons_limit", got.OK, got.RefusalKey)
		}
	}
	accepted := func(t *testing.T, got settingsEditVerdict) {
		t.Helper()
		if !got.OK {
			t.Fatalf("refused with %q %q, want accepted", got.RefusalKey, got.RefusalText)
		}
	}

	t.Run("one reserved button leaves room for 99", func(t *testing.T) {
		accepted(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(99, 1000), settingsEditState{}))
		refused(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(100, 1000), settingsEditState{}))
	})
	t.Run("no reserved button leaves room for 100", func(t *testing.T) {
		accepted(t, settingsEditReservedCheck(noReserve, settingsEditPartButtons, settingsEditButtonLines(100, 1000), settingsEditState{}))
	})
	t.Run("the rules button and the reserved row count together", func(t *testing.T) {
		withToken := settingsEditState{Text: "Hi {rules}"}
		refused(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(99, 1000), withToken))
		accepted(t, settingsEditReservedCheck(event, settingsEditPartButtons, settingsEditButtonLines(98, 1000), withToken))
	})
	t.Run("a text that adds the rules button counts the reserved row", func(t *testing.T) {
		stored := settingsEditState{Buttons: settingsEditStoredButtons(98, 1)}
		accepted(t, settingsEditReservedCheck(event, settingsEditPartText, "Hi {rules}", stored))
		stored.Buttons = settingsEditStoredButtons(99, 1)
		refused(t, settingsEditReservedCheck(event, settingsEditPartText, "Hi {rules}", stored))
		accepted(t, settingsEditReservedCheck(noReserve, settingsEditPartText, "Hi {rules}", stored))
	})
	t.Run("the Welcome page's verdicts are unchanged", func(t *testing.T) {
		accepted(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(100, 1000), settingsEditState{}))
		refused(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(101, 1000), settingsEditState{}))
		withToken := settingsEditState{Text: "Hi {rules}"}
		accepted(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(99, 1000), withToken))
		refused(t, settingsEditReservedCheck(welcome, settingsEditPartButtons, settingsEditButtonLines(100, 1000), withToken))
	})
}
