//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
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

// mreqCountJobStarts makes the runner refuse every job (memberReqMaxInFlight 0) and returns a
// counter of how many times the watcher tried to start one: each refused start logs exactly one
// "was shed" line, so the counter proves "no job was started" where the executor's own
// exemptions would hide it (an approved member's job makes no request either).
func mreqCountJobStarts(t *testing.T) func() int {
	t.Helper()
	mreqSwap(t, &memberReqMaxInFlight, 0)
	hook := mreqShedHook(t)
	t.Cleanup(hook.Reset)
	return func() int {
		n := 0
		for _, entry := range hook.AllEntries() {
			if strings.Contains(entry.Message, "was shed") {
				n++
			}
		}
		return n
	}
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
		starts := mreqCountJobStarts(t)

		env.mreqSend(member, "hello")
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts = %d, want none", n)
		}
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
		starts := mreqCountJobStarts(t)

		env.mreqSend(member, "hello")
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts = %d, want none", n)
		}
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

		starts := mreqCountJobStarts(t)
		env.mreqSend(subscribed, "hello")
		memberReqJobs.wait()
		if n := starts(); n != 0 {
			t.Errorf("job starts for the subscribed member = %d, want none", n)
		}
		if n := mreqLookupRequests(env, subscribed) + mreqMemberCalls(env, channel, subscribed.Id); n != 0 {
			t.Errorf("requests about the subscribed member = %d, want none", n)
		}

		mreqSwap(t, &memberReqMaxInFlight, 32)
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

	t.Run("a cached administrator without a photo starts no job on a miss (D-25)", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		admin := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		env.mreqPromote(admin, gotgbot.ChatMemberStatusAdministrator)
		env.mreqAdminList()
		starts := mreqCountJobStarts(t)

		for i := 0; i < 3; i++ {
			env.mreqSend(admin, "an administrator without a photo")
		}
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts for a known administrator = %d, want none", n)
		}
		if n := mreqLookupRequests(env, admin); n != 0 {
			t.Errorf("requests about a known administrator = %d, want none", n)
		}
		if env.mreqHold(admin.Id) != nil {
			t.Error("a known administrator was held")
		}
	})

	t.Run("an approved member without a photo starts no job on a miss (D-25)", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		approved := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("approve: %v", err)
		}
		starts := mreqCountJobStarts(t)

		for i := 0; i < 3; i++ {
			env.mreqSend(approved, "an approved member without a photo")
		}
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts for an approved member = %d, want none", n)
		}
		if n := mreqLookupRequests(env, approved); n != 0 {
			t.Errorf("requests about an approved member = %d, want none", n)
		}
		if env.mreqHold(approved.Id) != nil {
			t.Error("an approved member was held")
		}
	})

	t.Run("an approvals read that fails starts no job", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
			return false, errors.New("database unavailable")
		})
		starts := mreqCountJobStarts(t)

		env.mreqSend(member, "approvals cannot be read")
		memberReqJobs.wait()

		if n := starts(); n != 0 {
			t.Errorf("job starts on an approvals read that failed = %d, want none", n)
		}
		if n := mreqLookupRequests(env, member); n != 0 {
			t.Errorf("requests about the member = %d, want none (an unknown is never punished)", n)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("a member was held on an approvals read that failed")
		}
	})

	t.Run("a member the admin cache does not know and who is not approved starts exactly one job", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		env.mreqNoAdminList()
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		starts := mreqCountJobStarts(t)
		env.mreqSend(member, "a message that is shed")
		if n := starts(); n != 1 {
			t.Errorf("job starts for a member with a cache miss = %d, want exactly 1", n)
		}
		mreqSwap(t, &memberReqMaxInFlight, 32)

		env.mreqSend(member, "first message")
		memberReqJobs.wait()

		if n := env.mreqMemberLookups(member.Id); n != 1 {
			t.Errorf("live getChatMember requests = %d, want 1 (one job)", n)
		}
		if n := mreqLookupPhotoCalls(env); n != 1 {
			t.Errorf("getUserProfilePhotos requests = %d, want 1", n)
		}
		if !env.redis.Exists(memberReqPhotoKey(member.Id)) {
			t.Error("the photo positive was not cached by the job")
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("a member with a photo was held")
		}
	})

	t.Run("without Redis every message is a job and a lookup that cannot be answered punishes nobody", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		const messages = 3
		for i := 0; i < messages; i++ {
			state.script(member.Id, staffFakeError(500, "Internal Server Error"))
		}

		for i := 0; i < messages; i++ {
			env.mreqSend(member, "redis is down")
			memberReqJobs.wait()
		}

		if n := env.mreqMemberLookups(member.Id); n != messages {
			t.Errorf("jobs (live getChatMember requests) = %d, want one per message (%d)", n, messages)
		}
		if n := mreqLookupPhotoCalls(env); n != messages {
			t.Errorf("getUserProfilePhotos requests = %d, want one per message (%d)", n, messages)
		}
		if env.mreqHold(member.Id) != nil || len(env.mreqWrites()) != 0 || len(env.mreqNotices()) != 0 {
			t.Error("a member was punished on a lookup that could not be answered")
		}
	})

	t.Run("without Redis a full runner sheds the job with one line of IDs", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		first := mreqLookupMember(t, env, state, mreqHasPhoto)
		second := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		mreqSwap(t, &memberReqMaxInFlight, 1)
		hook := mreqShedHook(t)
		t.Cleanup(hook.Reset)
		started, release := make(chan struct{}), make(chan struct{})
		var startOnce, freeOnce sync.Once
		freed := func() { freeOnce.Do(func() { close(release) }) }
		t.Cleanup(freed)
		env.fake.setOnGetMember(func(_, userID int64) {
			if userID == first.Id {
				startOnce.Do(func() { close(started) })
				<-release
			}
		})

		env.mreqSend(first, "blocks the only slot")
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("the first job never started")
		}
		env.mreqSend(second, "shed")
		var shed int
		for _, entry := range hook.AllEntries() {
			if strings.Contains(entry.Message, "was shed") {
				shed++
				if !strings.Contains(entry.Message, fmt.Sprint(second.Id)) || strings.Contains(entry.Message, "Alex") {
					t.Errorf("shed line %q must name the IDs and no name", entry.Message)
				}
			}
		}
		if shed != 1 {
			t.Errorf("shed lines = %d, want 1", shed)
		}
		freed()
		memberReqJobs.wait()
		if env.mreqHold(second.Id) != nil || env.mreqHold(first.Id) != nil {
			t.Error("a member with a photo was held")
		}
	})
}

