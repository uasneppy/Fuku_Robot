//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// mreqMutedMember mutes a member without a last name through the real message watcher (the
// last name rule set to Mute) and returns the member, their hold and the mute notice. The
// environment's dispatcher has the requirements, warns and rules modules loaded.
func mreqMutedMember(t *testing.T, env *memberReqEnv) (gotgbot.User, *models.ChatRequirementHold, staffSentMessage) {
	t.Helper()
	env.loadMemberReq()
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
	member := env.mreqMember("Alex", "", "alex")
	notice := env.mreqMuteByMessage(member)
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil {
		t.Fatalf("hold after the mute = %+v, want a punished mute hold", hold)
	}
	if !isMemberReqMute(mreqLiveRecord(t, env, member.Id), hold.MuteUntil) {
		t.Fatal("the member's live record does not carry the requirement marker after the mute")
	}
	return member, hold, notice
}

// translator is the translator of the group's language.
func (e *memberReqEnv) translator() *i18n.Translator {
	return memberReqTranslator(context.Background(), e.chat.Id)
}

// mreqFix gives the member the last name the rule asked for.
func (e *memberReqEnv) mreqFix(member gotgbot.User) gotgbot.User {
	e.t.Helper()
	member.LastName = "Smith"
	e.fake.setProfile(member.Id, member.FirstName, member.LastName, member.Username)
	return member
}

// mreqNoticeEdits returns the text edits of message msgID of the group.
func (e *memberReqEnv) mreqNoticeEdits(msgID int64) []moduleBotCall {
	e.t.Helper()
	return e.edits(e.chat.Id, msgID)
}

// mreqWantFixedUnmute fails unless the press unmuted member with wantPerms (and no end date),
// answered once with wantAnswer as a toast or an alert, deleted the hold and edited the
// notice to wantNotice, with no keyboard.
func mreqWantFixedUnmute(t *testing.T, env *memberReqEnv, member gotgbot.User, notice staffSentMessage, restrictsBefore, answersBefore int,
	wantPerms gotgbot.ChatPermissions, wantAnswer mreqAnswer, wantNotice string) {
	t.Helper()
	restricts := env.mreqRestricts(member.Id)
	if len(restricts) != restrictsBefore+1 {
		t.Fatalf("restrictChatMember calls for the member = %d, want %d", len(restricts), restrictsBefore+1)
	}
	last := restricts[len(restricts)-1]
	if perms, ok := last.Params["permissions"].(gotgbot.ChatPermissions); !ok || !reflect.DeepEqual(perms, wantPerms) {
		t.Errorf("restored permissions = %+v, want %+v", last.Params["permissions"], wantPerms)
	}
	if _, set := last.Params["until_date"]; set {
		t.Errorf("the unmute carries until_date %v, want none", last.Params["until_date"])
	}
	if hold := env.mreqHold(member.Id); hold != nil {
		t.Errorf("hold after the unmute = %+v, want none", hold)
	}
	if got := env.mreqOnlyAnswer(answersBefore); got != wantAnswer {
		t.Errorf("answer = %+v, want %+v", got, wantAnswer)
	}
	text, keyboard := env.lastEdit(env.chat.Id, notice.MessageID)
	if text != wantNotice {
		t.Errorf("edited notice = %q, want %q", text, wantNotice)
	}
	if len(keyboard) != 0 {
		t.Errorf("edited notice keyboard = %+v, want none", keyboard)
	}
}

// mreqFixedNoticeText is the edited notice of a fix, in the translator's language, with the
// member's mention spliced in after translation.
func mreqFixedNoticeText(t *testing.T, tr *i18n.Translator, key string, member gotgbot.User) string {
	t.Helper()
	text, err := tr.GetString(key, i18n.TranslationParams{"name": memberReqNamePlaceholder})
	if err != nil {
		t.Fatalf("translate %s: %v", key, err)
	}
	return lockdownSplice(text, memberReqNamePlaceholder, memberReqMention(member, ""))
}

