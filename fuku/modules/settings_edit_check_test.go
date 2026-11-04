//go:build testtools

package modules

import (
	"fmt"
	"html"
	"reflect"
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

// settingsEditPartCheck runs the check of part of the Welcome page for msg with state st.
func settingsEditPartCheck(t *testing.T, part string, msg *gotgbot.Message, st settingsEditState) settingsEditVerdict {
	t.Helper()
	def := settingsEditCheckEnv(t, settingsPageWelcome)
	st.Kind = def.Kind
	return settingsEditCheck(settingsEditCheckInput{
		Def: def, Part: part, Msg: msg, State: st,
		Owner: gotgbot.User{Id: 1, FirstName: "Al"}, GroupTitle: "Group", Language: "en",
	})
}

func TestSettingsEditMediaKinds(t *testing.T) {
	accepted := map[string]*gotgbot.Message{
		"photo":    {Photo: []gotgbot.PhotoSize{{FileId: "small"}, {FileId: "large"}}},
		"video":    {Video: &gotgbot.Video{FileId: "video-1"}},
		"gif":      {Animation: &gotgbot.Animation{FileId: "gif-1"}, Document: &gotgbot.Document{FileId: "gif-1"}},
		"document": {Document: &gotgbot.Document{FileId: "doc-1"}},
		"sticker":  {Sticker: &gotgbot.Sticker{FileId: "sticker-1"}},
		"audio":    {Audio: &gotgbot.Audio{FileId: "audio-1"}},
		"voice":    {Voice: &gotgbot.Voice{FileId: "voice-1"}},
	}
	for name, msg := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			got := settingsEditPartCheck(t, settingsEditPartMedia, msg, settingsEditState{})
			if !got.OK {
				t.Fatalf("a %s was refused: %q", name, got.RefusalKey)
			}
			wantID, wantType := content.ExtractMedia(msg)
			if got.FileID != wantID || got.DataType != wantType || got.FileID == "" {
				t.Fatalf("stored (%q, %d), want what the commands store (%q, %d)", got.FileID, got.DataType, wantID, wantType)
			}
		})
	}

	t.Run("a GIF is stored as a document", func(t *testing.T) {
		got := settingsEditPartCheck(t, settingsEditPartMedia, accepted["gif"], settingsEditState{})
		if got.DataType != db.DOCUMENT {
			t.Fatalf("a GIF is stored as type %d, want the document type %d", got.DataType, db.DOCUMENT)
		}
	})

	refused := map[string]*gotgbot.Message{
		"video note": {VideoNote: &gotgbot.VideoNote{FileId: "note-1"}},
		"contact":    {Contact: &gotgbot.Contact{PhoneNumber: "1"}},
		"location":   {Location: &gotgbot.Location{}},
		"venue":      {Venue: &gotgbot.Venue{Title: "v"}},
		"poll":       {Poll: &gotgbot.Poll{Id: "p"}},
		"dice":       {Dice: &gotgbot.Dice{Emoji: "x", Value: 1}},
		"story":      {Story: &gotgbot.Story{}},
		"plain text": {Text: "just words"},
		"nothing":    {},
	}
	for name, msg := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			got := settingsEditPartCheck(t, settingsEditPartMedia, msg, settingsEditState{})
			if got.OK || got.RefusalKey != "settings_edit_refuse_need_media" {
				t.Fatalf("a %s gave OK=%v key=%q, want settings_edit_refuse_need_media", name, got.OK, got.RefusalKey)
			}
		})
	}

	t.Run("a caption on the media is not used and the verdict says so", func(t *testing.T) {
		msg := &gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: "p"}}, Caption: "hello"}
		got := settingsEditPartCheck(t, settingsEditPartMedia, msg, settingsEditState{})
		if !got.OK || !got.CaptionIgnored {
			t.Fatalf("OK=%v CaptionIgnored=%v, want both true", got.OK, got.CaptionIgnored)
		}
		if bare := settingsEditPartCheck(t, settingsEditPartMedia, accepted["photo"], settingsEditState{}); bare.CaptionIgnored {
			t.Fatal("CaptionIgnored is true for a photo with no caption")
		}
	})
}

