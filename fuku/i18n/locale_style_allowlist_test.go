//go:build testtools

package i18n

// localeStyleAllowList names the Ukrainian and English keys not yet converted to the style guide
// (docs/src/content/docs/contributing/locale-style.md). It was taken from TestLocaleStyle after the sample
// batch of 2026-11-18 and emptied by the batches that followed; it stays empty. Never add a key: a new text
// follows the guide from its first commit.
var localeStyleAllowList = map[string]bool{}

// localeStyleGoldenPinned names the Ukrainian and English keys that break a style rule but whose text a
// TestLegacy*Requests golden pins byte for byte. Goldens are never re-recorded, so these keep their old
// shape; a key leaves the list when its golden stops pinning it. Never add a key for any other reason.
var localeStyleGoldenPinned = map[string]bool{
	// TestLegacyFilterRequests pins the /filter and /stop success replies.
	"uk:filters_added_success":   true,
	"en:filters_added_success":   true,
	"uk:filters_removed_success": true,
	"en:filters_removed_success": true,
}