func TestMemberReqFixedUnmutes(t *testing.T) {
	t.Run("the group's live defaults, a toast and the notice edited", func(t *testing.T) {
		env := newMemberReqEnv(t)
		live := lockdownSnapshotPermissions(t)
		member, _, notice := mreqMutedMember(t, env)
		member = env.mreqFix(member)
		sentBefore := len(env.mreqNotices())
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, live,
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if n := len(env.mreqNotices()); n != sentBefore {
			t.Errorf("messages sent to the group = %d, want the %d before the press (a message hold sends no welcome)", n, sentBefore)
		}
		if got := mreqLiveRecord(t, env, member.Id); got.Status != gotgbot.ChatMemberStatusRestricted || got.UntilDate != 0 {
			t.Errorf("the member after the fix = %+v, want the unmuted record with no end date", got)
		}
	})

	t.Run("group language uk", func(t *testing.T) {
		env := newMemberReqEnvWith(t, settingsEnvOptions{RealLocale: true})
		if err := lang.ChangeGroupLanguage(env.chat.Id, "uk"); err != nil {
			t.Fatal(err)
		}
		member, _, notice := mreqMutedMember(t, env)
		member = env.mreqFix(member)
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		uk := i18n.MustNewTranslator("uk")
		toast, err := uk.GetString("mreq_fixed_done")
		if err != nil {
			t.Fatal(err)
		}
		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: toast}, mreqFixedNoticeText(t, uk, "mreq_notice_fixed", member))
	})
}

func TestMemberReqFixedBeforeNoticeStored(t *testing.T) {
	for name, stored := range map[string]int64{"notice id never stored": 0, "notice id names another message": 987654} {
		t.Run(name, func(t *testing.T) {
			env := newMemberReqEnv(t)
			member, hold, notice := mreqMutedMember(t, env)
			if err := db.DB.Model(&models.ChatRequirementHold{}).Where("id = ?", hold.ID).
				Update("notice_msg_id", stored).Error; err != nil {
				t.Fatal(err)
			}
			member = env.mreqFix(member)
			restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

			env.mreqPressFixed(member, notice)

			mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
				mreqAnswer{Text: staffMarker("mreq_fixed_done")},
				mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
			if edits := env.mreqNoticeEdits(stored); stored != 0 && len(edits) != 0 {
				t.Errorf("edits of the stored notice id %d = %d, want none: the pressed message is the one edited", stored, len(edits))
			}
		})
	}
}

// mreqFixedButtonData is the callback data of an "I fixed it" button for the given hold and
// member, built as the notice builds it.
func mreqFixedButtonData(t *testing.T, holdID, userID int64) string {
	t.Helper()
	data := encodeCallbackData(memberReqCallbackNamespace, map[string]string{
		"a": "f", "h": strconv.FormatInt(holdID, 10), "u": strconv.FormatInt(userID, 10),
	})
	if data == "" {
		t.Fatalf("button data for hold %d of user %d did not encode", holdID, userID)
	}
	return data
}

// mreqSeedHold stores a punished mute hold for the member in the chat, as the executor would
// have, and returns it as the database holds it.
func mreqSeedHold(t *testing.T, chatID, userID, muteUntil int64) *models.ChatRequirementHold {
	t.Helper()
	now := time.Now()
	won, err := memberreq.ClaimHold(context.Background(), &models.ChatRequirementHold{
		ChatID: chatID, UserID: userID, Rules: 1, Action: models.RequirementActionMute,
		MuteUntil: muteUntil, CreatedAt: now, PunishedAt: &now,
	})
	if err != nil || !won {
		t.Fatalf("seed hold of user %d in chat %d = %v, %v", userID, chatID, won, err)
	}
	hold, err := memberreq.GetHoldFresh(context.Background(), chatID, userID)
	if err != nil || hold == nil {
		t.Fatalf("read the seeded hold: %v, %v", hold, err)
	}
	return hold
}

