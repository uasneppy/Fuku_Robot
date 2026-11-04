//go:build testtools

package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// failingAdminCacheClient answers like adminCacheBotClient except that the methods named in
// failures return the scripted error.
type failingAdminCacheClient struct {
	inner    *adminCacheBotClient
	failures map[string]error
}

func (c *failingAdminCacheClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	if err, ok := c.failures[method]; ok {
		return nil, err
	}
	return c.inner.RequestWithContext(ctx, token, method, params, opts)
}

func (c *failingAdminCacheClient) GetAPIURL(opts *gotgbot.RequestOpts) string {
	return c.inner.GetAPIURL(opts)
}

func (c *failingAdminCacheClient) FileURL(token, path string, opts *gotgbot.RequestOpts) string {
	return c.inner.FileURL(token, path, opts)
}

// observedAdminEvents installs a recording observer for the test and returns a reader of
// the events it saw.
func observedAdminEvents(t *testing.T) func() []AdminListEvent {
	t.Helper()
	var mu sync.Mutex
	var events []AdminListEvent
	SetAdminListObserver(func(ev AdminListEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	})
	t.Cleanup(func() { SetAdminListObserver(nil) })
	return func() []AdminListEvent {
		mu.Lock()
		defer mu.Unlock()
		return append([]AdminListEvent(nil), events...)
	}
}

func adminObserverBot(status string, adminList string, failures map[string]error) *gotgbot.Bot {
	inner := &adminCacheBotClient{responses: map[string]json.RawMessage{
		"getChatMember:999": json.RawMessage(
			`{"status":"` + status + `","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`),
		"getChatAdministrators": json.RawMessage(adminList),
	}}
	bot := newAdminCacheBot(inner)
	bot.BotClient = &failingAdminCacheClient{inner: inner, failures: failures}
	return bot
}

const observerAdminList = `[` +
	`{"status":"creator","user":{"id":10,"is_bot":false,"first_name":"Owner"}},` +
	`{"status":"administrator","user":{"id":11,"is_bot":false,"first_name":"Admin"}},` +
	`{"status":"administrator","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}` +
	`]`

