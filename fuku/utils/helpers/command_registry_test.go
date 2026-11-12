//go:build testtools

package helpers

import (
	"fmt"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

// commandUpdate builds a message update whose text starts with a bot command entity.
func commandUpdate(bot *gotgbot.Bot, text string) *ext.Context {
	msg := &gotgbot.Message{
		MessageId: 1,
		Date:      1,
		Chat:      gotgbot.Chat{Id: -1001234, Type: "supergroup"},
		From:      &gotgbot.User{Id: 77, FirstName: "Member"},
		Text:      text,
		Entities:  []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: int64(len(text))}},
	}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)
}

func TestBotCommandRegistry(t *testing.T) {
	bot := &gotgbot.Bot{Token: "999:test", User: gotgbot.User{Id: 999, IsBot: true, Username: "FukuTestBot"}}
	noop := func(*gotgbot.Bot, *ext.Context) error { return nil }

	handler := NewBotCommand("Probe17", noop)
	reference := handlers.NewCommand("Probe17", noop)

	for _, text := range []string{"/probe17", "/Probe17", "/probe17@FukuTestBot", "/Probe17@fukutestbot", "/probe17@OtherBot", "/probe18"} {
		ctx := commandUpdate(bot, text)
		if got, want := handler.CheckUpdate(bot, ctx), reference.CheckUpdate(bot, ctx); got != want {
			t.Errorf("CheckUpdate(%q) = %v, the library's own handler says %v", text, got, want)
		}
	}
	if !handler.CheckUpdate(bot, commandUpdate(bot, "/Probe17@FukuTestBot")) {
		t.Error("the shim's handler does not match /Probe17@FukuTestBot")
	}
	if handler.Command != "probe17" {
		t.Errorf("handler command = %q, want the lower-cased name", handler.Command)
	}

	for name, want := range map[string]bool{"probe17": true, "PROBE17": true, "Probe17": true, "probe18": false, "": false} {
		if got := IsBotCommand(name); got != want {
			t.Errorf("IsBotCommand(%q) = %v, want %v", name, got, want)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("Race17x%d", i)
			NewBotCommand(name, noop)
			if !IsBotCommand(name) {
				t.Errorf("IsBotCommand(%q) = false right after registering it", name)
			}
			_ = IsBotCommand("probe18")
		}(i)
	}
	wg.Wait()
}
