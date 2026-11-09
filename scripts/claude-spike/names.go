package main

// The names mode of the spike tool: a labelled CSV of account names is put
// through the bot's own canonical form (namecanon.Of) and the bot's own names
// request (claude.Client.ClassifyNames), so the owner measures exactly what the
// bot sends. The summary, the sweep, the latency and the cost are the image
// mode's functions; this file adds the file reader, the repeat statistics and
// the names CSV. A name is printed only in the owner's own CSV, never on
// stderr: every message here carries row numbers and counts.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/namecanon"
)

const (
	// namesDefaultBar is the bar in names mode unless -bar is given. It is the
	// starting value of the bot's memberReqAIBar.
	namesDefaultBar = 0.9
	// namesRowCap is the most rows a run reads.
	namesRowCap = 5000
	// maxRepeat is the most times -repeat judges a name.
	maxRepeat = 10
	namesUnit = "names"
)

// namesBars is the sweep of the names mode.
var namesBars = []float64{0.5, 0.7, 0.8, 0.85, 0.9, 0.95, 0.97}

// namesRequiredColumns are the header columns a names file must have.
var namesRequiredColumns = []string{"label", "first_name", "last_name", "username"}

// nameRow is one usable row of the file. Num is its place among the file's data
// records (the first after the header is 1); the strings are as the owner wrote
// them and the canonical form is what the bot would send.
type nameRow struct {
	Num                   int
	Label                 string
	First, Last, Username string
	Canon                 namecanon.Canon
}

// namesSkipped counts the rows the reader did not use, by kind.
type namesSkipped struct {
	badRecord, badLabel, notJudgeable, overCap int
}

func (s namesSkipped) String() string {
	return fmt.Sprintf("%d malformed or with the wrong number of fields, %d with a label other than spam or ham, "+
		"%d with nothing to judge, %d past the %d-row cap",
		s.badRecord, s.badLabel, s.notJudgeable, s.overCap, namesRowCap)
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// namesColumns maps each column name of the header (lower-cased, trimmed) to
// its index, and checks that the four required columns are there.
func namesColumns(header []string) (map[string]int, error) {
	cols := make(map[string]int, len(header))
	for i, h := range header {
		name := strings.ToLower(strings.TrimSpace(h))
		if _, dup := cols[name]; !dup {
			cols[name] = i
		}
	}
	for _, want := range namesRequiredColumns {
		if _, ok := cols[want]; !ok {
			return nil, fmt.Errorf("the header must name the columns %s (and may add note)", strings.Join(namesRequiredColumns, ", "))
		}
	}
	return cols, nil
}

// readNames reads a names file: a header row, then one labelled row per account.
// A bad row is skipped and counted, never fatal; only an unusable header or a
// read error is an error. limit (0 means none) stops after that many usable
// rows; rows past namesRowCap are counted and not used.
func readNames(r io.Reader, limit int) ([]nameRow, namesSkipped, error) {
	var skip namesSkipped
	br := bufio.NewReader(r)
	if head, err := br.Peek(len(utf8BOM)); err == nil && bytes.Equal(head, utf8BOM) {
		_, _ = br.Discard(len(utf8BOM))
	}
	cr := csv.NewReader(br)
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, skip, nil
	}
	if err != nil {
		return nil, skip, errors.New("the header row is not valid CSV")
	}
	cols, err := namesColumns(header)
	if err != nil {
		return nil, skip, err
	}

	var rows []nameRow
	for num := 1; limit <= 0 || len(rows) < limit; num++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var parseErr *csv.ParseError
		switch {
		case errors.As(err, &parseErr):
			skip.badRecord++
			continue
		case err != nil:
			return nil, skip, errors.New("the file could not be read")
		case len(rows) >= namesRowCap:
			skip.overCap++
			continue
		}
		row, kind := namesRowOf(rec, len(header), cols, num)
		switch kind {
		case rowOK:
			rows = append(rows, row)
		case rowBadFields:
			skip.badRecord++
		case rowBadLabel:
			skip.badLabel++
		case rowNothing:
			skip.notJudgeable++
		}
	}
	return rows, skip, nil
}

// rowKind says why a record was or was not used.
type rowKind int

const (
	rowOK rowKind = iota
	rowBadFields
	rowBadLabel
	rowNothing
)

// namesRowOf turns one record into a usable row, or says why it is not one.
func namesRowOf(rec []string, width int, cols map[string]int, num int) (nameRow, rowKind) {
	if len(rec) != width {
		return nameRow{}, rowBadFields
	}
	label := strings.ToLower(strings.TrimSpace(rec[cols["label"]]))
	if label != labelSpam && label != labelHam {
		return nameRow{}, rowBadLabel
	}
	row := nameRow{
		Num:      num,
		Label:    label,
		First:    rec[cols["first_name"]],
		Last:     rec[cols["last_name"]],
		Username: rec[cols["username"]],
	}
	canon, ok := namecanon.Of(row.First, row.Last, row.Username)
	if !ok {
		return nameRow{}, rowNothing
	}
	row.Canon = canon
	return row, rowOK
}

