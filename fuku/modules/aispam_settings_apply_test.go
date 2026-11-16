//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/aispam"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// aispamTestProviderValue is the stand-in provider value of these tests; it is never logged.
const aispamTestProviderValue = "provider-test-value"

// withAISpamConfigured makes the AI spam filter available to the test (the provider value set and
// the global switch on) and puts config.AppConfig back when it ends. It is never combined with
// t.Parallel.
func withAISpamConfigured(t *testing.T) {
	t.Helper()
	withAISpamTestConfig(t, aispamTestProviderValue, true, 0)
}

// aispamNotConfiguredHow names why the AI spam filter is unavailable in withAISpamNotConfigured.
type aispamNotConfiguredHow int

const (
	// aispamProviderValueEmpty leaves the global switch on and the provider value empty.
	aispamProviderValueEmpty aispamNotConfiguredHow = iota
	// aispamSwitchOff sets the provider value and turns the global switch off.
	aispamSwitchOff
)

// withAISpamNotConfigured makes the AI spam filter unavailable to the test, by an empty provider
// value or by the global switch, so a test can prove both causes give the same page. It puts
// config.AppConfig back when the test ends and is never combined with t.Parallel.
func withAISpamNotConfigured(t *testing.T, how aispamNotConfiguredHow) {
	t.Helper()
	switch how {
	case aispamProviderValueEmpty:
		withAISpamTestConfig(t, "", true, 0)
	case aispamSwitchOff:
		withAISpamTestConfig(t, aispamTestProviderValue, false, 0)
	}
}

// aispamApplyChat mints a chat for an apply test and removes its AI spam row, chats row and cached
// copy before and after, because CI runs all of fuku/modules in one database.
func aispamApplyChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	aispamGoldenCleanup(t, chatID)
	return chatID
}

// aispamCountingWrite swaps the write seam for one that counts its calls and still writes, and
// restores the real one when the test ends.
func aispamCountingWrite(t *testing.T) *int {
	t.Helper()
	calls := 0
	saved := aispamWrites
	t.Cleanup(func() { aispamWrites = saved })
	aispamWrites.SetEnabled = func(chatID int64, enabled bool) error {
		calls++
		return saved.SetEnabled(chatID, enabled)
	}
	return &calls
}

func TestApplyAISpamEnabled(t *testing.T) {
	ctx := context.Background()

	t.Run("configured: on and off write and read back", func(t *testing.T) {
		withAISpamConfigured(t)
		chatID := aispamApplyChat(t)
		for _, tc := range []struct {
			on  bool
			key string
		}{
			{true, "aispam_enabled_success"},
			{false, "aispam_disabled_success"},
			{true, "aispam_enabled_success"},
		} {
			res := applyAISpamEnabled(ctx, chatID, tc.on)
			if res.Outcome != aispamApplied || res.Err != nil || res.Key != tc.key || res.Enabled != tc.on {
				t.Fatalf("applyAISpamEnabled(%v) = %+v, want applied with key %s and switch %v", tc.on, res, tc.key, tc.on)
			}
			want := "off"
			if tc.on {
				want = "on"
			}
			if got := aispamGoldenStored(t, chatID); got != want {
				t.Fatalf("stored row = %s, want %s", got, want)
			}
		}
	})

	for _, tc := range []struct {
		name    string
		how     aispamNotConfiguredHow
		outcome aispamApplyOutcome
		key     string
	}{
		{"provider value empty", aispamProviderValueEmpty, aispamNoKey, "aispam_no_key"},
		{"global switch off", aispamSwitchOff, aispamGloballyDisabled, "aispam_globally_disabled"},
	} {
		t.Run("not configured by the "+tc.name+": on is refused before any write or read", func(t *testing.T) {
			withAISpamNotConfigured(t, tc.how)
			chatID := aispamApplyChat(t)
			writes := aispamCountingWrite(t)

			res := applyAISpamEnabled(ctx, chatID, true)
			if res.Outcome != tc.outcome || res.Key != tc.key || res.Err != nil || res.Enabled {
				t.Fatalf("applyAISpamEnabled(on) = %+v, want outcome %v with key %s and nothing stored", res, tc.outcome, tc.key)
			}
			if *writes != 0 {
				t.Errorf("write seam calls = %d, want 0", *writes)
			}
			if got := aispamGoldenStored(t, chatID); got != "none" {
				t.Errorf("stored row = %s, want none", got)
			}
		})

		t.Run("not configured by the "+tc.name+": off always writes", func(t *testing.T) {
			withAISpamConfigured(t)
			chatID := aispamApplyChat(t)
			if res := applyAISpamEnabled(ctx, chatID, true); res.Outcome != aispamApplied {
				t.Fatalf("seed on = %+v", res)
			}
			withAISpamNotConfigured(t, tc.how)
			writes := aispamCountingWrite(t)

			res := applyAISpamEnabled(ctx, chatID, false)
			if res.Outcome != aispamApplied || res.Key != "aispam_disabled_success" || res.Enabled || res.Err != nil {
				t.Fatalf("applyAISpamEnabled(off) = %+v, want applied off", res)
			}
			if *writes != 1 {
				t.Errorf("write seam calls = %d, want 1", *writes)
			}
			if got := aispamGoldenStored(t, chatID); got != "off" {
				t.Errorf("stored row = %s, want off", got)
			}
		})
	}

	t.Run("a failing write is a save failure with the error", func(t *testing.T) {
		withAISpamConfigured(t)
		chatID := aispamApplyChat(t)
		boom := errors.New("database is down")
		saved := aispamWrites
		t.Cleanup(func() { aispamWrites = saved })
		aispamWrites.SetEnabled = func(int64, bool) error { return boom }

		res := applyAISpamEnabled(ctx, chatID, true)
		if res.Outcome != aispamSaveFailed || !errors.Is(res.Err, boom) || res.Key != "common_settings_save_failed" {
			t.Fatalf("applyAISpamEnabled() = %+v, want a save failure carrying the write error", res)
		}
	})

	t.Run("the write drops the cached copy the reader fills", func(t *testing.T) {
		withAISpamConfigured(t)
		chatID := aispamApplyChat(t)

		before, err := aispam.LoadEnabledContext(ctx, chatID)
		if err != nil || before {
			t.Fatalf("primed read = %v, %v, want off, nil", before, err)
		}
		if res := applyAISpamEnabled(ctx, chatID, true); res.Outcome != aispamApplied || !res.Enabled {
			t.Fatalf("applyAISpamEnabled(on) = %+v", res)
		}
		after, err := aispam.LoadEnabledContext(ctx, chatID)
		if err != nil || !after {
			t.Fatalf("read after the write = %v, %v, want on, nil (the key must have been dropped)", after, err)
		}
	})
}

