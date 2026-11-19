// Package xenopersona holds the static system text of Xenobot-72's
// conversations: who the bot is (persona.md) and what it knows of the series
// (archive.md). It is a standard-library leaf so the spike tool can use it
// without the pre-main database connection of fuku/config and fuku/db.
//
// The text is the same bytes for every chat and every user, which is what lets
// a provider cache it. Nothing that varies (a date, a chat title, a name) may
// ever be added here; that belongs in the dynamic system text.
package xenopersona

import (
	_ "embed"
	"strings"
)

//go:embed persona.md
var personaMD string

//go:embed archive.md
var archiveMD string

// separator joins the persona and the archive.
const separator = "\n\n---\n\n"

// staticText is built once: the persona, the separator, then the archive
// without its leading source comment.
var staticText = strings.TrimSpace(personaMD) + separator + stripLeadingComment(archiveMD)

// StaticSystemText returns the persona followed by the archive. It returns the
// same string on every call.
func StaticSystemText() string {
	return staticText
}

// stripLeadingComment drops an HTML comment at the very start of the archive.
// The comment names the source of record for maintainers; the model does not
// need it.
func stripLeadingComment(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<!--") {
		if _, rest, ok := strings.Cut(s, "-->"); ok {
			s = rest
		}
	}
	return strings.TrimSpace(s)
}
