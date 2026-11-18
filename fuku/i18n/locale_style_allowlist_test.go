//go:build testtools

package i18n

// localeStyleAllowList names the Ukrainian and English keys not yet converted to the style guide
// (docs/src/content/docs/contributing/locale-style.md). It was taken from TestLocaleStyle after the sample
// batch of 2026-11-18 and emptied by the batches that followed; it stays empty. Never add a key: a new text
// follows the guide from its first commit.
var localeStyleAllowList = map[string]bool{}