func TestSettingsEditMediaTextLong(t *testing.T) {
	photo := &gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: "p"}}}

	t.Run("a stored text over 1024 cannot be a caption", func(t *testing.T) {
		got := settingsEditPartCheck(t, settingsEditPartMedia, photo, settingsEditState{Text: strings.Repeat("a", 1025)})
		if got.OK || got.RefusalKey != "settings_edit_refuse_media_text_long" {
			t.Fatalf("OK=%v key=%q, want settings_edit_refuse_media_text_long", got.OK, got.RefusalKey)
		}
		if n, max := settingsEditParam(t, got, "n"), settingsEditParam(t, got, "max"); n != 1025 || max != 1024 {
			t.Fatalf("n = %d, max = %d, want 1025 and 1024", n, max)
		}
	})

	t.Run("a stored text whose worst-case fill is over 1024 cannot be a caption", func(t *testing.T) {
		text := strings.Repeat("a", 700) + strings.Repeat("{fullname}", 10)
		got := settingsEditPartCheck(t, settingsEditPartMedia, photo, settingsEditState{Text: text})
		if got.OK || got.RefusalKey != "settings_edit_refuse_media_text_long" {
			t.Fatalf("OK=%v key=%q, want settings_edit_refuse_media_text_long", got.OK, got.RefusalKey)
		}
		if n := settingsEditParam(t, got, "n"); n != 700+10*129 {
			t.Fatalf("n = %d, want %d", n, 700+10*129)
		}
	})

	t.Run("a stored text of 1024 is accepted", func(t *testing.T) {
		got := settingsEditPartCheck(t, settingsEditPartMedia, photo, settingsEditState{Text: strings.Repeat("a", 1024)})
		if !got.OK {
			t.Fatalf("a 1024 character text was refused: %q", got.RefusalKey)
		}
	})
}

// settingsEditButtonLines builds count button lines, the first n of them joined to their row.
func settingsEditButtonLines(count int, sameFrom int) string {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("[b%d](buttonurl://https://e.com/%d", i, i)
		if i >= sameFrom {
			lines[i] += ":same"
		}
		lines[i] += ")"
	}
	return strings.Join(lines, "\n")
}

func settingsEditButtonsCheck(t *testing.T, text string, st settingsEditState) settingsEditVerdict {
	t.Helper()
	return settingsEditPartCheck(t, settingsEditPartButtons, settingsEditTypedMessage(text), st)
}

func TestSettingsEditCheckButtonsParity(t *testing.T) {
	cases := map[string]string{
		"two buttons the second on the same row": "[Rules](buttonurl://https://example.com)\n[Chat](buttonurl://https://t.me/x:same)",
		"an empty name gets the default label":   "[](buttonurl://https://example.com)\n[Chat](buttonurl://https://t.me/x)",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			got := settingsEditButtonsCheck(t, text, settingsEditState{})
			if !got.OK {
				t.Fatalf("valid button lines were refused: %q", got.RefusalKey)
			}
			want := content.ExtractWelcome(&gotgbot.Message{Text: "/setwelcome " + text}, "welcome", "en").Buttons
			if len(want) == 0 || !reflect.DeepEqual(got.Buttons, want) {
				t.Fatalf("buttons = %+v, want what the commands store: %+v", got.Buttons, want)
			}
		})
	}

	t.Run("the default label is Greeting", func(t *testing.T) {
		got := settingsEditButtonsCheck(t, "[](buttonurl://https://example.com)", settingsEditState{})
		if !got.OK || len(got.Buttons) != 1 || got.Buttons[0].Name != "Greeting" {
			t.Fatalf("OK=%v buttons=%+v, want one button named Greeting", got.OK, got.Buttons)
		}
	})
}

