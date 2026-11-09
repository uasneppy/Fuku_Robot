package main

import (
	"bytes"
	"encoding/csv"
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

func TestNamesFileParse(t *testing.T) {
	read := func(t *testing.T, content string, limit int) ([]nameRow, namesSkipped, error) {
		t.Helper()
		return readNames(strings.NewReader(content), limit)
	}

	t.Run("columns in another order and a note column", func(t *testing.T) {
		rows, skip, err := read(t, "note,username,label,last_name,first_name\nfrom a test,@Anna_S,SPAM,Smith,Anna\n", 0)
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows = %d, err = %v", len(rows), err)
		}
		if rows[0].Label != labelSpam || rows[0].Canon.First != "Anna" || rows[0].Canon.Last != "Smith" || rows[0].Canon.Username != "anna_s" {
			t.Errorf("row = %+v", rows[0])
		}
		if skip != (namesSkipped{}) {
			t.Errorf("skipped = %+v, want none", skip)
		}
	})

	t.Run("a missing required column is an error", func(t *testing.T) {
		for _, header := range []string{"label,first_name,last_name\n", "first_name,last_name,username\n", "label,first_name,username\n"} {
			if _, _, err := read(t, header+"spam,A,B,C\n", 0); err == nil {
				t.Errorf("header %q was accepted", header)
			}
		}
	})

	t.Run("bad rows are skipped and counted by kind", func(t *testing.T) {
		rows, skip, err := read(t, "label,first_name,last_name,username\n"+
			"spam,Too,Few\n"+
			"maybe,Bad,Label,x\n"+
			"ham,,,\n"+
			"ham,Deleted Account,,\n"+
			"ham,ab\"c,d,e\n"+
			"ham,Good,Row,good_row\n", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].Canon.First != "Good" {
			t.Fatalf("rows = %+v, want only the good row", rows)
		}
		if rows[0].Num != 6 {
			t.Errorf("row number = %d, want 6 (its place among the data records)", rows[0].Num)
		}
		want := namesSkipped{badRecord: 2, badLabel: 1, notJudgeable: 2}
		if skip != want {
			t.Errorf("skipped = %+v, want %+v", skip, want)
		}
	})

	t.Run("a byte order mark, quoted fields and non-Latin names", func(t *testing.T) {
		content := "\uFEFFlabel,first_name,last_name,username\n" +
			"ham,\"Smith, John\",\"Line one\nline two\",\n" +
			"ham,محمد,علي,\n" +
			"ham,Ольга,Іваненко,olga_i\n"
		rows, skip, err := read(t, content, 0)
		if err != nil || len(rows) != 3 || skip != (namesSkipped{}) {
			t.Fatalf("rows = %d, skip = %+v, err = %v", len(rows), skip, err)
		}
		if rows[0].Canon.First != "Smith, John" || rows[0].Canon.Last != "Line one line two" {
			t.Errorf("quoted row = %+v", rows[0].Canon)
		}
		if rows[1].Canon.First != "محمد" || rows[2].Canon.Last != "Іваненко" {
			t.Errorf("non-Latin rows = %+v, %+v", rows[1].Canon, rows[2].Canon)
		}
	})

	t.Run("the row cap and -limit", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("label,first_name,last_name,username\n")
		for i := 0; i < namesRowCap+3; i++ {
			fmt.Fprintf(&b, "ham,Name%d,,\n", i)
		}
		rows, skip, err := read(t, b.String(), 0)
		if err != nil || len(rows) != namesRowCap || skip.overCap != 3 {
			t.Errorf("rows = %d, overCap = %d, err = %v; want %d and 3", len(rows), skip.overCap, err, namesRowCap)
		}
		rows, _, err = read(t, b.String(), 2)
		if err != nil || len(rows) != 2 {
			t.Errorf("with limit 2: rows = %d, err = %v", len(rows), err)
		}
	})

	t.Run("an empty file or one with no usable row sends nothing and exits 1", func(t *testing.T) {
		for name, content := range map[string]string{
			"empty":       "",
			"header only": "label,first_name,last_name,username\n",
			"all bad":     "label,first_name,last_name,username\nmaybe,Zyxwv,Qrstu,\nham,,,\n",
		} {
			stub := newNamesStub(t, flagWords)
			code, stdout, stderr := runNamesArgs(stub, "-names", writeNamesFile(t, content))
			if code != exitFailure || stub.count() != 0 {
				t.Errorf("%s: exit %d with %d requests, want %d and none", name, code, stub.count(), exitFailure)
			}
			if !strings.Contains(stderr, "no usable rows") || strings.Contains(stderr+stdout, "Zyxwv") {
				t.Errorf("%s: stderr %q should name counts only", name, stderr)
			}
		}
	})
}

