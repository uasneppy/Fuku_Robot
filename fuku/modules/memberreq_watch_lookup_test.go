//go:build testtools

package modules

import (
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqWatchLookupEnv builds a requirements environment whose watcher is loaded on the real
// dispatcher and whose bot answers the photo and channel lookups from a fresh fake state.
func mreqWatchLookupEnv(t *testing.T) (*memberReqEnv, *mreqLookupState) {
	t.Helper()
	env, state := mreqLookupEnv(t)
	env.loadMemberReq()
	return env, state
}

// mreqStorePass stores the Phase 13 clean verdict of member for the group's current settings, as
// a clean first message would, so a test can tell a pass hit from a clean-predicate run.
func mreqStorePass(t *testing.T, env *memberReqEnv, member gotgbot.User, settings *models.ChatRequirements) {
	t.Helper()
	key := env.mreqPassKeyOf(member.Id)
	if err := env.redis.Set(key, memberReqPassValue(settings, memberReqNamesOf(member))); err != nil {
		t.Fatalf("store pass %s: %v", key, err)
	}
	env.redis.SetTTL(key, memberReqPassTTL)
}

// mreqLookupRequests counts every request about member that a job would make or a lookup would
// cost: the live record, the photo and the channel half.
func mreqLookupRequests(env *memberReqEnv, member gotgbot.User) int {
	return env.mreqCallsAbout(member.Id, "getChatMember", "getUserProfilePhotos", "restrictChatMember", "banChatMember", "unbanChatMember")
}

func TestMemberReqWatcherLookupRules(t *testing.T) {
	photoBit := func(t *testing.T) int { return mreqBitOf(t, "ph") }
	channelBit := func(t *testing.T) int { return mreqBitOf(t, "ch") }

	t.Run("a member with clean names and no photo is muted once through the real dispatcher", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})

		env.mreqSend(member, "hello")
		memberReqJobs.wait()

		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Fatalf("restrict calls after the first message = %d, want 1", n)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != photoBit(t) || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil {
			t.Fatalf("hold = %+v, want a punished mute hold with the photo bit %d", hold, photoBit(t))
		}
		notices := env.mreqNotices()
		if len(notices) != 1 || !strings.Contains(mreqText(notices[0]), staffMarker("mreq_notice_mute")) ||
			!strings.Contains(mreqText(notices[0]), staffMarker("mreq_rule_photo")) {
			t.Fatalf("notices = %+v, want one mute notice naming the photo rule", notices)
		}

		env.mreqSend(member, "hello again")
		memberReqJobs.wait()
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrict calls after the second message = %d, want still 1 (the held path)", n)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices after the second message = %d, want still 1", n)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold is gone after a second message of a member who still has no photo")
		}
	})

	t.Run("a member with a photo cached and the pass stored starts no job and makes no request", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), memberReqPositiveTTL)
		mreqStorePass(t, env, member, settings)
		// A hold of an hour ago that only the clean predicate could delete: it survives, so the
		// watcher ran no predicate and wrote nothing.
		env.mreqOldHold(member.Id)

		env.mreqSend(member, "hello")
		memberReqJobs.wait()

		if n := mreqLookupRequests(env, member); n != 0 {
			t.Errorf("requests about the member = %d, want none", n)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was deleted: the watcher ran the clean predicate although the pass key hit")
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("messages posted = %d, want none", n)
		}
	})

	t.Run("a satisfied member without a pass runs the Phase 13 clean predicate and stores the pass", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), memberReqPositiveTTL)
		env.mreqOldHold(member.Id)

		env.mreqSend(member, "hello")
		memberReqJobs.wait()

		if env.mreqHold(member.Id) != nil {
			t.Error("the old hold survived a clean message of a member with a cached photo")
		}
		want := memberReqPassValue(settings, memberReqNamesOf(member))
		if got, err := env.redis.Get(env.mreqPassKeyOf(member.Id)); err != nil || got != want {
			t.Errorf("pass key = %q, %v, want %q", got, err, want)
		}
		if n := mreqLookupRequests(env, member); n != 0 {
			t.Errorf("requests about the member = %d, want none", n)
		}
	})

	t.Run("with only the channel rule on and no linked channel cached the message is clean", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		if err := env.redis.Set(memberReqLinkedKey(env.chat.Id), "0"); err != nil {
			t.Fatalf("set lc: %v", err)
		}
		env.redis.SetTTL(memberReqLinkedKey(env.chat.Id), memberReqLinkedTTL)
		mreqStorePass(t, env, member, settings)

		env.mreqSend(member, "hello")
		memberReqJobs.wait()

		if n := mreqLookupRequests(env, member); n != 0 {
			t.Errorf("requests about the member = %d, want none", n)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("a hold was written for a group with no linked channel")
		}
	})

	t.Run("a linked channel with the subscription cached is clean and with it missing is one job", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionMute})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		subscribed := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.setMember(channel, subscribed.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		left := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.setMember(channel, left.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		if err := env.redis.Set(memberReqLinkedKey(env.chat.Id), strconv.FormatInt(channel, 10)); err != nil {
			t.Fatalf("set lc: %v", err)
		}
		env.redis.SetTTL(memberReqLinkedKey(env.chat.Id), memberReqLinkedTTL)
		mreqSetKey(t, env, memberReqSubKey(channel, subscribed.Id), memberReqPositiveTTL)
		mreqStorePass(t, env, subscribed, settings)

		env.mreqSend(subscribed, "hello")
		memberReqJobs.wait()
		if n := mreqLookupRequests(env, subscribed) + mreqMemberCalls(env, channel, subscribed.Id); n != 0 {
			t.Errorf("requests about the subscribed member = %d, want none", n)
		}

		env.mreqSend(left, "hello")
		memberReqJobs.wait()
		hold := env.mreqHold(left.Id)
		if hold == nil || hold.Rules != channelBit(t) {
			t.Fatalf("hold of the member who left the channel = %+v, want the channel bit %d", hold, channelBit(t))
		}
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrict calls = %d, want exactly the one of the member who left", n)
		}
	})

	t.Run("broken names keep the Phase 13 behaviour: a job for a member, none for a known admin", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RulePhoto:    models.RequirementActionMute,
		})
		violator := env.mreqMember("Alex", "", "alex")
		mreqLookupCleanup(t, []int64{violator.Id}, nil)
		state.set(violator.Id, mreqPhotoAnswer{})
		admin := env.mreqMember("Adm", "", "adm")
		mreqLookupCleanup(t, []int64{admin.Id}, nil)
		state.set(admin.Id, mreqPhotoAnswer{})
		env.mreqPromote(admin, gotgbot.ChatMemberStatusAdministrator)
		env.mreqAdminList()

		env.mreqSend(admin, "I am an admin")
		memberReqJobs.wait()
		if n := mreqLookupRequests(env, admin); n != 0 {
			t.Errorf("requests about a known admin with broken names = %d, want none", n)
		}

		env.mreqSend(violator, "plain text")
		memberReqJobs.wait()
		hold := env.mreqHold(violator.Id)
		if want := mreqBitOf(t, "ln") | photoBit(t); hold == nil || hold.Rules != want {
			t.Fatalf("hold = %+v, want both the last name and the photo bits (%d)", hold, want)
		}
	})
}
