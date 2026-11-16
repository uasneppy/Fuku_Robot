//go:build testtools

package modules

import (
	"context"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqHeldEnv builds a lookup environment whose group deletes a held member's messages.
func mreqHeldEnv(t *testing.T) (*memberReqEnv, *mreqLookupState) {
	t.Helper()
	env, state := mreqLookupEnv(t)
	env.mreqDeleteMessages(true)
	return env, state
}

// mreqSeedHold stores, through the real repository, a hold of user that was claimed and
// confirmed punished age ago, so a message dated now is more than the clean predicate's two
// second margin newer than it unless age is zero.
func mreqSeedHeld(t *testing.T, env *memberReqEnv, user gotgbot.User, rules, action int, age time.Duration) {
	t.Helper()
	created := time.Now().Add(-age)
	hold := &models.ChatRequirementHold{
		ChatID: env.chat.Id, UserID: user.Id, Rules: rules, Action: action,
		CreatedAt: created, PunishedAt: &created,
	}
	if action == models.RequirementActionMute {
		hold.MuteUntil = memberReqMuteUntil(created)
	}
	won, err := memberreq.ClaimHold(context.Background(), hold)
	if err != nil || !won {
		t.Fatalf("seed hold: won = %t, error = %v", won, err)
	}
}

// mreqHeldNoPunishment fails the test when the executor wrote or posted anything for member: a
// Telegram write (restrict, ban or unban), a message to the group or a warn.
func mreqHeldNoPunishment(t *testing.T, env *memberReqEnv, member gotgbot.User) {
	t.Helper()
	if n := len(env.mreqWrites()); n != 0 {
		t.Errorf("ban, restrict and unban requests = %d, want none", n)
	}
	if n := len(env.mreqNotices()); n != 0 {
		t.Errorf("messages posted = %d, want none", n)
	}
	if row := env.mreqWarnRow(member.Id); row != nil {
		t.Errorf("warns row = %+v, want none", row)
	}
}

// mreqNoLastName mints a member whose names break the last name rule and whose photo answer is
// the given one.
func mreqNoLastName(t *testing.T, env *memberReqEnv, state *mreqLookupState, photo mreqPhotoAnswer) gotgbot.User {
	t.Helper()
	member := env.mreqMember("Alex", "", "alex")
	mreqLookupCleanup(t, []int64{member.Id}, nil)
	state.set(member.Id, photo)
	return member
}

func TestMemberReqLookupHeldPath(t *testing.T) {
	t.Run("a hold with name bits only and names that still break makes no lookup and deletes the message", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RulePhoto:    models.RequirementActionWarn,
			memberreq.RuleChannel:  models.RequirementActionWarn,
		})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := mreqNoLastName(t, env, state, mreqHasPhoto)
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ln"), models.RequirementActionWarn, time.Hour)
		chats := mreqChatCalls(env, env.chat.Id)

		c := env.mreqCase(member)
		got := env.mreqEnforce(c)

		if got.Outcome != memberReqHeld || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Held with the hold", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none", n)
		}
		if n := mreqMemberCalls(env, channel, member.Id) + mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
			t.Errorf("channel getChatMember requests = %d, want none", n)
		}
		if after := mreqChatCalls(env, env.chat.Id); after != chats {
			t.Errorf("getChat requests of the group = %d, want still %d", after, chats)
		}
		if deleted := env.mreqDeleted(); len(deleted) != 1 || deleted[0] != c.MessageID {
			t.Errorf("deleted messages = %v, want only %d (the held path)", deleted, c.MessageID)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold is gone")
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("a hold with a lookup bit and names that still break makes no lookup either", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RulePhoto:    models.RequirementActionWarn,
		})
		member := mreqNoLastName(t, env, state, mreqHasPhoto)
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ph"), models.RequirementActionWarn, time.Hour)

		c := env.mreqCase(member)
		if got := env.mreqEnforce(c); got.Outcome != memberReqHeld {
			t.Fatalf("outcome = %+v, want Held", got)
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none: a member whose names break spends no lookup", n)
		}
		if deleted := env.mreqDeleted(); len(deleted) != 1 || deleted[0] != c.MessageID {
			t.Errorf("deleted messages = %v, want only %d", deleted, c.MessageID)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold is gone")
		}
	})
}

