//go:build testtools

package modules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// summaryTestCard is the card every renderer test uses: a permanent ban.
func summaryTestCard() *staffActionCard {
	return &staffActionCard{
		Kind:       staffKindBan,
		Target:     4242,
		TargetName: "Mallory",
		Reason:     "spam",
	}
}

// summaryTitle is a unique title of exactly 64 runes.
func summaryTitle(i int) string {
	return fmt.Sprintf("G%03d-", i) + strings.Repeat("x", 59)
}

func summaryResult(i int, title string, outcome staffOutcome, reason staffReason) staffGroupResult {
	return staffGroupResult{
		Link:    models.StaffGroupLink{GroupChatID: int64(-1000 - i), GroupTitle: title},
		Outcome: outcome,
		Reason:  reason,
	}
}

// summaryResults builds n results with 64-rune titles and the given outcome per
// index.
func summaryResults(n int, pick func(i int) (staffOutcome, staffReason)) []staffGroupResult {
	results := make([]staffGroupResult, n)
	for i := range results {
		outcome, reason := pick(i)
		results[i] = summaryResult(i, summaryTitle(i), outcome, reason)
	}
	return results
}

var summaryTallyRe = regexp.MustCompile(`✅ (\d+) · ⏭ (\d+) · ❌ (\d+)`)

// summaryTallyOf reads the three numbers of the tally line of text.
func summaryTallyOf(t *testing.T, text string) (done, skipped, failed int) {
	t.Helper()
	match := summaryTallyRe.FindStringSubmatch(text)
	if match == nil {
		t.Fatalf("no tally line in:\n%s", text)
	}
	done, _ = strconv.Atoi(match[1])
	skipped, _ = strconv.Atoi(match[2])
	failed, _ = strconv.Atoi(match[3])
	return done, skipped, failed
}

// summaryGroupLines returns the lines after the blank line that follows the
// tally, which is where the group lines start.
func summaryGroupLines(t *testing.T, text string) []string {
	t.Helper()
	_, block, ok := strings.Cut(text, "\n\n")
	if !ok {
		t.Fatalf("summary has no blank line before the group lines:\n%s", text)
	}
	return strings.Split(block, "\n")
}

func wantFits(t *testing.T, text string) {
	t.Helper()
	if got := panelUTF16Len(text); got > staffPanelMaxUTF16 {
		t.Fatalf("text is %d UTF-16 units, want at most %d", got, staffPanelMaxUTF16)
	}
}

func TestStaffActionSummaryTally(t *testing.T) {
	tr := panelMarkerTranslator(t)
	card := summaryTestCard()

	t.Run("counts by outcome", func(t *testing.T) {
		results := summaryResults(8, func(i int) (staffOutcome, staffReason) {
			switch {
			case i < 4:
				return staffOutcomeDone, staffReasonBanned
			case i == 4:
				return staffOutcomeSkipped, staffReasonSkipTargetAdmin
			case i == 5:
				return staffOutcomeFailed, staffReasonFailLookup
			}
			return staffOutcomePending, ""
		})
		done, skipped, failed, pending := staffSummaryTally(results)
		if done != 4 || skipped != 1 || failed != 1 || pending != 2 {
			t.Fatalf("tally = %d/%d/%d/%d, want 4/1/1/2", done, skipped, failed, pending)
		}
		lines := strings.Split(renderStaffActionSummary(tr, card, results), "\n")
		if len(lines) < 2 || lines[1] != "✅ 4 · ⏭ 1 · ❌ 1" {
			t.Fatalf("line under the header = %q, want the tally %q", lines[1:2], "✅ 4 · ⏭ 1 · ❌ 1")
		}
	})

	t.Run("a run where every group is skipped", func(t *testing.T) {
		results := summaryResults(5, func(int) (staffOutcome, staffReason) {
			return staffOutcomeSkipped, staffReasonSkipIssuerNotAdmin
		})
		for name, text := range map[string]string{
			"progress": renderStaffActionSummary(tr, card, results),
			"final":    firstOf(renderStaffActionSummaryFinal(tr, card, results)),
		} {
			lines := strings.Split(text, "\n")
			if len(lines) < 2 || lines[1] != "✅ 0 · ⏭ 5 · ❌ 0" {
				t.Fatalf("%s: line under the header = %q, want %q", name, lines[1:2], "✅ 0 · ⏭ 5 · ❌ 0")
			}
		}
	})

	t.Run("done plus skipped plus failed is every group", func(t *testing.T) {
		results := summaryResults(7, func(i int) (staffOutcome, staffReason) {
			switch i % 3 {
			case 0:
				return staffOutcomeDone, staffReasonKicked
			case 1:
				return staffOutcomeSkipped, staffReasonSkipNotInGroup
			}
			return staffOutcomeFailed, staffReasonFailInternal
		})
		text, _ := renderStaffActionSummaryFinal(tr, card, results)
		done, skipped, failed := summaryTallyOf(t, text)
		if done+skipped+failed != len(results) {
			t.Fatalf("tally %d+%d+%d, want %d groups", done, skipped, failed, len(results))
		}
	})
}

