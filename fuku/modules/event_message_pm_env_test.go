//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// eventPMMember is the chat ID of the punished member's private chat: the moderation goldens punish
// user 42, so a private copy goes to chat 42.
const eventPMMember int64 = 42

// eventPMTestClient wraps a site's client and answers a send to the member's private chat as a
// message in a private chat, as Telegram would. Every request still reaches the wrapped client, which
// records it, so the private copy is read from the site's calls like any other request. Its script
// can refuse the next sends to the member, hold them until released, and name the member in the
// handler's getChat.
type eventPMTestClient struct {
	gotgbot.BotClient
	script *eventPMScript
}

// eventPMScript is what a test asks of the private chat: refusals answer the next sends to the member
// in order, hold (when set) keeps every send to the member waiting until it is closed, and memberName
// replaces the member's first name in the getChat answer.
type eventPMScript struct {
	mu         sync.Mutex
	refusals   []error
	hold       chan struct{}
	memberName string
}

func (c eventPMTestClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	toMember := fmt.Sprint(params["chat_id"]) == strconv.FormatInt(eventPMMember, 10)
	var refusal error
	var hold chan struct{}
	name := ""
	if c.script != nil {
		c.script.mu.Lock()
		if toMember && strings.HasPrefix(method, "send") {
			hold = c.script.hold
			if len(c.script.refusals) > 0 {
				refusal, c.script.refusals = c.script.refusals[0], c.script.refusals[1:]
			}
		}
		name = c.script.memberName
		c.script.mu.Unlock()
	}
	if toMember && method == "getChat" && name != "" {
		_, _ = c.BotClient.RequestWithContext(ctx, token, method, params, opts)
		return json.RawMessage(fmt.Sprintf(`{"id":%d,"type":"private","first_name":%q}`, eventPMMember, name)), nil
	}
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	response, err := c.BotClient.RequestWithContext(ctx, token, method, params, opts)
	if refusal != nil {
		return nil, refusal
	}
	if err != nil || !strings.HasPrefix(method, "send") || !toMember {
		return response, err
	}
	return json.RawMessage(fmt.Sprintf(`{"message_id":7001,"date":%d,"chat":{"id":%d,"type":"private","first_name":"Ann"}}`,
		time.Now().Unix(), eventPMMember)), nil
}

// eventPMScriptOf returns the site's private chat script.
func eventPMScriptOf(t *testing.T, s *botSite) *eventPMScript {
	t.Helper()
	client, ok := s.bot.BotClient.(eventPMTestClient)
	if !ok || client.script == nil {
		t.Fatal("the site has no private copy test client")
	}
	return client.script
}

// eventPMRefuse makes the site's next send to the member's private chat fail with err (each call
// queues one more refusal).
func eventPMRefuse(t *testing.T, s *botSite, err error) {
	t.Helper()
	script := eventPMScriptOf(t, s)
	script.mu.Lock()
	script.refusals = append(script.refusals, err)
	script.mu.Unlock()
}

// eventPMHold keeps every send to the member's private chat waiting until the returned release runs.
// Release is also registered in t.Cleanup, after newEventPMSite's wait, so it runs before the wait.
func eventPMHold(t *testing.T, s *botSite) (release func()) {
	t.Helper()
	script := eventPMScriptOf(t, s)
	gate := make(chan struct{})
	script.mu.Lock()
	script.hold = gate
	script.mu.Unlock()
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return release
}

// withTestEventPM registers a test-only event with a private copy: the event is appended to
// eventmsg.Events (so registerEventPM accepts it) and the spec is registered; both are put back when
// the test ends. A job never reads either table, so restoring them before the jobs end is safe.
func withTestEventPM(t *testing.T, event string, spec eventPMSpec) {
	t.Helper()
	mreqSwap(t, &eventmsg.Events, append(slices.Clone(eventmsg.Events), event))
	saved := eventPMSpecs
	eventPMSpecs = maps.Clone(saved)
	t.Cleanup(func() { eventPMSpecs = saved })
	registerEventPM(event, spec)
}

// withFastPMCopyPacer installs a private copy pacer built from the production options with a 1 ms
// interval and a 5 ms retry_after unit, and puts the previous one back when the test ends.
func withFastPMCopyPacer(t *testing.T) {
	t.Helper()
	previous := pmCopyPacer
	opts := pmCopyPacerOptions
	opts.Interval = time.Millisecond
	opts.RetryAfterUnit = 5 * time.Millisecond
	pmCopyPacer = ratelimit.NewTelegramPacer(opts)
	t.Cleanup(func() { pmCopyPacer = previous })
}

// waitEventPM blocks until no private copy job is in flight.
func waitEventPM(t *testing.T) {
	t.Helper()
	select {
	case <-pmCopyJobs.idleChan():
	case <-time.After(10 * time.Second):
		t.Fatal("the private copy jobs did not finish within 10 s")
	}
}

