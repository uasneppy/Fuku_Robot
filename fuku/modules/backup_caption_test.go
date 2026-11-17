//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/backup"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// TestBuildExportCaptionRealLocales renders the export caption through the real locale files:
// every value shows, no verb or brace is left, and the chat title, which is untrusted and sent
// with the HTML parse mode, is escaped.
func TestBuildExportCaptionRealLocales(t *testing.T) {
	withRealLocales(t)

	bkp := backup.NewBackupFormat(-1009916001001, `Evil <b>&"Chat"`, 4242, []string{"notes", "filters"})
	bkp.Data["notes"] = map[string]any{"x": 1}
	bkp.Data["filters"] = map[string]any{"x": 1}
	bkp.ExportedAt = bkp.ExportedAt.UTC()
	stamp := bkp.ExportedAt.Format("2006-01-02 15:04:05")

	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		caption := buildExportCaption(i18n.MustNewTranslator(code), bkp)
		if strings.ContainsAny(caption, "%{}") {
			t.Errorf("%s: the caption holds a verb or a brace: %q", code, caption)
		}
		for _, want := range []string{"Evil &lt;b&gt;&amp;&#34;Chat&#34;", stamp, "2", "• notes", "• filters"} {
			if !strings.Contains(caption, want) {
				t.Errorf("%s: the caption lacks %q: %q", code, want, caption)
			}
		}
		if strings.Contains(caption, "<b>&") {
			t.Errorf("%s: the chat title was not escaped: %q", code, caption)
		}
	}
}