// TestAISpamConfiguredMatchesCommand: in the four combinations of provider value and global
// switch, aispamConfigured is false exactly when /aispam on refuses (no write and one of the two
// refusal texts) and true exactly when it writes.
func TestAISpamConfiguredMatchesCommand(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 42, FirstName: "Admin"}
	tr := i18n.MustNewTranslator("uk")
	noKeyText, _ := tr.GetString("aispam_no_key")
	globalText, _ := tr.GetString("aispam_globally_disabled")
	enabledText, _ := tr.GetString("aispam_enabled_success")

	for _, tc := range []struct {
		provider string
		switchOn bool
		want     bool
		reply    string
	}{
		{aispamTestProviderValue, true, true, enabledText},
		{"", true, false, noKeyText},
		{aispamTestProviderValue, false, false, globalText},
		{"", false, false, noKeyText},
	} {
		name := fmt.Sprintf("value set %v, switch on %v", tc.provider != "", tc.switchOn)
		t.Run(name, func(t *testing.T) {
			resetAISpamRuntime()
			withAISpamTestConfig(t, tc.provider, tc.switchOn, 0)
			chatID := aispamApplyChat(t)
			writes := aispamCountingWrite(t)

			if got := aispamConfigured(); got != tc.want {
				t.Fatalf("aispamConfigured() = %v, want %v", got, tc.want)
			}

			client := aispamAdminClient()
			bot := newModuleTestBot(client)
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "AI Spam Configured"}
			dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
			LoadAISpam(dispatcher)
			if err := dispatcher.ProcessUpdate(bot, aispamMessageUpdate(chat, admin, 101, "/aispam on"), nil); err != nil {
				t.Fatalf("ProcessUpdate error = %v", err)
			}

			calls := client.callsFor("sendMessage")
			if len(calls) != 1 {
				t.Fatalf("sendMessage calls = %d, want 1", len(calls))
			}
			if text, _ := calls[0].Params["text"].(string); text != tc.reply {
				t.Errorf("reply = %q, want %q", text, tc.reply)
			}
			wrote := *writes == 1
			if wrote != tc.want {
				t.Errorf("/aispam on wrote = %v, want %v (configured)", wrote, tc.want)
			}
			if !tc.want && *writes != 0 {
				t.Errorf("a refused /aispam on made %d writes, want 0", *writes)
			}
		})
	}
}