func TestNamesSummaryAtBar(t *testing.T) {
	results := []result{
		{Label: labelSpam, Probability: 0.9},  // exactly the bar: flagged
		{Label: labelSpam, Probability: 0.95}, // flagged
		{Label: labelSpam, Probability: 0.4},  // missed
		{Label: labelHam, Probability: 0.92},  // false positive
		{Label: labelHam, Probability: 0.1},
		{Label: labelHam, Probability: 0.2},
		{Label: labelHam, Probability: 0.3},
		{Label: labelSpam, Blocked: true},
		{Label: labelHam, Failed: true},
	}
	s := summarize(results, 0.9)
	if s.TP != 2 || s.FN != 1 || s.FP != 1 || s.TN != 3 || s.Scored != 7 || s.Blocked != 1 || s.Failed != 1 {
		t.Fatalf("summary = %+v", s)
	}
	if got := falsePositiveRate(s); got != 0.25 {
		t.Errorf("false-positive rate = %v, want 0.25 (1 of 4 ordinary names)", got)
	}
	if got := s.Recall; got < 0.666 || got > 0.667 {
		t.Errorf("recall = %v, want 2/3", got)
	}

	if got := falsePositiveRate(summarize([]result{{Label: labelSpam, Probability: 0.99}}, 0.9)); got != 0 {
		t.Errorf("false-positive rate with no ordinary name = %v, want 0", got)
	}

	rows := sweep(results, namesBars)
	if len(rows) != 7 {
		t.Fatalf("sweep has %d rows, want 7", len(rows))
	}
	for i, bar := range []float64{0.5, 0.7, 0.8, 0.85, 0.9, 0.95, 0.97} {
		if rows[i].Bar != bar {
			t.Errorf("sweep row %d is at bar %v, want %v", i, rows[i].Bar, bar)
		}
	}
	if rows[4].TP != 2 || rows[5].TP != 1 {
		t.Errorf("a probability of exactly the bar must count as flagged: TP at 0.90 = %d (want 2), at 0.95 = %d (want 1)", rows[4].TP, rows[5].TP)
	}
}

