package formatting

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
)

func TestFillPlaceholders(t *testing.T) {
	t.Parallel()

	user := &gotgbot.User{Id: 7, FirstName: "A&B", LastName: "<c>"}
	noName := &gotgbot.User{Id: 9, FirstName: "", Username: ""}
	withUsername := &gotgbot.User{Id: 8, FirstName: "Ada", LastName: "L", Username: "ada<&>"}

	cases := []struct {
		name  string
		text  string
		user  *gotgbot.User
		title string
		count string
		want  string
	}{
		{"names and id are escaped", "{first} {last} ({id})", user, "T", "0", "A&amp;B &lt;c&gt; (7)"},
		{"a value holding a token is not expanded again", "{first}|{last}", &gotgbot.User{Id: 1, FirstName: "{last}", LastName: "x"}, "T", "0", "{last}|x"},
		{"username and mention are the at-name when there is one", "{username} {mention}", withUsername, "T", "0", "@ada&lt;&amp;&gt; @ada&lt;&amp;&gt;"},
		{"without a username both are the html mention", "{username}|{mention}", &gotgbot.User{Id: 5, FirstName: "A<b>"}, "T", "0",
			`<a href="tg://user?id=5">A&lt;b&gt;</a>|<a href="tg://user?id=5">A&lt;b&gt;</a>`},
		{"an empty first name gets the no-name word", "{first}", noName, "T", "0", "PersonWithNoName"},
		{"a nil user gets the no-name word and id 0", "{first}|{fullname}|{username}|{id}", nil, "T", "0",
			"PersonWithNoName|PersonWithNoName|PersonWithNoName|0"},
		{"count and chat name", "{count} in {chatname}", user, "<G & Co>", "42", "42 in &lt;G &amp; Co&gt;"},
		{"fullname joins both names", "{fullname}", withUsername, "T", "0", "Ada L"},
		{"rules tokens are stripped and nothing is added", "{rules} hi {rules:same}", user, "T", "0", " hi "},
		{"rules up is stripped too", "a{rules:up}b", user, "T", "0", "ab"},
		{"a text without braces is returned untouched", "plain & text", user, "T", "0", "plain & text"},
		{"an unknown token is left alone", "{nope} {first}", user, "T", "0", "{nope} A&amp;B"},
	}
	for _, c := range cases {
		if got := FillPlaceholders(c.text, c.user, c.title, c.count); got != c.want {
			t.Errorf("%s: FillPlaceholders(%q) = %q, want %q", c.name, c.text, got, c.want)
		}
	}
}

// TestFillPlaceholdersMatchesFormattingReplacer proves the editor's dry run fills a message the way a real join does.
func TestFillPlaceholdersMatchesFormattingReplacer(t *testing.T) {
	originalDB := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = originalDB })

	chat := &gotgbot.Chat{Id: -1001234567891, Title: "Pair & <Chat>"}
	bot := &gotgbot.Bot{
		Token:     "123:test",
		BotClient: formattingBotClient{},
		User:      gotgbot.User{Id: 123, IsBot: true, Username: "FormatBot"},
	}
	users := []*gotgbot.User{
		nil,
		{Id: 11, FirstName: "Zed"},
		{Id: 12, FirstName: "A&B", LastName: "<c>", Username: "zed_1"},
		{Id: 13, FirstName: ""},
	}
	texts := []string{
		"hello",
		"{first} {last} {fullname}",
		"{username}|{mention}|{id}",
		"welcome to {chatname}",
		"{count} members, {first}",
		"{unknown} {first}{",
	}
	for _, text := range texts {
		for _, u := range users {
			// The bot client answers getChatMemberCount with 42, so the same count goes to FillPlaceholders.
			want, _ := FormattingReplacer(bot, chat, u, text, nil)
			got := FillPlaceholders(text, u, chat.Title, "42")
			if got != want {
				t.Errorf("text %q user %+v: FillPlaceholders = %q, FormattingReplacer = %q", text, u, got, want)
			}
		}
	}
}

func TestVisibleLength(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"tags dropped, entity decoded, emoji is two units", "<b>Hi</b> &amp; 😀", 7},
		{"plain ascii", "hello", 5},
		{"link keeps its label only", `<a href="https://e.com/a/long/path">go</a>`, 2},
		{"an astral character is two units", "😀😀", 4},
		{"a bmp character is one unit", "я", 1},
	}
	for _, c := range cases {
		if got := VisibleLength(c.in); got != c.want {
			t.Errorf("%s: VisibleLength(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}
