//go:build testtools

package modules

import (
	"testing"

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
