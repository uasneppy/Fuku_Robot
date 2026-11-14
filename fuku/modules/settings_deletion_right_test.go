//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// settingsDelBotJSON is the bot's own getChatMember record with the given status and delete right.
func settingsDelBotJSON(status string, canDelete bool) string {
	return fmt.Sprintf(`{"status":%q,"user":{"id":%d,"is_bot":true,"first_name":"Fuku"},"can_delete_messages":%t}`,
		status, staffTestBotID, canDelete)
}

// settingsDelScreen is one deletion screen: how to draw it, the key whose text comes right before the
// status slot, the key right after it and the row widths it must keep whatever the right line says.
type settingsDelScreen struct {
	name   string
	draw   func(m *settingsDelMenu) string
	before string
	after  string
	widths string
}

var settingsDelScreens = []settingsDelScreen{
	{"hub", func(m *settingsDelMenu) string { text, _ := m.openHub(); return text },
		"settings_del_sec_member", "settings_del_hint_time", "[2 1 2]"},
	{"service list", func(m *settingsDelMenu) string { text, _ := m.open("dl", "s"); return text },
		"settings_del_svc_other", "settings_del_hint_cleanservice", "[2 2 2 2 2 2 2]"},
	{"bot list", func(m *settingsDelMenu) string { text, _ := m.open("dl", "b"); return text },
		"settings_del_bot_manual", "settings_del_hint_time", "[2 2 1 2]"},
	{"service row", func(m *settingsDelMenu) string { text, _ := m.open("dr", "sn"); return text },
		"settings_del_desc_pinned", "settings_del_currently", "[2 3 3 2]"},
	{"bot row with a hint", func(m *settingsDelMenu) string { text, _ := m.open("dr", "ba"); return text },
		"settings_del_hint_punish_auto", "settings_del_currently", "[3 3 2]"},
	// Phase 17: the Member messages section. The Self-destruct card is drawn by choosing a delay on a row
	// with no rule stored; it has no text after the status slot.
	{"member list", func(m *settingsDelMenu) string { text, _ := m.open("dl", "m"); return text },
		"settings_del_mem_self", "settings_del_hint_time", "[2 1 2]"},
	{"commands row", func(m *settingsDelMenu) string { text, _ := m.open("dr", "mc"); return text },
		"settings_del_hint_commands", "settings_del_currently", "[2 2]"},
	{"edited row", func(m *settingsDelMenu) string { text, _ := m.open("dr", "me"); return text },
		"settings_del_hint_edited", "settings_del_currently", "[3 3 2]"},
	{"self-destruct row", func(m *settingsDelMenu) string { text, _ := m.open("dr", "ms"); return text },
		"settings_del_hint_self", "settings_del_currently", "[1 3 3 2]"},
	{"self-destruct card", func(m *settingsDelMenu) string { m.choose("ms", "600"); text, _ := m.last(); return text },
		"settings_del_confirm_self", "", "[2 2]"},
}

// settingsDelRightCase is one answer of the bot's own record: a scripted record or a scripted error,
// the chat type, and the line it must draw (none, cannot or unknown).
type settingsDelRightCase struct {
	name  string
	raw   string
	err   error
	hang  bool
	basic bool
	want  string
	// reads is how many requests about the bot one draw makes: one, except that the pacer repeats a
	// call after a 429 (its MaxRetries of 3) before it gives up.
	reads int
}

func settingsDelRightCases() []settingsDelRightCase {
	return []settingsDelRightCase{
		{name: "creator", raw: settingsDelBotJSON("creator", false), want: "none"},
		{name: "administrator with the right", raw: settingsDelBotJSON("administrator", true), want: "none"},
		{name: "administrator without it", raw: settingsDelBotJSON("administrator", false), want: "cannot"},
		{name: "administrator without it in a basic group", raw: settingsDelBotJSON("administrator", false), basic: true, want: "none"},
		{name: "member", raw: settingsDelBotJSON("member", false), want: "cannot"},
		{name: "member in a basic group", raw: settingsDelBotJSON("member", false), basic: true, want: "cannot"},
		{name: "restricted", raw: settingsDelBotJSON("restricted", false), want: "cannot"},
		{name: "left", raw: settingsDelBotJSON("left", false), want: "cannot"},
		{name: "kicked", raw: settingsDelBotJSON("kicked", false), want: "cannot"},
		{name: "a 400", err: staffFakeError(400, "Bad Request: chat not found"), want: "cannot"},
		{name: "a 403", err: staffFakeError(403, "Forbidden: bot was kicked"), want: "cannot"},
		{name: "a 429", err: staffFake429(1), want: "unknown", reads: 4},
		{name: "a 5xx", err: staffFakeError(500, "Internal Server Error"), want: "unknown"},
		{name: "a transport error", err: errors.New("dial tcp: connection refused"), want: "unknown"},
		{name: "a timeout", hang: true, want: "unknown"},
		{name: "a shed pacer", err: ratelimit.ErrRateLimited, want: "unknown"},
		{name: "a null body", raw: "null", want: "unknown"},
	}
}