func TestNamesRepeatStability(t *testing.T) {
	file := writeNamesFile(t, "label,first_name,last_name,username\n"+
		"spam,Alternating,,\n"+
		"ham,Steady,,\n"+
		"ham,Flaky,,\n")
	answer := func(n namesSeen, call int) (int, string) {
		switch n.First {
		case "Alternating":
			if call%2 == 1 {
				return http.StatusOK, namesAnswer(0.97)
			}
			return http.StatusOK, namesAnswer(0.05)
		case "Flaky":
			if call == 3 { // a 401 is final: the client retries only a 429 or a 5xx
				return http.StatusUnauthorized, errorEnvelope("authentication_error", "no")
			}
			return http.StatusOK, namesAnswer(0.5)
		}
		return http.StatusOK, namesAnswer(0.05)
	}

	t.Run("repeat 5", func(t *testing.T) {
		stub := newNamesStub(t, answer)
		csvPath := filepath.Join(t.TempDir(), "out.csv")
		code, stdout, stderr := runNamesArgs(stub, "-names", file, "-repeat", "5", "-csv", csvPath)
		if code != exitOK {
			t.Fatalf("run exited %d; stderr: %s", code, stderr)
		}
		if stub.count() != 15 {
			t.Errorf("%d requests, want 5 per row (15)", stub.count())
		}
		for _, frag := range []string{
			"Repeats (each name judged 5 times",
			"names with all judgements answered: 2 (excluded, a judgement failed or was refused: 1)",
			"on both sides of the bar 0.90: 1",
			"largest spread of the probability: 0.9200 (row 1)",
			"Latency (all 15 requests)",
		} {
			if !strings.Contains(stdout, frag) {
				t.Errorf("stdout lacks %q:\n%s", frag, stdout)
			}
		}

		data, err := os.ReadFile(csvPath)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 4 || !strings.HasSuffix(lines[0], ",min,max,spread") {
			t.Fatalf("csv = %q", lines)
		}
		for i, tail := range []string{",0.0500,0.9700,0.9200", ",0.0500,0.0500,0.0000", ",,,"} {
			if !strings.HasSuffix(lines[i+1], tail) {
				t.Errorf("csv row %d = %q, want it to end with %q", i+1, lines[i+1], tail)
			}
		}
	})

	t.Run("repeat 1 prints no repeat report", func(t *testing.T) {
		stub := newNamesStub(t, answer)
		code, stdout, stderr := runNamesArgs(stub, "-names", file)
		if code != exitOK {
			t.Fatalf("run exited %d; stderr: %s", code, stderr)
		}
		if stub.count() != 3 {
			t.Errorf("%d requests, want one per row", stub.count())
		}
		if strings.Contains(stdout, "Repeats (") || strings.Contains(stdout, ",spread") {
			t.Errorf("a run without -repeat printed repeat output:\n%s", stdout)
		}
	})
}

func TestNamesModeKeyNeverPrinted(t *testing.T) {
	const marker = "Zyxwv Qrstu"
	file := writeNamesFile(t, "label,first_name,last_name,username\nspam,"+marker+",Casino,\nham,Plain,Person,\n")

	for name, answer := range map[string]func(namesSeen, int) (int, string){
		"key echoed in an error message": func(namesSeen, int) (int, string) {
			return http.StatusUnauthorized, errorEnvelope("authentication_error", "invalid x-api-key "+testKey)
		},
		"key echoed as the error type": func(namesSeen, int) (int, string) {
			return http.StatusUnauthorized, errorEnvelope(testKey, "no")
		},
		"answers": flagWords,
	} {
		stub := newNamesStub(t, answer)
		inner := stub.srv.Config.Handler
		stub.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Echo", testKey)
			inner.ServeHTTP(w, r)
		})
		csvPath := filepath.Join(t.TempDir(), "out.csv")

		_, stdout, stderr := runNamesArgs(stub, "-names", file, "-csv", csvPath, "-repeat", "2")
		data, err := os.ReadFile(csvPath)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for what, text := range map[string]string{"stdout": stdout, "stderr": stderr, "csv": string(data)} {
			if strings.Contains(text, testKey) {
				t.Errorf("%s: %s contains the key", name, what)
			}
		}
		if strings.Contains(stderr, "Zyxwv") || strings.Contains(stderr, "Plain") || strings.Contains(stderr, "Casino") {
			t.Errorf("%s: stderr carries a name: %q", name, stderr)
		}
		if strings.Contains(stdout, "Zyxwv") {
			t.Errorf("%s: with -csv set, stdout carries a name: %q", name, stdout)
		}
		if !strings.Contains(string(data), marker) {
			t.Errorf("%s: the owner's CSV does not carry the names", name)
		}
	}
}

