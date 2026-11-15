//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
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

// settingsMreqAIStrictKV is the st press of the AI rule with the absolute value v.
func settingsMreqAIStrictKV(v int) []string {
	return []string{"a", "st", "p", settingsPageMreq, "t", "ai", "v", strconv.Itoa(v)}
}

// settingsMreqAIYesData is the data of the AI card's Yes for strict action v with the expiry minute
// e, in the given menu kind.
func settingsMreqAIYesData(env *settingsMreqHereEnv, private bool, v int, e string) string {
	env.t.Helper()
	kv := []string{"a", "my", "t", "ai", "v", strconv.Itoa(v), "e", e}
	fields := env.groupFields(env.owner, kv...)
	if private {
		fields = env.privateFields(env.chat.Id, kv...)
	}
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		env.t.Fatalf("the Yes data %v did not encode", kv)
	}
	return data
}

// settingsMreqAIGateEnv is a requirements environment with the rule write counted, the Anthropic
// stand-in installed (so a request to it is seen) and every rule Off.
type settingsMreqAIGateEnv struct {
	*settingsMreqHereEnv
	writes *int
	ai     *mreqAIState
}

func newSettingsMreqAIGateEnv(t *testing.T, private, basic bool) *settingsMreqAIGateEnv {
	t.Helper()
	writes := settingsMreqCountWrites(t)
	ai := mreqInstallAIFake(t)
	env := newSettingsMreqHereEnv(t)
	if basic {
		env = settingsMreqBasicEnv(t, private)
	}
	env.seed([8]int{}, false, false)
	return &settingsMreqAIGateEnv{settingsMreqHereEnv: env, writes: writes, ai: ai}
}

// chatOf is the chat the menu's message is in.
func (e *settingsMreqAIGateEnv) chatOf(private bool) int64 {
	return settingsMreqMenuChat(e.settingsMreqHereEnv, private)
}

// wantRefused fails the test unless the press was refused as "AI not configured": one alert of
// settings_mreq_here_ai with the rule's name and no icon, one edit that is the Off-only screen with
// no notice and no card, nothing written and no request to the provider, and the stored punishment
// still stored.
func (e *settingsMreqAIGateEnv) wantRefused(t *testing.T, private bool, msgID int64, got settingsMreqPressed, stored int) {
	t.Helper()
	wantAnswer := staffMarker("settings_mreq_here_ai") + " " + staffMarker("settings_mreq_rule_ai")
	if got.Answers != 1 || !got.Alert || got.Answer != wantAnswer {
		t.Errorf("answers = %d, last = %q (alert %v), want one alert of %q", got.Answers, got.Answer, got.Alert, wantAnswer)
	}
	if got.Edits != 1 {
		t.Fatalf("the press edited the message %d times, want once (the Off-only screen)", got.Edits)
	}
	text, keyboard := e.lastEdit(e.chatOf(private), msgID)
	if want := settingsMreqAIScreenHead(settingsMreqAILine(), stored); !strings.Contains(text, want) {
		t.Errorf("the redraw %q lacks %q", text, want)
	}
	for _, leak := range []string{"settings_mreq_confirm", "settings_mreq_choose_hint", "settings_mreq_actions_hint", "settings_card_expired", "common_settings_save_failed"} {
		if strings.Contains(text, staffMarker(leak)) {
			t.Errorf("the redraw %q carries %s", text, leak)
		}
	}
	if strings.Contains(text, "⚠️") {
		t.Errorf("the redraw %q carries a notice", text)
	}
	if rows := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(rows, []int{1, 2}) {
		t.Errorf("rows = %v, want the Off-only [1 2]", rows)
	}
	if *e.writes != 0 {
		t.Errorf("the refused press made %d writes", *e.writes)
	}
	if n := e.ai.count(); n != 0 {
		t.Errorf("the refused press made %d provider requests", n)
	}
	if actions := settingsMreqStoredActions(t, e.chat.Id); actions != ([8]int{7: stored}) {
		t.Errorf("stored %v after a refused press, want the AI rule still on %d", actions, stored)
	}
}

