//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// The page's status for the AI rule (plan 15-08; D-14, UI-SPEC amendments M15 to M17). The AI is
// configured or not by the process alone: the page says so in words, offers Off alone while it is
// not, and the status read makes no Telegram call and no Redis command.

// settingsMreqAILine is the AI status line of the marker locale: the code-side icon, the key and the
// translated rule name.
func settingsMreqAILine() string {
	return "🚫 " + staffMarker("settings_mreq_here_ai") + " " + staffMarker("settings_mreq_rule_ai")
}

// settingsMreqAIScreenHead is the part of the AI rule's screen from its description to the
// Punishment line, with the status slot slot (empty for none).
func settingsMreqAIScreenHead(slot string, action int) string {
	head := staffMarker("settings_mreq_desc_ai") + "\n" + staffMarker("settings_mreq_hint_ai") + "\n"
	if slot != "" {
		head += slot + "\n"
	}
	return head + "\n" + staffMarker("settings_mreq_now") + " " + settingsMreqIcons[action] + " " + staffMarker(settingsMreqWordKeys[action])
}

func TestSettingsMreqAIStatus(t *testing.T) {
	type combo struct {
		name       string
		basic      bool
		configured bool
		stored     int
	}
	var combos []combo
	for _, basic := range []bool{false, true} {
		for _, configured := range []bool{true, false} {
			for _, stored := range []int{models.RequirementActionOff, models.RequirementActionWarn, models.RequirementActionBan} {
				group := map[bool]string{false: "supergroup", true: "basic group"}[basic]
				combos = append(combos, combo{
					name:  fmt.Sprintf("%s, configured=%v, stored %s", group, configured, settingsMreqWordKeys[stored]),
					basic: basic, configured: configured, stored: stored,
				})
			}
		}
	}

	for _, private := range []bool{false, true} {
		menu := map[bool]string{false: "group menu", true: "private menu"}[private]
		for _, c := range combos {
			t.Run(menu+": "+c.name, func(t *testing.T) {
				if c.configured {
					withAIConfigured(t)
				} else {
					withAINotConfigured(t, mreqAINoKey)
				}
				var env *settingsMreqHereEnv
				if c.basic {
					env = settingsMreqBasicEnv(t, private)
				} else {
					env = newSettingsMreqHereEnv(t)
				}
				env.seed([8]int{7: c.stored}, false, false)
				msgID := env.openMenuOf(private)
				aiLine, basicText := settingsMreqAILine(), "🚫 "+staffMarker("settings_mreq_here_basic")

				// The page: the AI line only while the rule is not Off and the AI is not configured,
				// after the group-level line, with the basic line still once.
				chats := mreqChatCalls(env.memberReqEnv, env.chat.Id)
				members := len(env.fake.callsFor("getChatMember"))
				text, keyboard := env.press(private, msgID, "a", "op", "p", settingsPageMreq)
				wantAI := !c.configured && c.stored != models.RequirementActionOff
				if got := strings.Count(text, aiLine); got != btoi(wantAI) {
					t.Errorf("the AI line appears %d times on the page %q, want %d", got, text, btoi(wantAI))
				}
				wantBasic := c.basic && c.stored != models.RequirementActionOff
				if got := strings.Count(text, basicText); got != btoi(wantBasic) {
					t.Errorf("the basic-group line appears %d times on the page %q, want %d", got, text, btoi(wantBasic))
				}
				if wantAI && wantBasic && !strings.Contains(text, basicText+"\n"+aiLine) {
					t.Errorf("the page %q does not list the AI line right after the basic-group line", text)
				}
				if wantAI && !wantBasic && !strings.Contains(text, settingsMreqSwitchLine("settings_mreq_line_delete", false)+"\n\n"+aiLine+"\n\n") {
					t.Errorf("the page %q does not open the cannot-work block with the AI line", text)
				}
				if !wantAI && !wantBasic && strings.Contains(text, "🚫") {
					t.Errorf("the page %q shows a cannot-work line with nothing to say", text)
				}
				if got := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(got, []int{2, 2, 2, 2, 2, 2}) {
					t.Errorf("the page rows = %v, want the six rows", got)
				}
				if got := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; got != btoi(private) {
					t.Errorf("the page made %d getChat requests, want %d (the pipeline's own)", got, btoi(private))
				}
				if got := len(env.fake.callsFor("getChatMember")) - members; got != 1 {
					t.Errorf("the page made %d getChatMember requests, want the pipeline's one", got)
				}

				// The AI rule's screen: one slot line (the AI line wins over the basic-group line),
				// the Off-only form exactly while the AI is not configured.
				slot := ""
				switch {
				case !c.configured:
					slot = aiLine
				case c.basic:
					slot = basicText
				}
				chats = mreqChatCalls(env.memberReqEnv, env.chat.Id)
				members = len(env.fake.callsFor("getChatMember"))
				text, keyboard = env.press(private, msgID, "a", "mo", "t", "ai")
				if want := settingsMreqAIScreenHead(slot, c.stored); !strings.Contains(text, want) {
					t.Errorf("the AI screen %q lacks %q", text, want)
				}
				if n := strings.Count(text, "🚫"); n != btoi(slot != "") {
					t.Errorf("the AI screen %q has %d status lines, want %d (one slot, one line)", text, n, btoi(slot != ""))
				}
				hasHints := strings.Contains(text, staffMarker("settings_mreq_choose_hint")) || strings.Contains(text, staffMarker("settings_mreq_actions_hint"))
				wantRows := []int{2, 3, 2}
				if !c.configured {
					wantRows = []int{1, 2}
				}
				if hasHints == !c.configured {
					t.Errorf("the AI screen %q: choice hints present = %v while configured = %v", text, hasHints, c.configured)
				}
				if got := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(got, wantRows) {
					t.Errorf("the AI screen rows = %v, want %v", got, wantRows)
				}
				if marked := strings.HasPrefix(keyboard[0][0].Text, "✅"); marked != (c.stored == models.RequirementActionOff) {
					t.Errorf("the first choice %q marked = %v with %s stored", keyboard[0][0].Text, marked, settingsMreqWordKeys[c.stored])
				}
				if got := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; got != btoi(private) {
					t.Errorf("the AI screen made %d getChat requests, want %d (the pipeline's own)", got, btoi(private))
				}
				if got := len(env.fake.callsFor("getChatMember")) - members; got != 1 {
					t.Errorf("the AI screen made %d getChatMember requests, want the pipeline's one", got)
				}

				// Every other rule's screen in a basic group still shows the basic-group line.
				if c.basic {
					text, _ = env.press(private, msgID, "a", "mo", "t", "un")
					if !strings.Contains(text, basicText) || strings.Contains(text, aiLine) {
						t.Errorf("the username screen %q must show the basic-group line and not the AI line", text)
					}
				}
			})
		}
	}

	t.Run("the status is the same with Redis down and with the switch off instead of the key", func(t *testing.T) {
		withAINotConfigured(t, mreqAISwitchOff)
		env := newSettingsMreqHereEnv(t)
		env.seed([8]int{7: models.RequirementActionMute}, false, false)
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		msgID := env.openMenuOf(false)
		text, _ := env.press(false, msgID, "a", "op", "p", settingsPageMreq)
		if !strings.Contains(text, settingsMreqAILine()) {
			t.Errorf("the page %q lacks the AI line with Redis down", text)
		}
		text, keyboard := env.press(false, msgID, "a", "mo", "t", "ai")
		if !strings.Contains(text, settingsMreqAIScreenHead(settingsMreqAILine(), models.RequirementActionMute)) || len(keyboard) != 2 {
			t.Errorf("the AI screen %q (%d rows) is not the Off-only form with Redis down", text, len(keyboard))
		}
	})

}

// btoi is 1 for true and 0 for false.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
