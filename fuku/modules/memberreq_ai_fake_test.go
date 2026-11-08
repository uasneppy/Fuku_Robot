//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/namecanon"
)

// The Anthropic stand-in of the AI spam-name rule's tests. It follows the image stand-in
// (raidAIImageEnv.serveClaude): the same answer envelope, a tool_use block named claude.ToolName
// with spam_probability. It records every decoded request and counts them. No test calls a real
// provider, and a test never sets a real key: withAIConfigured sets the fake key of the image
// tests (raidAITestKey).

// mreqAIStep is one answer of the stand-in. release is closed by the test's cleanup, so a step
// that hangs always ends with the request's context or the cleanup, never later.
type mreqAIStep func(w http.ResponseWriter, r *http.Request, release <-chan struct{})

// mreqAIProbability answers a spam probability in the forced tool call.
func mreqAIProbability(probability float64) mreqAIStep {
	return mreqAIBody(raidAIClaudeAnswer(probability))
}

// mreqAIBody answers HTTP 200 with the given body.
func mreqAIBody(body string) mreqAIStep {
	return func(w http.ResponseWriter, _ *http.Request, _ <-chan struct{}) {
		_, _ = w.Write([]byte(body))
	}
}

// mreqAIStatus answers an Anthropic error envelope with the given HTTP status and, when
// retryAfter is not empty, a retry-after header.
func mreqAIStatus(status int, errType, retryAfter string) mreqAIStep {
	return func(w http.ResponseWriter, _ *http.Request, _ <-chan struct{}) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(raidAIClaudeError(errType, "test error")))
	}
}

// mreqAIAnswerWithStop answers a well-formed message with the given stop reason and content.
func mreqAIAnswerWithStop(stop, content string) mreqAIStep {
	return mreqAIBody(`{"id":"msg_test","type":"message","role":"assistant","model":"` + raidAITestModel +
		`","content":` + content + `,"stop_reason":"` + stop + `","usage":{"input_tokens":700,"output_tokens":20}}`)
}

// mreqAIHang never answers: it returns when the request's context ends (the client gave up) or
// when the stand-in is released.
func mreqAIHang() mreqAIStep {
	return func(_ http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}
}

// mreqAIDrop closes the connection without an answer: a transport failure.
func mreqAIDrop() mreqAIStep {
	return func(w http.ResponseWriter, _ *http.Request, _ <-chan struct{}) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hijacker.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}
}

// mreqAIRequest is one request the stand-in received.
type mreqAIRequest struct {
	// Key is the x-api-key header.
	Key string
	// Body is the raw request body.
	Body []byte
}

// mreqAIState is the stand-in's script and record. The default step answers every request; it is
// a 0.1 probability until a test sets another.
type mreqAIState struct {
	mu        sync.Mutex
	def       mreqAIStep
	requests  []mreqAIRequest
	onRequest func()
	release   chan struct{}
	once      sync.Once
}

// setDefault sets the answer given when nothing is queued.
func (s *mreqAIState) setDefault(step mreqAIStep) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.def = step
}

// setOnRequest installs a hook that runs when a request arrives, before it is answered.
func (s *mreqAIState) setOnRequest(hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRequest = hook
}

// count is how many requests the stand-in received.
func (s *mreqAIState) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// received returns a copy of the recorded requests.
func (s *mreqAIState) received() []mreqAIRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]mreqAIRequest(nil), s.requests...)
}

// releaseHangs ends every step that is waiting, once.
func (s *mreqAIState) releaseHangs() {
	s.once.Do(func() { close(s.release) })
}

func (s *mreqAIState) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, mreqAIRequest{Key: r.Header.Get("x-api-key"), Body: body})
	step := s.def
	hook := s.onRequest
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	step(w, r, s.release)
}

// mreqInstallAIFake starts the stand-in, points the names request at it and restores both seams
// when the test ends. The default answer is a 0.1 probability.
func mreqInstallAIFake(t *testing.T) *mreqAIState {
	t.Helper()
	state := &mreqAIState{release: make(chan struct{}), def: mreqAIProbability(0.1)}
	server := httptest.NewServer(http.HandlerFunc(state.serve))
	t.Cleanup(server.Close)
	// Registered after the server's Close, so it runs first: a hung request ends before Close waits.
	t.Cleanup(state.releaseHangs)
	mreqSwap(t, &memberReqAIEndpoint, server.URL)
	mreqSwap(t, &memberReqAIHTTP, &http.Client{Timeout: 5 * time.Second})
	return state
}