// mreqPressRefused presses data on message msgID as from and fails unless the press answered
// exactly once, as an alert, with the text of key (and every extra marker in extras), changed
// no mute (no restrictChatMember call about member) and edited no message.
func mreqPressRefused(t *testing.T, env *memberReqEnv, from, member gotgbot.User, msgID int64, data, key string, extras ...string) {
	t.Helper()
	restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())
	editsBefore := len(callsToChat(env.fake.staffBotClient, "editMessageText", env.chat.Id))

	env.mreqPressData(from, msgID, data)

	got := env.mreqOnlyAnswer(answersBefore)
	if !got.Alert {
		t.Errorf("answer %q is a toast, want an alert", got.Text)
	}
	if len(extras) == 0 {
		if got.Text != staffMarker(key) {
			t.Errorf("answer = %q, want %q", got.Text, staffMarker(key))
		}
	} else if !strings.Contains(got.Text, staffMarker(key)) {
		t.Errorf("answer = %q, want it to hold %q", got.Text, staffMarker(key))
	}
	for _, extra := range extras {
		if !strings.Contains(got.Text, extra) {
			t.Errorf("answer = %q, want it to hold %q", got.Text, extra)
		}
	}
	if n := len(env.mreqRestricts(member.Id)); n != restrictsBefore {
		t.Errorf("restrictChatMember calls about the member = %d, want the %d before the press", n, restrictsBefore)
	}
	if n := len(callsToChat(env.fake.staffBotClient, "editMessageText", env.chat.Id)); n != editsBefore {
		t.Errorf("edits after the press = %d, want the %d before it: only a success edits the notice", n, editsBefore)
	}
}

// mreqWantHeld fails unless the member's hold is still the one given, row and all.
func mreqWantHeld(t *testing.T, env *memberReqEnv, want *models.ChatRequirementHold) {
	t.Helper()
	got := env.mreqHold(want.UserID)
	if got == nil || got.ID != want.ID || got.NoticeMsgID != want.NoticeMsgID || got.MuteUntil != want.MuteUntil ||
		got.Rules != want.Rules || got.Action != want.Action || got.WelcomeHeld != want.WelcomeHeld || got.PunishedAt == nil {
		t.Errorf("hold = %+v, want the unchanged %+v", got, want)
	}
}

