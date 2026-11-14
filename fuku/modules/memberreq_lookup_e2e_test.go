//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// mreqE2ERuleIndex is the position of the rule with this callback code in the page's eight stored
// punishments.
func mreqE2ERuleIndex(t *testing.T, code string) int {
	t.Helper()
	for i, c := range settingsMreqRuleCodes {
		if c == code {
			return i
		}
	}
	t.Fatalf("no rule with code %q", code)
	return -1
}

// mreqE2ESetRule sets the rule with the given code to a punishing action the way an owner does:
// the choice on the rule screen, then the card's Yes. It fails unless the stored punishment is
// the action afterwards.
func mreqE2ESetRule(t *testing.T, env *memberReqEnv, owner gotgbot.User, msgID int64, code string, action int) {
	t.Helper()
	env.press(owner, msgID, env.groupFields(owner, "a", "st", "p", settingsPageMreq, "t", code, "v", fmt.Sprint(action)))
	text, keyboard := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("settings_mreq_confirm")) || len(keyboard) < 1 || len(keyboard[0]) != 2 {
		t.Fatalf("the press of %s to %d drew %q with keyboard %+v, want the card", code, action, text, keyboard)
	}
	if got := settingsMreqStoredActions(t, env.chat.Id)[mreqE2ERuleIndex(t, code)]; got != 0 {
		t.Fatalf("drawing the card stored %d, want nothing written", got)
	}
	env.pressRaw(env.chat, owner, msgID, keyboard[0][0].CallbackData)
	if got := settingsMreqStoredActions(t, env.chat.Id)[mreqE2ERuleIndex(t, code)]; got != action {
		t.Fatalf("after Yes the stored punishment of %s = %d, want %d", code, got, action)
	}
}

// mreqE2EDeleteKeys deletes Redis keys a test changed the meaning of mid-way (a cached
// administrator status or linked channel it then changes in the fake).
func mreqE2EDeleteKeys(t *testing.T, keys ...string) {
	t.Helper()
	_, _, client := cache.GetCacheState()
	if client == nil {
		t.Fatal("no Redis client in the test environment")
	}
	for _, key := range keys {
		if err := client.Del(context.Background(), key).Err(); err != nil {
			t.Fatalf("delete %s: %v", key, err)
		}
	}
}

// mreqE2ENothingHappened fails the test unless a message of member left no punishment behind: no
// hold, no restrict, ban or unban call about them and no requirement notice.
func mreqE2ENothingHappened(t *testing.T, env *memberReqEnv, member gotgbot.User, noticesBefore int) {
	t.Helper()
	if hold := env.mreqHold(member.Id); hold != nil {
		t.Errorf("hold = %+v, want none", hold)
	}
	if n := len(env.mreqRestricts(member.Id)); n != 0 {
		t.Errorf("restrictChatMember calls about the member = %d, want none", n)
	}
	if n := len(env.mreqNotices()); n != noticesBefore {
		t.Errorf("messages to the group = %d, want the %d before: nobody is punished", n, noticesBefore)
	}
}

