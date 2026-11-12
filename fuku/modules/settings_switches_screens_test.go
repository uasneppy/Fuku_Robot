//go:build testtools

package modules

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/aispam"
)

// settingsSwWidths is the button count of every row of a keyboard.
func settingsSwWidths(keyboard [][]gotgbot.InlineKeyboardButton) []int {
	widths := make([]int, len(keyboard))
	for i, row := range keyboard {
		widths[i] = len(row)
	}
	return widths
}

// settingsSwOpenPrivate opens the Switches page of the env's chat in the private menu of a live
// owner and returns the owner, the message ID, the page text and its keyboard.
func settingsSwOpenPrivate(t *testing.T, env *settingsEnv) (gotgbot.User, int64, string, [][]gotgbot.InlineKeyboardButton) {
	t.Helper()
	owner := env.newOwner(env.chat)
	env.sendPrivate(owner, fmt.Sprintf("/start settings_%d", env.chat.Id))
	sentMain := settingsPrivateOnlySent(t, env, owner)
	button, ok := settingsPageButton(t, staffKeyboardOf(sentMain.Params["reply_markup"]), settingsPageSwitches)
	if !ok {
		t.Fatal("the private Main has no Switches button")
	}
	env.pressRaw(settingsPrivateChat(owner), owner, sentMain.MessageID, button.CallbackData)
	text, keyboard := env.lastEdit(owner.Id, sentMain.MessageID)
	return owner, sentMain.MessageID, text, keyboard
}

// settingsSwNoNetwork points the AI provider endpoint at a server that fails the test when it is
// reached, so a page that read the provider would be caught.
func settingsSwNoNetwork(t *testing.T) {
	t.Helper()
	withAISpamJevServer(t, func(http.ResponseWriter, *http.Request) {
		t.Error("the Switches page made a request to the AI provider")
	})
}

// settingsSwUsedMethods returns the Bot API methods the fake has been asked for so far.
func settingsSwUsedMethods(env *settingsEnv) []string {
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	var methods []string
	for _, call := range env.fake.calls {
		if !slices.Contains(methods, call.Method) {
			methods = append(methods, call.Method)
		}
	}
	return methods
}

func TestSettingsSwitchesAIStatus(t *testing.T) {
	configure := []struct {
		name      string
		available bool
		setup     func(*testing.T)
	}{
		{"configured", true, withAISpamConfigured},
		{"provider value empty", false, func(t *testing.T) { withAISpamNotConfigured(t, aispamProviderValueEmpty) }},
		{"global switch off", false, func(t *testing.T) { withAISpamNotConfigured(t, aispamSwitchOff) }},
	}
	for _, private := range []bool{false, true} {
		menu := map[bool]string{false: "group menu", true: "private menu"}[private]
		for _, c := range configure {
			for _, stored := range []bool{false, true} {
				name := fmt.Sprintf("%s: %s, stored %v", menu, c.name, stored)
				t.Run(name, func(t *testing.T) {
					c.setup(t)
					settingsSwNoNetwork(t)
					env := newSettingsSwitchesEnv(t)
					if stored {
						if err := aispam.SetAISpamEnabled(env.chat.Id, true); err != nil {
							t.Fatalf("seed: %v", err)
						}
					}
					var text string
					var keyboard [][]gotgbot.InlineKeyboardButton
					if private {
						_, _, text, keyboard = settingsSwOpenPrivate(t, env)
					} else {
						_, text, keyboard = settingsSwOpenPage(t, env)
					}

					sentence := "settings_sw_ai_off"
					if stored {
						sentence = "aispam_status_enabled"
					}
					if !strings.Contains(text, staffMarker(sentence)) {
						t.Errorf("page %q does not contain the state sentence %s", text, sentence)
					}
					hasNotice := strings.Contains(text, staffMarker("aispam_data_notice"))
					unavailable := "🚫 " + staffMarker("settings_sw_ai_unavailable")
					hasUnavailable := strings.Contains(text, unavailable)
					if hasNotice != c.available || hasUnavailable == c.available {
						t.Errorf("data notice %v and unavailable line %v with the filter available = %v, want exactly one of them, the notice when available",
							hasNotice, hasUnavailable, c.available)
					}
					for _, cause := range []string{"aispam_status_key_missing", "aispam_status_globally_disabled", "aispam_no_key", "aispam_globally_disabled"} {
						if strings.Contains(text, staffMarker(cause)) {
							t.Errorf("page %q names a cause (%s)", text, cause)
						}
					}

					toggle, drawn := settingsButtonByMarker(keyboard, "settings_btn_sw_aispam")
					wantDrawn := c.available || stored
					if drawn != wantDrawn {
						t.Errorf("AI toggle drawn = %v, want %v", drawn, wantDrawn)
					}
					wantWidths := []int{1, 1, 1, 2}
					if !wantDrawn {
						wantWidths = []int{1, 1, 2}
					}
					if got := settingsSwWidths(keyboard); !slices.Equal(got, wantWidths) {
						t.Errorf("keyboard widths = %v, want %v", got, wantWidths)
					}
					if drawn {
						want := "1"
						if stored {
							want = "0"
						}
						if fields := settingsButtonFields(t, toggle); fields["v"] != want || fields["t"] != "a" || fields["a"] != "tg" {
							t.Errorf("AI toggle fields = %v, want a=tg t=a v=%s", fields, want)
						}
					}
					// The read made no request beyond what the press pipeline asks of Telegram.
					for _, method := range settingsSwUsedMethods(env) {
						if !slices.Contains([]string{"sendMessage", "editMessageText", "answerCallbackQuery", "getChatMember", "getChat", "getChatAdministrators"}, method) {
							t.Errorf("opening the page called the Bot API method %s", method)
						}
					}
				})
			}
		}
	}
}

