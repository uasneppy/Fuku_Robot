//go:build testtools

package modules

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqCapKeys returns the cap counters of one chat in the environment's Redis.
func (e *memberReqEnv) mreqCapKeys(chatID int64) []string {
	e.t.Helper()
	prefix := memberReqCapKey(chatID, 0)
	prefix = prefix[:strings.LastIndex(prefix, ":")+1]
	var keys []string
	for _, key := range e.redis.Keys() {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys
}

// mreqPausedNotices is the number of pause notices the environments' fakes sent.
func mreqPausedNotices(envs ...*memberReqEnv) int {
	return mreqNoticeCount(staffMarker("mreq_notice_paused"), envs...)
}

// mreqFixedClock stops the clock the cap counts minutes with at a minute boundary plus 10 s, so
// a test never straddles a window.
func mreqFixedClock(t *testing.T) *time.Time {
	t.Helper()
	now := time.Now().Truncate(time.Minute).Add(10 * time.Second)
	mreqSwap(t, &raidNow, func() time.Time { return now })
	return &now
}

func TestMemberReqCapPausesPunishments(t *testing.T) {
	t.Run("two are punished, the third pauses with one notice, the fourth does nothing", func(t *testing.T) {
		mreqSwap(t, &memberReqCapPerMinute, 2)
		mreqFixedClock(t)
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		members := make([]gotgbot.User, 4)
		for i := range members {
			members[i] = env.mreqMember("Alex", "", "alex")
		}

		for i := 0; i < 2; i++ {
			if got := env.mreqEnforce(env.mreqCase(members[i])); got.Outcome != memberReqMuted {
				t.Fatalf("member %d = %+v, want Muted", i, got)
			}
		}
		if env.redis.Exists(memberReqPausedKey(env.chat.Id)) {
			t.Fatal("the pause latch is set before the cap was exceeded")
		}

		if got := env.mreqEnforce(env.mreqCase(members[2])); got.Outcome != memberReqNone {
			t.Fatalf("third member = %+v, want None: the cap refuses the punishment", got)
		}
		if !env.redis.Exists(memberReqPausedKey(env.chat.Id)) {
			t.Fatal("the cap did not set the pause latch")
		}
		if ttl := env.redis.TTL(memberReqPausedKey(env.chat.Id)); ttl != memberReqPause {
			t.Errorf("pause latch TTL = %s, want %s", ttl, memberReqPause)
		}
		if env.mreqHold(members[2].Id) != nil || mreqRestrictCalls(members[2].Id, env) != 0 {
			t.Error("the member over the cap was held or restricted")
		}

		if got := env.mreqEnforce(env.mreqCase(members[3])); got.Outcome != memberReqNone {
			t.Fatalf("fourth member = %+v, want None during the pause", got)
		}
		if env.mreqHold(members[3].Id) != nil || mreqRestrictCalls(members[3].Id, env) != 0 {
			t.Error("a member during the pause was held or restricted")
		}

		if n := mreqPausedNotices(env); n != 1 {
			t.Fatalf("pause notices = %d, want exactly one", n)
		}
		var paused string
		for _, sent := range env.mreqNotices() {
			if text := mreqText(sent); strings.Contains(text, staffMarker("mreq_notice_paused")) {
				paused = text
			}
		}
		if got, want := strings.Fields(paused), []string{staffMarker("mreq_notice_paused"), "10", "2"}; strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("pause notice = %q, want the key with minutes 10 and n 2", paused)
		}
		if n := mreqNoticeCount(staffMarker("mreq_notice_mute"), env); n != 2 {
			t.Errorf("mute notices = %d, want one for each of the two muted members (the cap never drops a due notice)", n)
		}
		if keys := env.mreqCapKeys(env.chat.Id); len(keys) != 1 {
			t.Errorf("cap counters = %v, want one for the minute", keys)
		} else if ttl := env.redis.TTL(keys[0]); ttl != 120*time.Second {
			t.Errorf("cap counter TTL = %s, want 120s", ttl)
		}
	})

	t.Run("another chat in the same Redis is not paused", func(t *testing.T) {
		mreqSwap(t, &memberReqCapPerMinute, 1)
		mreqFixedClock(t)
		env := newMemberReqEnv(t)
		other := env.addChat()
		memberReqCleanup(t, []int64{other.Id}, nil)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick})
		if err := memberreq.SetRuleAction(context.Background(), other.Id, memberreq.RuleLastName, models.RequirementActionKick); err != nil {
			t.Fatalf("SetRuleAction error = %v", err)
		}
		otherSettings, err := memberreq.GetSettingsFresh(context.Background(), other.Id)
		if err != nil {
			t.Fatalf("GetSettingsFresh error = %v", err)
		}
		inOther := func() memberReqResult {
			member := env.mreqMember("Alex", "", "alex")
			env.fake.setMember(other.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember, IsMember: true})
			return memberReqEnforce(context.Background(), env.bot, memberReqCase{
				Chat: other, User: member, Source: memberReqFromMessage,
				MessageID: env.updateID(), MessageDate: time.Now().Unix(), Settings: otherSettings,
			})
		}

		env.mreqEnforce(env.mreqCase(env.mreqMember("Alex", "", "alex")))
		if got := env.mreqEnforce(env.mreqCase(env.mreqMember("Alex", "", "alex"))); got.Outcome != memberReqNone {
			t.Fatalf("second member of the first chat = %+v, want None over the cap", got)
		}
		if !env.redis.Exists(memberReqPausedKey(env.chat.Id)) {
			t.Fatal("the first chat is not paused")
		}
		if got := inOther(); got.Outcome != memberReqRemoved {
			t.Errorf("first member of the other chat = %+v, want Removed: its own cap is untouched", got)
		}
		if env.redis.Exists(memberReqPausedKey(other.Id)) {
			t.Error("the other chat was paused by the first chat's cap")
		}
	})

	t.Run("a new minute counts afresh", func(t *testing.T) {
		mreqSwap(t, &memberReqCapPerMinute, 2)
		clock := mreqFixedClock(t)
		env := newMemberReqEnv(t)
		allow := func() bool { return memberReqCapAllow(context.Background(), env.bot, env.chat.Id) }

		first, second, third := allow(), allow(), allow()
		if !first || !second || third {
			t.Fatalf("attempts within one minute = %t, %t, %t, want the first two allowed and the third refused", first, second, third)
		}
		env.redis.Del(memberReqPausedKey(env.chat.Id))
		*clock = clock.Add(time.Minute)
		first, second = allow(), allow()
		if !first || !second {
			t.Errorf("attempts of the next minute = %t, %t, want both allowed", first, second)
		}
		if keys := env.mreqCapKeys(env.chat.Id); len(keys) != 2 {
			t.Errorf("cap counters = %v, want one per minute", keys)
		}
	})

	t.Run("two replicas tipping the cap at once post one notice", func(t *testing.T) {
		mreqSwap(t, &memberReqCapPerMinute, 2)
		mreqFixedClock(t)
		env := newMemberReqEnv(t)
		replica := env.mreqReplica()
		for i := 0; i < 2; i++ {
			if !memberReqCapAllow(context.Background(), env.bot, env.chat.Id) {
				t.Fatalf("attempt %d under the cap was refused", i)
			}
		}

		var wg sync.WaitGroup
		results := make(chan bool, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			bot := env.bot
			if i%2 == 1 {
				bot = replica.bot
			}
			go func() {
				defer wg.Done()
				results <- memberReqCapAllow(context.Background(), bot, env.chat.Id)
			}()
		}
		wg.Wait()
		close(results)
		for allowed := range results {
			if allowed {
				t.Error("an attempt over the cap was allowed")
			}
		}
		if n := mreqPausedNotices(env, replica); n != 1 {
			t.Errorf("pause notices over two replicas = %d, want exactly one", n)
		}
	})

	t.Run("a refused member's message is still deleted", func(t *testing.T) {
		mreqSwap(t, &memberReqCapPerMinute, 1)
		mreqFixedClock(t)
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick})
		env.mreqDeleteMessages(true)
		env.mreqEnforce(env.mreqCase(env.mreqMember("Alex", "", "alex")))

		over := env.mreqMember("Bo", "", "bo")
		c := env.mreqCase(over)
		if got := env.mreqEnforce(c); got.Outcome != memberReqNone {
			t.Fatalf("member over the cap = %+v, want None", got)
		}
		deleted := env.mreqDeleted()
		if len(deleted) != 2 || deleted[1] != c.MessageID {
			t.Errorf("deleted = %v, want the message of the member over the cap too", deleted)
		}
		if env.mreqHold(over.Id) != nil {
			t.Error("the member over the cap was held")
		}
	})
}

