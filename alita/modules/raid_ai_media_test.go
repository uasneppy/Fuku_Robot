//go:build testtools

package modules

import (
	"bytes"
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
	"github.com/divkix/Alita_Robot/alita/utils/gemini"
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
	// Breaker state is process-wide: a test that makes Gemini fail must not pause the next.
	resetRaidAIGeminiBreakers(t)

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

		// Two different images: the same one would be served from the verdict cache.
		env.sendMedia(members[0], gotgbot.Message{Photo: photoNamed("warn-a")})
		env.settleAI()
		env.sendMedia(members[1], gotgbot.Message{Photo: photoNamed("warn-b")})
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

		// Two different images: the same one would be served from the verdict cache.
		env.sendMedia(member, gotgbot.Message{Photo: photoNamed("act-a")})
		env.sendMedia(member, gotgbot.Message{Photo: photoNamed("act-b")})
		env.settleAI()

		if got := env.geminiRequests(); got != 2 {
			t.Fatalf("Gemini requests = %d, want 2: the case would prove nothing", got)
		}
		env.wantNoActions()
		if got := len(env.calls("unbanChatMember")); got != 0 {
			t.Errorf("unbanChatMember calls = %d, want 0", got)
		}
	})

	t.Run("frame table", func(t *testing.T) {
		thumb := &gotgbot.PhotoSize{FileId: "thumb", FileUniqueId: "uthumb", Width: 90, Height: 90}
		cases := []struct {
			name   string
			msg    gotgbot.Message
			wantID string // empty: no candidate
			kind   string
		}{
			{"a regular static sticker sends the sticker file", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "stk", FileUniqueId: "ustk", Type: "regular", Thumbnail: thumb},
			}, "stk", "sticker"},
			{"an animated sticker sends its thumbnail", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "tgs", Type: "regular", IsAnimated: true, Thumbnail: thumb},
			}, "thumb", "sticker"},
			{"a video sticker sends its thumbnail", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "webm", Type: "regular", IsVideo: true, Thumbnail: thumb},
			}, "thumb", "sticker"},
			{"an animation sends its thumbnail, whatever the document says", gotgbot.Message{
				Animation: &gotgbot.Animation{FileId: "gif", Thumbnail: thumb},
				Document:  &gotgbot.Document{FileId: "gif", Thumbnail: &gotgbot.PhotoSize{FileId: "doc-thumb"}},
			}, "thumb", "animation"},
			{"a video sends its thumbnail", gotgbot.Message{
				Video: &gotgbot.Video{FileId: "vid", Thumbnail: thumb},
			}, "thumb", "video"},
			{"a photo sends the size nearest 1024 px", gotgbot.Message{Photo: photoSizes()}, "p800", "photo"},
			{"a sticker without a frame sends nothing", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "tgs", Type: "regular", IsAnimated: true},
			}, "", ""},
			{"a video sticker without a thumbnail sends nothing", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "webm", Type: "regular", IsVideo: true},
			}, "", ""},
			{"a video without a thumbnail sends nothing", gotgbot.Message{
				Video: &gotgbot.Video{FileId: "vid"},
			}, "", ""},
			{"an animation without a thumbnail sends nothing", gotgbot.Message{
				Animation: &gotgbot.Animation{FileId: "gif"},
				Document:  &gotgbot.Document{FileId: "gif"},
			}, "", ""},
			{"a custom emoji sends nothing", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "emoji", Type: "custom_emoji", Thumbnail: thumb},
			}, "", ""},
			{"a mask sends nothing", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "mask", Type: "mask", Thumbnail: thumb},
			}, "", ""},
			{"a video note sends nothing", gotgbot.Message{
				VideoNote: &gotgbot.VideoNote{FileId: "note", Thumbnail: thumb},
			}, "", ""},
			{"a document sends nothing", gotgbot.Message{
				Document: &gotgbot.Document{FileId: "doc", FileName: "x.jpg", MimeType: "image/jpeg", Thumbnail: thumb},
			}, "", ""},
			{"a voice note sends nothing", gotgbot.Message{Voice: &gotgbot.Voice{FileId: "voice"}}, "", ""},
			{"a text sends nothing", gotgbot.Message{Text: "hello"}, "", ""},
			{"a photo over 5 MiB sends nothing", gotgbot.Message{
				Photo: []gotgbot.PhotoSize{{FileId: "huge", Width: 1024, Height: 1024, FileSize: gemini.MaxImageBytes + 1}},
			}, "", ""},
			{"a sticker over 5 MiB sends nothing", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "huge", Type: "regular", FileSize: gemini.MaxImageBytes + 1},
			}, "", ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, ok := raidAIMediaRef(&tc.msg)
				if tc.wantID == "" {
					if ok {
						t.Fatalf("raidAIMediaRef = %+v, want no candidate", got)
					}
					return
				}
				if !ok || got.FileID != tc.wantID || got.Kind != tc.kind {
					t.Errorf("raidAIMediaRef = %+v (ok %v), want file %q of kind %q", got, ok, tc.wantID, tc.kind)
				}
			})
		}
	})

	t.Run("kinds end to end", func(t *testing.T) {
		thumb := &gotgbot.PhotoSize{FileId: "thumb", FileUniqueId: "uthumb", Width: 90, Height: 90}
		cases := []struct {
			name     string
			msg      gotgbot.Message
			wantFile string
			wantMIME string
		}{
			{"a static sticker is sent as WebP", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "stk", FileUniqueId: "ustk", Type: "regular"},
			}, "stk", "image/webp"},
			{"a GIF is sent as its thumbnail", gotgbot.Message{
				Animation: &gotgbot.Animation{FileId: "gif", FileUniqueId: "ugif", Thumbnail: thumb},
				Document:  &gotgbot.Document{FileId: "gif", FileUniqueId: "ugif"},
			}, "thumb", "image/jpeg"},
			{"an animated sticker is sent as its thumbnail", gotgbot.Message{
				Sticker: &gotgbot.Sticker{FileId: "tgs", FileUniqueId: "utgs", Type: "regular", IsAnimated: true, Thumbnail: thumb},
			}, "thumb", "image/jpeg"},
			{"a video is sent as its thumbnail", gotgbot.Message{
				Video: &gotgbot.Video{FileId: "vid", FileUniqueId: "uvid", Thumbnail: thumb},
			}, "thumb", "image/jpeg"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				env := newRaidAIImageEnv(t)
				member := env.newMembers(1, 2*time.Hour)[0]
				env.serve("files/stk", raidAIServed{status: http.StatusOK, body: raidAITestWebP("sticker")})

				env.sendMedia(member, tc.msg)
				env.settleAI()

				if got := env.client.getFiles(); len(got) != 1 || got[0] != tc.wantFile {
					t.Fatalf("getFile asked for %v, want exactly %q", got, tc.wantFile)
				}
				if got := env.geminiRequests(); got != 1 {
					t.Fatalf("Gemini requests = %d, want 1", got)
				}
				images := inlineImages(t, env.geminiBody(0))
				if len(images) != 1 || images[0].MimeType != tc.wantMIME {
					t.Errorf("inline images = %+v, want exactly one %s", images, tc.wantMIME)
				}
				if n := aiVerdictSetSize(env.lockdownEnv, "av"); n != 1 {
					t.Errorf("verdict items = %d, want 1", n)
				}
				env.wantNoActions()
			})
		}
	})

	t.Run("refused files", func(t *testing.T) {
		cases := []struct {
			name string
			// setup shapes the getFile answer and the file server for the photo's p800 size.
			setup func(env *raidAIImageEnv)
			// wantDownload is whether the file server should be asked at all.
			wantDownload bool
		}{
			{"bytes that are HTML", func(env *raidAIImageEnv) {
				env.serve("files/p800", raidAIServed{status: http.StatusOK, body: []byte("<html><body>not an image</body></html>")})
			}, true},
			{"bytes that are a GIF image", func(env *raidAIImageEnv) {
				env.serve("files/p800", raidAIServed{status: http.StatusOK, body: []byte("GIF89a\x01\x00\x01\x00")})
			}, true},
			{"a body over 5 MiB with no size declared", func(env *raidAIImageEnv) {
				env.client.rawFile["p800"] = `{"file_id":"p800","file_unique_id":"u","file_path":"files/p800"}`
				env.serve("files/p800", raidAIServed{
					status: http.StatusOK,
					body:   append(raidAITestJPEG("big"), bytes.Repeat([]byte{0x41}, gemini.MaxImageBytes)...),
				})
			}, true},
			{"a file that declares over 5 MiB", func(env *raidAIImageEnv) {
				env.client.rawFile["p800"] = fmt.Sprintf(
					`{"file_id":"p800","file_unique_id":"u","file_size":%d,"file_path":"files/p800"}`, gemini.MaxImageBytes+1)
			}, false},
			{"an empty file path", func(env *raidAIImageEnv) {
				env.client.rawFile["p800"] = `{"file_id":"p800","file_unique_id":"u","file_size":100}`
			}, false},
			{"an absolute file path", func(env *raidAIImageEnv) {
				env.client.rawFile["p800"] = `{"file_id":"p800","file_unique_id":"u","file_size":100,"file_path":"/var/lib/telegram-bot-api/p800.jpg"}`
			}, false},
			{"a 404 from the file server", func(env *raidAIImageEnv) {
				env.serve("files/p800", raidAIServed{status: http.StatusNotFound, body: []byte("gone")})
			}, true},
			{"an empty download", func(env *raidAIImageEnv) {
				env.serve("files/p800", raidAIServed{status: http.StatusOK})
			}, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				env := newRaidAIImageEnv(t)
				member := env.newMembers(1, 2*time.Hour)[0]
				tc.setup(env)
				before := GetRaidAIStats()

				env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
				env.settleAI()

				if got := env.geminiRequests(); got != 0 {
					t.Errorf("Gemini requests = %d, want 0 for a refused file", got)
				}
				if got := env.fileHits.Load() > 0; got != tc.wantDownload {
					t.Errorf("file server asked = %v, want %v", got, tc.wantDownload)
				}
				if got := GetRaidAIStats(); got.Failed-before.Failed != 1 {
					t.Errorf("Failed grew by %d, want 1", got.Failed-before.Failed)
				}
				if n := aiVerdictSetSize(env.lockdownEnv, "av"); n != 0 {
					t.Errorf("verdict items = %d, want 0", n)
				}
				env.wantNoActions()
			})
		}
	})
}