// firstOf returns the first value of a two-value call.
func firstOf(text string, _ []string) string { return text }

func TestStaffActionSummaryFixedOrder(t *testing.T) {
	tr := panelMarkerTranslator(t)
	card := summaryTestCard()

	finish := func(order []int) []staffGroupResult {
		results := summaryResults(5, func(int) (staffOutcome, staffReason) { return staffOutcomePending, "" })
		for _, i := range order {
			results[i].Outcome, results[i].Reason = staffOutcomeDone, staffReasonBanned
		}
		return results
	}
	titlesOf := func(text string) []string {
		var titles []string
		for _, line := range summaryGroupLines(t, text) {
			_, title, _ := strings.Cut(line, " ")
			titles = append(titles, title)
		}
		return titles
	}

	first := renderStaffActionSummary(tr, card, finish([]int{0, 1}))
	second := renderStaffActionSummary(tr, card, finish([]int{4, 3}))

	want := make([]string, 5)
	for i := range want {
		want[i] = summaryTitle(i)
	}
	for name, text := range map[string]string{"first": first, "second": second} {
		got := titlesOf(text)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("%s: group order = %v, want link order %v", name, got, want)
		}
	}
	if first == second {
		t.Fatal("the two renders are identical, want different icons for different progress")
	}
	if lines := summaryGroupLines(t, first); !strings.HasPrefix(lines[0], "✅ ") || !strings.HasPrefix(lines[4], "⏳ ") {
		t.Fatalf("first render lines = %q, want group 0 done and group 4 pending", lines)
	}
}

func TestStaffActionSummaryCollapse(t *testing.T) {
	tr := panelMarkerTranslator(t)
	card := summaryTestCard()
	results := summaryResults(120, func(i int) (staffOutcome, staffReason) {
		switch {
		case i < 110:
			return staffOutcomeDone, staffReasonBanned
		case i < 116:
			return staffOutcomeSkipped, staffReasonSkipTargetAdmin
		}
		return staffOutcomeFailed, staffReasonFailTelegram
	})

	text, continuation := renderStaffActionSummaryFinal(tr, card, results)

	wantFits(t, text)
	if len(continuation) != 0 {
		t.Fatalf("got %d continuation messages, want none when only the done lines need to collapse", len(continuation))
	}
	collapsed := "✅ " + staffMarker("staff_act_summary_done_collapsed") + " 110"
	if strings.Count(text, collapsed) != 1 {
		t.Fatalf("summary has no single collapsed done line %q:\n%s", collapsed, text)
	}
	for i := 0; i < 110; i++ {
		if strings.Contains(text, summaryTitle(i)) {
			t.Fatalf("done group %d is still listed by name, want it inside the count", i)
		}
	}
	for i := 110; i < 120; i++ {
		line := summaryLine(t, text, summaryTitle(i))
		wantMarker := staffMarker("staff_act_skip_target_admin")
		if i >= 116 {
			wantMarker = staffMarker("staff_act_fail_telegram")
		}
		if !strings.Contains(line, wantMarker) {
			t.Fatalf("line for group %d = %q, want its reason %s", i, line, wantMarker)
		}
	}
	done, skipped, failed := summaryTallyOf(t, text)
	if done != 110 || skipped != 6 || failed != 4 {
		t.Fatalf("tally = %d/%d/%d, want 110/6/4", done, skipped, failed)
	}
}

func TestStaffActionSummaryProgressCollapse(t *testing.T) {
	tr := panelMarkerTranslator(t)
	card := summaryTestCard()

	t.Run("done and pending lines collapse and skipped and failed stay listed", func(t *testing.T) {
		results := summaryResults(120, func(i int) (staffOutcome, staffReason) {
			switch {
			case i < 30:
				return staffOutcomeDone, staffReasonBanned
			case i < 110:
				return staffOutcomePending, ""
			case i < 116:
				return staffOutcomeSkipped, staffReasonSkipTargetAdmin
			}
			return staffOutcomeFailed, staffReasonFailTelegram
		})

		text := renderStaffActionSummary(tr, card, results)

		wantFits(t, text)
		if !strings.Contains(text, "✅ "+staffMarker("staff_act_summary_done_collapsed")+" 30") {
			t.Fatalf("summary has no collapsed done line with 30:\n%s", text)
		}
		if !strings.Contains(text, "⏳ "+staffMarker("staff_act_summary_pending_collapsed")+" 80") {
			t.Fatalf("summary has no collapsed pending line with 80:\n%s", text)
		}
		for i := 0; i < 110; i++ {
			if strings.Contains(text, summaryTitle(i)) {
				t.Fatalf("done or pending group %d is still listed by name", i)
			}
		}
		for i := 110; i < 120; i++ {
			if !strings.Contains(text, summaryTitle(i)) {
				t.Fatalf("skipped or failed group %d is missing from the progress summary", i)
			}
		}
	})

	t.Run("a failed list too long for one message points to the next message", func(t *testing.T) {
		results := summaryResults(300, func(i int) (staffOutcome, staffReason) {
			if i%2 == 0 {
				return staffOutcomeFailed, staffReasonFailLookup
			}
			return staffOutcomePending, ""
		})

		text := renderStaffActionSummary(tr, card, results)

		wantFits(t, text)
		if !strings.HasSuffix(text, staffMarker("staff_act_summary_continued")) {
			t.Fatalf("progress summary does not end with the continued marker:\n%s", text)
		}
	})
}

