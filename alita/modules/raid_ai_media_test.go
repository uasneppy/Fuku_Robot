//go:build testtools

package modules

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/divkix/Alita_Robot/alita/config"
)

// raidAITestKey is the Gemini key of the image tests, shaped like a real one.
const raidAITestKey = "AIzaSyImageTestKey_0123456789abcdefgh"

// raidAITestModel is the model id of the image tests.
const raidAITestModel = "gemini-test-lite"

// raidAITestJPEG, raidAITestPNG and raidAITestWebP are the smallest byte strings
// http.DetectContentType takes for each type; tag makes them distinct.
func raidAITestJPEG(tag string) []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, tag...)
}

func raidAITestWebP(tag string) []byte {
	return append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), tag...)
}

// raidAIServed is what the file server answers for one file path.
type raidAIServed struct {
	status   int
	body     []byte
	location string
}

// raidAIFileClient is the env's fake with getFile answered from a table and FileURL
// pointing at the test file server, keeping the bot<token> path segment the real URL has.
type raidAIFileClient struct {
	*lockdownFake
	base string

	fmu     sync.Mutex
	rawFile map[string]string
	fileIDs []string
}

func (c *raidAIFileClient) FileURL(token, path string, _ *gotgbot.RequestOpts) string {
	return c.base + "/file/bot" + token + "/" + path
}

func (c *raidAIFileClient) RequestWithContext(
	ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method != "getFile" {
		return c.lockdownFake.RequestWithContext(ctx, token, method, params, opts)
	}
	id := fmt.Sprint(params["file_id"])
	c.fmu.Lock()
	defer c.fmu.Unlock()
	c.fileIDs = append(c.fileIDs, id)
	if raw, ok := c.rawFile[id]; ok {
		return json.RawMessage(raw), nil
	}
	return json.RawMessage(fmt.Sprintf(`{"file_id":%q,"file_unique_id":"u-%s","file_size":100,"file_path":"files/%s"}`, id, id, id)), nil
}

// getFiles are the file IDs getFile was asked for, in order.
func (c *raidAIFileClient) getFiles() []string {
	c.fmu.Lock()
	defer c.fmu.Unlock()
	return append([]string(nil), c.fileIDs...)
}

// raidAIImageEnv is a raid AI env with Gemini configured and answered by a test server, the
// Telegram file download served by another, and getFile answered by a wrapper of the fake.
type raidAIImageEnv struct {
	*lockdownEnv
	client *raidAIFileClient
	files  *httptest.Server

	mu       sync.Mutex
	served   map[string]raidAIServed
	handler  http.HandlerFunc
	bodies   [][]byte
	geminiN  atomic.Int64
	fileHits atomic.Int64
}

// newRaidAIImageEnv is newRaidAIEnv plus GEMINI_API_KEY and GEMINI_MODEL set in the config
// for the test, a file server, the wrapped bot client and a Gemini server counting its
// requests. Until a test installs another handler it answers a 0.9 spam probability.
func newRaidAIImageEnv(t *testing.T) *raidAIImageEnv {
	t.Helper()
	env := &raidAIImageEnv{lockdownEnv: newRaidAIEnv(t), served: make(map[string]raidAIServed)}

	previousKey, previousModel := config.AppConfig.GeminiAPIKey, config.AppConfig.GeminiModel
	config.AppConfig.GeminiAPIKey = raidAITestKey
	config.AppConfig.GeminiModel = raidAITestModel
	t.Cleanup(func() {
		config.AppConfig.GeminiAPIKey = previousKey
		config.AppConfig.GeminiModel = previousModel
	})

	env.files = httptest.NewServer(http.HandlerFunc(env.serveFile))
	t.Cleanup(env.files.Close)
	env.client = &raidAIFileClient{lockdownFake: env.fake, base: env.files.URL, rawFile: make(map[string]string)}
	env.bot.BotClient = env.client

	server := httptest.NewServer(http.HandlerFunc(env.serveGemini))
	t.Cleanup(server.Close)
	previousEndpoint, previousHTTP := raidAIGeminiEndpoint, raidAIGeminiHTTP
	raidAIGeminiEndpoint = server.URL
	raidAIGeminiHTTP = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() { raidAIGeminiEndpoint, raidAIGeminiHTTP = previousEndpoint, previousHTTP })

	env.answerProbability(0.9)
	return env
}

func (e *raidAIImageEnv) serveFile(w http.ResponseWriter, r *http.Request) {
	e.fileHits.Add(1)
	prefix := "/file/bot" + e.bot.Token + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, prefix)
	e.mu.Lock()
	entry, ok := e.served[path]
	e.mu.Unlock()
	if !ok {
		entry = raidAIServed{status: http.StatusOK, body: raidAITestJPEG(path)}
	}
	if entry.location != "" {
		http.Redirect(w, r, entry.location, http.StatusFound)
		return
	}
	w.WriteHeader(entry.status)
	_, _ = w.Write(entry.body)
}

func (e *raidAIImageEnv) serveGemini(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	e.geminiN.Add(1)
	e.mu.Lock()
	e.bodies = append(e.bodies, body)
	handler := e.handler
	e.mu.Unlock()
	handler(w, r)
}

