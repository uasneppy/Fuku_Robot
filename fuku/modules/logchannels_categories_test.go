//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// TestLogCategoriesAcceptOnlyLiveNames drives /log and /nolog through the real dispatcher: the two
// categories nothing ever posts to are unknown names, and their stored flags are not touched.
func TestLogCategoriesAcceptOnlyLiveNames(t *testing.T) {
	withRealLocales(t)
	env := newSetLogFlowEnv(t)
	if err := logchannels.Set(env.group.Id, env.group.Title, env.channel.Id); err != nil {
		t.Fatalf("Set: %v", err)
	}
	row := func() (admin, automated, other bool) {
		t.Helper()
		got := logchannels.Get(env.group.Id)
		if got == nil {
			t.Fatal("the log channel row is gone")
		}
		return got.CatAdmin, got.CatAutomated, got.CatOther
	}

	env.process(textUpdate(env.group, env.admin, "/nolog automated other"))
	if got, want := flowLastText(env.client, env.group.Id), ukText(t, "logs_unknown_category"); got != want {
		t.Fatalf("/nolog of the two dead names answered %q, want %q", got, want)
	}
	if admin, automated, other := row(); !admin || !automated || !other {
		t.Fatalf("flags after /nolog automated other: admin=%v automated=%v other=%v, want all on", admin, automated, other)
	}

	env.process(textUpdate(env.group, env.admin, "/nolog admin automated"))
	reply := flowLastText(env.client, env.group.Id)
	if !strings.Contains(reply, "admin") || strings.Contains(reply, "automated") {
		t.Fatalf("/nolog admin automated answered %q, want only admin listed", reply)
	}
	if admin, automated, _ := row(); admin || !automated {
		t.Fatalf("flags after /nolog admin automated: admin=%v automated=%v, want admin off and automated on", admin, automated)
	}

	env.process(textUpdate(env.group, env.admin, "/log admin"))
	if admin, _, _ := row(); !admin {
		t.Fatal("/log admin did not turn the admin category back on")
	}
}

// TestLogCategoryLocaleTexts holds the category list of /logcategories and of the module help to
// the four live names in every language.
func TestLogCategoryLocaleTexts(t *testing.T) {
	withRealLocales(t)
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		tr := i18n.MustNewTranslator(code)
		for _, key := range []string{"logs_categories", "logchannels_help_msg"} {
			text, err := tr.GetString(key)
			if err != nil {
				t.Fatalf("%s: %s: %v", code, key, err)
			}
			for _, name := range logchannels.AllCategories {
				if !strings.Contains(text, name) {
					t.Errorf("%s: %s does not name %q", code, key, name)
				}
			}
			if strings.Contains(text, "automated") {
				t.Errorf("%s: %s still names automated", code, key)
			}
			if key == "logs_categories" && strings.Contains(text, ", other") {
				t.Errorf("%s: %s still names other", code, key)
			}
		}
	}
}