func TestAdminListObserver(t *testing.T) {
	t.Run("a successful load is one loaded event with every member", func(t *testing.T) {
		withMemoryMarshaler(t)
		events := observedAdminEvents(t)
		got := LoadAdminCache(adminObserverBot("administrator", observerAdminList, nil), -100901)
		if !got.Cached || len(got.UserInfo) != 3 {
			t.Fatalf("LoadAdminCache() = %+v, want the three listed members", got)
		}
		seen := events()
		if len(seen) != 1 || seen[0].Kind != AdminListLoaded || seen[0].ChatID != -100901 || len(seen[0].Admins) != 3 {
			t.Fatalf("events = %+v, want one loaded event with three members", seen)
		}
	})

	t.Run("a bot that is a member is a not-admin event", func(t *testing.T) {
		withMemoryMarshaler(t)
		events := observedAdminEvents(t)
		LoadAdminCache(adminObserverBot("member", `[]`, nil), -100902)
		seen := events()
		if len(seen) != 1 || seen[0].Kind != AdminListBotNotAdmin || seen[0].ChatID != -100902 || len(seen[0].Admins) != 0 {
			t.Fatalf("events = %+v, want one not-admin event", seen)
		}
	})

	t.Run("a refused bot lookup is a gone event", func(t *testing.T) {
		withMemoryMarshaler(t)
		events := observedAdminEvents(t)
		refused := &gotgbot.TelegramError{Method: "getChatMember", Code: 403, Description: "Forbidden: bot was kicked"}
		LoadAdminCache(adminObserverBot("administrator", observerAdminList, map[string]error{"getChatMember": refused}), -100903)
		seen := events()
		if len(seen) != 1 || seen[0].Kind != AdminListBotGone || seen[0].ChatID != -100903 {
			t.Fatalf("events = %+v, want one gone event", seen)
		}
	})

	t.Run("no event for a server error, a rate limit or a transport error on the bot lookup", func(t *testing.T) {
		for name, failure := range map[string]error{
			"500":       &gotgbot.TelegramError{Method: "getChatMember", Code: 500, Description: "Internal"},
			"429":       &gotgbot.TelegramError{Method: "getChatMember", Code: 429, Description: "Too Many Requests"},
			"transport": errors.New("connection reset"),
		} {
			t.Run(name, func(t *testing.T) {
				withMemoryMarshaler(t)
				events := observedAdminEvents(t)
				LoadAdminCache(adminObserverBot("administrator", observerAdminList, map[string]error{"getChatMember": failure}), -100904)
				if seen := events(); len(seen) != 0 {
					t.Fatalf("events = %+v, want none", seen)
				}
			})
		}
	})

	t.Run("no event for an empty list", func(t *testing.T) {
		withMemoryMarshaler(t)
		events := observedAdminEvents(t)
		LoadAdminCache(adminObserverBot("administrator", `[]`, nil), -100905)
		if seen := events(); len(seen) != 0 {
			t.Fatalf("events = %+v, want none for an empty list", seen)
		}
	})

	t.Run("no event when every retry fails", func(t *testing.T) {
		withMemoryMarshaler(t)
		events := observedAdminEvents(t)
		failure := &gotgbot.TelegramError{Method: "getChatAdministrators", Code: 500, Description: "Internal"}
		got := LoadAdminCache(adminObserverBot("administrator", observerAdminList, map[string]error{"getChatAdministrators": failure}), -100906)
		if got.Cached {
			t.Fatalf("LoadAdminCache() = %+v, want the empty uncached result", got)
		}
		if seen := events(); len(seen) != 0 {
			t.Fatalf("events = %+v, want none after failed retries", seen)
		}
	})

	t.Run("no event when an invalidation lands during the list call", func(t *testing.T) {
		withMemoryMarshaler(t)
		events := observedAdminEvents(t)
		inner := &adminCacheBotClient{responses: map[string]json.RawMessage{
			"getChatMember:999":     json.RawMessage(`{"status":"administrator","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`),
			"getChatAdministrators": json.RawMessage(observerAdminList),
		}}
		client := &delayedAdminCacheClient{
			inner:            inner,
			adminListStarted: make(chan struct{}),
			releaseAdminList: make(chan struct{}),
		}
		bot := newAdminCacheBot(inner)
		bot.BotClient = client
		finished := make(chan struct{})
		go func() {
			LoadAdminCache(bot, -100907)
			close(finished)
		}()
		release := sync.OnceFunc(func() { close(client.releaseAdminList) })
		t.Cleanup(func() { release(); <-finished })
		<-client.adminListStarted
		InvalidateAdminCache(-100907)
		release()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("the overtaken load did not finish")
		}
		if seen := events(); len(seen) != 0 {
			t.Fatalf("events = %+v, want none for a load overtaken by an invalidation", seen)
		}
	})

	t.Run("a panicking observer does not change the result", func(t *testing.T) {
		withMemoryMarshaler(t)
		SetAdminListObserver(func(AdminListEvent) { panic("observer broke") })
		t.Cleanup(func() { SetAdminListObserver(nil) })
		got := LoadAdminCache(adminObserverBot("administrator", observerAdminList, nil), -100908)
		if !got.Cached || len(got.UserInfo) != 3 || got.UserMap[11].User.Id != 11 {
			t.Fatalf("LoadAdminCache() = %+v, want the unchanged admin cache", got)
		}
		if found, _ := GetAdminCacheList(-100908); !found {
			t.Fatal("the admin list was not stored")
		}
	})

	t.Run("a nil observer clears the hook", func(t *testing.T) {
		withMemoryMarshaler(t)
		events := observedAdminEvents(t)
		SetAdminListObserver(nil)
		LoadAdminCache(adminObserverBot("administrator", observerAdminList, nil), -100909)
		if seen := events(); len(seen) != 0 {
			t.Fatalf("events = %+v, want none after clearing the observer", seen)
		}
	})
}