func TestMemberReqWatcherLookupSingleflight(t *testing.T) {
	env, state := mreqWatchLookupEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
	member := mreqLookupMember(t, env, state, mreqHasPhoto)
	hook := mreqShedHook(t)
	t.Cleanup(hook.Reset)

	// The first photo request is held open for about 50 ms while the member's messages arrive.
	release := state.hold(member.Id)
	var arrivedOnce sync.Once
	arrived := make(chan struct{})
	state.setOnPhoto(func(int64) { arrivedOnce.Do(func() { close(arrived) }) })
	go func() {
		select {
		case <-arrived:
			time.Sleep(50 * time.Millisecond)
		case <-time.After(5 * time.Second):
		}
		release()
	}()
	t.Cleanup(release)

	for i := 0; i < 100; i++ {
		env.mreqSend(member, "message "+strconv.Itoa(i))
	}
	memberReqJobs.wait()

	if n := mreqLookupPhotoCalls(env); n != 1 {
		t.Fatalf("getUserProfilePhotos requests for 100 messages of one member = %d, want exactly 1", n)
	}
	if env.mreqHold(member.Id) != nil || len(env.mreqNotices()) != 0 || len(env.mreqWrites()) != 0 {
		t.Error("a member with a photo was held, noticed or restricted")
	}
	if !env.redis.Exists(memberReqPhotoKey(member.Id)) {
		t.Error("the photo positive does not exist after the flight")
	}
	for _, entry := range hook.AllEntries() {
		if strings.Contains(entry.Message, "was shed") {
			if !strings.Contains(entry.Message, fmt.Sprint(member.Id)) || !strings.Contains(entry.Message, fmt.Sprint(env.chat.Id)) ||
				strings.Contains(entry.Message, member.FirstName) {
				t.Errorf("shed line %q must carry the chat and user IDs and no name", entry.Message)
			}
		}
	}

	lookups := env.mreqMemberLookups(member.Id)
	env.mreqSend(member, "a later message")
	memberReqJobs.wait()
	if after := env.mreqMemberLookups(member.Id); after != lookups {
		t.Errorf("live lookups after a later message = %d, want still %d (clean from the cache, no job)", after, lookups)
	}
	if n := mreqLookupPhotoCalls(env); n != 1 {
		t.Errorf("getUserProfilePhotos requests after the later message = %d, want still 1", n)
	}
}