func TestSettingsMreqAIGate(t *testing.T) {
	strict := []int{models.RequirementActionWarn, models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute}

	settingsMreqRefusalMenus(t, func(t *testing.T, private bool) {
		t.Run("a punishing st with the AI not configured is refused with nothing written", func(t *testing.T) {
			for _, how := range []mreqAIOff{mreqAINoKey, mreqAISwitchOff} {
				for _, stored := range []int{models.RequirementActionOff, models.RequirementActionBan} {
					for _, v := range strict {
						if v == stored {
							continue
						}
						withAINotConfigured(t, how)
						env := newSettingsMreqAIGateEnv(t, private, false)
						if stored != models.RequirementActionOff {
							env.seed([8]int{7: stored}, false, false)
						}
						msgID := env.openMenuOf(private)
						members, chats := len(env.fake.callsFor("getChatMember")), mreqChatCalls(env.memberReqEnv, env.chat.Id)
						got := env.strictPress(private, msgID, settingsMreqAIStrictKV(v)...)
						env.wantRefused(t, private, msgID, got, stored)
						if n := len(env.fake.callsFor("getChatMember")) - members; n != 1 {
							t.Errorf("v=%d: %d getChatMember requests, want the pipeline's one", v, n)
						}
						if n := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; n != btoi(private) {
							t.Errorf("v=%d: %d getChat requests, want %d (the pipeline's own)", v, n, btoi(private))
						}
					}
				}
			}
		})

		t.Run("the card's Yes with the AI not configured is refused and replaces the card", func(t *testing.T) {
			for _, v := range strict[1:] {
				withAIConfigured(t)
				env := newSettingsMreqAIGateEnv(t, private, false)
				msgID := env.openMenuOf(private)
				got := env.strictPress(private, msgID, settingsMreqAIStrictKV(v)...)
				if got.Answers != 1 || got.Alert || got.Edits != 1 {
					t.Fatalf("v=%d: the card press = %+v, want one empty answer and one edit", v, got)
				}
				text, keyboard := env.lastEdit(env.chatOf(private), msgID)
				yes, _, _ := settingsMreqWantCard(t, text, keyboard, 7, v)

				withAINotConfigured(t, mreqAINoKey) // the key was removed since the card was drawn
				got = env.strictPressData(private, msgID, yes.CallbackData)
				env.wantRefused(t, private, msgID, got, 0)
			}
		})

		t.Run("a forged Yes is judged like a real one", func(t *testing.T) {
			withAINotConfigured(t, mreqAINoKey)
			env := newSettingsMreqAIGateEnv(t, private, false)
			msgID := env.openMenuOf(private)
			got := env.strictPressData(private, msgID, settingsMreqAIYesData(env.settingsMreqHereEnv, private, models.RequirementActionMute, settingsMreqExpiryMinute()))
			env.wantRefused(t, private, msgID, got, 0)
		})

		t.Run("Off writes at once and a press of the stored value makes no check", func(t *testing.T) {
			withAINotConfigured(t, mreqAINoKey)
			env := newSettingsMreqAIGateEnv(t, private, false)
			env.seed([8]int{7: models.RequirementActionBan}, false, false)
			msgID := env.openMenuOf(private)

			got := env.strictPress(private, msgID, settingsMreqAIStrictKV(models.RequirementActionBan)...)
			if got.Answers != 1 || got.Alert || got.Edits != 1 || *env.writes != 0 {
				t.Errorf("a press of the stored value: %+v, %d writes, want one empty answer, one edit and no write", got, *env.writes)
			}
			text, _ := env.lastEdit(env.chatOf(private), msgID)
			if strings.Contains(text, staffMarker("settings_mreq_confirm")) || !strings.Contains(text, settingsMreqAIScreenHead(settingsMreqAILine(), models.RequirementActionBan)) {
				t.Errorf("the stored value's redraw %q is not the Off-only screen with Ban shown", text)
			}

			got = env.strictPress(private, msgID, settingsMreqAIStrictKV(models.RequirementActionOff)...)
			if got.Answers != 1 || got.Alert || got.Edits != 1 || *env.writes != 1 {
				t.Errorf("Off: %+v, %d writes, want one empty answer, one edit and one write", got, *env.writes)
			}
			if actions := settingsMreqStoredActions(t, env.chat.Id); actions != [8]int{} {
				t.Errorf("after Off stored %v, want the stored Ban cleared", actions)
			}
			text, keyboard := env.lastEdit(env.chatOf(private), msgID)
			if !strings.Contains(text, settingsMreqAIScreenHead(settingsMreqAILine(), models.RequirementActionOff)) || !strings.HasPrefix(keyboard[0][0].Text, "✅") {
				t.Errorf("the screen after Off %q (%q) is not the Off-only form with Off marked", text, keyboard[0][0].Text)
			}
		})

		t.Run("an expired Yes redraws with the expiry note and makes no check", func(t *testing.T) {
			withAINotConfigured(t, mreqAINoKey)
			env := newSettingsMreqAIGateEnv(t, private, false)
			msgID := env.openMenuOf(private)
			got := env.strictPressData(private, msgID, settingsMreqAIYesData(env.settingsMreqHereEnv, private, models.RequirementActionMute, "1"))
			if got.Answers != 1 || got.Alert || got.Edits != 1 || *env.writes != 0 {
				t.Fatalf("an expired Yes: %+v, %d writes, want one empty answer, one edit and no write", got, *env.writes)
			}
			text, _ := env.lastEdit(env.chatOf(private), msgID)
			if !strings.Contains(text, staffMarker("settings_card_expired")) {
				t.Errorf("the expired Yes %q lacks the expiry note", text)
			}
		})

		t.Run("a presser demoted before the press is refused with settings_no_restrict before the check", func(t *testing.T) {
			withAINotConfigured(t, mreqAINoKey)
			env := newSettingsMreqAIGateEnv(t, private, false)
			env.fake.setMember(env.chat.Id, env.owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: false})
			msgID := env.openMenuOf(private)
			presses := map[string]func() settingsMreqPressed{
				"st": func() settingsMreqPressed {
					return env.strictPress(private, msgID, settingsMreqAIStrictKV(models.RequirementActionMute)...)
				},
				"my": func() settingsMreqPressed {
					return env.strictPressData(private, msgID, settingsMreqAIYesData(env.settingsMreqHereEnv, private, models.RequirementActionMute, settingsMreqExpiryMinute()))
				},
			}
			for name, press := range presses {
				got := press()
				if got.Answers != 1 || !got.Alert || got.Answer != staffMarker("settings_no_restrict") || got.Edits != 0 {
					t.Errorf("%s: %+v, want one settings_no_restrict alert and no edit", name, got)
				}
			}
			if *env.writes != 0 {
				t.Errorf("a refused press wrote %d times", *env.writes)
			}
		})

		t.Run("the gate judges the AI rule only", func(t *testing.T) {
			withAINotConfigured(t, mreqAINoKey)
			env := newSettingsMreqAIGateEnv(t, private, false)
			mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			msgID := env.openMenuOf(private)
			for i, code := range settingsMreqRuleCodes[:7] {
				got := env.strictPress(private, msgID, "a", "st", "p", settingsPageMreq, "t", code, "v", strconv.Itoa(models.RequirementActionMute))
				if got.Answers != 1 || got.Alert || got.Edits != 1 {
					t.Fatalf("rule %s: %+v, want one empty answer and one edit (a card)", code, got)
				}
				text, keyboard := env.lastEdit(env.chatOf(private), msgID)
				settingsMreqWantCard(t, text, keyboard, i, models.RequirementActionMute)
			}
			got := env.strictPress(private, msgID, "a", "st", "p", settingsPageMreq, "t", "ph", "v", strconv.Itoa(models.RequirementActionWarn))
			if got.Alert || settingsMreqStoredActions(t, env.chat.Id) != [8]int{5: models.RequirementActionWarn} {
				t.Errorf("Warn on the photo rule with the AI not configured: %+v, stored %v", got, settingsMreqStoredActions(t, env.chat.Id))
			}
		})

		t.Run("a screen drawn while configured is repaired by the first refused press", func(t *testing.T) {
			withAIConfigured(t)
			env := newSettingsMreqAIGateEnv(t, private, false)
			msgID := env.openMenuOf(private)
			text, keyboard := env.press(private, msgID, "a", "mo", "t", "ai")
			if rows := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(rows, []int{2, 3, 2}) || strings.Contains(text, "🚫") {
				t.Fatalf("the configured screen %q has rows %v, want the three-row form", text, rows)
			}
			withAINotConfigured(t, mreqAISwitchOff)
			got := env.strictPress(private, msgID, settingsMreqAIStrictKV(models.RequirementActionWarn)...)
			env.wantRefused(t, private, msgID, got, 0)
		})

		t.Run("a screen drawn Off-only still takes the single Off once the AI is configured, then shows three rows", func(t *testing.T) {
			withAINotConfigured(t, mreqAINoKey)
			env := newSettingsMreqAIGateEnv(t, private, false)
			env.seed([8]int{7: models.RequirementActionMute}, false, false)
			msgID := env.openMenuOf(private)
			_, keyboard := env.press(private, msgID, "a", "mo", "t", "ai")
			if rows := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(rows, []int{1, 2}) {
				t.Fatalf("the Off-only screen rows = %v", rows)
			}
			withAIConfigured(t)
			got := env.strictPress(private, msgID, settingsMreqAIStrictKV(models.RequirementActionOff)...)
			if got.Answers != 1 || got.Alert || *env.writes != 1 {
				t.Errorf("the single Off: %+v, %d writes, want one empty answer and one write", got, *env.writes)
			}
			text, keyboard := env.lastEdit(env.chatOf(private), msgID)
			if rows := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(rows, []int{2, 3, 2}) || strings.Contains(text, "🚫") {
				t.Errorf("the next draw %q has rows %v, want the three-row form", text, rows)
			}
		})

		for _, basic := range []bool{false, true} {
			group := map[bool]string{false: "supergroup", true: "basic group"}[basic]
			t.Run("with the AI configured in a "+group+" Warn writes, Kick, Ban and Mute ask first and Yes writes", func(t *testing.T) {
				withAIConfigured(t)
				env := newSettingsMreqAIGateEnv(t, private, basic)
				msgID := env.openMenuOf(private)
				got := env.strictPress(private, msgID, settingsMreqAIStrictKV(models.RequirementActionWarn)...)
				if got.Answers != 1 || got.Alert || *env.writes != 1 {
					t.Fatalf("Warn: %+v, %d writes, want one empty answer and one write", got, *env.writes)
				}
				if actions := settingsMreqStoredActions(t, env.chat.Id); actions != [8]int{7: models.RequirementActionWarn} {
					t.Errorf("after Warn stored %v", actions)
				}
				for _, action := range strict[1:] {
					env.seed([8]int{}, false, false)
					before := *env.writes
					got = env.strictPress(private, msgID, settingsMreqAIStrictKV(action)...)
					if got.Answers != 1 || got.Alert || got.Edits != 1 {
						t.Fatalf("%s: %+v, want one empty answer and one edit (the card)", settingsMreqWordKeys[action], got)
					}
					text, keyboard := env.lastEdit(env.chatOf(private), msgID)
					yes, _, _ := settingsMreqWantCard(t, text, keyboard, 7, action)
					if *env.writes != before {
						t.Fatalf("drawing the card wrote")
					}
					got = env.strictPressData(private, msgID, yes.CallbackData)
					if got.Answers != 1 || got.Alert {
						t.Errorf("Yes: %+v, want one empty answer", got)
					}
					if actions := settingsMreqStoredActions(t, env.chat.Id); actions != [8]int{7: action} {
						t.Errorf("after Yes stored %v, want the AI rule on %s", actions, settingsMreqWordKeys[action])
					}
				}
				if n := env.ai.count(); n != 0 {
					t.Errorf("the menu made %d provider requests", n)
				}
			})
		}
	})
}