func TestSettingsSwitchesAIGate(t *testing.T) {
	const menuMsgID = 7474

	// others presses Reports and Auto-approve off and on after a refusal, proving they are
	// unaffected by the AI gate.
	others := func(t *testing.T, env *settingsEnv, chatID int64) {
		t.Helper()
		listEditAdmin(env, env.chat, env.admin, true)
		settingsSwToggle(t, env, env.chat, menuMsgID, "r", "0")
		settingsSwToggle(t, env, env.chat, menuMsgID, "j", "1")
		if row, ok := settingsSwReportsRow(t, chatID); !ok || row.Status {
			t.Errorf("Reports after its press = %+v (row %v), want off", row, ok)
		}
		if row, ok := settingsEditStored(t, chatID); !ok || !row.ShouldAutoApprove {
			t.Errorf("Auto-approve after its press = %+v (row %v), want on", row, ok)
		}
	}

	t.Run("unavailable and stored off: a forged On is the alert, writes nothing and redraws the Off-only form", func(t *testing.T) {
		withAISpamNotConfigured(t, aispamProviderValueEmpty)
		settingsSwNoNetwork(t)
		env := newSettingsSwitchesEnv(t)
		answers := env.answerCount()
		settingsSwToggle(t, env, env.chat, menuMsgID, "a", "1")

		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press answered %d times, want once", got)
		}
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_sw_ai_unavailable")) {
			t.Errorf("answer = %q (alert %v), want the unavailable alert", answer, alert)
		}
		if _, has := settingsSwAIRow(t, env.chat.Id); has {
			t.Error("the refused press stored an ai_spam_settings row")
		}
		text, keyboard := env.lastEdit(env.chat.Id, menuMsgID)
		if !strings.Contains(text, "🚫 "+staffMarker("settings_sw_ai_unavailable")) || !strings.Contains(text, staffMarker("settings_sw_ai_off")) {
			t.Errorf("redrawn page %q, want the off sentence and the unavailable line", text)
		}
		if _, drawn := settingsButtonByMarker(keyboard, "settings_btn_sw_aispam"); drawn {
			t.Error("the Off-only form still draws the AI toggle")
		}
		if got := settingsSwWidths(keyboard); !slices.Equal(got, []int{1, 1, 2}) {
			t.Errorf("keyboard widths = %v, want [1 1 2]", got)
		}
		others(t, env, env.chat.Id)
	})

	t.Run("unavailable and stored on: Off writes and the toggle goes, a forged On changes nothing and keeps it", func(t *testing.T) {
		withAISpamNotConfigured(t, aispamSwitchOff)
		settingsSwNoNetwork(t)
		env := newSettingsSwitchesEnv(t)
		if err := aispam.SetAISpamEnabled(env.chat.Id, true); err != nil {
			t.Fatalf("seed: %v", err)
		}

		settingsSwToggle(t, env, env.chat, menuMsgID, "a", "1")
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_sw_ai_unavailable")) {
			t.Errorf("answer = %q (alert %v), want the unavailable alert", answer, alert)
		}
		_, keyboard := env.lastEdit(env.chat.Id, menuMsgID)
		if _, drawn := settingsButtonByMarker(keyboard, "settings_btn_sw_aispam"); !drawn {
			t.Error("a stored On must keep its toggle so it can be switched off")
		}
		if row, _ := settingsSwAIRow(t, env.chat.Id); !row.Enabled {
			t.Error("a refused On press changed the stored value")
		}

		settingsSwToggle(t, env, env.chat, menuMsgID, "a", "0")
		if row, _ := settingsSwAIRow(t, env.chat.Id); row.Enabled {
			t.Error("Off was refused while unavailable, it must always work")
		}
		text, keyboard := env.lastEdit(env.chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("settings_sw_ai_off")) {
			t.Errorf("redrawn page %q, want the off sentence", text)
		}
		if _, drawn := settingsButtonByMarker(keyboard, "settings_btn_sw_aispam"); drawn {
			t.Error("the toggle is still drawn after Off while unavailable")
		}
		others(t, env, env.chat.Id)
	})

	t.Run("available: On writes and draws the data notice", func(t *testing.T) {
		withAISpamConfigured(t)
		env := newSettingsSwitchesEnv(t)
		settingsSwToggle(t, env, env.chat, menuMsgID, "a", "1")
		if row, ok := settingsSwAIRow(t, env.chat.Id); !ok || !row.Enabled {
			t.Errorf("row after On = %+v (found %v), want enabled", row, ok)
		}
		text, _ := env.lastEdit(env.chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("aispam_status_enabled")) || !strings.Contains(text, staffMarker("aispam_data_notice")) {
			t.Errorf("redrawn page %q, want the on sentence and the data notice", text)
		}
		others(t, env, env.chat.Id)
	})

	t.Run("a failing write redraws the stored state with the save-failed notice", func(t *testing.T) {
		withAISpamConfigured(t)
		env := newSettingsSwitchesEnv(t)
		previous := aispamWrites
		aispamWrites.SetEnabled = func(int64, bool) error { return fmt.Errorf("disk is full") }
		t.Cleanup(func() { aispamWrites = previous })
		settingsSwToggle(t, env, env.chat, menuMsgID, "a", "1")
		text, _ := env.lastEdit(env.chat.Id, menuMsgID)
		if !strings.Contains(text, staffMarker("common_settings_save_failed")) || !strings.Contains(text, staffMarker("settings_sw_ai_off")) {
			t.Errorf("redrawn page %q, want the stored off sentence and the save-failed notice", text)
		}
	})
}
