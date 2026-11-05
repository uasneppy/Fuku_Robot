package greetings

// ClaimGreeting is a stub that the green commit replaces.
func ClaimGreeting(chatID, userID int64) (bool, error) {
	return false, nil
}

// ReleaseGreeting is a stub that the green commit replaces.
func ReleaseGreeting(chatID, userID int64) error {
	return nil
}