// serve sets what the file server answers for path.
func (e *raidAIImageEnv) serve(path string, entry raidAIServed) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.served[path] = entry
}

// answerWith installs the Gemini server's handler.
func (e *raidAIImageEnv) answerWith(handler http.HandlerFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = handler
}

// raidAIGeminiAnswer is a generateContent answer carrying probability, with the given
// model stage when it is not empty.
func raidAIGeminiAnswer(probability float64, stage string) string {
	text, _ := json.Marshal(map[string]any{"category": "scam", "spam_probability": probability})
	answer := map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"parts": []any{map[string]any{"text": string(text)}}},
			"finishReason": "STOP",
		}},
		"modelVersion": raidAITestModel,
	}
	if stage != "" {
		answer["modelStatus"] = map[string]any{"modelStage": stage}
	}
	raw, _ := json.Marshal(answer)
	return string(raw)
}

// answerProbability makes the Gemini server answer a spam probability.
func (e *raidAIImageEnv) answerProbability(probability float64) {
	e.answerWith(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(raidAIGeminiAnswer(probability, "")))
	})
}

// geminiRequests is how many requests the Gemini server received.
func (e *raidAIImageEnv) geminiRequests() int { return int(e.geminiN.Load()) }

// geminiBody returns the nth request body the Gemini server received.
func (e *raidAIImageEnv) geminiBody(n int) []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	if n >= len(e.bodies) {
		e.t.Fatalf("the Gemini server received %d requests, want at least %d", len(e.bodies), n+1)
	}
	return e.bodies[n]
}

// sendMedia has user send msg (media fields set by the caller) in the env's chat.
func (e *raidAIImageEnv) sendMedia(user gotgbot.User, msg gotgbot.Message) {
	e.t.Helper()
	msg.From = &user
	e.dispatchMessage(e.chat, msg)
}