// withAIConfigured turns the AI name check on for the test: ENABLE_AISPAM, the fake key and the
// fake model, all put back when it ends.
func withAIConfigured(t *testing.T) {
	t.Helper()
	previousSwitch, previousKey, previousModel := config.AppConfig.EnableAISpam, config.AppConfig.AnthropicAPIKey, config.AppConfig.AnthropicModel
	config.AppConfig.EnableAISpam = true
	config.AppConfig.AnthropicAPIKey = raidAITestKey
	config.AppConfig.AnthropicModel = raidAITestModel
	t.Cleanup(func() {
		config.AppConfig.EnableAISpam = previousSwitch
		config.AppConfig.AnthropicAPIKey = previousKey
		config.AppConfig.AnthropicModel = previousModel
	})
}

// mreqAICleanup deletes every fuku:memberreq:nv:* key (a verdict key holds a hash, not an ID, so
// it cannot be found by ID) now and when the test ends, then empties the in-process cache layer.
// userIDs and chatIDs are the test's members and chats: their rows and Redis keys go through
// memberReqCleanup's helpers when the test ends (the chats' keys only then, so the settings a
// test stored a moment ago are not deleted before it runs).
func mreqAICleanup(t *testing.T, chatIDs, userIDs []int64) {
	t.Helper()
	clean := func() {
		mreqAIDeleteVerdicts()
	}
	clean()
	t.Cleanup(clean)
	if len(userIDs) > 0 {
		memberReqCleanup(t, nil, userIDs)
	}
	if len(chatIDs) > 0 {
		t.Cleanup(func() { memberReqDeleteRedisKeys(chatIDs) })
	}
}

// mreqAIDeleteVerdicts removes every verdict key from Redis.
func mreqAIDeleteVerdicts() {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	ctx := context.Background()
	keys, err := client.Keys(ctx, memberReqAIVerdictPrefix+"*").Result()
	if err != nil {
		return
	}
	for _, key := range keys {
		client.Del(ctx, key)
	}
}

// mreqAIVerdictKeys lists the verdict keys now in Redis.
func mreqAIVerdictKeys(t *testing.T) []string {
	t.Helper()
	client := memberReqRedisClient()
	if client == nil {
		t.Fatal("no Redis client")
	}
	keys, err := client.Keys(context.Background(), memberReqAIVerdictPrefix+"*").Result()
	if err != nil {
		t.Fatalf("list verdict keys: %v", err)
	}
	return keys
}

// mreqAIDecoded is a names request as the stand-in received it.
type mreqAIDecoded struct {
	Model    string
	System   string
	Tools    int
	Users    []string
	UserText []string
}

// mreqAIDecode parses a recorded request body into the parts a test asserts on: the text of
// every block of every message (the names request has exactly one user text block).
func mreqAIDecode(t *testing.T, body []byte) mreqAIDecoded {
	t.Helper()
	var parsed struct {
		Model    string            `json:"model"`
		System   string            `json:"system"`
		Tools    []json.RawMessage `json:"tools"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode the names request: %v", err)
	}
	decoded := mreqAIDecoded{Model: parsed.Model, System: parsed.System, Tools: len(parsed.Tools)}
	for _, message := range parsed.Messages {
		decoded.Users = append(decoded.Users, message.Role)
		for _, block := range message.Content {
			if block.Type == "text" {
				decoded.UserText = append(decoded.UserText, block.Text)
			}
		}
	}
	return decoded
}

// mreqAINamesOf decodes the single user text block of a names request into its three fields and
// fails the test unless it is a JSON object with exactly first_name, last_name and username.
func mreqAINamesOf(t *testing.T, body []byte) claude.Names {
	t.Helper()
	decoded := mreqAIDecode(t, body)
	if len(decoded.UserText) != 1 || len(decoded.Users) != 1 || decoded.Users[0] != "user" {
		t.Fatalf("request messages = %v with text blocks %v, want one user message with one text block", decoded.Users, decoded.UserText)
	}
	var fields map[string]string
	if err := json.Unmarshal([]byte(decoded.UserText[0]), &fields); err != nil {
		t.Fatalf("the user text block is not a JSON object of strings: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("user text block has %d fields, want exactly first_name, last_name and username", len(fields))
	}
	for _, name := range []string{"first_name", "last_name", "username"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("user text block has no %s", name)
		}
	}
	return claude.Names{First: fields["first_name"], Last: fields["last_name"], Username: fields["username"]}
}

// mreqAICanon is the canonical form of three names, the string a request must carry.
func mreqAICanon(t *testing.T, first, last, username string) namecanon.Canon {
	t.Helper()
	canon, ok := namecanon.Of(first, last, username)
	if !ok {
		t.Fatalf("names %q %q %q are not judgeable", first, last, username)
	}
	return canon
}

// mreqAIRestartRoot puts the job runner back in the middle of a test, after StopMemberReqJobs
// cancelled its root context, so a second sighting can run. restoreMemberReqRunner does the same
// when the test ends.
func mreqAIRestartRoot() {
	memberReqDraining.Store(false)
	memberReqRoot.mu.Lock()
	defer memberReqRoot.mu.Unlock()
	memberReqRoot.ctx, memberReqRoot.cancel = context.WithCancel(context.Background())
}