// newEventPMSite is a moderation golden site (real locales, a fresh supergroup, the target's getChat)
// with miniredis, a fast private copy pacer and a client that answers the member's private chat. The
// chat's event messages and the private copy keys are removed when the test ends, after the jobs.
func newEventPMSite(t *testing.T) *botSite {
	t.Helper()
	withMiniredisServer(t)
	withFastPMCopyPacer(t)
	s := newModerationGoldenSite(t)
	s.bot.BotClient = eventPMTestClient{BotClient: s.bot.BotClient, script: &eventPMScript{}}
	eventPMCleanup(t, s.chat.Id)
	t.Cleanup(pmCopyJobs.wait)
	return s
}

// eventPMCleanup deletes the chats' event messages, options and cached reads (eventMsgCleanup) and
// every fuku:pmcopy:* key when the test ends.
func eventPMCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	eventMsgCleanup(t, chatIDs...)
	t.Cleanup(func() {
		client := cache.GetRedisClient()
		if client == nil {
			return
		}
		keys, _ := client.Keys(context.Background(), "fuku:pmcopy:*").Result()
		if len(keys) > 0 {
			client.Del(context.Background(), keys...)
		}
	})
}

// storeEventPM stores the private copy's text of one event and, with on, switches the copy on: the
// only way to turn a copy on is SetSilent(..., pm, false, ...), as the page's switch does.
func storeEventPM(t *testing.T, chatID int64, event, text string, on bool) {
	t.Helper()
	if text != "" {
		if err := eventmsg.SetText(t.Context(), chatID, event, eventmsg.DestPM, text, 1); err != nil {
			t.Fatalf("SetText(%d, %s, pm) error = %v", chatID, event, err)
		}
	}
	if on {
		if err := eventmsg.SetSilent(t.Context(), chatID, event, eventmsg.DestPM, false, 1); err != nil {
			t.Fatalf("SetSilent(%d, %s, pm) error = %v", chatID, event, err)
		}
	}
}

// eventPMCall is one recorded request and its position among every request of the site.
type eventPMCall struct {
	Index  int
	Method string
	Params map[string]any
}

// eventPMCalls splits the site's sends, after the jobs, into the sends to the group and the sends to
// the member's private chat, each with its position. The handler's getChat of the member is not a send.
func eventPMCalls(t *testing.T, s *botSite) (group, private []eventPMCall) {
	t.Helper()
	waitEventPM(t)
	s.client.callsFor("") // waits for the async posts, as every read of the fake does
	s.client.mu.Lock()
	defer s.client.mu.Unlock()
	groupID, memberID := strconv.FormatInt(s.chat.Id, 10), strconv.FormatInt(eventPMMember, 10)
	for i, call := range s.client.calls {
		chatID := fmt.Sprint(call.Params["chat_id"])
		if !strings.HasPrefix(call.Method, "send") {
			continue
		}
		switch chatID {
		case memberID:
			private = append(private, eventPMCall{Index: i, Method: call.Method, Params: call.Params})
		case groupID:
			group = append(group, eventPMCall{Index: i, Method: call.Method, Params: call.Params})
		}
	}
	return group, private
}

// eventPMParams decodes a recorded request through JSON, the form Telegram receives.
func eventPMParams(t *testing.T, call eventPMCall) map[string]any {
	t.Helper()
	return rulesCallParams(t, moduleBotCall{Method: call.Method, Params: call.Params})
}

// eventPMOnly checks that exactly one send reached the member's private chat, that it is method
// and comes after every send to the group, and returns its parameters.
func eventPMOnly(t *testing.T, s *botSite, method string) map[string]any {
	t.Helper()
	group, private := eventPMCalls(t, s)
	if len(private) != 1 {
		t.Fatalf("sends to the member's private chat = %d, want the one private copy", len(private))
	}
	if private[0].Method != method {
		t.Fatalf("private copy method = %s, want %s", private[0].Method, method)
	}
	for _, g := range group {
		if g.Index > private[0].Index {
			t.Fatalf("the group's %s came after the private copy, want the copy last", g.Method)
		}
	}
	params := eventPMParams(t, private[0])
	if _, ok := params["reply_parameters"]; ok {
		t.Fatalf("private copy reply_parameters = %v, want none", params["reply_parameters"])
	}
	if _, ok := params["message_thread_id"]; ok {
		t.Fatalf("private copy message_thread_id = %v, want none", params["message_thread_id"])
	}
	if params["parse_mode"] != "HTML" {
		t.Fatalf("private copy parse_mode = %v, want HTML", params["parse_mode"])
	}
	return params
}

// eventPMNone checks that nothing reached the member's private chat.
func eventPMNone(t *testing.T, s *botSite) {
	t.Helper()
	if _, private := eventPMCalls(t, s); len(private) != 0 {
		t.Fatalf("sends to the member's private chat = %d (%s), want none", len(private), private[0].Method)
	}
}