func TestStaffActionSummaryOverflow(t *testing.T) {
	tr := panelMarkerTranslator(t)
	card := summaryTestCard()
	results := summaryResults(300, func(int) (staffOutcome, staffReason) {
		return staffOutcomeFailed, staffReasonFailLookup
	})

	text, continuation := renderStaffActionSummaryFinal(tr, card, results)

	if len(continuation) == 0 {
		t.Fatal("got no continuation messages for 300 failed groups")
	}
	parts := append([]string{text}, continuation...)
	for i, part := range parts {
		wantFits(t, part)
		last := i == len(parts)-1
		hasContinued := strings.HasSuffix(part, staffMarker("staff_act_summary_continued"))
		if last && hasContinued {
			t.Fatalf("the last part %d ends with the continued marker, want none", i)
		}
		if !last && !hasContinued {
			t.Fatalf("part %d does not end with the continued marker", i)
		}
	}
	header := staffActionHeader(tr, card)
	for i, part := range continuation {
		if !strings.HasPrefix(part, header+"\n"+staffMarker("staff_act_summary_continuation")) {
			t.Fatalf("continuation %d does not start with the header and the continuation marker:\n%.200s", i, part)
		}
	}
	all := strings.Join(parts, "\n")
	for i := range results {
		if n := strings.Count(all, summaryTitle(i)); n != 1 {
			t.Fatalf("group %d appears %d times across the parts, want exactly once", i, n)
		}
	}
}

func TestStaffActionSummaryEscapes(t *testing.T) {
	tr := panelMarkerTranslator(t)
	card := summaryTestCard()

	t.Run("titles and reasons are escaped", func(t *testing.T) {
		failed := summaryResult(0, `<b>x</b> & 100%d`, staffOutcomeFailed, staffReasonFailTelegram)
		failed.Detail = telegramErrorDetail(&gotgbot.TelegramError{Code: 400, Description: "<i>r</i>"})

		text, _ := renderStaffActionSummaryFinal(tr, card, []staffGroupResult{failed})

		for _, want := range []string{"&lt;b&gt;x&lt;/b&gt; &amp; 100%d", "&lt;i&gt;r&lt;/i&gt;"} {
			if !strings.Contains(text, want) {
				t.Fatalf("summary does not contain the escaped %q:\n%s", want, text)
			}
		}
		for _, bad := range []string{"<b>x</b>", "<i>r</i>"} {
			if strings.Contains(text, bad) {
				t.Fatalf("summary contains the raw %q:\n%s", bad, text)
			}
		}
	})

	t.Run("length is measured after escaping", func(t *testing.T) {
		raw := strings.Repeat("<", 64)
		many := summaryResults(20, func(int) (staffOutcome, staffReason) { return staffOutcomeDone, staffReasonBanned })
		for i := range many {
			many[i].Link.GroupTitle = raw
		}
		text, continuation := renderStaffActionSummaryFinal(tr, card, many)
		wantFits(t, text)
		if len(continuation) != 0 {
			t.Fatalf("got %d continuation messages, want none", len(continuation))
		}
		// 20 lines of 256 units each cannot fit, although 20 raw lines would.
		if !strings.Contains(text, "✅ "+staffMarker("staff_act_summary_done_collapsed")+" 20") {
			t.Fatalf("20 groups of 256 escaped units were not collapsed:\n%s", text)
		}

		few, _ := renderStaffActionSummaryFinal(tr, card, many[:10])
		if got := strings.Count(few, strings.Repeat("&lt;", 64)); got != 10 {
			t.Fatalf("10 escaped titles listed %d times, want each on its own line", got)
		}
	})
}

func TestStaffActionSummaryAllLinesUnder40(t *testing.T) {
	tr := panelMarkerTranslator(t)
	card := summaryTestCard()
	results := make([]staffGroupResult, 39)
	for i := range results {
		results[i] = summaryResult(i, fmt.Sprintf("G%02d-", i)+strings.Repeat("y", 35), staffOutcomeDone, staffReasonBanned)
	}

	text, continuation := renderStaffActionSummaryFinal(tr, card, results)

	wantFits(t, text)
	if len(continuation) != 0 {
		t.Fatalf("got %d continuation messages, want none", len(continuation))
	}
	if strings.Contains(text, staffMarker("staff_act_summary_done_collapsed")) {
		t.Fatalf("39 groups were collapsed, want one line each:\n%s", text)
	}
	lines := 0
	for _, line := range summaryGroupLines(t, text) {
		if strings.HasPrefix(line, "✅ G") {
			lines++
		}
	}
	if lines != 39 {
		t.Fatalf("%d group lines, want 39", lines)
	}
}
