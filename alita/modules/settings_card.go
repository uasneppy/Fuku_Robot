package modules

import (
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// The two-tap confirm card of the settings menu (D-09), shared by Unlink and Lift. The card
// replaces the page text in the same message and its Yes button carries the card's expiry e, in
// unix minutes, and the ID of the object the card was drawn for (D-33). Nothing is stored: no timer and no Redis key, so a card
// works on any replica with Redis down, and an expired Yes is refused when it is pressed.

// settingsCardLifetime is how long a card's Yes stays valid. It is a variable only so a test
// can make a card expire at once.
var settingsCardLifetime = 5 * time.Minute

// settingsCardExpiry is the e field of a card made at now: the unix minute settingsCardLifetime
// ahead.
func settingsCardExpiry(now time.Time) string {
	return strconv.FormatInt(now.Add(settingsCardLifetime).Unix()/60, 10)
}

// settingsCardNumber reads a positive 63-bit number written in at most 19 digits and nothing
// else. ok is false for anything more or less.
func settingsCardNumber(raw string) (n int64, ok bool) {
	if raw == "" || len(raw) > 19 {
		return 0, false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// settingsCardExpired reads the e field of a Yes press and reports whether its minute is
// before the current unix minute. ok is false when e is not a positive 63-bit number written
// in digits only; the caller answers that as an invalid request before it runs anything.
func settingsCardExpired(fields map[string]string) (expired bool, ok bool) {
	minute, ok := settingsCardNumber(fields["e"])
	if !ok {
		return false, false
	}
	return minute < time.Now().Unix()/60, true
}

// settingsCardObjectID reads the field key of a Yes press: the ID of the object the card was
// drawn for (r, the lockdown row, on a Lift card; l, the link row, on an Unlink card). ok is
// false when it is missing or not a positive 63-bit number written in digits only; the caller
// answers that as an invalid request before any lookup, as it does for a bad e. A Yes acts
// only on the object whose ID it carries (D-33): the card stays on screen for minutes, and
// the object current at the second tap may be a newer one.
func settingsCardObjectID(fields map[string]string, key string) (id int64, ok bool) {
	return settingsCardNumber(fields[key])
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
