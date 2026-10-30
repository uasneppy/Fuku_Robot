package main

// The pure parts of the spike tool: labelling, CSV, the summary at a bar, the
// sweep of bars, percentiles and cost. Nothing here reads a file or the
// environment; writers are passed in.

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	labelSpam = "spam"
	labelHam  = "ham"
)

// imageExtensions are the file types the tool reads.
var imageExtensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

// labelOf returns the label of an image path relative to the sample folder: a
// spam or ham parent folder wins, else a spam_ or ham_ file-name prefix. Any
// other path, and any file that is not a jpg, jpeg, png or webp, is not
// labelled and the caller skips it.
func labelOf(p string) (string, bool) {
	p = strings.ReplaceAll(p, `\`, "/")
	if !imageExtensions[strings.ToLower(path.Ext(p))] {
		return "", false
	}
	switch strings.ToLower(path.Base(path.Dir(p))) {
	case labelSpam:
		return labelSpam, true
	case labelHam:
		return labelHam, true
	}
	name := strings.ToLower(path.Base(p))
	switch {
	case strings.HasPrefix(name, labelSpam+"_"):
		return labelSpam, true
	case strings.HasPrefix(name, labelHam+"_"):
		return labelHam, true
	}
	return "", false
}

// result is the outcome for one image. A blocked or failed image has no
// probability and stays out of the confusion matrix.
type result struct {
	File, Label   string
	Probability   float64
	Category      string
	Finish        string
	Duration      time.Duration
	PromptTokens  int
	OutputTokens  int
	ThoughtTokens int
	Blocked       bool
	Failed        bool
}

// csvHeader is the column list of the per-image output.
var csvHeader = []string{"file", "label", "probability", "category", "finish", "ms", "prompt_tokens", "output_tokens", "thought_tokens"}

// writeCSV writes the header and one row per result. A blocked or failed
// image has an empty probability and category.
func writeCSV(w io.Writer, results []result) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range results {
		probability, category := "", ""
		if !r.Blocked && !r.Failed {
			probability = strconv.FormatFloat(r.Probability, 'f', 4, 64)
			category = r.Category
		}
		row := []string{
			r.File,
			r.Label,
			probability,
			category,
			r.Finish,
			strconv.FormatInt(r.Duration.Milliseconds(), 10),
			strconv.Itoa(r.PromptTokens),
			strconv.Itoa(r.OutputTokens),
			strconv.Itoa(r.ThoughtTokens),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// summary is the confusion matrix and its rates at one bar. An image counts
// as flagged when its probability is at least the bar.
type summary struct {
	Bar               float64
	TP, FP, TN, FN    int
	Scored            int
	Blocked, Failed   int
	Accuracy          float64
	Precision, Recall float64
}

// ratio is a/b, or 0 when b is 0.
func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// summarize builds the confusion matrix at bar. Blocked and failed images are
// counted apart and left out of the matrix and of every rate.
func summarize(results []result, bar float64) summary {
	s := summary{Bar: bar}
	for _, r := range results {
		switch {
		case r.Blocked:
			s.Blocked++
			continue
		case r.Failed:
			s.Failed++
			continue
		}
		flagged := r.Probability >= bar
		switch {
		case r.Label == labelSpam && flagged:
			s.TP++
		case r.Label == labelSpam:
			s.FN++
		case flagged:
			s.FP++
		default:
			s.TN++
		}
	}
	s.Scored = s.TP + s.FP + s.TN + s.FN
	s.Accuracy = ratio(s.TP+s.TN, s.Scored)
	s.Precision = ratio(s.TP, s.TP+s.FP)
	s.Recall = ratio(s.TP, s.TP+s.FN)
	return s
}

// defaultBars is the sweep 0.50, 0.55 ... 0.95.
func defaultBars() []float64 {
	bars := make([]float64, 0, 10)
	for i := 0; i < 10; i++ {
		bars = append(bars, math.Round((0.50+0.05*float64(i))*100)/100)
	}
	return bars
}

// sweep summarizes the results at each bar.
func sweep(results []result, bars []float64) []summary {
	rows := make([]summary, 0, len(bars))
	for _, bar := range bars {
		rows = append(rows, summarize(results, bar))
	}
	return rows
}

// percentile returns the p-th percentile (0..100, nearest rank) of d without
// reordering it, or 0 for no durations.
func percentile(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), d...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// estimateCost is the price of a run in dollars: prompt tokens at the input
// price plus candidate and thought tokens at the output price, both given in
// dollars per million tokens.
func estimateCost(prompt, candidates, thoughts int, priceIn, priceOut float64) float64 {
	return (float64(prompt)*priceIn + float64(candidates+thoughts)*priceOut) / 1e6
}

// costPer1000 scales a run's cost to a thousand images, or 0 for no images.
func costPer1000(cost float64, images int) float64 {
	if images <= 0 {
		return 0
	}
	return cost / float64(images) * 1000
}

// totals are the latency, token and cost figures of a run.
type totals struct {
	Images                       int
	P50, P95, Max                time.Duration
	Prompt, Candidates, Thoughts int
	PriceIn, PriceOut            float64
	Cost, CostPer1000            float64
	TokenlessImages              int
}

// totalsOf adds up a run. Every image is one request and counts once; only an
// answered image carries token counts, so a blocked or failed one adds none
// and is reported in TokenlessImages.
func totalsOf(results []result, priceIn, priceOut float64) totals {
	t := totals{Images: len(results), PriceIn: priceIn, PriceOut: priceOut}
	durations := make([]time.Duration, 0, len(results))
	for _, r := range results {
		durations = append(durations, r.Duration)
		t.Prompt += r.PromptTokens
		t.Candidates += r.OutputTokens
		t.Thoughts += r.ThoughtTokens
		if r.Blocked || r.Failed {
			t.TokenlessImages++
		}
	}
	t.P50 = percentile(durations, 50)
	t.P95 = percentile(durations, 95)
	t.Max = percentile(durations, 100)
	t.Cost = estimateCost(t.Prompt, t.Candidates, t.Thoughts, priceIn, priceOut)
	t.CostPer1000 = costPer1000(t.Cost, t.Images)
	return t
}

// pricingNote tells the owner where the default prices must be checked.
const pricingNote = "verify at ai.google.dev/gemini-api/docs/pricing"

// writeSummary prints the confusion matrix at the chosen bar, the sweep of
// bars, and the latency, token and cost figures.
func writeSummary(w io.Writer, s summary, rows []summary, t totals) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

	p("Summary at bar %.2f\n", s.Bar)
	p("  images scored: %d (blocked: %d, failed: %d; these stay out of the matrix)\n", s.Scored, s.Blocked, s.Failed)
	p("  confusion matrix (flagged when probability >= bar):\n")
	p("                 flagged  not flagged\n")
	p("    spam         %7d  %11d\n", s.TP, s.FN)
	p("    ordinary     %7d  %11d\n", s.FP, s.TN)
	p("  accuracy: %.3f  precision: %.3f  recall: %.3f\n", s.Accuracy, s.Precision, s.Recall)

	p("\nSweep of bars\n")
	p("  bar   TP   FP   TN   FN  accuracy precision recall\n")
	for _, r := range rows {
		p("  %.2f %4d %4d %4d %4d  %8.3f %9.3f %6.3f\n", r.Bar, r.TP, r.FP, r.TN, r.FN, r.Accuracy, r.Precision, r.Recall)
	}

	p("\nLatency (all %d requests): p50 %s, p95 %s, max %s\n", t.Images, t.P50.Round(time.Millisecond), t.P95.Round(time.Millisecond), t.Max.Round(time.Millisecond))
	p("Tokens: prompt %d, candidates %d, thoughts %d (blocked or failed images add none: %d)\n", t.Prompt, t.Candidates, t.Thoughts, t.TokenlessImages)
	p("Estimated cost: $%.4f for %d images, $%.4f per 1000 images\n", t.Cost, t.Images, t.CostPer1000)
	p("  at $%.2f per million input tokens and $%.2f per million output tokens (candidates plus thoughts); %s\n", t.PriceIn, t.PriceOut, pricingNote)
}
