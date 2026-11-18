//go:build testtools

package modules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The send sites that used Telegram's legacy Markdown moved to HTML (plan of 2026-11-18, Part 4 step 4). A value a
// user typed and the bot splices into one of their texts must reach Telegram escaped, so it can neither break the
// message nor add markup. These tests send <&> through each spliced value.

const markdownSitesUnsafe = `<b>x</b> & <i>`

func TestMarkdownSitesEscapeSplicedValues(t *testing.T) {
	withRealLocales(t)
	escaped := "&lt;b&gt;x&lt;/b&gt; &amp; &lt;i&gt;"
	for _, code := range []string{"uk", "en", "ru"} {
		tr := i18n.MustNewTranslator(code)
		if got := purgeDoneText(tr, 3, strings.Fields(markdownSitesUnsafe)); !strings.Contains(got, escaped) || strings.Contains(got, "<i>") {
			t.Errorf("%s: purge reason %q is not escaped", code, got)
		}
		if got := noteClickForPrivateText(tr, markdownSitesUnsafe); !strings.Contains(got, escaped) || strings.Contains(got, "<i>") {
			t.Errorf("%s: note name %q is not escaped", code, got)
		}
	}
	if got := disablingCommandList([]string{"ban", "a<b>&"}); got != "\n• <code>ban</code>\n• <code>a&lt;b&gt;&amp;</code>" {
		t.Errorf("disablingCommandList = %q", got)
	}
}

func TestSetWarnLimitEscapesItsArgument(t *testing.T) {
	withRealLocales(t)
	chatID := uniqueModuleChatID()
	warnGoldenCleanup(t, chatID)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Escape"}
	ctx := newModuleMessageContext(bot, chat, gotgbot.User{Id: 777000, FirstName: "Telegram"}, "/setwarnlimit <b>&")
	if err := warnsModule.setWarnLimit(bot, ctx); err != ext.EndGroups {
		t.Fatalf("setWarnLimit = %v, want EndGroups", err)
	}
	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(calls))
	}
	var req struct {
		ParseMode string `json:"parse_mode"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal([]byte(legacyRequestJSON(t, calls[0])), &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if req.ParseMode != "HTML" || !strings.Contains(req.Text, "&lt;b&gt;&amp;") {
		t.Errorf("request %+v is not HTML with the argument escaped", req)
	}
}