func TestMemberReqFixedRefusals(t *testing.T) {
	t.Run("malformed data is an invalid request before any read", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		id, user := strconv.FormatInt(hold.ID, 10), strconv.FormatInt(member.Id, 10)
		cases := map[string]map[string]string{
			"missing h":   {"a": "f", "u": user},
			"missing u":   {"a": "f", "h": id},
			"missing a":   {"h": id, "u": user},
			"wrong a":     {"a": "x", "h": id, "u": user},
			"zero h":      {"a": "f", "h": "0", "u": user},
			"zero u":      {"a": "f", "h": id, "u": "0"},
			"signed h":    {"a": "f", "h": "-" + id, "u": user},
			"signed u":    {"a": "f", "h": id, "u": "-" + user},
			"plus u":      {"a": "f", "h": id, "u": "+" + user},
			"20-digit h":  {"a": "f", "h": "12345678901234567890", "u": user},
			"20-digit u":  {"a": "f", "h": id, "u": "12345678901234567890"},
			"extra key":   {"a": "f", "h": id, "u": user, "x": "1"},
			"non-numeric": {"a": "f", "h": "abc", "u": user},
		}
		for name, fields := range cases {
			data := encodeCallbackData(memberReqCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("%s: data did not encode", name)
			}
			lookups := env.mreqCallsAbout(member.Id, "getChatMember")
			mreqPressRefused(t, env, member, member, notice.MessageID, data, "common_callback_invalid_request")
			if n := env.mreqCallsAbout(member.Id, "getChatMember"); n != lookups {
				t.Errorf("%s: getChatMember calls = %d, want none after %d", name, n-lookups, lookups)
			}
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("no message and a private chat are invalid", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		data := env.mreqFixedData(notice)
		answersBefore := len(env.mreqAnswers())
		env.pressNoMessage(member, data)
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != staffMarker("common_callback_invalid_request") || !got.Alert {
			t.Errorf("answer without a message = %+v, want the invalid request alert", got)
		}
		answersBefore = len(env.mreqAnswers())
		env.pressRaw(gotgbot.Chat{Id: member.Id, Type: "private"}, member, notice.MessageID, data)
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != staffMarker("common_callback_invalid_request") || !got.Alert {
			t.Errorf("answer in a private chat = %+v, want the invalid request alert", got)
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("another member and an admin are refused before any read", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		other := env.mreqMember("Bo", "Li", "bo")
		member = env.mreqFix(member)
		data := env.mreqFixedData(notice)
		// With the hold gone a press that read it would answer stale: not_yours proves no read.
		if _, taken, err := memberreq.TakeHoldByID(context.Background(), hold.ID, env.chat.Id, member.Id); err != nil || !taken {
			t.Fatalf("take the hold: %v, %v", taken, err)
		}
		for name, presser := range map[string]gotgbot.User{"another member": other, "an admin": env.admin} {
			lookups := env.mreqCallsAbout(member.Id, "getChatMember")
			mreqPressRefused(t, env, presser, member, notice.MessageID, data, "mreq_fixed_not_yours")
			if n := env.mreqCallsAbout(member.Id, "getChatMember"); n != lookups {
				t.Errorf("%s: getChatMember calls about the member = %d, want none", name, n-lookups)
			}
		}
	})

	t.Run("a hold of another chat, of another member or already taken is stale", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		member = env.mreqFix(member)
		otherChat := uniqueModuleChatID()
		memberReqCleanup(t, []int64{otherChat}, nil)
		elsewhere := mreqSeedHold(t, otherChat, member.Id, hold.MuteUntil)
		bystander := env.mreqMember("Bo", "Li", "bo")
		theirs := mreqSeedHold(t, env.chat.Id, bystander.Id, hold.MuteUntil)

		mreqPressRefused(t, env, member, member, notice.MessageID, mreqFixedButtonData(t, elsewhere.ID, member.Id), "mreq_fixed_stale")
		mreqPressRefused(t, env, member, member, notice.MessageID, mreqFixedButtonData(t, theirs.ID, member.Id), "mreq_fixed_stale")
		mreqPressRefused(t, env, member, member, notice.MessageID, mreqFixedButtonData(t, hold.ID+1000000, member.Id), "mreq_fixed_stale")
		mreqWantHeld(t, env, hold)
		mreqWantHeld(t, env, theirs)
		if got, err := memberreq.GetHoldFresh(context.Background(), otherChat, member.Id); err != nil || got == nil {
			t.Errorf("the hold of the other chat = %+v, %v, want it untouched", got, err)
		}

		// Pressed once for real, the same button is stale on the second press.
		env.mreqPressFixed(member, notice)
		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_stale")
	})

	t.Run("a getChatMember error cannot be checked", func(t *testing.T) {
		errs := map[string][]error{
			"400":     {staffFakeError(400, "Bad Request: chat not found")},
			"429":     {staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1)},
			"502":     {staffFakeError(502, "Bad Gateway")},
			"timeout": {context.DeadlineExceeded},
		}
		for name, scripted := range errs {
			t.Run(name, func(t *testing.T) {
				env := newMemberReqEnv(t)
				member, hold, notice := mreqMutedMember(t, env)
				member = env.mreqFix(member)
				env.fake.script("getChatMember", env.chat.Id, scripted...)

				mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")

				mreqWantHeld(t, env, hold)
			})
		}
	})

	t.Run("a profile still without a last name is still broken", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		tr := env.translator()

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken",
			memberReqRulesText(tr, memberReqRules[0].Bit))

		mreqWantHeld(t, env, hold)
		if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
			t.Errorf("the member after the refused press = %+v, want still muted with the marker", live)
		}
	})

	t.Run("a member muted for two rules who fixed one stays held and is told the other", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RuleUsername: models.RequirementActionMute,
		})
		member := env.mreqMember("Alex", "", "")
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != memberReqRules[0].Bit|memberReqRules[1].Bit {
			t.Fatalf("hold = %+v, want one hold for both rules", hold)
		}
		member = env.mreqFix(member)
		tr := env.translator()

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken",
			memberReqRulesText(tr, memberReqRules[1].Bit))
		if text := env.mreqAnswers()[len(env.mreqAnswers())-1].Text; strings.Contains(text, memberReqRulesText(tr, memberReqRules[0].Bit)) {
			t.Errorf("answer %q names the fixed rule", text)
		}
		mreqWantHeld(t, env, hold)

		// Adding the username too frees them.
		member.Username = "alex"
		env.fake.setProfile(member.Id, member.FirstName, member.LastName, member.Username)
		answersBefore := len(env.mreqAnswers())
		env.mreqPressFixed(member, notice)
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != staffMarker("mreq_fixed_done") || got.Alert {
			t.Errorf("answer after both fixes = %+v, want the done toast", got)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("the hold is still there after both rules were fixed")
		}
	})

	t.Run("a rule since set to Off is not checked and not named", func(t *testing.T) {
		env := newMemberReqEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RuleUsername: models.RequirementActionMute,
		})
		member := env.mreqMember("Alex", "", "")
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleUsername: models.RequirementActionOff})
		tr := env.translator()

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken",
			memberReqRulesText(tr, memberReqRules[0].Bit))
		if text := env.mreqAnswers()[len(env.mreqAnswers())-1].Text; strings.Contains(text, memberReqRulesText(tr, memberReqRules[1].Bit)) {
			t.Errorf("answer %q names the rule that is Off", text)
		}
		mreqWantHeld(t, env, hold)

		// With the last name fixed the Off rule does not hold them back.
		member = env.mreqFix(member)
		answersBefore := len(env.mreqAnswers())
		env.mreqPressFixed(member, notice)
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != staffMarker("mreq_fixed_done") || got.Alert {
			t.Errorf("answer = %+v, want the done toast: the username rule is Off", got)
		}
	})

	t.Run("a getChat failure and a permissions lookup error change nothing", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		member = env.mreqFix(member)

		env.fake.script("getChat", env.chat.Id, staffFakeError(502, "Bad Gateway"))
		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")
		mreqWantHeld(t, env, hold)

		mreqSwap(t, &lockdownSnapshotLookup, func(int64) (*models.ChatLockdown, error) {
			return nil, errors.New("lockdown lookup failed")
		})
		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")
		mreqWantHeld(t, env, hold)
		if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
			t.Errorf("the member after both refusals = %+v, want still muted with the marker", live)
		}
	})

	t.Run("group language uk", func(t *testing.T) {
		env := newMemberReqEnvWith(t, settingsEnvOptions{RealLocale: true})
		if err := lang.ChangeGroupLanguage(env.chat.Id, "uk"); err != nil {
			t.Fatal(err)
		}
		member, _, notice := mreqMutedMember(t, env)
		other := env.mreqMember("Bo", "Li", "bo")
		uk := i18n.MustNewTranslator("uk")
		want := func(key string) string {
			text, err := uk.GetString(key)
			if err != nil {
				t.Fatal(err)
			}
			return text
		}
		for _, c := range []struct {
			name, key string
			from      gotgbot.User
			data      string
		}{
			{"not yours", "mreq_fixed_not_yours", other, env.mreqFixedData(notice)},
			{"invalid", "common_callback_invalid_request", member, "mreq|v1|a=f"},
			{"stale", "mreq_fixed_stale", member, mreqFixedButtonData(t, 1, member.Id)},
		} {
			answersBefore := len(env.mreqAnswers())
			env.mreqPressData(c.from, notice.MessageID, c.data)
			if got := env.mreqOnlyAnswer(answersBefore); got.Text != want(c.key) || !got.Alert {
				t.Errorf("%s: answer = %+v, want the uk text %q as an alert", c.name, got, want(c.key))
			}
		}
		answersBefore := len(env.mreqAnswers())
		env.mreqPressFixed(member, notice)
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != memberReqRulesTextAnswer(t, uk, memberReqRules[0].Bit) || !got.Alert {
			t.Errorf("still broken: answer = %+v, want the uk text with the uk rule phrase", got)
		}
	})
}