func TestMemberReqCapWithoutRedis(t *testing.T) {
	mreqSwap(t, &memberReqCapPerMinute, 2)
	env := newMemberReqEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
	env.mreqDeleteMessages(true)
	mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })

	for i := 0; i < 5; i++ {
		member := env.mreqMember("Alex", "", "alex")
		c := env.mreqCase(member)
		if got := env.mreqEnforce(c); got.Outcome != memberReqMuted {
			t.Fatalf("member %d = %+v, want Muted: with no Redis there is no cap", i, got)
		}
		if calls := mreqRestrictCalls(member.Id, env); calls != 1 {
			t.Errorf("member %d: restrictChatMember calls = %d, want exactly one", i, calls)
		}
		if deleted := env.mreqDeleted(); len(deleted) != i+1 || deleted[i] != c.MessageID {
			t.Errorf("member %d: deleted = %v, want every message so far", i, deleted)
		}
	}
	if n := mreqPausedNotices(env); n != 0 {
		t.Errorf("pause notices = %d, want none without Redis", n)
	}
	if env.redis.Exists(memberReqPausedKey(env.chat.Id)) || len(env.mreqCapKeys(env.chat.Id)) != 0 {
		t.Error("a cap counter or a pause latch was written with the Redis client removed")
	}
}