// inlineImages parses a Gemini request body into the inline images it carries.
func inlineImages(t *testing.T, body []byte) []struct{ MimeType, Data string } {
	t.Helper()
	var parsed struct {
		Contents []struct {
			Parts []struct {
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode the Gemini request: %v", err)
	}
	var images []struct{ MimeType, Data string }
	for _, content := range parsed.Contents {
		for _, part := range content.Parts {
			if part.InlineData != nil {
				images = append(images, struct{ MimeType, Data string }{part.InlineData.MimeType, part.InlineData.Data})
			}
		}
	}
	return images
}

// captureLogs replaces the global logger's hooks with one recording hook at level, and puts
// the hooks, level and output back when the test ends. No redaction hook runs meanwhile.
func captureLogs(t *testing.T, level log.Level) *logrustest.Hook {
	t.Helper()
	previousHooks := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
	hook := logrustest.NewGlobal()
	previousLevel := log.GetLevel()
	previousOut := log.StandardLogger().Out
	log.SetLevel(level)
	t.Cleanup(func() {
		log.SetLevel(previousLevel)
		log.SetOutput(previousOut)
		log.StandardLogger().ReplaceHooks(previousHooks)
	})
	return hook
}

// photoSizes is a photo of the sizes Telegram sends: the file ID names the width.
func photoSizes() []gotgbot.PhotoSize {
	var sizes []gotgbot.PhotoSize
	for _, w := range []int64{90, 320, 800, 1280} {
		id := fmt.Sprintf("p%d", w)
		sizes = append(sizes, gotgbot.PhotoSize{FileId: id, FileUniqueId: "u" + id, Width: w, Height: w * 3 / 4})
	}
	return sizes
}

func TestRaidAIMedia(t *testing.T) {
	t.Run("a new member's photo is judged and counts", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		before := GetRaidAIStats()

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes(), Caption: "SPAM t.me/x"})
		env.settleAI()

		if got := env.client.getFiles(); len(got) != 1 || got[0] != "p800" {
			t.Fatalf("getFile asked for %v, want exactly the size nearest 1024 px (p800)", got)
		}
		if got := env.fileHits.Load(); got != 1 {
			t.Errorf("downloads = %d, want 1", got)
		}
		if got := env.geminiRequests(); got != 1 {
			t.Fatalf("Gemini requests = %d, want 1", got)
		}
		body := env.geminiBody(0)
		images := inlineImages(t, body)
		if len(images) != 1 || images[0].MimeType != "image/jpeg" {
			t.Fatalf("inline images = %+v, want exactly one image/jpeg", images)
		}
		if want := base64.StdEncoding.EncodeToString(raidAITestJPEG("files/p800")); images[0].Data != want {
			t.Errorf("the image sent is not the downloaded bytes")
		}
		for _, leak := range []string{"t.me", member.FirstName, env.chat.Title, fmt.Sprint(member.Id), "SPAM"} {
			if strings.Contains(string(body), leak) {
				t.Errorf("the Gemini request contains %q: only the image and the instruction may leave", leak)
			}
		}
		items, err := env.redis.ZMembers(raidKey(env.chat.Id, "av"))
		if err != nil || len(items) != 1 || !strings.HasPrefix(items[0], "m:") {
			t.Errorf("verdict items = %v (error %v), want one m:<message id>", items, err)
		}
		if got := GetRaidAIStats(); got.Spam-before.Spam != 1 || got.Checked-before.Checked != 1 {
			t.Errorf("stats moved by %+v, want 1 checked and 1 spam", got)
		}
		env.wantNoActions()
	})

	t.Run("below the bar", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		env.answerProbability(0.5)
		before := GetRaidAIStats()

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
		env.settleAI()

		if got := env.geminiRequests(); got != 1 {
			t.Fatalf("Gemini requests = %d, want 1: the case would prove nothing", got)
		}
		if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 0 {
			t.Errorf("verdict items = %d, want 0 below the bar", got)
		}
		if got := GetRaidAIStats(); got.Kept-before.Kept != 1 || got.Spam != before.Spam {
			t.Errorf("stats = %+v (before %+v), want 1 more kept and no more spam", got, before)
		}
		env.wantNoActions()
	})

	t.Run("the bar is inclusive", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		env.answerProbability(raidAIImageBar)

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
		env.settleAI()

		if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 1 {
			t.Errorf("verdict items = %d, want 1 at exactly the bar", got)
		}
	})

	t.Run("blocked", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"promptFeedback":{"blockReason":"IMAGE_SAFETY"}}`))
		})
		before := GetRaidAIStats()

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
		env.settleAI()

		got := GetRaidAIStats()
		if got.Blocked-before.Blocked != 1 {
			t.Errorf("Blocked grew by %d, want 1", got.Blocked-before.Blocked)
		}
		if got.Failed != before.Failed || got.Checked != before.Checked || got.Spam != before.Spam {
			t.Errorf("stats = %+v (before %+v), want a block to be neither a failure nor a verdict", got, before)
		}
		if n := aiVerdictSetSize(env.lockdownEnv, "av"); n != 0 {
			t.Errorf("verdict items = %d, want 0 for a blocked image", n)
		}
		env.wantNoActions()
	})

	t.Run("a failed answer is counted and counts for nothing", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) })
		before := GetRaidAIStats()

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
		env.settleAI()

		if got := GetRaidAIStats(); got.Failed-before.Failed != 1 {
			t.Errorf("Failed grew by %d, want 1", got.Failed-before.Failed)
		}
		if n := aiVerdictSetSize(env.lockdownEnv, "av"); n != 0 {
			t.Errorf("verdict items = %d, want 0", n)
		}
	})

	t.Run("an established member's photo is not fetched", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 25*time.Hour)[0]

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes(), Caption: "SPAM t.me/x"})
		env.settleAI()

		if got := env.client.getFiles(); len(got) != 0 {
			t.Errorf("getFile asked for %v, want nothing for an established member", got)
		}
		if got := env.geminiRequests(); got != 0 {
			t.Errorf("Gemini requests = %d, want 0", got)
		}
	})

	t.Run("no key, nothing leaves", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		config.AppConfig.GeminiAPIKey = ""

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes(), Caption: "SPAM t.me/x"})
		env.settleAI()

		if got := env.client.getFiles(); len(got) != 0 {
			t.Errorf("getFile asked for %v, want nothing without GEMINI_API_KEY", got)
		}
		if got := env.fileHits.Load(); got != 0 {
			t.Errorf("downloads = %d, want 0", got)
		}
		if got := env.geminiRequests(); got != 0 {
			t.Errorf("Gemini requests = %d, want 0", got)
		}
	})

	t.Run("a deprecated model warns once", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		members := env.newMembers(2, 2*time.Hour)
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(raidAIGeminiAnswer(0.1, "DEPRECATED")))
		})
		raidAIModelWarned.Store(false)
		t.Cleanup(func() { raidAIModelWarned.Store(false) })
		hook := captureLogs(t, log.InfoLevel)

		env.sendMedia(members[0], gotgbot.Message{Photo: photoSizes()})
		env.settleAI()
		env.sendMedia(members[1], gotgbot.Message{Photo: photoSizes()})
		env.settleAI()

		if got := env.geminiRequests(); got != 2 {
			t.Fatalf("Gemini requests = %d, want 2: the case would prove nothing", got)
		}
		warnings := 0
		for _, entry := range hook.AllEntries() {
			if entry.Level != log.WarnLevel {
				continue
			}
			line := entry.Message + fmt.Sprint(entry.Data)
			if strings.Contains(line, raidAITestModel) && strings.Contains(line, "DEPRECATED") {
				warnings++
			}
		}
		if warnings != 1 {
			t.Errorf("model-stage warnings = %d, want exactly 1 naming the model and the stage", warnings)
		}
	})

	t.Run("the raid image path never acts", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]

		env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
		env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
		env.settleAI()

		if got := env.geminiRequests(); got != 2 {
			t.Fatalf("Gemini requests = %d, want 2: the case would prove nothing", got)
		}
		env.wantNoActions()
		if got := len(env.calls("unbanChatMember")); got != 0 {
			t.Errorf("unbanChatMember calls = %d, want 0", got)
		}
	})
}
