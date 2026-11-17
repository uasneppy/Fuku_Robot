//go:build testtools

package modules

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/filters"
)

// TestFilterNoformatFromMemberAnswersNormally proves that a member who writes "<keyword> noformat" gets the
// filter's normal answer, with no admin refusal, and that the message still reaches the later handler groups.
func TestFilterNoformatFromMemberAnswersNormally(t *testing.T) {
	var later atomic.Int32
	e := newPhase18E2E(t, func(e *phase18E2E) {
		LoadFilters(e.dispatcher)
		e.dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, func(*gotgbot.Bot, *ext.Context) error {
			later.Add(1)
			return ext.ContinueGroups
		}), 11)
	})
	filterGoldenCleanup(t, e.chat.Id)
	if err := filters.AddFilter(e.chat.Id, "hello", "<b>hi there</b>", "", nil, db.TEXT); err != nil {
		t.Fatalf("AddFilter: %v", err)
	}
	dbcache.ResetLocalForTest()
	member := e.member("Member")

	e.post(member, "hello noformat")
	if got := noteSentTo(e, e.chat.Id, "hi there"); got != 1 {
		t.Fatalf("the filter answered %d times, want once", got)
	}
	for _, call := range callsToChat(e.fake.staffBotClient, "sendMessage", e.chat.Id) {
		if text, _ := call.Params["text"].(string); !strings.Contains(text, "<b>hi there</b>") {
			t.Fatalf("a member got the raw filter: %q", text)
		}
	}
	if got := e.repliesWith("chat_status_user_admin_cmd_error"); got != 0 {
		t.Fatalf("a member was refused %d times", got)
	}
	if got := later.Load(); got != 1 {
		t.Fatalf("the later group saw the message %d times, want once", got)
	}
}
