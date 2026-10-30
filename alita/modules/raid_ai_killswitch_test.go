//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/config"
)

// switchAISpamOff sets the ENABLE_AISPAM kill switch off for one test, leaving the keys alone.
func switchAISpamOff(t *testing.T) {
	t.Helper()
	previous := config.AppConfig.EnableAISpam
	config.AppConfig.EnableAISpam = false
	t.Cleanup(func() { config.AppConfig.EnableAISpam = previous })
}

// TestRaidAIKillSwitch proves ENABLE_AISPAM=false stops the whole raid AI path, the text and
// image checks included, even with both provider keys set (WR-02).
func TestRaidAIKillSwitch(t *testing.T) {
	t.Run("the configured checks follow the switch", func(t *testing.T) {
		withAIKeys(t, "typesafe-test-key", "anthropic-test-key")
		if !raidAITextConfigured() || !raidAIImageConfigured() || !raidAIConfigured() {
			t.Fatal("with both keys and the switch on the raid AI path must be configured")
		}
		switchAISpamOff(t)
		if raidAITextConfigured() || raidAIImageConfigured() || raidAIConfigured() {
			t.Errorf("text %v, image %v, any %v with ENABLE_AISPAM off, want all false",
				raidAITextConfigured(), raidAIImageConfigured(), raidAIConfigured())
		}
	})

	t.Run("a new member's photo is not fetched or sent to the image provider", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		switchAISpamOff(t)
		before := GetRaidAIStats()

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
		env.settleAI()

		if got := env.client.getFiles(); len(got) != 0 {
			t.Errorf("getFile asked for %v, want nothing with ENABLE_AISPAM off", got)
		}
		if got := env.claudeRequests(); got != 0 {
			t.Errorf("Claude requests = %d, want 0", got)
		}
		if got := GetRaidAIStats(); got != before {
			t.Errorf("stats moved from %+v to %+v, want no change", before, got)
		}
	})

	t.Run("an image job already queued is dropped", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		switchAISpamOff(t)

		raidAIProcessJob(raidAIJob{
			bot:       env.bot,
			chat:      env.chat,
			userID:    member.Id,
			messageID: 7,
			itemKey:   "m:7",
			media:     &raidAIMedia{FileID: "p800", FileUniqueID: "up800", Kind: "photo"},
		})

		if got := env.client.getFiles(); len(got) != 0 {
			t.Errorf("getFile asked for %v, want nothing", got)
		}
		if got := env.claudeRequests(); got != 0 {
			t.Errorf("Claude requests = %d, want 0", got)
		}
	})

	t.Run("a text job already queued is dropped", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		switchAISpamOff(t)

		raidAIProcessJob(raidAIJob{
			bot: env.bot, chat: env.chat, userID: member.Id, messageID: 7, itemKey: "m:7",
			text: spamLink, linkDomains: []string{"t.me"},
		})

		if got := env.aiRequests(); got != 0 {
			t.Errorf("TypeSafe requests = %d, want 0", got)
		}
	})

	t.Run("a new member's link text is not sent to TypeSafe", func(t *testing.T) {
		env := newRaidAIEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		switchAISpamOff(t)

		env.sendText(member, spamLink)
		env.settleAI()

		if got := env.aiRequests(); got != 0 {
			t.Errorf("TypeSafe requests = %d, want 0", got)
		}
	})

	t.Run("/autolockdown says AI checks are not set up", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		withAIKeys(t, "typesafe-test-key", "anthropic-test-key")
		switchAISpamOff(t)

		env.send(env.admin, "/autolockdown")

		env.wantReplyHas(staffMarker("lockdown_auto_ai_unavailable"))
	})
}