func TestSettingsEditButtonRefusals(t *testing.T) {
	refuse := func(t *testing.T, got settingsEditVerdict, key string) {
		t.Helper()
		if got.OK || got.RefusalKey != key {
			t.Fatalf("OK=%v key=%q, want %s", got.OK, got.RefusalKey, key)
		}
	}

	t.Run("no readable button is a syntax refusal", func(t *testing.T) {
		refuse(t, settingsEditButtonsCheck(t, "no buttons here", settingsEditState{}), "settings_edit_refuse_buttons_syntax")
	})

	t.Run("a link that is not http or https names the button's number only", func(t *testing.T) {
		got := settingsEditButtonsCheck(t, "[a](buttonurl://https://e.com)\n[b](buttonurl://javascript:alert(1))", settingsEditState{})
		refuse(t, got, "settings_edit_refuse_button_url")
		if n := settingsEditParam(t, got, "n"); n != 2 {
			t.Fatalf("n = %d, want 2", n)
		}
		if len(got.RefusalParams) != 1 {
			t.Fatalf("the refusal carries %v, want only the button's number", got.RefusalParams)
		}
	})

	t.Run("a photo is not a buttons message", func(t *testing.T) {
		msg := &gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: "p"}}}
		refuse(t, settingsEditPartCheck(t, settingsEditPartButtons, msg, settingsEditState{}), "settings_edit_refuse_need_text")
	})

	t.Run("100 buttons are accepted and 101 are not", func(t *testing.T) {
		if got := settingsEditButtonsCheck(t, settingsEditButtonLines(100, 1000), settingsEditState{}); !got.OK {
			t.Fatalf("100 buttons were refused: %q", got.RefusalKey)
		}
		refuse(t, settingsEditButtonsCheck(t, settingsEditButtonLines(101, 1000), settingsEditState{}), "settings_edit_refuse_buttons_limit")
	})

	t.Run("8 in a row are accepted and 9 are not", func(t *testing.T) {
		if got := settingsEditButtonsCheck(t, settingsEditButtonLines(8, 1), settingsEditState{}); !got.OK {
			t.Fatalf("8 buttons in a row were refused: %q", got.RefusalKey)
		}
		refuse(t, settingsEditButtonsCheck(t, settingsEditButtonLines(9, 1), settingsEditState{}), "settings_edit_refuse_buttons_limit")
	})

	t.Run("a {rules} token in the text counts one more button", func(t *testing.T) {
		withToken := settingsEditState{Text: "Welcome {rules}"}
		if got := settingsEditButtonsCheck(t, settingsEditButtonLines(99, 1000), withToken); !got.OK {
			t.Fatalf("99 buttons with {rules} were refused: %q", got.RefusalKey)
		}
		refuse(t, settingsEditButtonsCheck(t, settingsEditButtonLines(100, 1000), withToken), "settings_edit_refuse_buttons_limit")
	})

	t.Run("{rules:same} widens the last row", func(t *testing.T) {
		withSame := settingsEditState{Text: "Welcome {rules:same}"}
		refuse(t, settingsEditButtonsCheck(t, settingsEditButtonLines(8, 1), withSame), "settings_edit_refuse_buttons_limit")
		if got := settingsEditButtonsCheck(t, settingsEditButtonLines(7, 1), withSame); !got.OK {
			t.Fatalf("7 buttons in a row with {rules:same} were refused: %q", got.RefusalKey)
		}
	})
}

func TestSettingsEditCheckMalformedEntities(t *testing.T) {
	broken := func() *gotgbot.Message {
		return settingsEditTypedMessage("hi", gotgbot.MessageEntity{Type: "bold", Offset: 50, Length: 4})
	}
	cases := []struct {
		page, part string
	}{
		{settingsPageWelcome, settingsEditPartText},
		{settingsPageGoodbye, settingsEditPartText},
		{settingsPageRules, settingsEditPartText},
		{settingsPageWelcome, settingsEditPartButtons},
	}
	for _, tc := range cases {
		t.Run(tc.page+"/"+tc.part, func(t *testing.T) {
			def := settingsEditCheckEnv(t, tc.page)
			got := settingsEditCheck(settingsEditCheckInput{
				Def: def, Part: tc.part, Msg: broken(), State: settingsEditState{Kind: def.Kind},
				Owner: gotgbot.User{Id: 1, FirstName: "Al"}, GroupTitle: "Group", Language: "en",
			})
			if got.OK || got.RefusalKey != "settings_edit_refuse_need_text" {
				t.Fatalf("OK=%v key=%q, want settings_edit_refuse_need_text", got.OK, got.RefusalKey)
			}
		})
	}
}
