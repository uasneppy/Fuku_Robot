//go:build testtools

package modules

import (
	"fmt"
	"html"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// staffSeamsEnglish returns a translator over the real en.yml strings, so a
// length test sees the lengths users see.
func staffSeamsEnglish(t *testing.T) *i18n.Translator {
	t.Helper()
	raw, err := os.ReadFile("../../locales/en.yml")
	if err != nil {
		t.Fatalf("read locale en: %v", err)
	}
	tr, err := i18n.NewTestTranslator(string(raw))
	if err != nil {
		t.Fatalf("parse locale en: %v", err)
	}
	return tr
}

func TestStaffRowSplitMatchesRow(t *testing.T) {
	tr := staffSeamsEnglish(t)
	locked := time.Date(2026, 10, 30, 12, 34, 0, 0, time.UTC)

	healths := []struct {
		name        string
		health      string
		healthKnown bool
	}{
		{"ok", models.StaffHealthOK, true},
		{"bot missing", models.StaffHealthBotMissing, true},
		{"bot not admin", models.StaffHealthBotNotAdmin, true},
		{"bot cannot restrict", models.StaffHealthBotCannotRestrict, true},
		{"unknown", "", false},
	}
	owners := []struct {
		name  string
		state chat_status.OwnerResult
	}{
		{"owner match", chat_status.OwnerMatch},
		{"owner unknown", chat_status.OwnerUnknown},
	}
	titles := []string{"Plain Group", "Fish & <b>Chips</b> \u00e9\u00e8 \U0001F680 \u4e2d\u6587"}

	for _, h := range healths {
		for _, o := range owners {
			for _, lockedSince := range []time.Time{{}, locked} {
				for ti, title := range titles {
					name := fmt.Sprintf("%s/%s/locked=%t/title%d", h.name, o.name, !lockedSince.IsZero(), ti)
					t.Run(name, func(t *testing.T) {
						row := staffLinkRow{
							Link: models.StaffGroupLink{
								ID:          7,
								GroupChatID: -1002345678901,
								StaffChatID: -1001111111111,
								OwnerUserID: 42,
								GroupTitle:  title,
							},
							Health:      h.health,
							HealthKnown: h.healthKnown,
							OwnerState:  o.state,
							LockedSince: lockedSince,
						}
						head := renderStaffRowHead(row)
						status := renderStaffRowStatus(tr, row)

						if got, want := head+"\n"+status, renderStaffRow(tr, row); got != want {
							t.Fatalf("head + newline + status =\n%q\nwant renderStaffRow =\n%q", got, want)
						}
						if !strings.Contains(head, staffDisplayTitle(title)) {
							t.Errorf("head %q must carry the escaped title %q", head, staffDisplayTitle(title))
						}
						if !strings.Contains(head, "<code>-1002345678901</code>") {
							t.Errorf("head %q must carry the chat ID in code", head)
						}
						for _, leak := range []string{title, html.EscapeString(title), staffDisplayTitle(title), "-1002345678901"} {
							if strings.Contains(status, leak) {
								t.Errorf("status %q must not contain %q", status, leak)
							}
						}
						if status == "" {
							t.Errorf("status of a row must not be empty")
						}
					})
				}
			}
		}
	}
}

func TestStaffPanelTextLimit(t *testing.T) {
	tr := staffSeamsEnglish(t)
	now := time.Date(2026, 10, 30, 9, 8, 7, 0, time.UTC)
	group := panelRenderGroup()

	view := func(rows []staffLinkRow) staffPanelView {
		return staffPanelView{
			staffGroup: group,
			total:      len(rows),
			shown:      rows,
			page:       0,
			pages:      1,
			updatedAt:  now,
		}
	}

	t.Run("limit 0 is the default cap", func(t *testing.T) {
		for _, n := range []int{0, 1, 3, 8} {
			rows := panelRows(n, func(int) string { return strings.Repeat("&", 64) })
			for i := range rows {
				rows[i].Health = models.StaffHealthBotCannotRestrict
			}
			zero := view(rows)
			capped := view(rows)
			capped.limit = staffPanelMaxUTF16

			zeroText, zeroShown := staffPanelTextFor(tr, zero)
			cappedText, cappedShown := staffPanelTextFor(tr, capped)
			if zeroText != cappedText || len(zeroShown) != len(cappedShown) {
				t.Errorf("%d rows: limit 0 gave %d rows and %q, limit %d gave %d rows and %q",
					n, len(zeroShown), zeroText, staffPanelMaxUTF16, len(cappedShown), cappedText)
			}
			if got := panelUTF16Len(zeroText); got > staffPanelMaxUTF16 {
				t.Errorf("%d rows: text is %d UTF-16 units, want at most %d", n, got, staffPanelMaxUTF16)
			}
		}
	})

	t.Run("a small limit swaps the help and drops rows with the note", func(t *testing.T) {
		rows := panelRows(8, func(i int) string { return "Group" + string(rune('A'+i)) })
		const limit = 600

		small := view(rows)
		small.limit = limit
		text, shown := staffPanelTextFor(tr, small)

		if got := panelUTF16Len(text); got > limit {
			t.Fatalf("text is %d UTF-16 units, want at most %d:\n%s", got, limit, text)
		}
		hint, _ := tr.GetString("staff_panel_help_hint")
		if !strings.Contains(text, hint) {
			t.Errorf("text must use the help hint %q", hint)
		}
		if len(shown) == 0 || len(shown) >= len(rows) {
			t.Fatalf("%d rows kept, want some but fewer than %d", len(shown), len(rows))
		}
		note, _ := tr.GetString("staff_panel_truncated", i18n.TranslationParams{"count": len(rows) - len(shown)})
		if !strings.Contains(text, note) {
			t.Errorf("text must carry the note %q for the %d dropped rows", note, len(rows)-len(shown))
		}
		// The default limit keeps every one of these rows, so the drop is the limit's doing.
		_, defaultShown := staffPanelTextFor(tr, view(rows))
		if len(defaultShown) != len(rows) {
			t.Fatalf("default limit kept %d rows, want all %d", len(defaultShown), len(rows))
		}
	})

	t.Run("a text that fits is unchanged by any limit it fits", func(t *testing.T) {
		rows := panelRows(1, func(int) string { return "Tiny" })
		base, baseShown := staffPanelTextFor(tr, view(rows))
		need := panelUTF16Len(base)
		for _, limit := range []int{0, need, need + 1, staffPanelMaxUTF16} {
			v := view(rows)
			v.limit = limit
			text, shown := staffPanelTextFor(tr, v)
			if text != base || len(shown) != len(baseShown) {
				t.Errorf("limit %d changed a text that fits in %d units: %q", limit, need, text)
			}
		}
	})
}
