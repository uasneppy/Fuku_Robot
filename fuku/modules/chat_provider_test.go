package modules

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
)

// chatProviderStandIn is one httptest server that answers both providers' conversation calls and
// records every body it received.
type chatProviderStandIn struct {
	mu     sync.Mutex
	bodies []string
}

func (s *chatProviderStandIn) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.bodies = append(s.bodies, string(body))
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, ":generateContent") {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Gemini unit on duty."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":4}}`)
		return
	}
	_, _ = io.WriteString(w, `{"model":"claude-haiku-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"Claude unit on duty."}],"usage":{"input_tokens":10,"output_tokens":4}}`)
}

// TestChatProvidersRequest drives both real adapters through the watcher: each answers, the
// request names no user ID, chat ID or username, and the first system block is the same bytes for
// two different groups and members, which is what makes the provider's cache hit.
func TestChatProvidersRequest(t *testing.T) {
	standIn := &chatProviderStandIn{}
	server := httptest.NewServer(http.HandlerFunc(standIn.handler))
	t.Cleanup(server.Close)

	envA := newChatEnv(t)
	envB := newChatEnv(t)
	chatSwap(t, &chatProviderOf, chatProviderFor)
	chatSwap(t, &chatAnthropicEndpoint, server.URL)
	chatSwap(t, &chatGeminiEndpoint, server.URL)
	chatSwap(t, &chatProviderHTTP, server.Client())
	cfg := config.AppConfig
	chatSwap(t, &cfg.ChatAnthropicModel, "claude-haiku-5-5")
	chatSwap(t, &cfg.ChatGeminiModel, "gemini-4-hydrogen")

	alice := envA.member("Alice")
	alice.Username = "alice_handle"
	envA.deliver(envA.mention(alice, "status report"))

	if err := chatconv.SetProvider(context.Background(), envB.chatID, chatconv.ProviderGoogle, 1); err != nil {
		t.Fatalf("SetProvider: %v", err)
	}
	bob := envB.member("Bob")
	bob.Username = "bob_handle"
	envB.deliver(envB.mention(bob, "status report"))

	if got := envA.sends(); len(got) != 1 || got[0] != "Claude unit on duty." {
		t.Fatalf("anthropic group sends = %q", got)
	}
	if got := envB.sends(); len(got) != 1 || got[0] != "Gemini unit on duty." {
		t.Fatalf("google group sends = %q", got)
	}
	if len(standIn.bodies) != 2 {
		t.Fatalf("provider bodies = %d, want 2", len(standIn.bodies))
	}
	forbidden := []string{
		strconv.FormatInt(alice.Id, 10), strconv.FormatInt(bob.Id, 10),
		strconv.FormatInt(envA.chatID, 10), strconv.FormatInt(envB.chatID, 10),
		"alice_handle", "bob_handle",
	}
	for _, body := range standIn.bodies {
		for _, value := range forbidden {
			if strings.Contains(body, value) {
				t.Fatalf("a provider body carries %q", value)
			}
		}
	}

	var claudeBody struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
	}
	var geminiBody struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
	}
	if err := json.Unmarshal([]byte(standIn.bodies[0]), &claudeBody); err != nil || len(claudeBody.System) != 2 {
		t.Fatalf("anthropic body: %v, %d system blocks", err, len(claudeBody.System))
	}
	if err := json.Unmarshal([]byte(standIn.bodies[1]), &geminiBody); err != nil || len(geminiBody.SystemInstruction.Parts) != 2 {
		t.Fatalf("gemini body: %v", err)
	}
	if claudeBody.System[0].Text != geminiBody.SystemInstruction.Parts[0].Text {
		t.Fatalf("the static system block differs between two groups")
	}
}
