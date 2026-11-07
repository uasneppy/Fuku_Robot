//go:build testtools

package modules

import (
	"context"
	"reflect"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
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
		custom := gotgbot.ChatPermissions{CanSendMessages: true, CanSendPhotos: true, CanInviteUsers: true}
		env.fake.chatPerms[env.chat.Id] = custom
		member, _, notice := mreqMutedMember(t, env)
		member = env.mreqFix(member)
		sentBefore := len(env.mreqNotices())
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, custom,
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
		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, defaultUnmutePermissions(),
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

			mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, defaultUnmutePermissions(),
				mreqAnswer{Text: staffMarker("mreq_fixed_done")},
				mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
			if edits := env.mreqNoticeEdits(stored); stored != 0 && len(edits) != 0 {
				t.Errorf("edits of the stored notice id %d = %d, want none: the pressed message is the one edited", stored, len(edits))
			}
		})
	}
}

