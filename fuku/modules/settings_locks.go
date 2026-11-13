package modules

// The Locks page of the settings menu (page k, Phase 18). This file holds the one table the page
// is drawn from and its tests read; the page itself, its pager and its controls are registered by
// a later plan, and every lock is written through applyLock, the function /lock and /unlock use.

// settingsLockPageSize is how many locks one page of the Locks page shows (A18-A11): the smallest
// size that keeps the tallest page inside the private menu's row budget.
const settingsLockPageSize = 12

// settingsLockRow is one lock the Locks page shows.
type settingsLockRow struct {
	// Code is the control key a button carries, two lower-case letters. It is never the lock's name,
	// and it differs from the pager's action code and from every core action code. A later lock is
	// one row here and one entry in lockMap or restrMap.
	Code string
	// Kind is the lock's name in /locktypes and the key applyLock stores, one entry of lockMap or
	// restrMap.
	Kind string
	// NameKey is the locale key of the lock's name, which labels its button and its state line.
	NameKey string
	// Page is the page of the Locks page the row is drawn on: 1 or 2.
	Page int
}

// settingsLockRows lists the 22 locks in the order the page draws them: the 16 locks of lockMap
// and the six restrictions of restrMap, 12 on page 1 and 10 on page 2.
var settingsLockRows = []settingsLockRow{
	{Code: "sk", Kind: "sticker", NameKey: "settings_lock_sticker", Page: 1},
	{Code: "gf", Kind: "gif", NameKey: "settings_lock_gif", Page: 1},
	{Code: "ph", Kind: "photo", NameKey: "settings_lock_photo", Page: 1},
	{Code: "vd", Kind: "video", NameKey: "settings_lock_video", Page: 1},
	{Code: "vn", Kind: "videonote", NameKey: "settings_lock_videonote", Page: 1},
	{Code: "vo", Kind: "voice", NameKey: "settings_lock_voice", Page: 1},
	{Code: "au", Kind: "audio", NameKey: "settings_lock_audio", Page: 1},
	{Code: "dc", Kind: "document", NameKey: "settings_lock_document", Page: 1},
	{Code: "ct", Kind: "contact", NameKey: "settings_lock_contact", Page: 1},
	{Code: "lc", Kind: "location", NameKey: "settings_lock_location", Page: 1},
	{Code: "gm", Kind: "game", NameKey: "settings_lock_game", Page: 1},
	{Code: "ur", Kind: "url", NameKey: "settings_lock_url", Page: 1},
	{Code: "fw", Kind: "forward", NameKey: "settings_lock_forward", Page: 2},
	{Code: "rt", Kind: "rtl", NameKey: "settings_lock_rtl", Page: 2},
	{Code: "ac", Kind: "anonchannel", NameKey: "settings_lock_anonchannel", Page: 2},
	{Code: "bt", Kind: "bots", NameKey: "settings_lock_bots", Page: 2},
	{Code: "ms", Kind: "messages", NameKey: "settings_lock_messages", Page: 2},
	{Code: "cm", Kind: "comments", NameKey: "settings_lock_comments", Page: 2},
	{Code: "md", Kind: "media", NameKey: "settings_lock_media", Page: 2},
	{Code: "ot", Kind: "other", NameKey: "settings_lock_other", Page: 2},
	{Code: "pv", Kind: "previews", NameKey: "settings_lock_previews", Page: 2},
	{Code: "al", Kind: "all", NameKey: "settings_lock_all", Page: 2},
}