// loadNames opens and reads the names file at path.
func loadNames(path string, limit int) ([]nameRow, namesSkipped, error) {
	f, err := os.Open(path) // #nosec G304 -- the owner names the file
	if err != nil {
		return nil, namesSkipped{}, err
	}
	defer func() { _ = f.Close() }()
	return readNames(f, limit)
}

// nameOutcome is every judgement of one row, in order. Attempts holds exactly
// -repeat results.
type nameOutcome struct {
	Row      nameRow
	Attempts []result
}

// judgeName sends one row's canonical names and turns the answer into a result.
// File holds the row number, never a name.
func judgeName(client *claude.Client, row nameRow, key string) result {
	r := result{File: strconv.Itoa(row.Num), Label: row.Label}
	started := time.Now()
	verdict, err := client.ClassifyNames(context.Background(), claude.Names{
		First:    row.Canon.First,
		Last:     row.Canon.Last,
		Username: row.Canon.Username,
	})
	r.Duration = time.Since(started)
	return finishResult(r, verdict, err, key)
}

// judgeAll judges every row repeat times, one row's repeats in sequence on one
// worker.
func judgeAll(client *claude.Client, rows []nameRow, cfg *config, key string) []nameOutcome {
	out := make([]nameOutcome, len(rows))
	forEachParallel(len(rows), cfg.concurrency, func(i int) {
		o := nameOutcome{Row: rows[i], Attempts: make([]result, 0, cfg.repeat)}
		for k := 0; k < cfg.repeat; k++ {
			o.Attempts = append(o.Attempts, judgeName(client, rows[i], key))
		}
		out[i] = o
	})
	return out
}

// firstResults are the first judgement of each row: what the summary and the
// sweep are computed from, so a row counts once however often it was repeated.
func firstResults(outcomes []nameOutcome) []result {
	out := make([]result, len(outcomes))
	for i, o := range outcomes {
		out[i] = o.Attempts[0]
	}
	return out
}

// allResults are every request of the run, which latency, tokens and cost count.
func allResults(outcomes []nameOutcome) []result {
	var out []result
	for _, o := range outcomes {
		out = append(out, o.Attempts...)
	}
	return out
}

// runNames is the names mode: read the file, judge every row, write the CSV and
// the summary. Nothing is sent before run found the key.
func runNames(client *claude.Client, cfg *config, key string, stdout, stderr io.Writer) int {
	rows, skip, err := loadNames(cfg.names, cfg.limit)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "claude-spike: cannot use %s: %v\n", cfg.names, err)
		return exitFailure
	}
	if len(rows) == 0 {
		_, _ = fmt.Fprintf(stderr, "claude-spike: no usable rows in %s (skipped: %s); nothing was sent.\n", cfg.names, skip)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stderr, "Sending %d names to Anthropic (model %s, %d at a time, each judged %d time(s)). Skipped: %s.\n",
		len(rows), cfg.model, cfg.concurrency, cfg.repeat, skip)

	outcomes := judgeAll(client, rows, cfg, key)
	if code := withCSVOut(cfg, stdout, stderr, func(w io.Writer) error { return writeNamesCSV(w, outcomes, cfg.repeat > 1) }); code != exitOK {
		return code
	}
	writeNamesSummary(stdout, outcomes, skip, cfg)

	all := allResults(outcomes)
	failed := 0
	for _, r := range all {
		if r.Failed {
			failed++
		}
	}
	if failed == len(all) {
		_, _ = fmt.Fprintln(stderr, "Every request failed. Run -models-get first to check the key and the model id (-probe exercises the image request only).")
		return exitFailure
	}
	return exitOK
}

// namesCSVHeader is the column list of the per-name output; the three repeat
// columns are added when -repeat is above 1.
var namesCSVHeader = []string{"row", "label", "first_name", "last_name", "username", "probability", "finish", "ms", "input_tokens", "output_tokens"}

var namesRepeatHeader = []string{"min", "max", "spread"}

