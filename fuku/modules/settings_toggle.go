package modules

import (
	"context"
	"slices"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
)

// settingsToggleResult is what the apply step of a boolean control reports to settingsToggleSet.
// S is the page's own state type. Key is the locale key of a warn notice for a refusal or a
// failed save, and is empty for a stored value. After a stored value Stored is true and State is
// the state to draw (the row the apply function read back); when the write went through but the
// read-back failed Stored is false and the helper reads the page again. A control that answered
// the press itself (an alert) reports the unchanged state with Stored true and an empty Key.
type settingsToggleResult[S any] struct {
	State  S
	Stored bool
	Key    string
}

// settingsToggleSet is the one press of a boolean control of the settings menu: it reads the
// page's state before, calls the control's apply function with the absolute target
// (value == "1"; the control's Validate has already refused anything but "0" and "1"), and
// returns the page redrawn from the state the apply function read back. A refusal or a failed
// save redraws the stored state with the result's key as a warn notice. A page that cannot be
// read before or after the press is the cannot-open edit and a nil screen. It knows no page: the
// read, the apply step and the draw are the caller's functions, so every toggle of the phase
// shares this sequence.
func settingsToggleSet[S any](
	p *settingsPress,
	value string,
	read func(ctx context.Context) (S, error),
	apply func(ctx context.Context, before S, on bool) settingsToggleResult[S],
	draw func(state S, notice string) settingsScreen,
) *settingsScreen {
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout)
	defer cancel()

	before, err := read(ctx)
	if err != nil {
		log.Warnf("[Settings] page of chat %d could not be read before a press: %v", p.group.Id, err)
		p.cannotOpen()
		return nil
	}
	result := apply(ctx, before, value == "1")

	state := before
	switch {
	case result.Key != "":
		// A refusal or a failed save leaves the stored state as it was before.
	case result.Stored:
		state = result.State
	default:
		if state, err = read(ctx); err != nil {
			log.Warnf("[Settings] page of chat %d could not be read after a press: %v", p.group.Id, err)
			p.cannotOpen()
			return nil
		}
	}
	notice := ""
	if result.Key != "" {
		notice = settingsNoticeWarn(settingsText(p.tr, result.Key))
	}
	screen := draw(state, notice)
	return &screen
}

// settingsToggleCell is one cell of settingsToggleGrid: the control it presses, the icon and name
// that make its label, and the state the cell shows, so a press sends the opposite.
type settingsToggleCell struct {
	Control string
	Icon    string
	Name    string
	On      bool
}

// settingsToggleGrid appends a two-column grid of toggle cells to the keyboard: each cell is
// "icon name" and sends the absolute target value the opposite of the state it shows (so a double
// tap writes the same value), the cells are filled left to right and an odd last cell stands alone
// on its row. It is pure and knows no page: the caller chooses the icons (the check and the cross
// of a switch, the lock and the open lock of a lock) and the names.
func settingsToggleGrid(k *settingsKeyboard, page string, cells []settingsToggleCell) {
	var buttons []gotgbot.InlineKeyboardButton
	for _, cell := range cells {
		buttons = append(buttons, settingsToggleButton(k, page, cell.Control, cell.Icon+" "+cell.Name, cell.On)...)
	}
	for row := range slices.Chunk(buttons, 2) {
		k.row(row...)
	}
}
