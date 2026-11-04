//go:build testtools

package modules

import (
	"fmt"
	"html"
	"strings"
	"testing"
	"unicode/utf16"

	tgmd2html "github.com/PaulSonOfLars/gotg_md2html"
	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// shiftEntities returns entities with every offset moved by the UTF-16 length of prefix, the way
// Telegram numbers the entities of a command message ("/setwelcome " plus the text) that the
// door's text is compared with.
func shiftEntities(entities []gotgbot.MessageEntity, prefix string) []gotgbot.MessageEntity {
	shift := int64(len(utf16.Encode([]rune(prefix))))
	out := make([]gotgbot.MessageEntity, len(entities))
	for i, e := range entities {
		e.Offset += shift
		out[i] = e
	}
	return out
}

// settingsEditCheckEnv installs the real locale files and returns the page definition of key.
func settingsEditCheckEnv(t *testing.T, key string) settingsEditPageDef {
	t.Helper()
	withRealLocales(t)
	def, ok := settingsEditPageByKey(key)
	if !ok {
		t.Fatalf("no editor page %q", key)
	}
	return def
}

// settingsEditTypedMessage is a private message of the owner with the given text and entities.
func settingsEditTypedMessage(text string, entities ...gotgbot.MessageEntity) *gotgbot.Message {
	return &gotgbot.Message{Text: text, Entities: entities, Chat: gotgbot.Chat{Type: "private"}}
}

// settingsEditParityCases are the typed texts whose stored HTML must equal the commands'.
var settingsEditParityCases = []struct {
	name     string
	text     string
	entities []gotgbot.MessageEntity
}{
	{
		name: "bold link and emoji",
		text: "Hi 😀 bold link",
		entities: []gotgbot.MessageEntity{
			{Type: "bold", Offset: 6, Length: 4},
			{Type: "text_link", Offset: 11, Length: 4, Url: "https://example.com"},
		},
	},
	{
		name:     "spoiler",
		text:     "a secret here",
		entities: []gotgbot.MessageEntity{{Type: "spoiler", Offset: 2, Length: 6}},
	},
	{
		name:     "right to left word",
		text:     "שלום עולם and more",
		entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 0, Length: 4}},
	},
	{
		name: "bold and italic on the same words",
		text: "styled words here",
		entities: []gotgbot.MessageEntity{
			{Type: "bold", Offset: 0, Length: 13},
			{Type: "italic", Offset: 0, Length: 6},
		},
	},
	{name: "placeholder", text: "Welcome {first} to {chatname}"},
	{name: "typed stars with no entity", text: "this is *x* typed"},
}

func TestSettingsEditCheckTextParity(t *testing.T) {
	for _, page := range []string{settingsPageWelcome, settingsPageGoodbye} {
		def := settingsEditCheckEnv(t, page)
		command := "/setwelcome "
		kind := "welcome"
		if page == settingsPageGoodbye {
			command, kind = "/setgoodbye ", "goodbye"
		}
		for _, tc := range settingsEditParityCases {
			t.Run(page+"/"+tc.name, func(t *testing.T) {
				typed := settingsEditTypedMessage(tc.text, tc.entities...)
				got := settingsEditCheck(settingsEditCheckInput{
					Def: def, Part: settingsEditPartText, Msg: typed,
					State: settingsEditState{Kind: def.Kind}, Owner: gotgbot.User{Id: 1, FirstName: "Al"},
					GroupTitle: "Group", Language: "en",
				})
				if !got.OK {
					t.Fatalf("typed text refused: key %q text %q", got.RefusalKey, got.RefusalText)
				}
				cmd := &gotgbot.Message{
					Text:     command + tc.text,
					Entities: shiftEntities(tc.entities, command),
				}
				want := content.ExtractWelcome(cmd, kind, "en")
				if want.DataType == -1 {
					t.Fatalf("the command refused the text: %q", want.ErrorMsg)
				}
				if got.Text != want.Text {
					t.Fatalf("stored HTML differs from the command's\n got: %q\nwant: %q", got.Text, want.Text)
				}
			})
		}
	}
}

func TestSettingsEditCheckRulesParity(t *testing.T) {
	def := settingsEditCheckEnv(t, settingsPageRules)
	const command = "/setrules "
	for _, tc := range settingsEditParityCases {
		t.Run(tc.name, func(t *testing.T) {
			typed := settingsEditTypedMessage(tc.text, tc.entities...)
			got := settingsEditCheck(settingsEditCheckInput{
				Def: def, Part: settingsEditPartText, Msg: typed,
				State: settingsEditState{Kind: def.Kind}, Owner: gotgbot.User{Id: 1, FirstName: "Al"},
				GroupTitle: "Group", Language: "en",
			})
			if !got.OK {
				t.Fatalf("typed text refused: key %q", got.RefusalKey)
			}
			cmd := &gotgbot.Message{Text: command + tc.text, Entities: shiftEntities(tc.entities, command)}
			parts := strings.SplitN(cmd.OriginalMDV2(), " ", 2)
			if len(parts) != 2 {
				t.Fatalf("command text has no argument: %q", cmd.OriginalMDV2())
			}
			want := tgmd2html.MD2HTMLV2(parts[1])
			if got.Text != want {
				t.Fatalf("stored HTML differs from /setrules\n got: %q\nwant: %q", got.Text, want)
			}
		})
	}
}

