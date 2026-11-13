//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// languageCallbackPress presses /lang's button for code in a group as from and returns the requests
// the bot made, one per line.
func languageCallbackPress(t *testing.T, chatID int64, from gotgbot.User, code string) string {
	t.Helper()
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Language Wiring"}
	data := encodeCallbackData("change_language", map[string]string{"l": code})
	if err := languagesModule.langBtnHandler(bot, newModuleCallbackContext(bot, chat, from, data)); err != ext.EndGroups {
		t.Fatalf("handler error = %v, want EndGroups", err)
	}
	return languageGoldenCalls(t, client, chatID)
}

// TestLanguageCallbackWritesThroughApplyGroupLanguage proves the wiring the goldens cannot see: the
// group branch of the change_language callback stores a language through applyGroupLanguage, the
// function the settings menu's Language page uses, so the page and the buttons can never accept or
// refuse different things. The write seam applyGroupLanguage owns (languageWrites) is replaced by a
// recorder, and a real press of the button must reach it exactly once with the group and the code.
func TestLanguageCallbackWritesThroughApplyGroupLanguage(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	recorder := func(t *testing.T, fail error) *[]string {
		t.Helper()
		var writes []string
		mreqSwap(t, &languageWrites, func(chatID int64, code string) error {
			writes = append(writes, fmt.Sprintf("%d %s", chatID, code))
			return fail
		})
		return &writes
	}

	t.Run("an administrator's press is one write through the function", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		languageGoldenCleanup(t, []int64{chatID}, nil)
		writes := recorder(t, nil)
		out := languageCallbackPress(t, chatID, admin, "es")
		if want := fmt.Sprintf("%d es", chatID); len(*writes) != 1 || (*writes)[0] != want {
			t.Errorf("the press wrote %v, want exactly [%s] through applyGroupLanguage", *writes, want)
		}
		if !strings.Contains(out, "editMessageText") {
			t.Errorf("the press did not edit the message into the confirmation:\n%s", out)
		}
	})

	t.Run("a failed write is the save-failed answer and edits nothing", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		languageGoldenCleanup(t, []int64{chatID}, nil)
		writes := recorder(t, errors.New("database is down"))
		out := languageCallbackPress(t, chatID, admin, "es")
		if len(*writes) != 1 {
			t.Errorf("the press wrote %d times, want once", len(*writes))
		}
		if !strings.Contains(out, "answerCallbackQuery") || strings.Contains(out, "editMessageText") {
			t.Errorf("a failed write must answer and edit nothing:\n%s", out)
		}
	})

	t.Run("a refused press never reaches the write", func(t *testing.T) {
		for name, tc := range map[string]struct {
			from gotgbot.User
			code string
		}{
			"a plain member":         {from: member, code: "es"},
			"an unsupported code":    {from: admin, code: "de"},
			"an upper-case code":     {from: admin, code: "UK"},
			"an unsupported, member": {from: member, code: "de"},
		} {
			t.Run(name, func(t *testing.T) {
				chatID := uniqueModuleChatID()
				languageGoldenCleanup(t, []int64{chatID}, nil)
				writes := recorder(t, nil)
				languageCallbackPress(t, chatID, tc.from, tc.code)
				if len(*writes) != 0 {
					t.Errorf("a refused press wrote %v, want nothing", *writes)
				}
			})
		}
	})
}
