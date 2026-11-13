//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsLocksStates are the lock maps a page is drawn from: no lock, every lock and every other one.
func settingsLocksStates() map[string]map[string]bool {
	all, alternate := map[string]bool{}, map[string]bool{}
	for i, row := range settingsLockRows {
		all[row.Kind] = true
		alternate[row.Kind] = i%2 == 0
	}
	return map[string]map[string]bool{"none": {}, "all": all, "alternate": alternate}
}

// settingsLocksWantWidths is the row widths of one page: the grid, the pager row and the navigation row.
func settingsLocksWantWidths(page int) []int {
	if page == 1 {
		return []int{2, 2, 2, 2, 2, 2, 1, 2}
	}
	return []int{2, 2, 2, 2, 2, 1, 2}
}

func TestSettingsLocksCallbackBytes(t *testing.T) {
	newSettingsEnv(t) // installs the marker locale the translator reads
	tr := i18n.MustNewTranslator("en")
	for _, variant := range settingsWarnsViews(tr, "G") {
		t.Run(variant.name, func(t *testing.T) {
			largestToggle, largestPager := 0, 0
			for stateName, state := range settingsLocksStates() {
				for page := 1; page <= settingsLocksPages; page++ {
					label := fmt.Sprintf("%s page %d", stateName, page)
					screen := renderSettingsLocks(variant.view, state, page, "", "")
					if screen.Omitted != 0 {
						t.Errorf("%s omitted %d buttons for overflow, want none", label, screen.Omitted)
					}
					for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
						n := len(button.CallbackData)
						if n < 1 || n > 64 {
							t.Errorf("%s button %q has %d bytes of callback data, want 1..64", label, button.Text, n)
						}
						switch decoded, _ := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); decoded.Fields["a"] {
						case settingsActToggle:
							largestToggle = max(largestToggle, n)
						case settingsActLocksPage:
							largestPager = max(largestPager, n)
						}
					}
				}
			}
			wantToggle, wantPager := 46, 37
			if !variant.view.Private {
				wantToggle, wantPager = 42, 33
			}
			if largestToggle != wantToggle || largestPager != wantPager {
				t.Errorf("the largest toggle is %d bytes and the largest pager %d, want %d and %d", largestToggle, largestPager, wantToggle, wantPager)
			}
		})
	}
}

func TestSettingsLocksPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	titles := map[string]string{
		"ampersands": strings.Repeat("&", 64),
		"astral":     strings.Repeat("😀", 64),
		"multibyte":  strings.Repeat("Ж", 64),
	}
	const cellMax = 20
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			press := &settingsPress{tr: tr}
			rights := []string{
				"",
				"🚫 " + settingsText(tr, "settings_del_no_right"),
				settingsUnknownState + " " + settingsText(tr, "settings_del_right_unknown"),
			}
			// The longest notices: the command's own failure sentence for each lock, and the refusal.
			notices := []string{"", settingsNoticeWarn(settingsText(tr, "connections_is_user_connected_bot_not_admin"))}
			for _, row := range settingsLockRows {
				notices = append(notices,
					settingsLocksFailureNotice(press, "locks_lock_failed", row),
					settingsLocksFailureNotice(press, "locks_unlock_failed", row))
			}
			for titleName, title := range titles {
				for _, variant := range settingsWarnsViews(tr, title) {
					budget := 9
					if variant.view.Private {
						budget = 8
					}
					for stateName, state := range settingsLocksStates() {
						for page := 1; page <= settingsLocksPages; page++ {
							for _, right := range rights {
								for _, notice := range notices {
									label := fmt.Sprintf("%s %s %s %s page %d", code, titleName, variant.name, stateName, page)
									screen := renderSettingsLocks(variant.view, state, page, right, notice)
									if units := panelUTF16Len(screen.Text); units > 3800 || units == 0 {
										t.Errorf("%s is %d UTF-16 units, want 1..3800", label, units)
									}
									if screen.Omitted != 0 {
										t.Errorf("%s omitted %d buttons", label, screen.Omitted)
									}
									for _, placeholder := range []string{"{state}", "{icon}", "{page}", "{pages}", "%!", "%s"} {
										if strings.Contains(screen.Text, placeholder) {
											t.Errorf("%s keeps the unfilled %s", label, placeholder)
										}
									}
									rows := screen.Keyboard.InlineKeyboard
									if got := settingsSwWidths(rows); !slices.Equal(got, settingsLocksWantWidths(page)) {
										t.Errorf("%s keyboard widths = %v, want %v", label, got, settingsLocksWantWidths(page))
									}
									if len(rows) > budget {
										t.Errorf("%s has %d rows, want at most %d", label, len(rows), budget)
									}
									for _, button := range settingsAllButtons(rows) {
										if strings.TrimSpace(button.Text) == "" {
											t.Errorf("%s has a button with an empty label", label)
										}
										if decoded, ok := decodeCallbackData(button.CallbackData, settingsCallbackNamespace); ok &&
											decoded.Fields["a"] == settingsActToggle && utf8.RuneCountInString(button.Text) > cellMax {
											t.Errorf("%s cell %q is %d code points, want at most %d", label, button.Text, utf8.RuneCountInString(button.Text), cellMax)
										}
									}
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestSettingsLocksCost(t *testing.T) {
	const menuMsgID = 7919

	t.Run("Main makes no Telegram call for the Locks line", func(t *testing.T) {
		env := newSettingsLocksEnv(t)
		tr := i18n.MustNewTranslator("en")
		before := settingsLocksCallTotal(env.settingsEnv)
		line, ok := settingsLocksMainState(env.bot, tr, &env.chat)
		if !ok || line == "" {
			t.Fatalf("the Locks state = %q (%v), want a state", line, ok)
		}
		if got := settingsLocksCallTotal(env.settingsEnv) - before; got != 0 {
			t.Errorf("the Locks line of Main made %d Telegram calls, want none", got)
		}
	})

	t.Run("a draw makes the pipeline's calls and one read of the bot", func(t *testing.T) {
		env := newSettingsLocksEnv(t)
		settingsLocksWarmBotRight(env)
		env.setBot(settingsDelBotJSON("administrator", true))
		for _, kv := range [][]string{
			{"a", "op", "p", settingsPageLocks},
			{"a", "tg", "p", settingsPageLocks, "t", settingsLocksCode(t, "gif"), "v", "1"},
			{"a", settingsActLocksPage, "v", "2"},
		} {
			before := settingsMenuMarkOf(env.settingsEnv)
			botReads, presserReads := env.botReads(env.chat.Id), env.memberLookupsOf(env.chat.Id, env.admin.Id)
			env.pressRaw(env.chat, env.admin, menuMsgID, encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, kv...)))
			after := settingsMenuMarkOf(env.settingsEnv)
			if got := env.botReads(env.chat.Id) - botReads; got != 1 {
				t.Errorf("press %v read the bot %d times, want exactly one", kv, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, env.admin.Id) - presserReads; got != 1 {
				t.Errorf("press %v looked up the presser %d times, want the pipeline's one", kv, got)
			}
			if got := after.chats - before.chats; got != 0 {
				t.Errorf("press %v made %d getChat calls, want none in a group menu", kv, got)
			}
			if got := after.edits - before.edits; got != 1 {
				t.Errorf("press %v edited %d times, want one", kv, got)
			}
		}
	})

	t.Run("a forged press makes no call", func(t *testing.T) {
		env := newSettingsLocksEnv(t)
		before := settingsMenuMarkOf(env.settingsEnv)
		botReads := env.botReads(env.chat.Id)
		fields := env.groupFields(env.admin, "a", "st", "p", settingsPageLocks, "t", settingsLocksCode(t, "gif"), "v", "1")
		env.pressRaw(env.chat, env.admin, menuMsgID, encodeCallbackData(settingsCallbackNamespace, fields))
		settingsMenuWantRefused(t, env.settingsEnv, before, "common_callback_invalid_request", true)
		if got := env.botReads(env.chat.Id) - botReads; got != 0 {
			t.Errorf("a forged press read the bot %d times, want none", got)
		}
	})
}

// settingsLocksCallTotal is how many Telegram requests the env's fake has received.
func settingsLocksCallTotal(env *settingsEnv) int {
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	return len(env.fake.calls)
}