// lockedBuffer is a bytes.Buffer the logger and the test can use at the same time.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// everythingLogged is all the global logger wrote through out and the hook, as one string.
func everythingLogged(out *lockedBuffer, hook *logrustest.Hook) string {
	var all strings.Builder
	all.WriteString(out.String())
	for _, entry := range hook.AllEntries() {
		all.WriteString(entry.Message)
		all.WriteString(fmt.Sprint(entry.Data))
	}
	return all.String()
}

// TestRaidAISecretsNeverLogged shows no failure of an image check writes the Gemini key or
// the bot token (which is in the Telegram file URL) anywhere: the global logger runs at
// debug level with its hooks replaced by an empty set, so the redaction hook does not run
// and only the code under test keeps the secrets out.
func TestRaidAISecretsNeverLogged(t *testing.T) {
	const botToken = "999:test"
	cases := []struct {
		name string
		// setup breaks one half of the check; it returns strings that must not be logged
		// besides the key and the token, and a function reading what must stay at zero.
		setup func(t *testing.T, env *raidAIImageEnv) (forbidden []string, untouched func() int64)
		// gemini is whether a direct Classify call is also made to read the client's error.
		gemini bool
		// download is whether a direct raidAIDownload call is also made to read its error.
		download bool
	}{
		{name: "a Gemini 400 that echoes the key", gemini: true, setup: func(t *testing.T, env *raidAIImageEnv) ([]string, func() int64) {
			env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"code":400,"message":"API key not valid: ` + raidAITestKey +
					`. Please pass a valid API key.","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID"}]}}`))
			})
			return nil, nil
		}},
		{name: "a Gemini 500 whose body is the key", gemini: true, setup: func(t *testing.T, env *raidAIImageEnv) ([]string, func() int64) {
			env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(raidAITestKey))
			})
			return nil, nil
		}},
		{name: "a Gemini server closed before the call", gemini: true, setup: func(t *testing.T, env *raidAIImageEnv) ([]string, func() int64) {
			dead := httptest.NewServer(http.NotFoundHandler())
			raidAIGeminiEndpoint = dead.URL
			dead.Close()
			return nil, nil
		}},
		{name: "a Gemini call that hangs past a shortened timeout", gemini: true, setup: func(t *testing.T, env *raidAIImageEnv) ([]string, func() int64) {
			env.answerWith(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
			raidAIGeminiHTTP = &http.Client{Timeout: 200 * time.Millisecond}
			return nil, nil
		}},
		{name: "a file server closed before the download", download: true, setup: func(t *testing.T, env *raidAIImageEnv) ([]string, func() int64) {
			env.files.Close()
			return nil, nil
		}},
		{name: "a file server that redirects to a second server", setup: func(t *testing.T, env *raidAIImageEnv) ([]string, func() int64) {
			var hits atomic.Int64
			second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				_, _ = w.Write(raidAITestJPEG("stolen"))
			}))
			t.Cleanup(second.Close)
			env.serve("files/p800", raidAIServed{location: second.URL + "/stolen"})
			return []string{strings.TrimPrefix(second.URL, "http://")}, hits.Load
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newRaidAIImageEnv(t)
			member := env.newMembers(1, 2*time.Hour)[0]
			forbidden, untouched := tc.setup(t, env)
			forbidden = append(forbidden, raidAITestKey, botToken, "/file/bot")

			out := &lockedBuffer{}
			hook := captureLogs(t, log.DebugLevel)
			log.SetOutput(out)
			before := GetRaidAIStats()

			var directErr error
			switch {
			case tc.gemini:
				_, directErr = raidAIGeminiClient().Classify(context.Background(),
					gemini.Image{MIMEType: "image/jpeg", Data: raidAITestJPEG("direct")})
			case tc.download:
				_, directErr = raidAIDownload(context.Background(), env.bot, "p800")
			}
			if (tc.gemini || tc.download) && directErr == nil {
				t.Fatal("the direct call succeeded: the case would prove nothing")
			}
			if directErr != nil {
				for _, secret := range forbidden {
					if strings.Contains(directErr.Error(), secret) {
						t.Errorf("the returned error %q contains %q", directErr, secret)
					}
				}
			}

			env.sendMedia(member, gotgbot.Message{Photo: photoSizes()})
			env.settleAI()

			if got := GetRaidAIStats(); got.Failed-before.Failed != 1 {
				t.Errorf("Failed grew by %d, want 1", got.Failed-before.Failed)
			}
			logged := everythingLogged(out, hook)
			if !strings.Contains(logged, "[RaidDetection]") {
				t.Fatal("the failure logged nothing: the case would prove nothing")
			}
			for _, secret := range forbidden {
				if strings.Contains(logged, secret) {
					t.Errorf("the log contains %q", secret)
				}
			}
			if untouched != nil {
				if got := untouched(); got != 0 {
					t.Errorf("the redirect target received %d requests, want none", got)
				}
			}
			if n := aiVerdictSetSize(env.lockdownEnv, "av"); n != 0 {
				t.Errorf("verdict items = %d, want 0 for a failed check", n)
			}
		})
	}
}
