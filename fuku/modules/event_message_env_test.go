//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// eventMsgCleanup deletes the chats' stored event messages and their cached reads when the test ends.
func eventMsgCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatEventMessage{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(eventmsg.CacheKey(id))
		}
		dbcache.ResetLocalForTest()
	})
}

// eventMsgStore stores the group text of one event through the repository and registers the cleanup.
func eventMsgStore(t *testing.T, chatID int64, event, text string) {
	t.Helper()
	eventMsgCleanup(t, chatID)
	if err := eventmsg.SetText(t.Context(), chatID, event, eventmsg.DestGroup, text, 1); err != nil {
		t.Fatalf("SetText(%d, %s) error = %v", chatID, event, err)
	}
}

// eventMsgMethods is the method of every call the client recorded, in order.
func eventMsgMethods(client *moduleBotClient) []string {
	client.callsFor("") // waits for the async posts, as every read of the fake does
	client.mu.Lock()
	defer client.mu.Unlock()
	methods := make([]string, 0, len(client.calls))
	for _, call := range client.calls {
		methods = append(methods, call.Method)
	}
	return methods
}

// eventMsgIndex is the position of the first call of method, or -1.
func eventMsgIndex(methods []string, method string) int {
	for i, m := range methods {
		if m == method {
			return i
		}
	}
	return -1
}

// eventMsgFailingClient wraps a site's client and refuses each chosen method once with a chosen
// error. The wrapped client still records the call, so a refused send is counted like any other.
type eventMsgFailingClient struct {
	gotgbot.BotClient
	mu   sync.Mutex
	fail map[string]error
}

func (c *eventMsgFailingClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	response, err := c.BotClient.RequestWithContext(ctx, token, method, params, opts)
	c.mu.Lock()
	failure := c.fail[method]
	delete(c.fail, method)
	c.mu.Unlock()
	if failure != nil {
		return nil, failure
	}
	return response, err
}

// eventMsgRefuseOnce makes the site's next call of method fail with err.
func eventMsgRefuseOnce(s *botSite, method string, err error) {
	if failing, ok := s.bot.BotClient.(*eventMsgFailingClient); ok {
		failing.mu.Lock()
		failing.fail[method] = err
		failing.mu.Unlock()
		return
	}
	s.bot.BotClient = &eventMsgFailingClient{BotClient: s.bot.BotClient, fail: map[string]error{method: err}}
}

// eventMsgStoreRow stores a whole message (text, media and buttons) of one event through the repository.
func eventMsgStoreRow(t *testing.T, chatID int64, event, text, fileID string, mediaType int, buttons []models.Button) {
	t.Helper()
	eventMsgStore(t, chatID, event, text)
	if err := eventmsg.SetMedia(t.Context(), chatID, event, eventmsg.DestGroup, fileID, mediaType, 1); err != nil {
		t.Fatalf("SetMedia error = %v", err)
	}
	if err := eventmsg.SetButtons(t.Context(), chatID, event, eventmsg.DestGroup, buttons, 1); err != nil {
		t.Fatalf("SetButtons error = %v", err)
	}
}
