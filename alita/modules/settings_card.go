package modules

import (
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// The two-tap confirm card of the settings menu (D-09), shared by Unlink and, in plan 09-12,
// Lift. The card replaces the page text in the same message and its Yes button carries the
// card's expiry e, in unix minutes. Nothing is stored: no timer and no Redis key, so a card
// works on any replica with Redis down, and an expired Yes is refused when it is pressed.

// settingsCardLifetime is how long a card's Yes stays valid. It is a variable only so a test
// can make a card expire at once.
var settingsCardLifetime = 5 * time.Minute

// settingsCardExpiry is the e field of a card made at now: the unix minute settingsCardLifetime
// ahead.
func settingsCardExpiry(now time.Time) string {
	return strconv.FormatInt(now.Add(settingsCardLifetime).Unix()/60, 10)
}

// settingsCardExpired reads the e field of a Yes press and reports whether its minute is
// before the current unix minute. ok is false when e is not a positive 63-bit number written
// in digits only; the caller answers that as an invalid request before it runs anything.
func settingsCardExpired(fields map[string]string) (expired bool, ok bool) {
	raw := fields["e"]
	if raw == "" || len(raw) > 19 {
		return false, false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return false, false
		}
	}
	minute, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || minute <= 0 {
		return false, false
	}
	return minute < time.Now().Unix()/60, true
}

// settingsCardRows adds the card's rows: Yes, carrying yes plus a fresh expiry, and No, which
// returns to page noPage, then the navigation row.
func settingsCardRows(k *settingsKeyboard, yes map[string]string, noPage string) {
	yesFields := make(map[string]string, len(yes)+1)
	for key, value := range yes {
		yesFields[key] = value
	}
	yesFields["e"] = settingsCardExpiry(time.Now())

	var row []gotgbot.InlineKeyboardButton
	if button, ok := k.button(settingsText(k.view.Tr, "settings_btn_yes"), yesFields); ok {
		row = append(row, button)
	}
	if button, ok := k.button(settingsText(k.view.Tr, "settings_btn_no"),
		map[string]string{"a": settingsActCancel, "p": noPage}); ok {
		row = append(row, button)
	}
	k.row(row...)
	k.navRow(false)
}
