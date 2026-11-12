package modules

import (
	"context"

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