func TestMemberReqHeldRelease(t *testing.T) {
	photoBit := func(t *testing.T) int { return mreqBitOf(t, "ph") }

	t.Run("a warn hold for the photo is released by a message of a member who now has one", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.loadMemberReq()
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionWarn, time.Hour)

		env.mreqSend(member, "now I have a photo")
		memberReqJobs.wait()

		if env.mreqHold(member.Id) != nil {
			t.Error("the hold survived the message of a member who fixed the photo")
		}
		if !env.redis.Exists(memberReqPhotoKey(member.Id)) {
			t.Error("the photo positive was not cached by the release lookup")
		}
		if deleted := env.mreqDeleted(); len(deleted) != 0 {
			t.Errorf("deleted messages = %v, want none: the member is clean now", deleted)
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("a member still without a photo keeps the hold, is asked again at the next message and is never cached", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionWarn, time.Hour)

		first := env.mreqCase(member)
		if got := env.mreqEnforce(first); got.Outcome != memberReqHeld {
			t.Fatalf("first outcome = %+v, want Held", got)
		}
		second := env.mreqCase(member)
		if got := env.mreqEnforce(second); got.Outcome != memberReqHeld {
			t.Fatalf("second outcome = %+v, want Held", got)
		}

		if n := mreqLookupPhotoCalls(env); n != 2 {
			t.Errorf("getUserProfilePhotos requests = %d, want one per message (2): a missing photo is never cached", n)
		}
		if env.redis.Exists(memberReqPhotoKey(member.Id)) {
			t.Error("a photo key exists for a member with no photo")
		}
		if deleted := env.mreqDeleted(); len(deleted) != 2 || deleted[0] != first.MessageID || deleted[1] != second.MessageID {
			t.Errorf("deleted messages = %v, want both held messages", deleted)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold is gone")
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("an unanswerable photo lookup keeps the hold", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.script(member.Id, staffFakeError(500, "Internal Server Error"))
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionWarn, time.Hour)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Fatalf("outcome = %+v, want Held", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was released on a lookup that could not be answered")
		}
	})

	t.Run("a hold whose lookup rule is now Off is released with no lookup", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RulePhoto:    models.RequirementActionOff,
		})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionWarn, time.Hour)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNone {
			t.Fatalf("outcome = %+v, want None", got)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("the hold of a rule that is Off survived")
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none", n)
		}
	})

	t.Run("a hold with name bits only is released when the names are clean now, with no lookup", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RulePhoto:    models.RequirementActionWarn,
		})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ln"), models.RequirementActionWarn, time.Hour)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNone {
			t.Fatalf("outcome = %+v, want None", got)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("the hold of a fixed name survived: with a lookup rule on, no clean path would ever end it")
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none: a name hold needs no lookup", n)
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("a hold for a name and the photo is released only when both are fixed", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RulePhoto:    models.RequirementActionWarn,
		})
		both := mreqBitOf(t, "ln") | photoBit(t)
		fixedName := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		mreqSeedHeld(t, env, fixedName, both, models.RequirementActionWarn, time.Hour)
		fixedBoth := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSeedHeld(t, env, fixedBoth, both, models.RequirementActionWarn, time.Hour)

		if got := env.mreqEnforce(env.mreqCase(fixedName)); got.Outcome != memberReqHeld {
			t.Errorf("name fixed, no photo: outcome = %+v, want Held", got)
		}
		if got := env.mreqEnforce(env.mreqCase(fixedBoth)); got.Outcome != memberReqNone {
			t.Errorf("name and photo fixed: outcome = %+v, want None", got)
		}
		if env.mreqHold(fixedName.Id) == nil || env.mreqHold(fixedBoth.Id) != nil {
			t.Error("the holds are not as expected: the first stays, the second is gone")
		}
	})

	t.Run("an unreadable setting keeps the hold", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionWarn, time.Hour)
		c := env.mreqCase(member)
		mreqSwap(t, &memberReqSettingsLookup, func(context.Context, int64) (*models.ChatRequirements, error) {
			return nil, context.DeadlineExceeded
		})

		if got := env.mreqEnforce(c); got.Outcome != memberReqHeld {
			t.Fatalf("outcome = %+v, want Held", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold was released on a setting that could not be read")
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none", n)
		}
	})

	t.Run("a punished mute hold with its marker is released without any unmute once an admin lifted the mute", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionMute, time.Hour)

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNone {
			t.Fatalf("outcome = %+v, want None", got)
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("the old mute hold survived a message of a member who has a photo")
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("the two second margin still protects a hold created at the time of the message", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionMute, 0)

		c := env.mreqCase(member)
		if got := env.mreqEnforce(c); got.Outcome != memberReqHeld {
			t.Fatalf("outcome = %+v, want Held", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("a hold created within two seconds of the message was deleted")
		}
		if deleted := env.mreqDeleted(); len(deleted) != 1 || deleted[0] != c.MessageID {
			t.Errorf("deleted messages = %v, want only %d", deleted, c.MessageID)
		}
		mreqHeldNoPunishment(t, env, member)
	})

	t.Run("a join case is never released and makes no lookup", func(t *testing.T) {
		env, state := mreqHeldEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionWarn})
		member := mreqLookupMember(t, env, state, mreqHasPhoto)
		mreqSeedHeld(t, env, member, photoBit(t), models.RequirementActionWarn, time.Hour)
		join := env.mreqCase(member)
		join.Source, join.MessageID, join.MessageDate = memberReqFromJoin, 0, 0

		hold := env.mreqHold(member.Id)
		if memberReqHeldReleased(context.Background(), env.bot, join, hold) {
			t.Error("a join case was released")
		}
		if got := env.mreqEnforce(join); got.Outcome != memberReqHeld {
			t.Errorf("outcome = %+v, want Held", got)
		}
		if env.mreqHold(member.Id) == nil {
			t.Error("the hold is gone")
		}
		if n := mreqLookupPhotoCalls(env); n != 0 {
			t.Errorf("getUserProfilePhotos requests = %d, want none", n)
		}
	})
}