// writeNamesCSV writes the header and one row per name: the names as the owner
// wrote them (the one place they are printed), and the probability, finish, ms
// and tokens of the name's first judgement. With repeats it adds the minimum,
// maximum and spread over all judgements, empty when one of them failed or was
// refused.
func writeNamesCSV(w io.Writer, outcomes []nameOutcome, repeats bool) error {
	header := namesCSVHeader
	if repeats {
		header = append(append([]string(nil), header...), namesRepeatHeader...)
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, o := range outcomes {
		first := o.Attempts[0]
		probability := ""
		if !first.Blocked && !first.Failed {
			probability = strconv.FormatFloat(first.Probability, 'f', 4, 64)
		}
		row := []string{
			strconv.Itoa(o.Row.Num), o.Row.Label, o.Row.First, o.Row.Last, o.Row.Username,
			probability, first.Finish,
			strconv.FormatInt(first.Duration.Milliseconds(), 10),
			strconv.Itoa(first.InputTokens), strconv.Itoa(first.OutputTokens),
		}
		if repeats {
			row = append(row, spreadCells(o)...)
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// spreadCells are the min, max and spread cells of one row, or three empty
// cells when the row has no complete set of answers.
func spreadCells(o nameOutcome) []string {
	lo, hi, ok := spreadOf(o)
	if !ok {
		return []string{"", "", ""}
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }
	return []string{f(lo), f(hi), f(hi - lo)}
}

// spreadOf is the lowest and highest probability over all of a row's
// judgements. ok is false when any of them failed or was refused, because the
// row then has no complete set of answers.
func spreadOf(o nameOutcome) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, r := range o.Attempts {
		if r.Failed || r.Blocked {
			return 0, 0, false
		}
		lo = math.Min(lo, r.Probability)
		hi = math.Max(hi, r.Probability)
	}
	return lo, hi, len(o.Attempts) > 0
}

// repeatReport is the stability of repeated judgements at a bar.
type repeatReport struct {
	Repeats      int
	Judged       int
	Excluded     int
	Straddle     int
	MaxSpread    float64
	MaxSpreadRow int
	MeanSpread   float64
}

// repeatReportOf measures how much a row's repeated probabilities vary. A row
// straddles the bar when one judgement is below it and another is at or above
// it; a row with a failed or refused judgement is excluded and counted.
func repeatReportOf(outcomes []nameOutcome, bar float64) repeatReport {
	rep := repeatReport{}
	var sum float64
	for _, o := range outcomes {
		rep.Repeats = len(o.Attempts)
		lo, hi, ok := spreadOf(o)
		if !ok {
			rep.Excluded++
			continue
		}
		rep.Judged++
		spread := hi - lo
		sum += spread
		if lo < bar && hi >= bar {
			rep.Straddle++
		}
		if spread > rep.MaxSpread {
			rep.MaxSpread, rep.MaxSpreadRow = spread, o.Row.Num
		}
	}
	if rep.Judged > 0 {
		rep.MeanSpread = sum / float64(rep.Judged)
	}
	return rep
}

// falsePositiveRate is the share of ordinary names flagged at the summary's
// bar, 0 when there are no ordinary names.
func falsePositiveRate(s summary) float64 {
	return ratio(s.FP, s.FP+s.TN)
}

// writeNamesSummary prints the counts by label, the image mode's summary, sweep,
// latency and cost over the names, the false-positive rate, the mean tokens and,
// with -repeat, the stability report. Numbers only: no name.
func writeNamesSummary(w io.Writer, outcomes []nameOutcome, skip namesSkipped, cfg *config) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	firsts := firstResults(outcomes)

	spam := 0
	for _, r := range firsts {
		if r.Label == labelSpam {
			spam++
		}
	}
	p("Names judged: %d (spam: %d, ham: %d), each %d time(s). Skipped: %s.\n\n", len(firsts), spam, len(firsts)-spam, cfg.repeat, skip)

	s := summarize(firsts, cfg.bar)
	s.Unit = namesUnit
	t := totalsOf(allResults(outcomes), cfg.priceIn, cfg.priceOut)
	t.Unit = namesUnit
	writeSummary(w, s, sweep(firsts, namesBars), t)

	p("\n  false-positive rate at bar %.2f: %.4f (%d of %d ordinary names flagged)\n", s.Bar, falsePositiveRate(s), s.FP, s.FP+s.TN)
	if answered := t.Images - t.TokenlessImages; answered > 0 {
		p("  mean tokens per answered request: input %.1f, output %.1f\n", float64(t.Input)/float64(answered), float64(t.Output)/float64(answered))
	}
	if cfg.repeat > 1 {
		writeRepeatReport(w, repeatReportOf(outcomes, cfg.bar), cfg.bar)
	}
}

// writeRepeatReport prints the stability of repeated judgements.
func writeRepeatReport(w io.Writer, rep repeatReport, bar float64) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	p("\nRepeats (each name judged %d times; the minimum, maximum and spread of each name are in the CSV)\n", rep.Repeats)
	p("  names with all judgements answered: %d (excluded, a judgement failed or was refused: %d)\n", rep.Judged, rep.Excluded)
	p("  names whose judgements lie on both sides of the bar %.2f: %d\n", bar, rep.Straddle)
	p("  largest spread of the probability: %.4f (row %d), mean spread: %.4f\n", rep.MaxSpread, rep.MaxSpreadRow, rep.MeanSpread)
}
