package main

import (
	"bytes"
	"encoding/csv"
	"math"
	"strings"
	"testing"
	"time"
)

func TestSpikeLabels(t *testing.T) {
	cases := []struct {
		path  string
		label string
		ok    bool
	}{
		{"spam/a.jpg", "spam", true},
		{"ham/b.png", "ham", true},
		{"x/spam_c.webp", "spam", true},
		{"x/ham_d.jpg", "ham", true},
		{"x/e.jpg", "", false},
		{"notes.txt", "", false},
		{"spam/notes.txt", "", false},
		{"ham/F.JPEG", "ham", true},
	}
	for _, tc := range cases {
		label, ok := labelOf(tc.path)
		if ok != tc.ok || label != tc.label {
			t.Errorf("labelOf(%q) = %q, %v; want %q, %v", tc.path, label, ok, tc.label, tc.ok)
		}
	}
}

// spikeFixture holds fixed results: at bar 0.85 there are 2 true positives,
// 1 false positive (a ham image at exactly the bar), 2 true negatives and
// 1 false negative, plus one blocked and one failed image.
func spikeFixture() []result {
	return []result{
		{File: "spam/1.jpg", Label: "spam", Probability: 0.95, Finish: "ok", Duration: 1200 * time.Millisecond, InputTokens: 300, OutputTokens: 25},
		{File: "spam/2.jpg", Label: "spam", Probability: 0.90, Finish: "ok", Duration: 900 * time.Millisecond, InputTokens: 300, OutputTokens: 20},
		{File: "spam/3.jpg", Label: "spam", Probability: 0.60, Finish: "ok", Duration: 1000 * time.Millisecond, InputTokens: 300, OutputTokens: 20},
		{File: "ham/4.jpg", Label: "ham", Probability: 0.85, Finish: "ok", Duration: 800 * time.Millisecond, InputTokens: 300, OutputTokens: 20},
		{File: "ham/5.jpg", Label: "ham", Probability: 0.10, Finish: "ok", Duration: 700 * time.Millisecond, InputTokens: 300, OutputTokens: 20},
		{File: "ham/6.jpg", Label: "ham", Probability: 0.20, Finish: "ok", Duration: 600 * time.Millisecond, InputTokens: 300, OutputTokens: 20},
		{File: "spam/7.jpg", Label: "spam", Finish: "blocked", Duration: 400 * time.Millisecond, Blocked: true},
		{File: "ham/8.jpg", Label: "ham", Finish: "failed HTTP 429 rate_limit_error", Duration: 300 * time.Millisecond, Failed: true},
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSpikeSummary(t *testing.T) {
	results := spikeFixture()

	s := summarize(results, 0.85)
	if s.TP != 2 || s.FP != 1 || s.TN != 2 || s.FN != 1 {
		t.Fatalf("matrix at 0.85 = TP %d FP %d TN %d FN %d; want 2 1 2 1", s.TP, s.FP, s.TN, s.FN)
	}
	if s.Blocked != 1 || s.Failed != 1 {
		t.Errorf("blocked %d failed %d; want 1 and 1", s.Blocked, s.Failed)
	}
	if s.Scored != 6 {
		t.Errorf("scored = %d; blocked and failed must stay out of the matrix, want 6", s.Scored)
	}
	if !near(s.Accuracy, 4.0/6.0) || !near(s.Precision, 2.0/3.0) || !near(s.Recall, 2.0/3.0) {
		t.Errorf("accuracy %.4f precision %.4f recall %.4f; want 0.6667 each", s.Accuracy, s.Precision, s.Recall)
	}

	bars := defaultBars()
	if len(bars) != 10 || bars[0] != 0.50 || bars[9] != 0.95 {
		t.Fatalf("defaultBars() = %v; want 0.50 to 0.95 in ten steps of 0.05", bars)
	}
	rows := sweep(results, bars)
	if len(rows) != len(bars) {
		t.Fatalf("sweep rows = %d; want %d", len(rows), len(bars))
	}
	want := map[float64][4]int{ // TP FP TN FN
		0.50: {3, 1, 2, 0},
		0.60: {3, 1, 2, 0},
		0.65: {2, 1, 2, 1},
		0.90: {2, 0, 3, 1},
		0.95: {1, 0, 3, 2},
	}
	for _, row := range rows {
		exp, ok := want[row.Bar]
		if !ok {
			continue
		}
		got := [4]int{row.TP, row.FP, row.TN, row.FN}
		if got != exp {
			t.Errorf("bar %.2f: TP FP TN FN = %v; want %v", row.Bar, got, exp)
		}
		delete(want, row.Bar)
	}
	if len(want) != 0 {
		t.Errorf("sweep has no row for bars %v", want)
	}

	var in []time.Duration
	for i := 20; i >= 1; i-- { // unsorted on purpose
		in = append(in, time.Duration(i)*time.Millisecond)
	}
	if got := percentile(in, 50); got != 10*time.Millisecond {
		t.Errorf("p50 = %v; want 10ms", got)
	}
	if got := percentile(in, 95); got != 19*time.Millisecond {
		t.Errorf("p95 = %v; want 19ms", got)
	}
	if got := percentile(in, 100); got != 20*time.Millisecond {
		t.Errorf("max = %v; want 20ms", got)
	}
	if in[0] != 20*time.Millisecond {
		t.Error("percentile reordered its input")
	}
	if got := percentile(nil, 95); got != 0 {
		t.Errorf("percentile of nothing = %v; want 0", got)
	}

	// A comma in a file name must be quoted, and a blocked image has no probability.
	var buf bytes.Buffer
	odd := append([]result{{File: "spam/a,b.jpg", Label: "spam", Probability: 0.91, Finish: "ok", Duration: 1234 * time.Millisecond, InputTokens: 11, OutputTokens: 22}}, results[6])
	if err := writeCSV(&buf, odd); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not readable: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("CSV has %d records; want a header and two rows", len(records))
	}
	if got := strings.Join(records[0], "|"); got != "file|label|probability|finish|ms|input_tokens|output_tokens" {
		t.Errorf("CSV header = %q", got)
	}
	if got := strings.Join(records[1], "|"); got != "spam/a,b.jpg|spam|0.9100|ok|1234|11|22" {
		t.Errorf("CSV row 1 = %q", got)
	}
	if got := strings.Join(records[2], "|"); got != "spam/7.jpg|spam||blocked|400|0|0" {
		t.Errorf("CSV row 2 (blocked, no probability) = %q", got)
	}
}

func TestSpikeCost(t *testing.T) {
	got := estimateCost(1_000_000, 200_000, 0.10, 0.50)
	if !near(got, 0.20) {
		t.Errorf("estimateCost = %.6f; want 0.20 (0.10 for input, 0.10 for 200,000 output tokens)", got)
	}
	if got := estimateCost(0, 0, 0.10, 0.50); got != 0 {
		t.Errorf("estimateCost of nothing = %v; want 0", got)
	}
	if got := costPer1000(0.60, 200); !near(got, 3.0) {
		t.Errorf("costPer1000(0.60, 200) = %v; want 3.0", got)
	}
	if got := costPer1000(0.60, 0); got != 0 {
		t.Errorf("costPer1000 with no images = %v; want 0", got)
	}

	// The totals add the verdicts' tokens and count every image once.
	tot := totalsOf(spikeFixture(), 0.10, 0.50)
	if tot.Images != 8 || tot.Input != 1800 || tot.Output != 125 || tot.TokenlessImages != 2 {
		t.Errorf("totals = %+v; want 8 images, 1800 input, 125 output tokens, 2 tokenless", tot)
	}
	if !near(tot.Cost, estimateCost(1800, 125, 0.10, 0.50)) {
		t.Errorf("total cost = %v", tot.Cost)
	}
	if tot.Max != 1200*time.Millisecond {
		t.Errorf("max latency = %v; want 1.2s", tot.Max)
	}
}