// settingsEditGreetingCheck runs the check of the text part of a Welcome page for a typed text
// with the stored state st.
func settingsEditGreetingCheck(t *testing.T, text string, st settingsEditState) settingsEditVerdict {
	t.Helper()
	def := settingsEditCheckEnv(t, settingsPageWelcome)
	st.Kind = def.Kind
	return settingsEditCheck(settingsEditCheckInput{
		Def: def, Part: settingsEditPartText, Msg: settingsEditTypedMessage(text), State: st,
		Owner: gotgbot.User{Id: 1, FirstName: "Al"}, GroupTitle: "Group", Language: "en",
	})
}

// settingsEditParam reads a refusal parameter as an int.
func settingsEditParam(t *testing.T, v settingsEditVerdict, name string) int {
	t.Helper()
	n, ok := v.RefusalParams[name].(int)
	if !ok {
		t.Fatalf("refusal parameter %q is %#v, want an int", name, v.RefusalParams[name])
	}
	return n
}

func TestSettingsEditTextLimits(t *testing.T) {
	photo := settingsEditState{FileID: "file-1", MediaType: db.PHOTO}

	t.Run("a text over 4096 code points is refused with the commands' words", func(t *testing.T) {
		text := strings.Repeat("a", 4097)
		got := settingsEditGreetingCheck(t, text, settingsEditState{})
		if got.OK {
			t.Fatal("a 4097 character text was accepted")
		}
		want := content.TextLengthRefusal(text, db.TEXT, "en")
		if want == "" || got.RefusalText != want {
			t.Fatalf("RefusalText = %q, want the commands' %q", got.RefusalText, want)
		}
	})

	t.Run("a text of exactly 4096 is accepted", func(t *testing.T) {
		got := settingsEditGreetingCheck(t, strings.Repeat("a", 4096), settingsEditState{})
		if !got.OK {
			t.Fatalf("a 4096 character text was refused: %q %q", got.RefusalKey, got.RefusalText)
		}
	})

	t.Run("with media stored 1025 is refused as a caption", func(t *testing.T) {
		text := strings.Repeat("a", 1025)
		got := settingsEditGreetingCheck(t, text, photo)
		if got.OK {
			t.Fatal("a 1025 character caption was accepted")
		}
		want := content.TextLengthRefusal(text, db.PHOTO, "en")
		if want == "" || got.RefusalText != want {
			t.Fatalf("RefusalText = %q, want the commands' caption refusal %q", got.RefusalText, want)
		}
	})

	t.Run("with media stored 1024 is accepted", func(t *testing.T) {
		got := settingsEditGreetingCheck(t, strings.Repeat("a", 1024), photo)
		if !got.OK {
			t.Fatalf("a 1024 character caption was refused: %q %q", got.RefusalKey, got.RefusalText)
		}
	})
}

func TestSettingsEditFilledLength(t *testing.T) {
	// The worst-case member has a 64 letter first and last name: {fullname} is 64 + 1 + 64 = 129 units.
	const worstFullName = 129
	filler := func(n int) string { return strings.Repeat("a", n) }

	t.Run("fine for the owner but too long for the worst-case member", func(t *testing.T) {
		got := settingsEditGreetingCheck(t, filler(3900)+strings.Repeat("{fullname}", 10), settingsEditState{})
		if got.OK || got.RefusalKey != "settings_edit_refuse_too_long_filled" {
			t.Fatalf("verdict OK=%v key=%q, want settings_edit_refuse_too_long_filled", got.OK, got.RefusalKey)
		}
		if n, max := settingsEditParam(t, got, "n"), settingsEditParam(t, got, "max"); n != 3900+10*worstFullName || max != 4096 {
			t.Fatalf("n = %d, max = %d, want %d and 4096", n, max, 3900+10*worstFullName)
		}
	})

	t.Run("with media stored the limit is 1024", func(t *testing.T) {
		photo := settingsEditState{FileID: "file-1", MediaType: db.PHOTO}
		got := settingsEditGreetingCheck(t, filler(700)+strings.Repeat("{fullname}", 10), photo)
		if got.OK || got.RefusalKey != "settings_edit_refuse_too_long_filled" {
			t.Fatalf("verdict OK=%v key=%q, want settings_edit_refuse_too_long_filled", got.OK, got.RefusalKey)
		}
		if max := settingsEditParam(t, got, "max"); max != 1024 {
			t.Fatalf("max = %d, want 1024", max)
		}
	})

	t.Run("a text that fits for everyone is accepted", func(t *testing.T) {
		got := settingsEditGreetingCheck(t, filler(100)+strings.Repeat("{fullname}", 10), settingsEditState{})
		if !got.OK {
			t.Fatalf("a short filled text was refused: %q", got.RefusalKey)
		}
	})

	t.Run("last name alone is empty for a member without one", func(t *testing.T) {
		got := settingsEditGreetingCheck(t, "{last}", settingsEditState{})
		if got.OK || got.RefusalKey != "settings_edit_refuse_need_text" {
			t.Fatalf("verdict OK=%v key=%q, want settings_edit_refuse_need_text", got.OK, got.RefusalKey)
		}
	})

	t.Run("a last name with other words is accepted", func(t *testing.T) {
		if got := settingsEditGreetingCheck(t, "Hi {last}", settingsEditState{}); !got.OK {
			t.Fatalf("Hi {last} was refused: %q", got.RefusalKey)
		}
	})

	t.Run("a text of only blanks is refused", func(t *testing.T) {
		got := settingsEditGreetingCheck(t, "  \n ", settingsEditState{})
		if got.OK || got.RefusalKey != "settings_edit_refuse_need_text" {
			t.Fatalf("verdict OK=%v key=%q, want settings_edit_refuse_need_text", got.OK, got.RefusalKey)
		}
	})
}

