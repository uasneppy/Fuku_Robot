//go:build testtools

package content

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// TestMain installs the real English and Ukrainian locale files once, before any test runs, so the refusal texts
// the tests compare are the real ones. Installing it here (not per test) keeps parallel tests race free.
func TestMain(m *testing.M) {
	files := make(map[string]string)
	for _, code := range []string{"uk", "en"} {
		raw, err := os.ReadFile("../../../locales/" + code + ".yml")
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s.yml: %v\n", code, err)
			os.Exit(1)
		}
		files[code] = string(raw)
	}
	if _, err := i18n.OverrideManagerLanguagesForTest(files); err != nil {
		fmt.Fprintf(os.Stderr, "install locales: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// expectedRefusal builds the reply the commands send for an over-long text, from the real locale.
func expectedRefusal(t *testing.T, key string, count int) string {
	t.Helper()
	tr := i18n.MustNewTranslator("en")
	template, err := tr.GetString(key)
	if err != nil || template == "" {
		t.Fatalf("locale key %s missing: %v", key, err)
	}
	return fmt.Sprintf(template, count)
}

func TestTextLengthRefusal(t *testing.T) {
	t.Parallel()

	t.Run("text over 4096 code points is refused with its count", func(t *testing.T) {
		t.Parallel()
		got := TextLengthRefusal(strings.Repeat("a", 4097), db.TEXT, "en")
		want := expectedRefusal(t, "content_text_too_long", 4097)
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("text of exactly 4096 is accepted", func(t *testing.T) {
		t.Parallel()
		if got := TextLengthRefusal(strings.Repeat("a", 4096), db.TEXT, "en"); got != "" {
			t.Fatalf("expected no refusal, got %q", got)
		}
	})

	t.Run("caption over 1024 is refused with its count", func(t *testing.T) {
		t.Parallel()
		got := TextLengthRefusal(strings.Repeat("a", 1025), db.PHOTO, "en")
		want := expectedRefusal(t, "content_caption_too_long", 1025)
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("caption of exactly 1024 is accepted", func(t *testing.T) {
		t.Parallel()
		if got := TextLengthRefusal(strings.Repeat("a", 1024), db.PHOTO, "en"); got != "" {
			t.Fatalf("expected no refusal, got %q", got)
		}
	})

	t.Run("a long text is fine as text and refused as a caption", func(t *testing.T) {
		t.Parallel()
		text := strings.Repeat("a", 2000)
		if got := TextLengthRefusal(text, db.TEXT, "en"); got != "" {
			t.Fatalf("text: expected no refusal, got %q", got)
		}
		if got := TextLengthRefusal(text, db.STICKER, "en"); got == "" {
			t.Fatalf("caption: expected a refusal")
		}
	})

	t.Run("code points are counted, not bytes", func(t *testing.T) {
		t.Parallel()
		// 4096 emoji are 4096 code points although far more bytes.
		if got := TextLengthRefusal(strings.Repeat("😀", 4096), db.TEXT, "en"); got != "" {
			t.Fatalf("expected no refusal, got %q", got)
		}
	})
}

func TestTextLengthRefusalMatchesExtractWelcome(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 4100)
	msg := &gotgbot.Message{Text: "/setwelcome " + long}
	result := ExtractWelcome(msg, "welcome", "en")
	if result.DataType != -1 {
		t.Fatalf("expected the command to refuse the text, got dataType %d", result.DataType)
	}
	if want := TextLengthRefusal(result.Text, db.TEXT, "en"); want == "" || result.ErrorMsg != want {
		t.Fatalf("command reply %q differs from TextLengthRefusal %q", result.ErrorMsg, want)
	}
}

func TestValidButtonURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want bool
	}{
		{"https://e.com", true},
		{"http://e.com/x", true},
		{"https://e.com/a?b=c#d", true},
		{"javascript:alert(1)", false},
		{"file:///etc/passwd", false},
		{"tg://resolve?domain=x", false},
		{"e.com", false},
		{"", false},
		{"https://", false},
		{"//e.com", false},
		{"https://e.com/%zz", false},
	}
	for _, c := range cases {
		if got := ValidButtonURL(c.raw); got != c.want {
			t.Errorf("ValidButtonURL(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestExtractMedia(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		msg      *gotgbot.Message
		wantID   string
		wantType int
	}{
		{"sticker", &gotgbot.Message{Sticker: &gotgbot.Sticker{FileId: "st"}}, "st", db.STICKER},
		{"animation also carries a document: stored as an animation", &gotgbot.Message{
			Animation: &gotgbot.Animation{FileId: "an"},
			Document:  &gotgbot.Document{FileId: "doc"},
		}, "an", db.ANIMATION},
		{"animation alone is stored as an animation", &gotgbot.Message{Animation: &gotgbot.Animation{FileId: "an"}}, "an", db.ANIMATION},
		{"photo gives the largest size", &gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: "small"}, {FileId: "big"}}}, "big", db.PHOTO},
		{"audio", &gotgbot.Message{Audio: &gotgbot.Audio{FileId: "au"}}, "au", db.AUDIO},
		{"voice", &gotgbot.Message{Voice: &gotgbot.Voice{FileId: "vo"}}, "vo", db.VOICE},
		{"video", &gotgbot.Message{Video: &gotgbot.Video{FileId: "vi"}}, "vi", db.VIDEO},
		{"video note", &gotgbot.Message{VideoNote: &gotgbot.VideoNote{FileId: "vn"}}, "vn", db.VIDEO_NOTE},
		{"text only", &gotgbot.Message{Text: "hi"}, "", -1},
		{"nil message", nil, "", -1},
	}
	for _, c := range cases {
		id, typ := ExtractMedia(c.msg)
		if id != c.wantID || typ != c.wantType {
			t.Errorf("%s: got (%q, %d), want (%q, %d)", c.name, id, typ, c.wantID, c.wantType)
		}
		// The editor and the commands must agree on every message, except that a GIF is an
		// animation for greetings and stays a document for notes, filters and pins.
		if typ == db.ANIMATION {
			continue
		}
		cid, ctyp := extractMediaFromReply(c.msg)
		if id != cid || typ != ctyp {
			t.Errorf("%s: ExtractMedia differs from extractMediaFromReply", c.name)
		}
	}
}
