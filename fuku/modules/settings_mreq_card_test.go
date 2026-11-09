//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsMreqConfirmKeys are the contract's consequence lines of the card by strict action 2..4,
// and settingsMreqAIConfirmKeys the AI rule's own (amendment M19: the generic sentence says members
// already in the group are included, which is false for the AI rule).
var (
	settingsMreqConfirmKeys = map[int]string{
		models.RequirementActionKick: "settings_mreq_confirm_kick",
		models.RequirementActionBan:  "settings_mreq_confirm_ban",
		models.RequirementActionMute: "settings_mreq_confirm_mute",
	}
	settingsMreqAIConfirmKeys = map[int]string{
		models.RequirementActionKick: "settings_mreq_confirm_ai_kick",
		models.RequirementActionBan:  "settings_mreq_confirm_ai_ban",
		models.RequirementActionMute: "settings_mreq_confirm_ai_mute",
	}
)

// settingsMreqConfirmKeyOf is the consequence key of rule i's card for a strict action: the AI
// rule's (the last rule) own, the generic one for every other rule.
func settingsMreqConfirmKeyOf(i, action int) string {
	if settingsMreqRuleCodes[i] == "ai" {
		return settingsMreqAIConfirmKeys[action]
	}
	return settingsMreqConfirmKeys[action]
}

// settingsMreqWantCard fails the test unless the edit is the card of rule i set to action: the
// question with the rule's name and the plain option word, the consequence line, [Yes | No] and
// [Back | Close]. It returns the Yes and No buttons and the Back button.
func settingsMreqWantCard(t *testing.T, text string, keyboard [][]gotgbot.InlineKeyboardButton, i, action int) (yes, no, back gotgbot.InlineKeyboardButton) {
	t.Helper()
	markers := []string{
		staffMarker("settings_header") + " " + staffMarker("settings_page_mreq"),
		staffMarker("settings_mreq_confirm") + " " + staffMarker(settingsMreqRuleKeys[i]) + " " + staffMarker(settingsMreqWordKeys[action]),
		staffMarker(settingsMreqConfirmKeyOf(i, action)),
	}
	at := 0
	for _, marker := range markers {
		idx := strings.Index(text[at:], marker)
		if idx < 0 {
			t.Fatalf("the card %q lacks %q after offset %d", text, marker, at)
		}
		at += idx + len(marker)
	}
	for _, leak := range []string{"settings_mreq_choose_hint", "settings_mreq_now", "settings_mreq_intro"} {
		if strings.Contains(text, staffMarker(leak)) {
			t.Errorf("the card carries %s: %q", leak, text)
		}
	}
	// Only the rule's own consequence: no other rule's sentence, and never the generic one on the AI
	// card (it would promise that members already in the group are judged).
	for other, key := range settingsMreqConfirmKeys {
		if wrong := settingsMreqConfirmKeyOf(i, other); wrong != key && strings.Contains(text, staffMarker(key)) {
			t.Errorf("the %s card carries the generic %s: %q", settingsMreqRuleCodes[i], key, text)
		}
		if aiKey := settingsMreqAIConfirmKeys[other]; settingsMreqConfirmKeyOf(i, other) != aiKey && strings.Contains(text, staffMarker(aiKey)) {
			t.Errorf("the %s card carries the AI sentence %s: %q", settingsMreqRuleCodes[i], aiKey, text)
		}
	}
	if len(keyboard) != 2 || len(keyboard[0]) != 2 || len(keyboard[1]) != 2 {
		t.Fatalf("the card keyboard = %+v, want [Yes | No] and [Back | Close]", keyboard)
	}
	yes, no, back = keyboard[0][0], keyboard[0][1], keyboard[1][0]
	if !containsMarker(yes.Text, "settings_btn_yes") || !containsMarker(no.Text, "settings_btn_no") ||
		!containsMarker(back.Text, "settings_btn_back") || !containsMarker(keyboard[1][1].Text, "settings_btn_close") {
		t.Errorf("card buttons = %q %q %q %q, want Yes, No, Back, Close", yes.Text, no.Text, back.Text, keyboard[1][1].Text)
	}
	yesFields := settingsButtonFields(t, yes)
	if yesFields["a"] != "my" || yesFields["t"] != settingsMreqRuleCodes[i] || yesFields["v"] != fmt.Sprint(action) ||
		yesFields["e"] == "" || len(yesFields) != 5 {
		t.Errorf("Yes fields = %v, want exactly a=my, t=%s, v=%d, e and the owner", yesFields, settingsMreqRuleCodes[i], action)
	}
	if noFields := settingsButtonFields(t, no); noFields["a"] != "no" || noFields["p"] != settingsPageMreq || len(noFields) != 3 {
		t.Errorf("No fields = %v, want a=no and p=m", noFields)
	}
	return yes, no, back
}