func TestNamesModeExclusiveFlags(t *testing.T) {
	file := writeNamesFile(t, sixNames)
	dir := t.TempDir()

	for _, args := range [][]string{
		{"-names", file, "-dir", dir},
		{"-names", file, "-probe"},
		{"-names", file, "-models-get"},
		{"-names", file, "-repeat", "0"},
		{"-names", file, "-repeat", "11"},
		{"-names", file, "-repeat", "many"},
		{"-dir", dir, "-repeat", "3"},
		{"-names", file, "-endpoint", "http://api.anthropic.com"},
		{"-names", file, "-endpoint", "http://localhost.evil.example"},
		{"-names", file, "-bar", "1.5"},
	} {
		tr := &recordingTransport{t: t, forbid: true}
		var stdout, stderr bytes.Buffer
		code := run(args, envOf("ANTHROPIC_API_KEY", testKey), &stdout, &stderr, tr)
		if code != exitUsage {
			t.Errorf("run %v exited %d, want %d", args, code, exitUsage)
		}
		if tr.count() != 0 {
			t.Errorf("run %v sent %d requests", args, tr.count())
		}
		if strings.Contains(stdout.String()+stderr.String(), testKey) {
			t.Errorf("run %v printed the key", args)
		}
	}

	for _, c := range []struct {
		args []string
		bar  float64
	}{
		{[]string{"-names", file}, 0.9},
		{[]string{"-dir", dir}, 0.85},
		{[]string{"-names", file, "-bar", "0.8"}, 0.8},
		{[]string{"-names", file, "-bar", "0.85"}, 0.85},
	} {
		cfg, _, _, code := parseFlags(c.args, &bytes.Buffer{})
		if code >= 0 || cfg == nil {
			t.Fatalf("parseFlags(%v) = code %d", c.args, code)
		}
		if cfg.bar != c.bar {
			t.Errorf("parseFlags(%v): bar = %v, want %v", c.args, cfg.bar, c.bar)
		}
	}
}

// A name that starts with = + - @, a tab or a carriage return is a formula when the owner opens the
// names CSV in a spreadsheet, and the names are the one attacker-controlled text the tool writes
// (review IN-03). Such a cell is written with a single quote in front; every other cell is as it was.
func TestNamesCSVNeutralisesFormulas(t *testing.T) {
	hostile := []string{
		`=HYPERLINK("http://x.invalid","click")`, "+1+1", "-2+3", "@SUM(A1)", "\t=1+1", "\r=1+1",
	}
	var outcomes []nameOutcome
	for i, name := range hostile {
		outcomes = append(outcomes, nameOutcome{
			Row:      nameRow{Num: i + 1, Label: "spam", First: name, Last: name, Username: name},
			Attempts: []result{{Probability: 0.9, Finish: "tool_use"}},
		})
	}
	outcomes = append(outcomes, nameOutcome{
		Row:      nameRow{Num: 99, Label: "ham", First: "Alex", Last: "a=b", Username: "x-y@z"},
		Attempts: []result{{Probability: 0.1, Finish: "tool_use"}},
	})

	var out bytes.Buffer
	if err := writeNamesCSV(&out, outcomes, false); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatalf("the output is not valid CSV: %v", err)
	}
	if len(records) != len(hostile)+2 {
		t.Fatalf("records = %d, want %d", len(records), len(hostile)+2)
	}
	for i, name := range hostile {
		row := records[i+1]
		for col := 2; col <= 4; col++ {
			if row[col] != "'"+name {
				t.Errorf("row %d column %d = %q, want %q", i+1, col, row[col], "'"+name)
			}
		}
	}
	plain := records[len(records)-1]
	if plain[2] != "Alex" || plain[3] != "a=b" || plain[4] != "x-y@z" {
		t.Errorf("an ordinary row = %q, want the names exactly as written", plain[2:5])
	}
	if plain[0] != "99" || plain[1] != "ham" || plain[5] != "0.1000" {
		t.Errorf("the other cells of an ordinary row changed: %q", plain)
	}
}