func TestSettingsEditTextButtonLines(t *testing.T) {
	got := settingsEditGreetingCheck(t, "See us\n[x](buttonurl://https://e.com)", settingsEditState{})
	if got.OK || got.RefusalKey != "settings_edit_refuse_text_buttons" {
		t.Fatalf("verdict OK=%v key=%q, want settings_edit_refuse_text_buttons", got.OK, got.RefusalKey)
	}
}

// settingsEditRulesPrefixLength is the visible length of the longest text the /rules message
// puts before the rules, over the eight languages, for a group title.
func settingsEditRulesPrefixLength(t *testing.T, title string) int {
	t.Helper()
	withRealLocales(t)
	longest := 0
	for _, lang := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		tr := i18n.MustNewTranslator(lang)
		header, _ := tr.GetString("rules_for_chat_header")
		template, _ := tr.GetString("rules_for_chat")
		if header == "" || template == "" {
			t.Fatalf("locale %s lacks the rules heading keys", lang)
		}
		for _, prefix := range []string{
			fmt.Sprintf(header, html.EscapeString(title)) + "\n\n",
			fmt.Sprintf(template, html.EscapeString(title), ""),
		} {
			if n := formatting.VisibleLength(prefix); n > longest {
				longest = n
			}
		}
	}
	return longest
}

func settingsEditRulesCheck(t *testing.T, text, title string) settingsEditVerdict {
	t.Helper()
	def := settingsEditCheckEnv(t, settingsPageRules)
	return settingsEditCheck(settingsEditCheckInput{
		Def: def, Part: settingsEditPartText, Msg: settingsEditTypedMessage(text),
		State: settingsEditState{Kind: def.Kind}, Owner: gotgbot.User{Id: 1, FirstName: strings.Repeat("A", 60)},
		GroupTitle: title, Language: "en",
	})
}

func TestSettingsEditCheckRules(t *testing.T) {
	const title = "My Group"

	t.Run("rules that make /rules exactly 4096 are accepted", func(t *testing.T) {
		prefix := settingsEditRulesPrefixLength(t, title)
		if got := settingsEditRulesCheck(t, strings.Repeat("a", 4096-prefix), title); !got.OK {
			t.Fatalf("a /rules message of 4096 units was refused: %q", got.RefusalKey)
		}
	})

	t.Run("rules that make /rules 4097 are refused with the number and the limit", func(t *testing.T) {
		prefix := settingsEditRulesPrefixLength(t, title)
		got := settingsEditRulesCheck(t, strings.Repeat("a", 4097-prefix), title)
		if got.OK || got.RefusalKey != "settings_edit_refuse_too_long_filled" {
			t.Fatalf("verdict OK=%v key=%q, want settings_edit_refuse_too_long_filled", got.OK, got.RefusalKey)
		}
		if n, max := settingsEditParam(t, got, "n"), settingsEditParam(t, got, "max"); n != 4097 || max != 4096 {
			t.Fatalf("n = %d, max = %d, want 4097 and 4096", n, max)
		}
	})

	t.Run("placeholders are not filled in rules", func(t *testing.T) {
		prefix := settingsEditRulesPrefixLength(t, title)
		text := strings.Repeat("{first}", (4096-prefix)/len("{first}"))
		if got := settingsEditRulesCheck(t, text, title); !got.OK {
			t.Fatalf("rules holding {first} were refused (it must count as written): %q", got.RefusalKey)
		}
	})

	t.Run("blank rules are refused", func(t *testing.T) {
		got := settingsEditRulesCheck(t, "   ", title)
		if got.OK || got.RefusalKey != "settings_edit_refuse_need_text" {
			t.Fatalf("verdict OK=%v key=%q, want settings_edit_refuse_need_text", got.OK, got.RefusalKey)
		}
	})
}
