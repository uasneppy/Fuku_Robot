package content

import (
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// commandArgs is the words after the command of msg's text or, for a command sent as a media caption, its
// caption. A message with no text and no caption has no arguments.
func commandArgs(msg *gotgbot.Message) []string {
	fields := strings.Fields(msg.GetText())
	if len(fields) < 2 {
		return nil
	}
	return fields[1:]
}