func TestMemberReqHeldReleaseChannel(t *testing.T) {
	channelBit := func(t *testing.T) int { return mreqBitOf(t, "ch") }
	type shape struct {
		name     string
		released bool
		setup    func(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User) int64
	}
	// linked links a fresh channel where the bot has botStatus and the member has memberStatus.
	linked := func(botStatus, memberStatus string) func(*testing.T, *memberReqEnv, *mreqLookupState, gotgbot.User) int64 {
		return func(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User) int64 {
			t.Helper()
			channel := mreqChannel(t, state, env.chat.Id, botStatus)
			state.setMember(channel, member.Id, mreqChannelMember{Status: memberStatus})
			return channel
		}
	}
	shapes := []shape{
		{"the member is subscribed", true, linked(gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)},
		{"the group has no linked channel (inert)", true, func(t *testing.T, env *memberReqEnv, state *mreqLookupState, _ gotgbot.User) int64 {
			t.Helper()
			state.link(env.chat.Id, 0)
			t.Cleanup(func() { memberReqDeleteRedisKeys([]int64{env.chat.Id}) })
			return 0
		}},
		{"the bot is not an administrator of the channel (inert)", true, linked(gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft)},
		{"Telegram refuses both lookups with 403 (inert)", true, func(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User) int64 {
			t.Helper()
			channel := linked(gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)(t, env, state, member)
			state.scriptMember(channel, member.Id, staffFakeError(403, "Forbidden"))
			state.scriptMember(channel, staffTestBotID, staffFakeError(403, "Forbidden"))
			return channel
		}},
		{"the member left a channel the bot administers", false, linked(gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)},
		{"the member lookup fails with a 500 and the bot administers the channel", false, func(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User) int64 {
			t.Helper()
			channel := linked(gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)(t, env, state, member)
			state.scriptMember(channel, member.Id, staffFakeError(500, "Internal Server Error"))
			return channel
		}},
		{"both lookups fail with a 500", false, func(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User) int64 {
			t.Helper()
			channel := linked(gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)(t, env, state, member)
			state.scriptMember(channel, member.Id, staffFakeError(500, "Internal Server Error"))
			state.scriptMember(channel, staffTestBotID, staffFakeError(500, "Internal Server Error"))
			return channel
		}},
		{"the member lookup does not answer in time", false, func(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User) int64 {
			t.Helper()
			channel := linked(gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)(t, env, state, member)
			release := state.holdKey(mreqChannelKey("getChatMember", channel, member.Id))
			t.Cleanup(release)
			mreqSwap(t, &memberReqLookupJobBudget, 150*time.Millisecond)
			return channel
		}},
	}
	for _, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			env, state := mreqHeldEnv(t)
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionWarn})
			member := mreqLookupMember(t, env, state, mreqHasPhoto)
			s.setup(t, env, state, member)
			mreqSeedHeld(t, env, member, channelBit(t), models.RequirementActionWarn, time.Hour)

			c := env.mreqCase(member)
			got := env.mreqEnforce(c)

			if s.released {
				if got.Outcome != memberReqNone || env.mreqHold(member.Id) != nil {
					t.Errorf("outcome = %+v, hold = %+v, want the hold released", got, env.mreqHold(member.Id))
				}
				if deleted := env.mreqDeleted(); len(deleted) != 0 {
					t.Errorf("deleted messages = %v, want none", deleted)
				}
			} else {
				if got.Outcome != memberReqHeld || env.mreqHold(member.Id) == nil {
					t.Errorf("outcome = %+v, hold = %+v, want the hold kept", got, env.mreqHold(member.Id))
				}
				if deleted := env.mreqDeleted(); len(deleted) != 1 || deleted[0] != c.MessageID {
					t.Errorf("deleted messages = %v, want only %d (the held path)", deleted, c.MessageID)
				}
			}
			mreqHeldNoPunishment(t, env, member)
		})
	}
}
