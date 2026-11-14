//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/semaphore"

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

// mreqAIGated holds every request until open is closed (or the request's context ends), then
// answers with then. A test closes open to let the held requests go; a closed channel lets later
// requests straight through.
func mreqAIGated(open <-chan struct{}, then mreqAIStep) mreqAIStep {
	return func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		select {
		case <-open:
		case <-r.Context().Done():
			return
		case <-release:
			return
		}
		then(w, r, release)
	}
}

// mreqAIDelayed waits d before it answers with then, so requests overlap.
func mreqAIDelayed(d time.Duration, then mreqAIStep) mreqAIStep {
	return func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		select {
		case <-time.After(d):
		case <-r.Context().Done():
			return
		}
		then(w, r, release)
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
	inflight  int
	maxFlight int
	queue     []mreqAIStep
}

// enqueue adds answers that are given, one per request in order, before the default one.
func (s *mreqAIState) enqueue(steps ...mreqAIStep) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, steps...)
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
	s.inflight++
	if s.inflight > s.maxFlight {
		s.maxFlight = s.inflight
	}
	step := s.def
	if len(s.queue) > 0 {
		step = s.queue[0]
		s.queue = s.queue[1:]
	}
	hook := s.onRequest
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inflight--
		s.mu.Unlock()
	}()
	if hook != nil {
		hook()
	}
	step(w, r, s.release)
}

// maxInFlight is the most requests the stand-in served at the same time.
func (s *mreqAIState) maxInFlight() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxFlight
}

