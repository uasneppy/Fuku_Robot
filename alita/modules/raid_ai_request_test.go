//go:build testtools

package modules

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/rules"
	"github.com/divkix/Alita_Robot/alita/db/warns"
)

// jsonKeys returns the sorted keys of a decoded JSON object.
func jsonKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestRaidAITextRequestIsMinimal proves what a raid-only TypeSafe call sends: the chat's
// title and language, the sender's first-seen days and messages-last-hour, and the text
// with its link and forward flags. The chat description, the rules, the warn count, a
// reply flag and any recent-message history never go (WR-03), although the chat has rules
// and the sender has a warning that /aispam itself would send.
func TestRaidAITextRequestIsMinimal(t *testing.T) {
	env := newRaidAIEnv(t)
	member := env.newMembers(1, 2*time.Hour)[0]

	const ruleText = "RULES-SENTINEL no advertising here"
	if err := rules.SetChatRules(env.chat.Id, ruleText); err != nil {
		t.Fatalf("set the chat rules: %v", err)
	}
	if _, _, err := warns.WarnUser(member.Id, env.chat.Id, "WARN-SENTINEL"); err != nil {
		t.Fatalf("warn the member: %v", err)
	}

	var mu sync.Mutex
	var bodies [][]byte
	withAISpamJevServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read the TypeSafe request: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, raw)
		mu.Unlock()
		_, _ = w.Write([]byte(aispamDecisionBody(0.1, "none")))
	})

	// A reply, so the reply flag would show if it were sent.
	env.dispatchMessage(env.chat, gotgbot.Message{
		From:           &member,
		Text:           spamLink,
		ReplyToMessage: &gotgbot.Message{MessageId: 1, Chat: env.chat},
	})
	env.settleAI()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("TypeSafe received %d requests, want exactly 1", len(bodies))
	}
	for _, leak := range []string{"RULES-SENTINEL", "WARN-SENTINEL", `"warn_count"`, `"description"`, `"rules"`, `"recent_from_sender"`, `"is_reply"`} {
		if strings.Contains(string(bodies[0]), leak) {
			t.Errorf("the raid-only request contains %q, want it left out", leak)
		}
	}

	var request struct {
		State map[string]map[string]any `json:"state"`
	}
	if err := json.Unmarshal(bodies[0], &request); err != nil {
		t.Fatalf("decode the request: %v", err)
	}
	if got := strings.Join(jsonKeys(toAny(request.State)), ","); got != "chat,message,sender" {
		t.Errorf("state keys = %s, want chat,message,sender", got)
	}
	wantKeys := map[string][]string{
		"chat":    {"language", "title"},
		"sender":  {"first_seen_days_ago", "messages_last_hour"},
		"message": {"has_link", "is_forward", "link_domains", "text"},
	}
	for part, want := range wantKeys {
		if got := jsonKeys(request.State[part]); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("state.%s keys = %v, want %v", part, got, want)
		}
	}
	if got := request.State["chat"]["title"]; got != env.chat.Title {
		t.Errorf("state.chat.title = %v, want %q", got, env.chat.Title)
	}
	if got := request.State["message"]["text"]; got != spamLink {
		t.Errorf("state.message.text = %v, want the message text", got)
	}
	if got := request.State["message"]["has_link"]; got != true {
		t.Errorf("state.message.has_link = %v, want true", got)
	}
	if got := request.State["sender"]["messages_last_hour"]; got != float64(1) {
		t.Errorf("state.sender.messages_last_hour = %v, want 1", got)
	}
}

// toAny widens a decoded state object so jsonKeys can list its keys.
func toAny(in map[string]map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