func TestMemberReqPassCacheWithLookups(t *testing.T) {
	t.Run("a pass hit with no lookup positive still hands a job and a missing photo is punished", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqStorePass(t, env, member, settings)

		env.mreqSend(member, "the pass key is stored")
		memberReqJobs.wait()

		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ph") {
			t.Fatalf("hold = %+v, want the photo hold: a pass hit must not hide a missing photo", hold)
		}
	})

	t.Run("an unknown or inert lookup leaves no verdict and the next message asks again", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute, memberreq.RuleChannel: models.RequirementActionMute})
		// The photo lookup is unknown (a 500) and the channel is inert: the bot is no
		// administrator of the linked channel (a 403 for its own lookup).
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		state.scriptMember(channel, staffTestBotID, staffFakeError(403, "Forbidden"))
		for i := 0; i < 2; i++ {
			state.script(member.Id, staffFakeError(500, "Internal Server Error"))
		}

		env.mreqSend(member, "first")
		memberReqJobs.wait()
		first := mreqLookupPhotoCalls(env)
		env.mreqSend(member, "second")
		memberReqJobs.wait()

		if first != 1 || mreqLookupPhotoCalls(env) != 2 {
			t.Errorf("photo requests after the first and second message = %d and %d, want 1 and 2 (an unknown is never cached)", first, mreqLookupPhotoCalls(env))
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("a member was held on a lookup that was unknown or inert")
		}
		_, _, client := cache.GetCacheState()
		keys, err := client.Keys(context.Background(), "fuku:memberreq:*").Result()
		if err != nil {
			t.Fatalf("keys: %v", err)
		}
		for _, key := range keys {
			if strings.Contains(key, fmt.Sprint(member.Id)) {
				t.Errorf("key %q names the member: an unknown or inert lookup must leave no verdict of its own", key)
			}
		}
	})

	t.Run("a lookup positive is never read as a pass for another user or another chat", func(t *testing.T) {
		env, _ := mreqWatchLookupEnv(t)
		settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute, memberreq.RuleChannel: models.RequirementActionMute})
		alice, bob := mreqLookupUser(t), mreqLookupUser(t)
		channelX, channelY := mreqChannelID(t), mreqChannelID(t)
		otherChat := uniqueModuleChatID()
		memberReqCleanup(t, []int64{otherChat}, nil)
		set := func(key, value string, ttl time.Duration) {
			t.Helper()
			if err := env.redis.Set(key, value); err != nil {
				t.Fatalf("set %s: %v", key, err)
			}
			env.redis.SetTTL(key, ttl)
		}
		set(memberReqPhotoKey(alice), "1", memberReqPositiveTTL)
		set(memberReqLinkedKey(env.chat.Id), strconv.FormatInt(channelX, 10), memberReqLinkedTTL)
		set(memberReqSubKey(channelX, alice), "1", memberReqPositiveTTL)
		set(env.mreqPassKeyOf(alice), "value", memberReqPassTTL)

		if satisfied, _ := memberReqLookupCached(settings, env.chat.Id, alice, "value", memberReqNames{}); !satisfied {
			t.Fatal("alice has every positive and was not satisfied")
		}
		if satisfied, hit := memberReqLookupCached(settings, env.chat.Id, bob, "value", memberReqNames{}); satisfied || hit {
			t.Errorf("bob read as satisfied=%t passHit=%t from alice's keys, want neither", satisfied, hit)
		}
		set(memberReqLinkedKey(env.chat.Id), strconv.FormatInt(channelY, 10), memberReqLinkedTTL)
		if satisfied, _ := memberReqLookupCached(settings, env.chat.Id, alice, "value", memberReqNames{}); satisfied {
			t.Error("a subscription of another channel satisfied the channel the group is linked to")
		}
		set(memberReqLinkedKey(otherChat), "0", memberReqLinkedTTL)
		set(memberReqPhotoKey(bob), "1", memberReqPositiveTTL)
		if _, hit := memberReqLookupCached(settings, otherChat, alice, "value", memberReqNames{}); hit {
			t.Error("alice's pass key of one chat was read as a pass in another chat")
		}
	})
}