// scriptBot installs the script that answers the bot's own getChatMember with err (or hangs until the
// request ends) and records how many callback answers had been made when each of those reads arrived.
func (e *settingsDelEnv) scriptBot(hang bool, err error) *[]int {
	var answersAtRead []int
	e.script.set(func(ctx context.Context, method string, params map[string]any) error {
		if method != "getChatMember" || staffParamInt(params, "user_id") != staffTestBotID {
			return nil
		}
		answersAtRead = append(answersAtRead, e.answerCount())
		if hang {
			<-ctx.Done()
			return ctx.Err()
		}
		return err
	})
	return &answersAtRead
}

func TestSettingsDeletionRightLine(t *testing.T) {
	noRight, unknown := staffMarker("settings_del_no_right"), staffMarker("settings_del_right_unknown")
	for _, tc := range settingsDelRightCases() {
		t.Run(tc.name, func(t *testing.T) {
			mreqSwap(t, &settingsDelHereTimeout, 50*time.Millisecond)
			env := newSettingsDelEnv(t)
			m := env.groupMenu()
			if tc.basic {
				m.chat.Type = "group"
			}
			if tc.raw != "" {
				env.setBot(tc.raw)
			}
			answersAtRead := env.scriptBot(tc.hang, tc.err)

			for _, screen := range settingsDelScreens {
				answers, reads := env.answerCount(), len(*answersAtRead)
				text := screen.draw(m)
				_, keyboard := m.last()

				// Exactly one line, or none, with its icon as a code-side prefix.
				wantNo, wantUnknown := 0, 0
				switch tc.want {
				case "cannot":
					wantNo = 1
				case "unknown":
					wantUnknown = 1
				}
				if got := strings.Count(text, noRight); got != wantNo {
					t.Errorf("%s: %d no-right lines, want %d: %q", screen.name, got, wantNo, text)
				}
				if got := strings.Count(text, unknown); got != wantUnknown {
					t.Errorf("%s: %d unknown lines, want %d: %q", screen.name, got, wantUnknown, text)
				}
				if wantNo == 1 && !strings.Contains(text, "🚫 "+noRight) {
					t.Errorf("%s: the no-right line has no 🚫 prefix: %q", screen.name, text)
				}
				if wantUnknown == 1 && !strings.Contains(text, "❔ "+unknown) {
					t.Errorf("%s: the unknown line has no ❔ prefix: %q", screen.name, text)
				}
				// The slot: after the text of screen.before, before the text of screen.after.
				if wantNo+wantUnknown == 1 {
					at := strings.Index(text, noRight)
					if wantUnknown == 1 {
						at = strings.Index(text, unknown)
					}
					if before := strings.Index(text, staffMarker(screen.before)); before < 0 || at < before {
						t.Errorf("%s: the line is not after %s: %q", screen.name, screen.before, text)
					}
					if after := strings.LastIndex(text, staffMarker(screen.after)); screen.after != "" && (after < 0 || at > after) {
						t.Errorf("%s: the line is not before %s: %q", screen.name, screen.after, text)
					}
				}
				// Never an error, a refusal or the cannot-open edit, and no button is disabled.
				if containsMarker(text, "settings_cannot_open") || len(keyboard) == 0 {
					t.Errorf("%s drew the cannot-open edit: %q", screen.name, text)
				}
				if got := fmt.Sprint(settingsDelWidths(keyboard)); got != screen.widths {
					t.Errorf("%s: row widths = %s, want %s whatever the right line says", screen.name, got, screen.widths)
				}
				for _, button := range settingsAllButtons(keyboard) {
					if button.CallbackData == "" {
						t.Errorf("%s: a dead button %q", screen.name, button.Text)
					}
				}
				if answer, alert := env.lastAnswer(); answer != "" || alert {
					t.Errorf("%s: answered %q alert=%v, want the empty answer", screen.name, answer, alert)
				}
				// Answered exactly once, and before the one live read.
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("%s: the press was answered %d times, want once", screen.name, got)
				}
				wantReads := max(tc.reads, 1)
				if got := len(*answersAtRead) - reads; got != wantReads {
					t.Errorf("%s: %d reads of the bot, want %d", screen.name, got, wantReads)
				} else if at := (*answersAtRead)[reads]; at != answers+1 {
					t.Errorf("%s: the press had %d answers when the bot was read, want it answered first (%d)", screen.name, at, answers+1)
				}
			}

			// A rule can be set with the line showing, and the redraw shows the line once.
			m.choose("sn", "600")
			if delay, ok := env.stored(deletion.KindPinned); !ok || delay != 600 {
				t.Errorf("with the line showing, stored pinned = %d (%v), want 600", delay, ok)
			}
			text, _ := m.last()
			wantLines := 1
			if tc.want == "none" {
				wantLines = 0
			}
			if got := strings.Count(text, noRight) + strings.Count(text, unknown); got != wantLines {
				t.Errorf("the redraw after a write has %d right lines, want %d: %q", got, wantLines, text)
			}

			// A Self-destruct card is drawn and confirmed while the line shows: the line is on the card and
			// on the redrawn row, the rule is stored, and the button of the card is a live Yes.
			m.choose("ms", "3600")
			text, _ = m.last()
			if got := strings.Count(text, noRight) + strings.Count(text, unknown); got != wantLines {
				t.Errorf("the card has %d right lines, want %d: %q", got, wantLines, text)
			}
			yes, _ := settingsDelCardOf(t, m)
			m.pressButton(yes)
			if delay, ok := env.stored(deletion.KindMemberSelfDestruct); !ok || delay != 3600 {
				t.Errorf("with the line showing, a confirmed card stored %d (%v), want 3600", delay, ok)
			}
			text, _ = m.last()
			if got := strings.Count(text, noRight) + strings.Count(text, unknown); got != wantLines {
				t.Errorf("the row redrawn after the Yes has %d right lines, want %d: %q", got, wantLines, text)
			}
		})
	}

	t.Run("the private menu draws the line too", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		m := env.privateMenu()
		env.setBot(settingsDelBotJSON("member", false))
		text, _ := m.open("dr", "bw")
		if strings.Count(text, noRight) != 1 {
			t.Errorf("private bot row screen = %q, want the no-right line once", text)
		}
	})
}