// memberReqRulesTextAnswer is the still-broken answer for the rules in the mask, in tr's language.
func memberReqRulesTextAnswer(t *testing.T, tr *i18n.Translator, broken int) string {
	t.Helper()
	text, err := tr.GetString("mreq_fixed_still_broken", i18n.TranslationParams{"rules": memberReqRulesText(tr, broken)})
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestMemberReqFixedNeverLifts(t *testing.T) {
	until := func(offset time.Duration) func(int64) *gotgbot.RestrictChatMemberOpts {
		return func(int64) *gotgbot.RestrictChatMemberOpts {
			return &gotgbot.RestrictChatMemberOpts{UntilDate: time.Now().Add(offset).Unix()}
		}
	}
	replace := map[string]func(marker int64) *gotgbot.RestrictChatMemberOpts{
		"an admin's /mute (no end date)":      func(int64) *gotgbot.RestrictChatMemberOpts { return nil },
		"a /tmute of one hour":                until(time.Hour),
		"a staff mute of a day":               until(24 * time.Hour),
		"the captcha's re-mute (no end date)": func(int64) *gotgbot.RestrictChatMemberOpts { return nil },
		"antiflood's mute (no end date)":      func(int64) *gotgbot.RestrictChatMemberOpts { return nil },
		"a warn-limit mute (no end date)":     func(int64) *gotgbot.RestrictChatMemberOpts { return nil },
		"a mute three seconds past the marker": func(marker int64) *gotgbot.RestrictChatMemberOpts {
			return &gotgbot.RestrictChatMemberOpts{UntilDate: marker + 3}
		},
		"a mute three seconds before the marker": func(marker int64) *gotgbot.RestrictChatMemberOpts {
			return &gotgbot.RestrictChatMemberOpts{UntilDate: marker - 3}
		},
	}
	for name, opts := range replace {
		t.Run(name, func(t *testing.T) {
			env := newMemberReqEnv(t)
			member, hold, notice := mreqMutedMember(t, env)
			member = env.mreqFix(member)
			if _, err := env.bot.RestrictChatMember(env.chat.Id, member.Id, MutedPermissions, opts(hold.MuteUntil)); err != nil {
				t.Fatal(err)
			}
			data := env.mreqFixedData(notice)

			mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")
			mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")

			mreqWantHeld(t, env, hold)
		})
	}

	t.Run("a member an admin unmuted", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		member = env.mreqFix(member)
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember, IsMember: true, CanSendMessages: true})
		data := env.mreqFixedData(notice)

		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")
		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")

		mreqWantHeld(t, env, hold)
	})

	t.Run("a member who left", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member, hold, notice := mreqMutedMember(t, env)
		member = env.mreqFix(member)
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_not_ours")

		mreqWantHeld(t, env, hold)
	})
}