func TestMemberReqWatcherLookupContinuesGroups(t *testing.T) {
	env, state := mreqWatchLookupEnv(t)
	settings := env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute, memberreq.RuleChannel: models.RequirementActionMute})
	channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
	seen := env.mreqWatchGroups()
	member := func(photo mreqPhotoAnswer) gotgbot.User {
		m := mreqLookupMember(t, env, state, photo)
		state.setMember(channel, m.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		return m
	}
	text := func(u gotgbot.User) *gotgbot.Message {
		return &gotgbot.Message{Chat: env.chat, From: &u, Text: "hello", Date: time.Now().Unix()}
	}

	// run drives msg through the watcher directly and then through the real dispatcher, and
	// requires ext.ContinueGroups from the first and both neighbouring groups from the second.
	run := func(t *testing.T, msg *gotgbot.Message) {
		t.Helper()
		if err := memberReqModule.memberReqWatcher(env.bot, env.mreqWatcherCtx(msg)); !errors.Is(err, ext.ContinueGroups) {
			t.Errorf("watcher returned %v, want ext.ContinueGroups", err)
		}
		before, beforeLate := seen.count("g-1"), seen.count("g3")
		env.mreqProcess(msg)
		memberReqJobs.wait()
		if seen.count("g-1") != before+1 || seen.count("g3") != beforeLate+1 {
			t.Error("the update did not reach both group -1 and group 3")
		}
	}

	t.Run("a job handed off", func(t *testing.T) { run(t, text(member(mreqPhotoAnswer{}))) })
	t.Run("a job shed by a full runner", func(t *testing.T) {
		mreqSwap(t, &memberReqMaxInFlight, 0)
		run(t, text(member(mreqPhotoAnswer{})))
	})
	t.Run("a clean member from the cache", func(t *testing.T) {
		m := member(mreqHasPhoto)
		mreqSetKey(t, env, memberReqPhotoKey(m.Id), memberReqPositiveTTL)
		if err := env.redis.Set(memberReqLinkedKey(env.chat.Id), strconv.FormatInt(channel, 10)); err != nil {
			t.Fatalf("set lc: %v", err)
		}
		env.redis.SetTTL(memberReqLinkedKey(env.chat.Id), memberReqLinkedTTL)
		mreqSetKey(t, env, memberReqSubKey(channel, m.Id), memberReqPositiveTTL)
		mreqStorePass(t, env, m, settings)
		run(t, text(m))
	})
	t.Run("an exempt member", func(t *testing.T) {
		m := member(mreqPhotoAnswer{})
		env.mreqPromote(m, gotgbot.ChatMemberStatusAdministrator)
		env.mreqAdminList()
		run(t, text(m))
	})
	t.Run("a Redis failure", func(t *testing.T) {
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		run(t, text(member(mreqHasPhoto)))
	})
	t.Run("a recovered panic", func(t *testing.T) {
		mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
			panic("injected into the approvals seam")
		})
		run(t, text(member(mreqPhotoAnswer{})))
	})
}

// mreqChannelID mints a channel ID whose sub: and lc: keys are deleted when the test ends.
func mreqChannelID(t *testing.T) int64 {
	t.Helper()
	id := uniqueModuleChatID()
	memberReqCleanup(t, []int64{id}, nil)
	return id
}
