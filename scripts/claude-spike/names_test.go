package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/namecanon"
)

// namesSeen is the user object of one names request, as the server decoded it.
type namesSeen struct {
	First    string `json:"first_name"`
	Last     string `json:"last_name"`
	Username string `json:"username"`
}

// namesStub is an httptest server that answers names requests. It records each
// request's user text and key header; answer picks the status and body from the
// decoded names and the 1-based number of that names value's call.
type namesStub struct {
	srv    *httptest.Server
	answer func(n namesSeen, call int) (int, string)

	mu    sync.Mutex
	seen  []namesSeen
	keys  []string
	calls map[namesSeen]int
}

func newNamesStub(t *testing.T, answer func(n namesSeen, call int) (int, string)) *namesStub {
	t.Helper()
	s := &namesStub{answer: answer, calls: map[namesSeen]int{}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 1 || len(body.Messages[0].Content) != 1 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var n namesSeen
		if err := json.Unmarshal([]byte(body.Messages[0].Content[0].Text), &n); err != nil {
			http.Error(w, "bad names", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.seen = append(s.seen, n)
		s.keys = append(s.keys, r.Header.Get("x-api-key"))
		s.calls[n]++
		call := s.calls[n]
		s.mu.Unlock()

		status, out := s.answer(n, call)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *namesStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.seen)
}

// namesAnswer is a Messages answer carrying a forced tool call with probability p.
func namesAnswer(p float64) string {
	return fmt.Sprintf(`{"id":"msg_01","type":"message","role":"assistant","model":"`+claude.DefaultModel+`",`+
		`"content":[{"type":"tool_use","id":"toolu_01","name":"`+claude.ToolName+`","input":{"spam_probability":%v}}],`+
		`"stop_reason":"tool_use","stop_details":null,"usage":{"input_tokens":760,"output_tokens":20}}`, p)
}

// flagWords answers 0.97 for a name that mentions casino or "write me" and 0.05
// for every other.
func flagWords(n namesSeen, _ int) (int, string) {
	all := strings.ToLower(n.First + " " + n.Last + " " + n.Username)
	if strings.Contains(all, "casino") || strings.Contains(all, "write me") {
		return http.StatusOK, namesAnswer(0.97)
	}
	return http.StatusOK, namesAnswer(0.05)
}

// writeNamesFile writes a names file into a fresh temp dir and returns its path.
func writeNamesFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "names.csv")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// runNamesArgs runs the tool against the stub with the fake key.
func runNamesArgs(stub *namesStub, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	full := append([]string{"-endpoint", stub.srv.URL}, args...)
	code = run(full, envOf("ANTHROPIC_API_KEY", testKey), &out, &errOut, stub.srv.Client().Transport)
	return code, out.String(), errOut.String()
}

const sixNames = "label,first_name,last_name,username\n" +
	"spam,🎰 Casino Bonus,,win_big\n" +
	"ham,Maria,García,maria_g\n" +
	"ham,Иван,Петров,\n" +
	"spam,Ｆｒｅｅ　Ｃａｓｉｎｏ,,\n" +
	"ham,李,明,@Li_Ming\n" +
	"spam,Write me,+7 999 000 11 22,\n"

func TestRunNamesMode(t *testing.T) {
	t.Run("one request per row, in the bot's canonical form, summary at 0.9", func(t *testing.T) {
		stub := newNamesStub(t, flagWords)
		file := writeNamesFile(t, sixNames)

		code, stdout, stderr := runNamesArgs(stub, "-names", file)
		if code != exitOK {
			t.Fatalf("run exited %d; stderr: %s", code, stderr)
		}
		if stub.count() != 6 {
			t.Fatalf("%d requests, want one per judgeable row (6)", stub.count())
		}

		var want []string
		for _, raw := range [][3]string{
			{"🎰 Casino Bonus", "", "win_big"}, {"Maria", "García", "maria_g"}, {"Иван", "Петров", ""},
			{"Ｆｒｅｅ　Ｃａｓｉｎｏ", "", ""}, {"李", "明", "@Li_Ming"}, {"Write me", "+7 999 000 11 22", ""},
		} {
			c, ok := namecanon.Of(raw[0], raw[1], raw[2])
			if !ok {
				t.Fatalf("fixture row %v is not judgeable", raw)
			}
			want = append(want, fmt.Sprintf("%q|%q|%q", c.First, c.Last, c.Username))
		}
		var got []string
		for _, n := range stub.seen {
			got = append(got, fmt.Sprintf("%q|%q|%q", n.First, n.Last, n.Username))
		}
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(want, "\n") != strings.Join(got, "\n") {
			t.Errorf("requests carried:\n%s\nwant the canonical names:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		for _, k := range stub.keys {
			if k != testKey {
				t.Error("the key from ANTHROPIC_API_KEY did not reach the x-api-key header")
				break
			}
		}

		for _, frag := range []string{
			"spam: 3", "ham: 3", "Summary at bar 0.90", "accuracy: 1.000", "precision: 1.000", "recall: 1.000",
			"false-positive rate", "Sweep of bars", "0.97", "Latency (all 6 requests)", "p95",
			"Tokens: input 4560, output 120", "per 1000 names",
		} {
			if !strings.Contains(stdout, frag) {
				t.Errorf("stdout lacks %q:\n%s", frag, stdout)
			}
		}
	})

	t.Run("a given -bar wins over the names default", func(t *testing.T) {
		stub := newNamesStub(t, flagWords)
		file := writeNamesFile(t, sixNames)
		code, stdout, stderr := runNamesArgs(stub, "-names", file, "-bar", "0.8")
		if code != exitOK {
			t.Fatalf("run exited %d; stderr: %s", code, stderr)
		}
		if !strings.Contains(stdout, "Summary at bar 0.80") {
			t.Errorf("stdout has no summary at the given bar 0.80:\n%s", stdout)
		}
	})

	t.Run("every request failing exits 1", func(t *testing.T) {
		stub := newNamesStub(t, func(namesSeen, int) (int, string) {
			return http.StatusInternalServerError, errorEnvelope("api_error", "boom")
		})
		file := writeNamesFile(t, sixNames)
		code, _, stderr := runNamesArgs(stub, "-names", file)
		if code != exitFailure {
			t.Fatalf("run exited %d, want %d", code, exitFailure)
		}
		if !strings.Contains(stderr, "Every request failed") {
			t.Errorf("stderr does not say every request failed: %q", stderr)
		}
	})

	t.Run("no key sends nothing", func(t *testing.T) {
		stub := newNamesStub(t, flagWords)
		file := writeNamesFile(t, sixNames)
		var out, errOut bytes.Buffer
		code := run([]string{"-endpoint", stub.srv.URL, "-names", file}, envOf(), &out, &errOut, stub.srv.Client().Transport)
		if code != exitUsage {
			t.Fatalf("run without a key exited %d, want %d", code, exitUsage)
		}
		if stub.count() != 0 {
			t.Errorf("%d requests were sent without a key", stub.count())
		}
	})
}
