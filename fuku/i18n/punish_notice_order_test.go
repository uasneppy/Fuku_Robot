//go:build testtools

package i18n

import (
	"fmt"
	"strings"
	"testing"
)

// TestPunishNoticeArgumentOrder renders the blacklist and forward blocklist punishment notices as the bot fills
// them (the member first, the reason second) in every language: each names both, and in Hindi, whose sentence
// reads "<reason> के कारण <member> को ...", the reason comes before the member.
func TestPunishNoticeArgumentOrder(t *testing.T) {
	installRealLocales(t)
	const member, reason = "MEMBER-MENTION", "REASON-TEXT"
	for _, code := range staffLocales {
		tr := MustNewTranslator(code)
		for _, key := range []string{"blacklists_bl_watcher_banned_user", "blacklists_bl_watcher_kicked_user", "blacklists_bl_watcher_muted_user"} {
			template, err := tr.GetString(key)
			if err != nil || template == "" {
				t.Fatalf("%s %s: missing (%v)", code, key, err)
			}
			text := fmt.Sprintf(template, member, reason)
			if strings.Contains(text, "%!") || !strings.Contains(text, member) || !strings.Contains(text, reason) {
				t.Fatalf("%s %s renders %q", code, key, text)
			}
			if code == "hi" && strings.Index(text, reason) > strings.Index(text, member) {
				t.Fatalf("hi %s renders %q: the reason must come before the member", key, text)
			}
		}
	}
}
