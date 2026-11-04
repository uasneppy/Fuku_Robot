//go:build testtools

package modules

import (
	"strings"
	"testing"
	"unicode/utf16"

	tgmd2html "github.com/PaulSonOfLars/gotg_md2html"
	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/content"
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