// settingsMreqStoredActions is the eight stored punishments of chatID (all off when it has no row).
func settingsMreqStoredActions(t *testing.T, chatID int64) [8]int {
	t.Helper()
	stored, _ := settingsMreqStored(t, chatID)
	return settingsMreqActionsOf(stored)
}

// settingsMreqExpiryMinute is the e field of a card made now: the unix minute five minutes ahead.
func settingsMreqExpiryMinute() string {
	return fmt.Sprint(time.Now().Add(5*time.Minute).Unix() / 60)
}

func TestSettingsMreqCard(t *testing.T) {
	// The AI is configured: with it not configured a punishing AI press is refused and no card is
	// drawn, which TestSettingsMreqAIGate proves.
	withAIConfigured(t)
	env := newSettingsEnv(t)
	// The channel rule's punishing presses pass the live link check only in a linked group.
	settingsMreqLinkChannel(t, env)
	owner := env.newOwner(env.chat)
	writes := settingsMreqCountWrites(t)
	msgID := settingsMreqOpenPage(t, env, owner)
	choose := func(from gotgbot.User, code string, action int) {
		t.Helper()
		env.press(from, msgID, env.groupFields(from, "a", "st", "p", settingsPageMreq, "t", code, "v", fmt.Sprint(action)))
	}

	for i, code := range settingsMreqRuleCodes {
		for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
			t.Run(fmt.Sprintf("%s to %s asks first and Yes writes", code, settingsMreqWordKeys[action]), func(t *testing.T) {
				settingsMreqSeed(t, env.chat.Id, [8]int{}, false, false)
				before := *writes
				choose(owner, code, action)
				text, keyboard := env.lastEdit(env.chat.Id, msgID)
				yes, _, _ := settingsMreqWantCard(t, text, keyboard, i, action)
				if *writes != before || settingsMreqStoredActions(t, env.chat.Id) != [8]int{} {
					t.Fatalf("drawing the card wrote (%d writes, stored %v)", *writes-before, settingsMreqStoredActions(t, env.chat.Id))
				}

				env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
				var want [8]int
				want[i] = action
				if got := settingsMreqStoredActions(t, env.chat.Id); got != want {
					t.Fatalf("after Yes stored %v, want %v", got, want)
				}
				text, keyboard = env.lastEdit(env.chat.Id, msgID)
				settingsMreqWantRuleScreen(t, text, i, action)
				settingsMreqWantRuleKeyboard(t, keyboard, i, action)
				if strings.Contains(text, "⚠️") {
					t.Errorf("Yes redraw carries a notice: %q", text)
				}
			})
		}
	}

	t.Run("Off and Warn draw no card", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [8]int{models.RequirementActionMute}, false, false)
		for _, action := range []int{models.RequirementActionWarn, models.RequirementActionOff} {
			choose(owner, "ln", action)
			text, _ := env.lastEdit(env.chat.Id, msgID)
			if strings.Contains(text, staffMarker("settings_mreq_confirm")) {
				t.Errorf("value %d drew a card: %q", action, text)
			}
			if got := settingsMreqStoredActions(t, env.chat.Id)[0]; got != action {
				t.Errorf("value %d stored %d", action, got)
			}
		}
	})

	t.Run("the four exits are asymmetric", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [8]int{}, false, false)
		choose(owner, "ar", models.RequirementActionBan)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		yes, no, back := settingsMreqWantCard(t, text, keyboard, 3, models.RequirementActionBan)

		// No redraws the page, nothing written.
		env.pressRaw(env.chat, owner, msgID, no.CallbackData)
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantPage(t, text, [8]int{}, false, false)
		settingsMreqWantSwitchRow(t, keyboard, false, false)

		// An expired Yes redraws the rule screen with the expiry note and writes nothing.
		saved := settingsCardLifetime
		settingsCardLifetime = -2 * time.Minute
		t.Cleanup(func() { settingsCardLifetime = saved })
		choose(owner, "ar", models.RequirementActionBan)
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		expiredYes, _, _ := settingsMreqWantCard(t, text, keyboard, 3, models.RequirementActionBan)
		settingsCardLifetime = saved
		env.pressRaw(env.chat, owner, msgID, expiredYes.CallbackData)
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantRuleScreen(t, text, 3, 0)
		settingsMreqWantRuleKeyboard(t, keyboard, 3, 0)
		if want := settingsNoticeWarnMarker("settings_card_expired"); !strings.Contains(text, want) {
			t.Errorf("the expired Yes redraw %q lacks the notice %q", text, want)
		}
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{} {
			t.Errorf("an expired Yes wrote %v", got)
		}

		// Back opens Main.
		env.pressRaw(env.chat, owner, msgID, back.CallbackData)
		text, _ = env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_header_main")) || strings.Contains(text, staffMarker("settings_mreq_confirm")) {
			t.Errorf("the card's Back drew %q, want Main", text)
		}

		// A live Yes still works from the first card.
		env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
		if got := settingsMreqStoredActions(t, env.chat.Id); got[3] != models.RequirementActionBan {
			t.Errorf("a live Yes stored %v, want the Arabic rule on Ban", got)
		}
	})

	t.Run("a presser demoted between the two taps is refused at Yes", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [8]int{}, false, false)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
		helperMsg := settingsMreqOpenPage(t, env, helper)
		env.press(helper, helperMsg, env.groupFields(helper, "a", "st", "p", settingsPageMreq, "t", "zh", "v", "4"))
		text, keyboard := env.lastEdit(env.chat.Id, helperMsg)
		yes, _, _ := settingsMreqWantCard(t, text, keyboard, 4, models.RequirementActionMute)

		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: false})
		edits, answers := len(env.edits(env.chat.Id, helperMsg)), env.answerCount()
		env.pressRaw(env.chat, helper, helperMsg, yes.CallbackData)
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_no_restrict")) {
			t.Errorf("answer = %q (alert %v), want the settings_no_restrict alert", answer, alert)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("Yes answered %d times, want once", got)
		}
		if got := len(env.edits(env.chat.Id, helperMsg)); got != edits {
			t.Error("a refused Yes edited the card")
		}
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{} {
			t.Errorf("a refused Yes wrote %v", got)
		}
	})

	t.Run("a Yes whose write fails redraws the rule screen with the save-failed notice", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [8]int{}, false, false)
		choose(owner, "un", models.RequirementActionKick)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		yes, _, _ := settingsMreqWantCard(t, text, keyboard, 1, models.RequirementActionKick)
		settingsMreqSeams(t)
		memberReqWrites.Action = func(context.Context, int64, memberreq.Rule, int) error { return errors.New("write failed") }
		env.pressRaw(env.chat, owner, msgID, yes.CallbackData)
		text, keyboard = env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantRuleScreen(t, text, 1, 0)
		settingsMreqWantRuleKeyboard(t, keyboard, 1, 0)
		if want := settingsNoticeWarnMarker("common_settings_save_failed"); !strings.Contains(text, want) {
			t.Errorf("the redraw %q lacks the notice %q", text, want)
		}
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{} {
			t.Errorf("a failed write changed the rules to %v", got)
		}
	})

	t.Run("same value is judged by a fresh read, never the cache", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [8]int{}, false, false)
		if _, err := memberreq.GetSettings(context.Background(), env.chat.Id); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		// Behind the cache: the row is now Mute, the cached copy still says Off.
		if err := db.DB.Model(&models.ChatRequirements{}).Where("chat_id = ?", env.chat.Id).
			Update("last_name_action", models.RequirementActionMute).Error; err != nil {
			t.Fatalf("change the row behind the cache: %v", err)
		}
		t.Cleanup(func() {
			dbcache.DeleteCache(dbcache.CacheKey("member_requirements", env.chat.Id))
			dbcache.ResetLocalForTest()
		})
		if cached, err := memberreq.GetSettings(context.Background(), env.chat.Id); err != nil || cached.LastNameAction != models.RequirementActionOff {
			t.Fatalf("the cache = %+v, %v, want the stale Off this test depends on", cached, err)
		}

		before := *writes
		choose(owner, "ln", models.RequirementActionMute)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if strings.Contains(text, staffMarker("settings_mreq_confirm")) || *writes != before {
			t.Errorf("a press of the stored Mute drew a card or wrote (%d writes): %q", *writes-before, text)
		}
		choose(owner, "ln", models.RequirementActionBan)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		settingsMreqWantCard(t, text, keyboard, 0, models.RequirementActionBan)

		t.Run("an unreadable fresh row draws no card and writes nothing", func(t *testing.T) {
			saved := settingsMreqFresh
			t.Cleanup(func() { settingsMreqFresh = saved })
			settingsMreqFresh = func(context.Context, int64) (*models.ChatRequirements, error) {
				return nil, errors.New("fresh read failed")
			}
			before := *writes
			choose(owner, "ln", models.RequirementActionKick)
			text, _ := env.lastEdit(env.chat.Id, msgID)
			if strings.Contains(text, staffMarker("settings_mreq_confirm")) || *writes != before {
				t.Errorf("a failed fresh read drew a card or wrote (%d writes): %q", *writes-before, text)
			}
			if want := settingsNoticeWarnMarker("common_settings_save_failed"); !strings.Contains(text, want) {
				t.Errorf("the redraw %q lacks the notice %q", text, want)
			}
		})
	})

	t.Run("a forged my is refused before any lookup", func(t *testing.T) {
		settingsMreqSeed(t, env.chat.Id, [8]int{}, false, false)
		e := settingsMreqExpiryMinute()
		for name, fields := range map[string]map[string]string{
			"v=1":           env.groupFields(owner, "a", "my", "t", "ln", "v", "1", "e", e),
			"v=0":           env.groupFields(owner, "a", "my", "t", "ln", "v", "0", "e", e),
			"v=5":           env.groupFields(owner, "a", "my", "t", "ln", "v", "5", "e", e),
			"v=04":          env.groupFields(owner, "a", "my", "t", "ln", "v", "04", "e", e),
			"an empty e":    env.groupFields(owner, "a", "my", "t", "ln", "v", "4", "e", ""),
			"a bad e":       env.groupFields(owner, "a", "my", "t", "ln", "v", "4", "e", "12x"),
			"a zero e":      env.groupFields(owner, "a", "my", "t", "ln", "v", "4", "e", "0"),
			"no e":          env.groupFields(owner, "a", "my", "t", "ln", "v", "4"),
			"no t":          env.groupFields(owner, "a", "my", "v", "4", "e", e),
			"an unknown t":  env.groupFields(owner, "a", "my", "t", "xx", "v", "4", "e", e),
			"no v":          env.groupFields(owner, "a", "my", "t", "ln", "e", e),
			"an extra page": env.groupFields(owner, "a", "my", "t", "ln", "v", "4", "e", e, "p", settingsPageMreq),
		} {
			lookups, edits, answers := env.memberLookupsOf(env.chat.Id, owner.Id), len(env.edits(env.chat.Id, msgID)), env.answerCount()
			env.press(owner, msgID, fields)
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("%s answered %q (alert %v), want the invalid request alert", name, answer, alert)
			}
			if got := env.answerCount() - answers; got != 1 {
				t.Errorf("%s answered %d times, want once", name, got)
			}
			if got := env.memberLookupsOf(env.chat.Id, owner.Id); got != lookups {
				t.Errorf("%s made %d getChatMember calls, want none", name, got-lookups)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("%s edited the message", name)
			}
			if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{} {
				t.Errorf("%s wrote %v", name, got)
			}
		}
	})
}
