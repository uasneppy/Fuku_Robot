package lockdown

import "time"

// RecentlyLiftedFresh reports whether any lockdown of the chat, in any state, started its
// lift after since. It is the scaffold the test compiles against and answers false until
// the query is written.
func RecentlyLiftedFresh(chatID int64, since time.Time) (bool, error) {
	return false, nil
}