func TestMemberReqFixedDuringLockdown(t *testing.T) {
	env := newMemberReqEnv(t)
	member, _, notice := mreqMutedMember(t, env)
	pre := `{"can_send_messages":true,"can_send_photos":true,"can_invite_users":true}`
	mreqStartLockdown(t, env.chat.Id, pre)
	member = env.mreqFix(member)
	var want gotgbot.ChatPermissions
	if err := json.Unmarshal([]byte(pre), &want); err != nil {
		t.Fatal(err)
	}
	restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

	env.mreqPressFixed(member, notice)

	mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, want,
		mreqAnswer{Text: staffMarker("mreq_fixed_done_lockdown"), Alert: true},
		mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed_lockdown", member))
}

func TestMemberReqFixedRestoreOnFailure(t *testing.T) {
	env := newMemberReqEnv(t)
	member, hold, notice := mreqMutedMember(t, env)
	member = env.mreqFix(member)
	if err := db.DB.Model(&models.ChatRequirementHold{}).Where("id = ?", hold.ID).Update("welcome_held", true).Error; err != nil {
		t.Fatal(err)
	}
	hold = env.mreqHold(member.Id)
	env.fake.script("restrictChatMember", env.chat.Id, staffFakeError(502, "Bad Gateway"))

	restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())
	env.mreqPressFixed(member, notice)

	if got := env.mreqOnlyAnswer(answersBefore); got.Text != staffMarker("mreq_fixed_check_failed") || !got.Alert {
		t.Errorf("answer = %+v, want the check-failed alert", got)
	}
	if n := len(env.mreqRestricts(member.Id)); n != restrictsBefore+1 {
		t.Errorf("restrictChatMember calls about the member = %d, want the %d before plus the one failed unmute", n, restrictsBefore)
	}
	if edits := env.mreqNoticeEdits(notice.MessageID); len(edits) != 0 {
		t.Errorf("edits of the notice after a failed unmute = %d, want none", len(edits))
	}
	mreqWantHeld(t, env, hold)
	if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
		t.Errorf("the member after the failed unmute = %+v, want still muted with the marker", live)
	}

	// The same button works on the next tap.
	restrictsBefore, answersBefore = len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())
	env.mreqPressFixed(member, notice)
	mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
		mreqAnswer{Text: staffMarker("mreq_fixed_done")}, mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
}