func TestMemberReqLookupEndToEnd(t *testing.T) {
	t.Run("photo", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		owner := env.newOwner(env.chat)
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		msgID := settingsMreqOpenPage(t, env.settingsEnv, owner)
		mreqE2ESetRule(t, env, owner, msgID, "ph", models.RequirementActionMute)
		if got := settingsMreqStoredActions(t, env.chat.Id); got != [8]int{5: models.RequirementActionMute} {
			t.Fatalf("stored punishments = %v, want only the photo rule on Mute", got)
		}

		// A member with clean names and no photo is muted once, with one notice.
		notice := env.mreqMuteByMessage(member)
		photoRule, ok := memberReqRuleByCode("ph")
		if !ok {
			t.Fatal("no photo rule")
		}
		if !strings.Contains(mreqText(notice), staffMarker(photoRule.PhraseKey)) || !strings.HasSuffix(mreqText(notice), " "+staffMarker("mreq_remedy_photo")) {
			t.Errorf("notice %q must name the photo phrase and end with the photo remedy", mreqText(notice))
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqBitOf(t, "ph") || hold.PunishedAt == nil {
			t.Fatalf("hold = %+v, want a punished hold with the photo bit", hold)
		}
		restricts := env.mreqRestricts(member.Id)
		if len(restricts) != 1 {
			t.Fatalf("restrictChatMember calls about the member = %d, want one", len(restricts))
		}
		if _, marker := restricts[0].Params["until_date"]; !marker {
			t.Errorf("the mute %+v carries no marker end date", restricts[0].Params)
		}

		// A second message makes no second punishment and no second notice.
		sentAfterMute := len(env.mreqNotices())
		env.mreqSend(member, "plain text again")
		memberReqJobs.wait()
		if n := len(env.mreqRestricts(member.Id)); n != 1 {
			t.Errorf("restrictChatMember calls after a second message = %d, want still one", n)
		}
		if n := len(env.mreqNotices()); n != sentAfterMute {
			t.Errorf("messages to the group after a second message = %d, want the %d after the mute: %q", n, sentAfterMute, env.mreqSentTexts())
		}

		// The member adds a photo and taps the button: a fresh lookup frees them.
		state.set(member.Id, mreqHasPhoto)
		answersBefore := len(env.mreqAnswers())
		env.mreqPressFixed(member, notice)
		mreqWantFixedUnmute(t, env, member, notice, 1, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if !env.redis.Exists(memberReqPhotoKey(member.Id)) {
			t.Error("the photo positive is not cached after the member was freed")
		}

		// Their next message starts no job: the positive answers.
		jobStarts := mreqCountJobStarts(t)
		photoBefore := mreqLookupPhotoCalls(env)
		env.mreqSend(member, "plain text after the fix")
		memberReqJobs.wait()
		if n := jobStarts(); n != 0 {
			t.Errorf("a message of a member with a photo tried to start %d jobs, want none", n)
		}
		if n := mreqLookupPhotoCalls(env); n != photoBefore {
			t.Errorf("getUserProfilePhotos requests after the fix = %d, want the %d before", n, photoBefore)
		}
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold after the member's next message = %+v, want none", hold)
		}
	})

	t.Run("channel", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		owner := env.newOwner(env.chat)
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		msgID := settingsMreqOpenPage(t, env.settingsEnv, owner)
		mreqE2ESetRule(t, env, owner, msgID, "ch", models.RequirementActionMute)

		// A member who left the channel is muted with the channel phrase and no remedy tail.
		notice := env.mreqMuteByMessage(member)
		channelRule, ok := memberReqRuleByCode("ch")
		if !ok {
			t.Fatal("no channel rule")
		}
		if !strings.Contains(mreqText(notice), staffMarker(channelRule.PhraseKey)) || strings.Contains(mreqText(notice), staffMarker("mreq_remedy_photo")) {
			t.Errorf("notice %q must name the channel phrase and carry no photo remedy", mreqText(notice))
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqBitOf(t, "ch") || hold.PunishedAt == nil {
			t.Fatalf("hold = %+v, want a punished hold with the channel bit", hold)
		}
		if n := len(env.mreqRestricts(member.Id)); n != 1 {
			t.Fatalf("restrictChatMember calls about the member = %d, want one", n)
		}

		// The member subscribes and taps the button: freed.
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		answersBefore := len(env.mreqAnswers())
		env.mreqPressFixed(member, notice)
		mreqWantFixedUnmute(t, env, member, notice, 1, answersBefore, defaultUnmutePermissions(),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))

		// The bot is no longer an administrator of the channel: an unsubscribed member is not
		// punished and the page says why.
		state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		mreqE2EDeleteKeys(t, memberReqBotAdminKey(channel))
		stranger := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.setMember(channel, stranger.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		noticesBefore := len(env.mreqNotices())
		env.mreqSend(stranger, "plain text")
		memberReqJobs.wait()
		mreqE2ENothingHappened(t, env, stranger, noticesBefore)
		env.press(owner, msgID, env.groupFields(owner, "a", "op", "p", settingsPageMreq))
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if want := settingsMreqHereLine("🚫 ", "settings_mreq_here_notadmin"); !strings.Contains(text, want) {
			t.Errorf("the page %q lacks the not-administrator line %q", text, want)
		}

		// The group is unlinked: nobody is punished and a punishing press on the rule is refused.
		state.link(env.chat.Id, 0)
		mreqE2EDeleteKeys(t, memberReqLinkedKey(env.chat.Id), memberReqBotAdminKey(channel))
		state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusAdministrator})
		loner := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.setMember(channel, loner.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		noticesBefore = len(env.mreqNotices())
		env.mreqSend(loner, "plain text")
		memberReqJobs.wait()
		mreqE2ENothingHappened(t, env, loner, noticesBefore)

		answersBefore = len(env.mreqAnswers())
		env.press(owner, msgID, env.groupFields(owner, "a", "st", "p", settingsPageMreq, "t", "ch", "v", fmt.Sprint(models.RequirementActionBan)))
		got := env.mreqOnlyAnswer(answersBefore)
		if !got.Alert || got.Text != settingsMreqHereLine("", "settings_mreq_here_nolink") {
			t.Errorf("answer = %+v, want one alert of settings_mreq_here_nolink", got)
		}
		text, _ = env.lastEdit(env.chat.Id, msgID)
		if strings.Contains(text, staffMarker("settings_mreq_confirm")) {
			t.Errorf("the refused press drew a card: %q", text)
		}
		if stored := settingsMreqStoredActions(t, env.chat.Id)[mreqE2ERuleIndex(t, "ch")]; stored != models.RequirementActionMute {
			t.Errorf("stored channel punishment = %d after the refused press, want the Mute from before", stored)
		}
	})
}