func TestSettingsDeletionCost(t *testing.T) {
	env := newSettingsDelEnv(t)
	m := env.groupMenu()
	chatID := env.chat.Id
	owner := env.owner.Id
	draw := func(do func()) (bot, presser int) {
		t.Helper()
		bots, pressers := env.botReads(chatID), env.memberLookupsOf(chatID, owner)
		do()
		return env.botReads(chatID) - bots, env.memberLookupsOf(chatID, owner) - pressers
	}

	// Main makes no call about the bot.
	if bot, _ := draw(func() { m.press("a", "bk") }); bot != 0 {
		t.Errorf("Main made %d reads of the bot, want 0", bot)
	}
	// The hub, a list and a row screen make exactly one each; the pipeline's own call is unchanged.
	for _, tc := range []struct {
		name string
		do   func()
	}{
		{"hub", func() { m.press("a", "op", "p", "d") }},
		{"list", func() { m.press("a", "dl", "t", "s") }},
		{"row", func() { m.press("a", "dr", "t", "sn") }},
		{"a choice", func() { m.choose("sn", "600") }},
		// Phase 17: every member screen is one read of the bot too, a choice and a card's draw and Yes.
		{"member list", func() { m.press("a", "dl", "t", "m") }},
		{"commands row", func() { m.press("a", "dr", "t", "mc") }},
		{"edited row", func() { m.press("a", "dr", "t", "me") }},
		{"self-destruct row", func() { m.press("a", "dr", "t", "ms") }},
		{"a commands choice", func() { m.choose("mc", "0") }},
		{"an edited choice", func() { m.choose("me", "60") }},
		{"a self-destruct card", func() { m.choose("ms", "600") }},
		{"the card's Yes", func() {
			yes, _ := settingsDelCardOf(t, m)
			m.pressButton(yes)
		}},
	} {
		bot, presser := draw(tc.do)
		if bot != 1 || presser != 1 {
			t.Errorf("%s: %d reads of the bot and %d of the presser, want 1 and 1", tc.name, bot, presser)
		}
	}
	// A press of the stored state still redraws once and writes nothing.
	if bot, _ := draw(func() { m.choose("sn", "600") }); bot != 1 {
		t.Errorf("a press of the stored state made %d reads of the bot, want 1", bot)
	}
	if env.redis.Exists("fuku:delete:pace:next") == false {
		t.Error("the reads did not go through the deletion pacer (no slot key in Redis)")
	}

	// With Redis down the page makes no extra call and the line is the same.
	env.setBot(settingsDelBotJSON("member", false))
	withRedis, _ := m.open("dl", "s")
	marshal, manager, client := cache.GetCacheState()
	cache.SetCacheState(marshal, manager, nil)
	t.Cleanup(func() { cache.SetCacheState(marshal, manager, client) })
	var withoutRedis string
	if bot, presser := draw(func() { withoutRedis, _ = m.open("dl", "s") }); bot != 1 || presser != 1 {
		t.Errorf("with Redis down: %d reads of the bot and %d of the presser, want 1 and 1", bot, presser)
	}
	if withRedis != withoutRedis {
		t.Errorf("the screen differs with Redis down:\nwith:    %q\nwithout: %q", withRedis, withoutRedis)
	}
}