func TestMemberReqFixedTwoReplicas(t *testing.T) {
	env := newMemberReqEnv(t)
	member, hold, notice := mreqMutedMember(t, env)
	member = env.mreqFix(member)
	replica := env.mreqReplica(member)
	replica.fake.setMember(replica.chat.Id, member.Id, staffFakeMember{
		Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: hold.MuteUntil,
	})
	data := env.mreqFixedData(notice)
	envRestricts := len(env.mreqRestricts(member.Id))

	updates := make([]*gotgbot.Update, 0, 2)
	for _, e := range []*memberReqEnv{env, replica} {
		id := e.updateID()
		updates = append(updates, &gotgbot.Update{UpdateId: id, CallbackQuery: &gotgbot.CallbackQuery{
			Id: fmt.Sprintf("mreq-%d", id), From: member, Data: data, ChatInstance: "mreq-test",
			Message: gotgbot.Message{MessageId: notice.MessageID, Date: 1, Chat: e.chat},
		}})
	}
	var wg sync.WaitGroup
	errs := make([]error, len(updates))
	for i, e := range []*memberReqEnv{env, replica} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = e.dispatcher.ProcessUpdate(e.bot, updates[i], nil)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("ProcessUpdate on process %d error = %v", i, err)
		}
	}

	if n := len(env.mreqRestricts(member.Id)) - envRestricts + len(replica.mreqRestricts(member.Id)); n != 1 {
		t.Errorf("restrictChatMember calls for the unmute in both processes = %d, want exactly 1", n)
	}
	var done, stale int
	for _, answer := range append(env.mreqAnswers(), replica.mreqAnswers()...) {
		switch answer.Text {
		case staffMarker("mreq_fixed_done"):
			done++
		case staffMarker("mreq_fixed_stale"):
			stale++
		}
	}
	if done != 1 || stale != 1 {
		t.Errorf("answers: %d done and %d stale, want one of each", done, stale)
	}
	if env.mreqHold(member.Id) != nil {
		t.Error("the hold is still there after the unmute")
	}
}

// mreqNoticeWith returns the newest message the group received that holds marker.
func (e *memberReqEnv) mreqNoticeWith(marker string) staffSentMessage {
	e.t.Helper()
	notices := e.mreqNotices()
	for i := len(notices) - 1; i >= 0; i-- {
		if strings.Contains(mreqText(notices[i]), marker) {
			return notices[i]
		}
	}
	e.t.Fatalf("no message of the group holds %q (messages %v)", marker, e.mreqSentTexts())
	return staffSentMessage{}
}

func TestMemberReqFixedHeldWelcome(t *testing.T) {
	setup := func(t *testing.T) *memberReqEnv {
		t.Helper()
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
		env.loadMemberReq()
		if err := greetings.SetWelcomeOnce(env.chat.Id, true); err != nil {
			t.Fatalf("SetWelcomeOnce error = %v", err)
		}
		return env
	}

	t.Run("a member muted on join gets one welcome when they fix it, and Greet once claims them then", func(t *testing.T) {
		env := setup(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqJoin(member)
		notice := env.mreqNoticeWith(staffMarker("mreq_notice_mute"))
		if hold := env.mreqHold(member.Id); hold == nil || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a mute hold with the welcome held", hold)
		}
		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 0 {
			t.Fatalf("welcomes before the press = %d, want none", got)
		}
		if got := greetedRows(t, env.chat.Id); got != 0 {
			t.Fatalf("greeted_members rows before the press = %d, want none: a held member is not claimed", got)
		}
		member = env.mreqFix(member)

		env.mreqPressFixed(member, notice)

		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, mreqWelcomeWord+" "+member.FirstName); got != 1 {
			t.Fatalf("welcomes after the press = %d, want exactly one (texts %v)", got, texts)
		}
		if got := greetedRows(t, env.chat.Id); got != 1 {
			t.Errorf("greeted_members rows after the press = %d, want the member claimed once", got)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold after the press = %+v, want none", hold)
		}

		// A second press is stale and sends no second welcome.
		answersBefore := len(env.mreqAnswers())
		env.mreqPressFixed(member, notice)
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != staffMarker("mreq_fixed_stale") || !got.Alert {
			t.Errorf("second press answer = %+v, want the stale alert", got)
		}
		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 1 {
			t.Errorf("welcomes after the second press = %d, want still 1", got)
		}
	})

	t.Run("a member muted by the message check gets no welcome", func(t *testing.T) {
		env := setup(t)
		member := env.mreqMember("Alex", "", "alex")
		notice := env.mreqMuteByMessage(member)
		if hold := env.mreqHold(member.Id); hold == nil || hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a message hold with no welcome held", hold)
		}
		member = env.mreqFix(member)

		env.mreqPressFixed(member, notice)

		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord); got != 0 {
			t.Errorf("welcomes after the press = %d, want none for a message hold", got)
		}
		if got := greetedRows(t, env.chat.Id); got != 0 {
			t.Errorf("greeted_members rows = %d, want none", got)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("the hold is still there after the unmute")
		}
	})
}

