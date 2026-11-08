//go:build testtools

package modules

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqSeedLink stores the cached linked channel of the environment's group, as an earlier read
// would have left it.
func mreqSeedLink(t *testing.T, env *memberReqEnv, channel int64, ttl time.Duration) {
	t.Helper()
	mreqSetValue(t, env, memberReqLinkedKey(env.chat.Id), strconv.FormatInt(channel, 10), ttl)
}

// mreqAnotherChannel makes a second channel where the bot is an administrator and member is
// gone, so a punishment that rested on it would be a breach, and returns its ID.
func mreqAnotherChannel(t *testing.T, state *mreqLookupState, member gotgbot.User) int64 {
	t.Helper()
	other := uniqueModuleChatID()
	state.setMember(other, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusAdministrator})
	state.setMember(other, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
	mreqLookupCleanup(t, nil, []int64{other})
	return other
}

// TestMemberReqStaleLink is WR-05: a punishment never rests on a linked-channel fact older than
// one request, so an owner who unlinks or relinks the group is never punished for the old channel.
func TestMemberReqStaleLink(t *testing.T) {
	t.Run("a positive link is cached for 60 seconds and none for 10 minutes", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		if got, known := mreqLinked(env, memberReqLookupMode{}); !known || got != channel {
			t.Fatalf("linked channel = (%d, %v), want (%d, true)", got, known, channel)
		}
		if ttl := env.redis.TTL(memberReqLinkedKey(env.chat.Id)); ttl <= 50*time.Second || ttl > time.Minute {
			t.Errorf("TTL of a positive link = %s, want about 60 s", ttl)
		}

		none, noneState := mreqLookupEnv(t)
		noneState.link(none.chat.Id, 0)
		mreqLookupCleanup(t, nil, []int64{none.chat.Id})
		if got, known := mreqLinked(none, memberReqLookupMode{}); !known || got != 0 {
			t.Fatalf("none link = (%d, %v), want (0, true)", got, known)
		}
		if ttl := none.redis.TTL(memberReqLinkedKey(none.chat.Id)); ttl < 9*time.Minute || ttl > 10*time.Minute {
			t.Errorf("TTL of none = %s, want about 10 minutes", ttl)
		}
	})

	t.Run("a relink between the cached read and the breach punishes nobody and the new link is judged next", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, oldChannel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		mreqSeedLink(t, env, oldChannel, time.Minute)
		newChannel := mreqAnotherChannel(t, state, member)
		state.link(env.chat.Id, newChannel)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if value, err := env.redis.Get(memberReqLinkedKey(env.chat.Id)); err != nil || value != strconv.FormatInt(newChannel, 10) {
			t.Errorf("cached link = %q (%v), want the new channel %d after the fresh read", value, err, newChannel)
		}

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved {
			t.Fatalf("second outcome = %+v, want Removed: the new link is the one judged", got)
		}
		if n := mreqMemberCalls(env, oldChannel, member.Id); n != 1 {
			t.Errorf("getChatMember requests for the old channel = %d, want 1 (only the first, stale attempt)", n)
		}
	})

	t.Run("an unlink between the cached read and the breach punishes nobody", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		mreqSeedLink(t, env, channel, time.Minute)
		state.link(env.chat.Id, 0)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if value, err := env.redis.Get(memberReqLinkedKey(env.chat.Id)); err != nil || value != "0" {
			t.Errorf("cached link = %q (%v), want none", value, err)
		}
	})

	t.Run("an unreadable link on the breach path is unknown and the next message judges again", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		mreqSeedLink(t, env, channel, time.Minute)
		state.scriptChat(env.chat.Id, staffFakeError(500, "Internal Server Error"))

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqRemoved {
			t.Fatalf("second outcome = %+v, want Removed: the link reads fine now and has not changed", got)
		}
	})

	t.Run("an unchanged link keeps the breach and costs exactly one fresh getChat", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		mreqSeedLink(t, env, channel, time.Minute)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		if n := mreqChatCalls(env, env.chat.Id); n != 1 {
			t.Errorf("getChat requests for the group = %d, want 1 (the fresh re-read before the breach)", n)
		}
	})

	t.Run("a subscribed member costs no extra getChat", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)
		mreqSeedLink(t, env, channel, time.Minute)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if n := mreqChatCalls(env, env.chat.Id); n != 0 {
			t.Errorf("getChat requests for the group = %d, want none for a pass", n)
		}
	})
}

// mreqMissFirstRedisRead makes the first Redis client lookup of the lookup layer answer "no
// Redis" (a cache miss) and every later one the real client, so a flight that reads its cache
// again before asking Telegram finds what a previous flight wrote in between.
func mreqMissFirstRedisRead(t *testing.T) {
	t.Helper()
	real := memberReqRedisClient
	var calls atomic.Int64
	mreqSwap(t, &memberReqRedisClient, func() *redis.Client {
		if calls.Add(1) == 1 {
			return nil
		}
		return real()
	})
}

// TestMemberReqFlightsReadCacheAgain is IN-03: the linked-channel and bot-status flights read
// their cache once more inside the flight, like the photo and subscription flights, so a caller
// that missed the cache just before the previous flight wrote it makes no second request.
func TestMemberReqFlightsReadCacheAgain(t *testing.T) {
	t.Run("the linked channel", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		mreqSeedLink(t, env, channel, time.Minute)
		mreqMissFirstRedisRead(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		got, known := memberReqLinkedChannel(ctx, env.bot, env.chat.Id, memberReqLookupMode{})
		if !known || got != channel {
			t.Fatalf("linked channel = (%d, %v), want (%d, true)", got, known, channel)
		}
		if n := mreqChatCalls(env, env.chat.Id); n != 0 {
			t.Errorf("getChat requests = %d, want none: the flight reads the cache again", n)
		}
	})

	t.Run("the bot's status", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		mreqSetKey(t, env, memberReqBotAdminKey(channel), time.Minute)
		mreqMissFirstRedisRead(t)

		if got := mreqBotStatus(env, channel, memberReqLookupMode{}); got != memberReqBotAdmin {
			t.Fatalf("status = %d, want administrator", got)
		}
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
			t.Errorf("bot status requests = %d, want none: the flight reads the cache again", n)
		}
	})
}