// waitCount waits until the stand-in has received n requests and fails the test after 5 seconds.
func (s *mreqAIState) waitCount(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.count() < n {
		if time.Now().After(deadline) {
			t.Fatalf("the stand-in received %d requests, want %d", s.count(), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
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
// fake model, all put back when it ends. It also installs a fresh breaker of the name check, whose
// state is process-wide: a test that trips it (a rejected key pauses every group) must not pause the
// tests after it.
func withAIConfigured(t *testing.T) {
	t.Helper()
	mreqAIFreshBreaker(t)
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

// mreqAIOff says how withAINotConfigured makes the AI name check not configured.
type mreqAIOff int

const (
	// mreqAINoKey leaves ENABLE_AISPAM on and removes the Anthropic key.
	mreqAINoKey mreqAIOff = iota
	// mreqAISwitchOff keeps the key and turns ENABLE_AISPAM off (the global AI kill switch).
	mreqAISwitchOff
)

// withAINotConfigured makes the AI name check not configured for the test, one of the two ways the
// process can lack it, and puts config.AppConfig back when the test ends. It installs no fake and
// no breaker: a not configured rule never reaches either.
func withAINotConfigured(t *testing.T, how mreqAIOff) {
	t.Helper()
	previousSwitch, previousKey := config.AppConfig.EnableAISpam, config.AppConfig.AnthropicAPIKey
	switch how {
	case mreqAINoKey:
		config.AppConfig.EnableAISpam = true
		config.AppConfig.AnthropicAPIKey = ""
	case mreqAISwitchOff:
		config.AppConfig.EnableAISpam = false
		config.AppConfig.AnthropicAPIKey = raidAITestKey
	}
	t.Cleanup(func() {
		config.AppConfig.EnableAISpam = previousSwitch
		config.AppConfig.AnthropicAPIKey = previousKey
	})
}

// mreqAICleanup deletes every fuku:memberreq:nv:* key (a verdict key holds a hash, not an ID, so
// it cannot be found by ID) and the baselines fuku:memberreq:nm:<user> of userIDs now and when
// the test ends, then empties the in-process cache layer. userIDs and chatIDs are the test's
// members and chats: their rows and Redis keys go through memberReqCleanup's helpers when the
// test ends (the chats' keys, the markers fuku:memberreq:nu:<chat>:<user> among them, only then,
// so the settings a test stored a moment ago are not deleted before it runs).
func mreqAICleanup(t *testing.T, chatIDs, userIDs []int64) {
	t.Helper()
	// The refusal latch (SEC-02) is process state like the breaker: every test starts with an empty
	// one and puts the previous one back, so a latch a test set never reaches another.
	mreqSwap(t, &memberReqAILatches, newMemberReqAILatch())
	clean := func() {
		mreqAIDeleteVerdicts()
		mreqAIDeleteBudgets()
		mreqAIDeleteBaselines(userIDs)
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

// mreqAIFreshBreaker replaces the name check's breaker with an empty one for the test, and puts the
// previous one back when it ends.
func mreqAIFreshBreaker(t *testing.T) {
	t.Helper()
	mreqSwap(t, &memberReqAIBreakers, newMemberReqAIBreaker())
}

// mreqAIDeleteBudgets removes every hourly budget key of the name check from Redis.
func mreqAIDeleteBudgets() {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	ctx := context.Background()
	keys, err := client.Keys(ctx, memberReqAIBudgetPrefix+"*").Result()
	if err != nil {
		return
	}
	for _, key := range keys {
		client.Del(ctx, key)
	}
}

// withAIGate replaces the in-flight gate and its wait for the test.
func withAIGate(t *testing.T, size int64, wait time.Duration) {
	t.Helper()
	mreqSwap(t, &memberReqAIGate, semaphore.NewWeighted(size))
	mreqSwap(t, &memberReqAIGateWait, wait)
}

// withAIBudgets replaces the three hourly budgets (account, group, fleet) for the test.
func withAIBudgets(t *testing.T, user, chat, global int64) {
	t.Helper()
	mreqSwap(t, &memberReqAIUserHourlyBudget, user)
	mreqSwap(t, &memberReqAIChatHourlyBudget, chat)
	mreqSwap(t, &memberReqAIGlobalHourlyBudget, global)
}

// mreqAIDeleteBaselines removes the baselines of the given users from Redis.
func mreqAIDeleteBaselines(userIDs []int64) {
	client := memberReqRedisClient()
	if client == nil {
		return
	}
	for _, id := range userIDs {
		client.Del(context.Background(), memberReqAIBaselineKey(id))
	}
}

// mreqAIMarkNew records the member as joining the chat, the way the join entry does: it sets the
// marker fuku:memberreq:nu:<chat>:<user>, so the rule judges the member at their next message
// instead of only baselining them. The chat's keys are deleted by memberReqCleanup.
func mreqAIMarkNew(t *testing.T, chatID, userID int64) {
	t.Helper()
	if memberReqRedisClient() == nil {
		t.Fatal("no Redis client")
	}
	memberReqAIMarkJoin(context.Background(), chatID, userID)
	if _, _, ok := mreqAIMarkerOf(t, chatID, userID); !ok {
		t.Fatalf("the marker of user %d in chat %d was not written", userID, chatID)
	}
}

// mreqAIBaselineOf reads a user's baseline and its remaining life; ok is false when there is none.
func mreqAIBaselineOf(t *testing.T, userID int64) (value string, ttl time.Duration, ok bool) {
	t.Helper()
	return mreqAIKeyState(t, memberReqAIBaselineKey(userID))
}

// mreqAIMarkerOf reads a member's marker in a chat and its remaining life; ok is false when absent.
func mreqAIMarkerOf(t *testing.T, chatID, userID int64) (value string, ttl time.Duration, ok bool) {
	t.Helper()
	return mreqAIKeyState(t, memberReqAIMarkerKey(chatID, userID))
}

func mreqAIKeyState(t *testing.T, key string) (value string, ttl time.Duration, ok bool) {
	t.Helper()
	client := memberReqRedisClient()
	if client == nil {
		t.Fatal("no Redis client")
	}
	ctx := context.Background()
	value, err := client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", 0, false
	}
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return value, client.TTL(ctx, key).Val(), true
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

// mreqAINameSentinel opens every name the sweeps give a member. It is not a word of any language or
// of any log line, so a line that carries it carries a member's name.
const mreqAINameSentinel = "ZZAISentinel"

// mreqAIUniqueSeq numbers the names of mreqAIUnique.
var mreqAIUniqueSeq atomic.Int64

// mreqAIUnique is a counter that names a member without using the member's ID digits: a sweep that
// asserts the ID digits absent from a request body or a log line cannot have them inside a name.
func mreqAIUnique() string {
	return strconv.FormatInt(mreqAIUniqueSeq.Add(1), 10)
}

// mreqAIStaticForbidden is what no log line of an AI path may hold whatever the shape: the sentinel
// names of the sweeps and the ordinary names of the AI tests (in both cases), the API key, the opening
// words of the code-owned instruction, the probabilities the stand-in answers in the failure shapes
// and the three Redis key families of the rule (a key shows a hash, and a hash is derived from a name).
var mreqAIStaticForbidden = []string{
	mreqAINameSentinel, strings.ToLower(mreqAINameSentinel), "Promo", "promo", "Casino", "casino", "Maria", "maria",
	raidAITestKey, claude.NamesInstruction[:32],
	"0.97", "1.5", "-0.1",
	memberReqAIVerdictPrefix, memberReqAIBaselinePrefix, memberReqAIMarkerPrefix,
}

// mreqAIForbiddenOf is what a log line must not hold about one member: the names hash and the verdict
// key hash of their names. A hash of a name is derived from the name, so it is as private as the name.
func mreqAIForbiddenOf(t *testing.T, user gotgbot.User) []string {
	t.Helper()
	canon := mreqAICanon(t, user.FirstName, user.LastName, user.Username)
	return []string{canon.Hash, namecanon.VerdictKey(raidAITestModel, canon)}
}