func TestMemberReqFixedCallbackBytes(t *testing.T) {
	env := newMemberReqEnv(t)
	for _, c := range []struct {
		name         string
		hold, member int64
		want         int
	}{
		{"typical", 123456, 1234567890, 33},
		{"worst: a 19-digit hold and a 16-digit member", 9223372036854775807, 9007199254740991, 52},
	} {
		keyboard := memberReqFixedButton(env.translator(), c.hold, c.member)
		if len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 1 {
			t.Fatalf("%s: keyboard = %+v, want one button", c.name, keyboard)
		}
		data := keyboard.InlineKeyboard[0][0].CallbackData
		if len(data) != c.want || len(data) > 64 {
			t.Errorf("%s: data %q is %d bytes, want %d (cap 64)", c.name, data, len(data), c.want)
		}
		decoded, ok := decodeCallbackData(data, memberReqCallbackNamespace)
		if !ok {
			t.Fatalf("%s: data %q does not decode", c.name, data)
		}
		if len(decoded.Fields) != 3 {
			t.Errorf("%s: fields = %v, want exactly a, h and u (no name, rule or text)", c.name, decoded.Fields)
		}
		hold, member, ok := memberReqParseFixed(decoded.Fields)
		if !ok || hold != c.hold || member != c.member {
			t.Errorf("%s: parsed %d, %d, %t, want %d, %d", c.name, hold, member, ok, c.hold, c.member)
		}
	}
}

func TestMemberReqEnforcementLive(t *testing.T) {
	// No withMemberReqLive: this is the production default, stored by the module's init().
	if !memberReqLive() {
		t.Fatal("memberReqLive() = false, want true by default once the I fixed it handler is registered")
	}

	// The handler is registered: a dispatcher loaded with the module answers an mreq| button,
	// and the watcher is at group -2 (RemoveGroup says whether the group exists).
	fresh := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadMemberRequirements(fresh)
	if !fresh.RemoveGroup(memberReqWatchGroup) || memberReqWatchGroup != -2 {
		t.Errorf("the watcher's group %d is not registered by LoadMemberRequirements, want group -2", memberReqWatchGroup)
	}
	if !fresh.RemoveGroup(0) {
		t.Error("the mreq| button is not registered at group 0 by LoadMemberRequirements")
	}

	env := newSettingsEnv(t)
	LoadMemberRequirements(env.dispatcher)
	member, other := uniqueLinkOwnerID(), uniqueLinkOwnerID()
	answersBefore := len(env.fake.callsFor("answerCallbackQuery"))
	env.pressRaw(env.chat, gotgbot.User{Id: other, FirstName: "Bo"}, 77, mreqFixedButtonData(t, 5, member))
	answers := env.fake.callsFor("answerCallbackQuery")
	if len(answers) != answersBefore+1 {
		t.Fatalf("answers = %d, want exactly one for a press by another member", len(answers)-answersBefore)
	}
	if text := fmt.Sprint(answers[len(answers)-1].Params["text"]); text != staffMarker("mreq_fixed_not_yours") {
		t.Errorf("answer = %q, want mreq_fixed_not_yours", text)
	}
}
