//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// mreqPhotoMuted builds the lookup environment with the real watcher loaded, sets the photo
// rule to Mute and mutes a member whose names break nothing and whose photo is missing, through
// the real dispatcher. It returns the member, their hold and the mute notice, with the lookup
// fake still answering "no photo" for them.
func mreqPhotoMuted(t *testing.T) (*memberReqEnv, *mreqLookupState, gotgbot.User, *models.ChatRequirementHold, staffSentMessage) {
	t.Helper()
	env, state := mreqWatchLookupEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
	member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
	notice := env.mreqMuteByMessage(member)
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Rules != mreqBitOf(t, "ph") || hold.PunishedAt == nil {
		t.Fatalf("hold after the mute = %+v, want a punished hold with the photo bit", hold)
	}
	return env, state, member, hold, notice
}

func TestMemberReqFixedLookups(t *testing.T) {
	t.Run("a member who added a photo is freed by a fresh lookup that ignores a stale positive", func(t *testing.T) {
		env, state, member, _, notice := mreqPhotoMuted(t)
		state.set(member.Id, mreqHasPhoto)
		// A stale positive from before the member removed their photo: one minute left.
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), time.Minute)
		photoBefore := mreqLookupPhotoCalls(env)
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if n := mreqLookupPhotoCalls(env); n != photoBefore+1 {
			t.Errorf("getUserProfilePhotos requests = %d, want %d: the press must ask again and not trust the cached positive", n, photoBefore+1)
		}
		if ttl := env.redis.TTL(memberReqPhotoKey(member.Id)); ttl < 10*time.Minute {
			t.Errorf("photo key ttl = %s, want the positive refreshed to %s", ttl, memberReqPositiveTTL)
		}
	})

	t.Run("a member whose photo is still missing is told so and the stale positive is deleted", func(t *testing.T) {
		env, _, member, hold, notice := mreqPhotoMuted(t)
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), memberReqPositiveTTL)
		data := env.mreqFixedData(notice)

		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_still_broken", staffMarker("mreq_rule_photo"))

		if env.redis.Exists(memberReqPhotoKey(member.Id)) {
			t.Error("the stale photo positive is still cached after a definite breach")
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("broken first", mreqFixedLookupsBrokenFirst)
	t.Run("a lookup that cannot be answered", mreqFixedLookupsUnanswerable)
	t.Run("slow lookups", mreqFixedLookupsSlow)
	t.Run("real locale", mreqFixedLookupsRealLocale)
}

// mreqChannelMuted builds the lookup environment with the real watcher loaded, sets the channel
// rule to Mute and mutes a member who left a channel the bot administers, through the real
// dispatcher. It returns the member, the channel ID, their hold and the mute notice.
func mreqChannelMuted(t *testing.T) (*memberReqEnv, *mreqLookupState, gotgbot.User, int64, *models.ChatRequirementHold, staffSentMessage) {
	t.Helper()
	env, state := mreqWatchLookupEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionMute})
	member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
	notice := env.mreqMuteByMessage(member)
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Rules != mreqBitOf(t, "ch") || hold.PunishedAt == nil {
		t.Fatalf("hold after the mute = %+v, want a punished hold with the channel bit", hold)
	}
	return env, state, member, channel, hold, notice
}

// mreqPressTrace records, in order, which seams one press reached.
type mreqPressTrace struct {
	mu     sync.Mutex
	events []string
	// liveAtApprovals is how many getChatMember requests about the member had been made when the
	// approvals read ran: more than before the press means the live marker check came first.
	liveAtApprovals int
}

func (p *mreqPressTrace) add(event string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *mreqPressTrace) log() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

// mreqTracePress wraps the approvals read, the settings read and the lookup requests of the
// environment, so a test can read the order they ran in. Install it after the member was muted.
func mreqTracePress(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User) *mreqPressTrace {
	t.Helper()
	trace := &mreqPressTrace{}
	approved := memberReqApprovedLookup
	mreqSwap(t, &memberReqApprovedLookup, func(ctx context.Context, chatID, userID int64) (bool, error) {
		trace.mu.Lock()
		trace.liveAtApprovals = env.mreqCallsAbout(member.Id, "getChatMember")
		trace.mu.Unlock()
		trace.add("approvals")
		return approved(ctx, chatID, userID)
	})
	settings := memberReqSettingsLookup
	mreqSwap(t, &memberReqSettingsLookup, func(ctx context.Context, chatID int64) (*models.ChatRequirements, error) {
		trace.add("settings")
		return settings(ctx, chatID)
	})
	state.setOnPhoto(func(user int64) {
		if user == member.Id {
			trace.add("photo")
		}
	})
	state.setOnChannel(func(method string, chat, _ int64) {
		// The getChat of the group itself is the unmute's own permissions read, not a lookup.
		if method == "getChat" && chat == env.chat.Id {
			return
		}
		trace.add("channel:" + method)
	})
	return trace
}

// mreqApprove approves the member in the group through the real repository.
func mreqApprove(t *testing.T, env *memberReqEnv, member gotgbot.User) {
	t.Helper()
	if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
		t.Fatalf("approve %d: %v", member.Id, err)
	}
}

// mreqLookupTraffic counts the requests a lookup makes about the member or their channel: the
// photo requests and every getChat or getChatMember of the channel (the bot's own included).
func mreqLookupTraffic(env *memberReqEnv, channel, member int64) int {
	n := mreqLookupPhotoCalls(env)
	if channel != 0 {
		n += mreqMemberCalls(env, channel, member) + mreqMemberCalls(env, channel, staffTestBotID)
	}
	return n
}

func TestMemberReqFixedApproved(t *testing.T) {
	// freed presses the button of an approved, still-broken member and fails unless they were
	// freed with the done toast, the approvals read being the only seam reached.
	freed := func(t *testing.T, env *memberReqEnv, state *mreqLookupState, member gotgbot.User, notice staffSentMessage, channel int64, perms gotgbot.ChatPermissions) {
		t.Helper()
		mreqApprove(t, env, member)
		trace := mreqTracePress(t, env, state, member)
		trafficBefore := mreqLookupTraffic(env, channel, member.Id)
		liveBefore := env.mreqCallsAbout(member.Id, "getChatMember")
		chatsBefore := mreqChatCalls(env, env.chat.Id)
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		// The one getChat of the group is the unmute's permissions read: no linked channel lookup.
		if n := mreqChatCalls(env, env.chat.Id); n != chatsBefore+1 {
			t.Errorf("getChat requests of the group = %d, want %d (the permissions read only)", n, chatsBefore+1)
		}
		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, perms,
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if got := trace.log(); !reflect.DeepEqual(got, []string{"approvals"}) {
			t.Errorf("seams reached = %v, want only the approvals read (no settings read, no lookup)", got)
		}
		if n := mreqLookupTraffic(env, channel, member.Id); n != trafficBefore {
			t.Errorf("lookup requests = %d, want the %d before the press", n, trafficBefore)
		}
		trace.mu.Lock()
		defer trace.mu.Unlock()
		if trace.liveAtApprovals <= liveBefore {
			t.Errorf("getChatMember requests at the approvals read = %d, want more than the %d before the press: the live marker check comes first", trace.liveAtApprovals, liveBefore)
		}
	}

	t.Run("an approved member holding a name rule mute is freed with no settings read and no lookup", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		member, _, notice := mreqMutedMember(t, env)
		freed(t, env, state, member, notice, 0, lockdownSnapshotPermissions(t))
	})

	t.Run("an approved member holding a photo rule mute who has no photo is freed", func(t *testing.T) {
		env, state, member, _, notice := mreqPhotoMuted(t)
		freed(t, env, state, member, notice, 0, lockdownSnapshotPermissions(t))
	})

	t.Run("an approved member holding a channel rule mute who left the channel is freed", func(t *testing.T) {
		env, state, member, channel, _, notice := mreqChannelMuted(t)
		freed(t, env, state, member, notice, channel, defaultUnmutePermissions())
	})

	t.Run("an approved member muted on join is freed and gets the held welcome once", func(t *testing.T) {
		env := mreqJoinEnv(t, map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute}, true)
		env.loadMemberReq()
		if err := greetings.SetWelcomeOnce(env.chat.Id, true); err != nil {
			t.Fatalf("SetWelcomeOnce error = %v", err)
		}
		member := env.mreqMember("Alex", "", "alex")
		env.mreqJoin(member)
		notice := env.mreqNoticeWith(staffMarker("mreq_notice_mute"))
		if hold := env.mreqHold(member.Id); hold == nil || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a mute hold with the welcome held", hold)
		}
		mreqApprove(t, env, member)

		env.mreqPressFixed(member, notice)

		if got := mreqCountText(env.mreqSentTexts(), mreqWelcomeWord+" "+member.FirstName); got != 1 {
			t.Errorf("welcomes after the press = %d, want exactly one (texts %v)", got, env.mreqSentTexts())
		}
		if env.mreqHold(member.Id) != nil {
			t.Error("the hold is still there after an approved member was freed")
		}
	})

	t.Run("an approvals read that fails changes nothing", func(t *testing.T) {
		env, state, member, hold, notice := mreqPhotoMuted(t)
		trace := mreqTracePress(t, env, state, member)
		mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
			trace.add("approvals")
			return false, errors.New("the approvals read failed")
		})

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")

		mreqWantHeld(t, env, hold)
		if got := trace.log(); !reflect.DeepEqual(got, []string{"approvals"}) {
			t.Errorf("seams reached = %v, want only the approvals read: an unknown ends the press before any rule is judged", got)
		}
		if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
			t.Errorf("the member after the refusal = %+v, want still muted with the marker", live)
		}
	})

	t.Run("a member who is not approved goes on to step 5, settings before lookups", func(t *testing.T) {
		env, state, member, _, notice := mreqPhotoMuted(t)
		state.set(member.Id, mreqHasPhoto)
		trace := mreqTracePress(t, env, state, member)
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if got := trace.log(); !reflect.DeepEqual(got, []string{"approvals", "settings", "photo"}) {
			t.Errorf("seams reached = %v, want approvals, then the settings read, then the photo lookup", got)
		}
	})

	t.Run("a member who is not approved and still has no photo is told so", func(t *testing.T) {
		env, _, member, hold, notice := mreqPhotoMuted(t)
		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_photo"))
		mreqWantHeld(t, env, hold)
	})

	t.Run("another member and an admin are refused before the approvals read", func(t *testing.T) {
		env, state, member, hold, notice := mreqPhotoMuted(t)
		mreqApprove(t, env, member)
		other := env.mreqMember("Bo", "Li", "bo")
		trace := mreqTracePress(t, env, state, member)
		data := env.mreqFixedData(notice)
		for name, presser := range map[string]gotgbot.User{"another member": other, "an admin": env.admin} {
			mreqPressRefused(t, env, presser, member, notice.MessageID, data, "mreq_fixed_not_yours")
			if got := trace.log(); len(got) != 0 {
				t.Errorf("%s: seams reached = %v, want none", name, got)
			}
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("an approved member whose mute is not the requirements' is never freed and nothing is read", func(t *testing.T) {
		env, state, member, hold, notice := mreqPhotoMuted(t)
		mreqApprove(t, env, member)
		if _, err := env.bot.RestrictChatMember(env.chat.Id, member.Id, MutedPermissions, nil); err != nil {
			t.Fatal(err)
		}
		trace := mreqTracePress(t, env, state, member)

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_not_ours")

		if got := trace.log(); len(got) != 0 {
			t.Errorf("seams reached = %v, want none: the live marker check comes before approvals", got)
		}
		mreqWantHeld(t, env, hold)
	})
}

// mreqPressTiming presses the button and returns how long the press took.
func mreqPressTiming(env *memberReqEnv, member gotgbot.User, notice staffSentMessage) time.Duration {
	env.t.Helper()
	started := time.Now()
	env.mreqPressFixed(member, notice)
	return time.Since(started)
}

func mreqFixedLookupsBrokenFirst(t *testing.T) {
	telegram := func(code int) error { return staffFakeError(code, fmt.Sprintf("test error %d", code)) }

	t.Run("names, photo and channel all broken name exactly the three phrases", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RulePhoto:    models.RequirementActionMute,
			memberreq.RuleChannel:  models.RequirementActionMute,
		})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := env.mreqMember("Alex", "", "alex")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqPhotoAnswer{})
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken",
			staffMarker("mreq_rule_lastname"), staffMarker("mreq_rule_photo"), staffMarker("mreq_rule_channel"))

		answer := env.mreqAnswers()[len(env.mreqAnswers())-1].Text
		for _, other := range []string{"mreq_rule_username", "mreq_rule_russian", "mreq_rule_arabic", "mreq_rule_chinese"} {
			if strings.Contains(answer, staffMarker(other)) {
				t.Errorf("answer %q names %s, which is not broken", answer, other)
			}
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("five or more broken rules answer with the count and a rule that is Off is not counted", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RuleUsername: models.RequirementActionMute,
			memberreq.RuleRussian:  models.RequirementActionMute,
			memberreq.RuleArabic:   models.RequirementActionMute,
			memberreq.RulePhoto:    models.RequirementActionMute,
			memberreq.RuleChannel:  models.RequirementActionMute,
			memberreq.RuleChinese:  models.RequirementActionOff,
		})
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		// The first name breaks the Russian, Arabic and (Off) Chinese rules; no last name, no username.
		member := env.mreqMember("\u042b\u0627\u4e2d", "", "")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqPhotoAnswer{})
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)
		data := env.mreqFixedData(notice)

		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_still_broken_many", " 6")
		mreqWantHeld(t, env, hold)

		// Adding a last name leaves five.
		member.LastName = "Smith"
		env.fake.setProfile(member.Id, member.FirstName, member.LastName, member.Username)
		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_still_broken_many", " 5")
		mreqWantHeld(t, env, hold)
	})

	t.Run("a name rule still broken beats a photo lookup that cannot be answered", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RulePhoto:    models.RequirementActionMute,
		})
		member := env.mreqMember("Alex", "", "alex")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqPhotoAnswer{})
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)
		state.script(member.Id, telegram(500))

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_lastname"))

		if answer := env.mreqAnswers()[len(env.mreqAnswers())-1].Text; strings.Contains(answer, staffMarker("mreq_rule_photo")) {
			t.Errorf("answer %q names the photo rule, whose lookup was not answered", answer)
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("a definite photo breach beats an unanswerable channel lookup", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RulePhoto:   models.RequirementActionMute,
			memberreq.RuleChannel: models.RequirementActionMute,
		})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		state.set(member.Id, mreqPhotoAnswer{})
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)
		state.scriptMember(channel, member.Id, telegram(500))

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_photo"))

		if answer := env.mreqAnswers()[len(env.mreqAnswers())-1].Text; strings.Contains(answer, staffMarker("mreq_rule_channel")) {
			t.Errorf("answer %q names the channel rule, whose lookup was not answered", answer)
		}
		mreqWantHeld(t, env, hold)
	})

	t.Run("an added photo and a subscribed member free them and refresh both caches", func(t *testing.T) {
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RulePhoto:   models.RequirementActionMute,
			memberreq.RuleChannel: models.RequirementActionMute,
		})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		state.set(member.Id, mreqPhotoAnswer{})
		notice := env.mreqMuteByMessage(member)
		state.set(member.Id, mreqHasPhoto)
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, defaultUnmutePermissions(),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
		if ttl := env.redis.TTL(memberReqPhotoKey(member.Id)); ttl < 10*time.Minute {
			t.Errorf("photo key ttl = %s, want the positive cached", ttl)
		}
		if ttl := env.redis.TTL(memberReqSubKey(channel, member.Id)); ttl < 10*time.Minute {
			t.Errorf("subscription key ttl = %s, want the positive cached", ttl)
		}
	})

	t.Run("a channel member who left after a cached positive is not waved through", func(t *testing.T) {
		env, _, member, channel, hold, notice := mreqChannelMuted(t)
		mreqSetKey(t, env, memberReqSubKey(channel, member.Id), memberReqPositiveTTL)

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_still_broken", staffMarker("mreq_rule_channel"))

		if env.redis.Exists(memberReqSubKey(channel, member.Id)) {
			t.Error("the stale subscription positive is still cached after a definite breach")
		}
		mreqWantHeld(t, env, hold)
	})
}

// mreqUnknownShape builds a press in which the lookup of one rule cannot be answered, and
// returns what it needs: the member muted for that rule, the notice, the hold and the press's
// environment. hangs shapes get short budgets because the lookup never answers.
func mreqUnknownShape(t *testing.T, shape mreqUnanswerable) (*memberReqEnv, gotgbot.User, staffSentMessage, *models.ChatRequirementHold) {
	t.Helper()
	var (
		env     *memberReqEnv
		state   *mreqLookupState
		member  gotgbot.User
		channel int64
		hold    *models.ChatRequirementHold
		notice  staffSentMessage
	)
	if shape.rule == memberreq.RuleChannel {
		env, state, member, channel, hold, notice = mreqChannelMuted(t)
	} else {
		env, state, member, hold, notice = mreqPhotoMuted(t)
	}
	if shape.hangs {
		mreqSwap(t, &memberReqFixedLookupBudget, 300*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
		// Runs before the row cleanups: the abandoned flight has ended by its own timeout.
		t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })
	}
	shape.prepare(t, env, state, env.chat.Id, channel, member.Id)
	return env, member, notice, hold
}

func mreqFixedLookupsUnanswerable(t *testing.T) {
	for _, shape := range mreqUnanswerableShapes() {
		t.Run(shape.name, func(t *testing.T) {
			env, member, notice, hold := mreqUnknownShape(t, shape)
			started := time.Now()

			mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")

			if took := time.Since(started); took >= memberReqFixedTimeout {
				t.Errorf("the press took %s, want it inside %s", took, memberReqFixedTimeout)
			}
			mreqWantHeld(t, env, hold)
			if live := mreqLiveRecord(t, env, member.Id); !isMemberReqMute(live, hold.MuteUntil) {
				t.Errorf("the member after the refusal = %+v, want still muted with the marker", live)
			}
		})
	}
}

func mreqFixedLookupsSlow(t *testing.T) {
	t.Run("a slow but answered lookup frees the member inside the press's time", func(t *testing.T) {
		env, state, member, _, notice := mreqPhotoMuted(t)
		state.set(member.Id, mreqHasPhoto)
		release := state.hold(member.Id)
		timer := time.AfterFunc(400*time.Millisecond, release)
		t.Cleanup(func() { timer.Stop(); release() })
		restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())

		took := mreqPressTiming(env, member, notice)

		if took >= memberReqFixedTimeout {
			t.Errorf("the press took %s, want it inside %s", took, memberReqFixedTimeout)
		}
		mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, lockdownSnapshotPermissions(t),
			mreqAnswer{Text: staffMarker("mreq_fixed_done")},
			mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
	})

	t.Run("a lookup that uses up its budget answers check failed and leaves room for the answer", func(t *testing.T) {
		env, state, member, _, notice := mreqPhotoMuted(t)
		mreqSwap(t, &memberReqFixedLookupBudget, 300*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
		t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })
		t.Cleanup(state.hold(member.Id))

		started := time.Now()
		env.mreqPressFixed(member, notice)

		if took := time.Since(started); took >= memberReqFixedTimeout {
			t.Errorf("the press took %s, want it inside %s", took, memberReqFixedTimeout)
		}
		if got := env.mreqAnswers()[len(env.mreqAnswers())-1]; got.Text != staffMarker("mreq_fixed_check_failed") || !got.Alert {
			t.Errorf("answer = %+v, want the check failed alert", got)
		}
	})
}

func mreqFixedLookupsRealLocale(t *testing.T) {
	t.Run("the answers and the notice edit are in the group's language", func(t *testing.T) {
		env := newMemberReqEnvWith(t, settingsEnvOptions{RealLocale: true})
		withFastLookupPacer(t)
		state := newMreqLookupState()
		mreqInstallLookupFake(env, state)
		env.loadMemberReq()
		if err := lang.ChangeGroupLanguage(env.chat.Id, "uk"); err != nil {
			t.Fatal(err)
		}
		env.mreqRules(map[memberreq.Rule]int{memberreq.RulePhoto: models.RequirementActionMute})
		member := mreqLookupMember(t, env, state, mreqPhotoAnswer{})
		notice := env.mreqMuteByMessage(member)
		uk := i18n.MustNewTranslator("uk")
		answersBefore := len(env.mreqAnswers())

		env.mreqPressFixed(member, notice)

		photo := mreqBitOf(t, "ph")
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != memberReqStillBrokenText(uk, photo) || !got.Alert {
			t.Errorf("still broken: answer = %+v, want the uk text %q as an alert", got, memberReqStillBrokenText(uk, photo))
		}
		state.set(member.Id, mreqHasPhoto)
		answersBefore = len(env.mreqAnswers())
		env.mreqPressFixed(member, notice)
		if got := env.mreqOnlyAnswer(answersBefore); got.Text != memberReqText(uk, "mreq_fixed_done") {
			t.Errorf("done: answer = %+v, want the uk toast", got)
		}
		if text, _ := env.lastEdit(env.chat.Id, notice.MessageID); text != mreqFixedNoticeText(t, uk, "mreq_notice_fixed", member) {
			t.Errorf("edited notice = %q, want the uk fix text", text)
		}
	})
}

// mreqNoMemberLookup scripts the member's own channel lookup to fail with a 500, so only the
// bot's own standing can decide the channel rule at the tap.
func mreqNoMemberLookup(state *mreqLookupState, channel, member int64) {
	state.scriptMember(channel, member, staffFakeError(500, "test error 500"))
}

func TestMemberReqChannelInert(t *testing.T) {
	telegram := func(code int) error { return staffFakeError(code, fmt.Sprintf("test error %d", code)) }
	persistent429 := []error{staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1)}

	type inertCase struct {
		name string
		// prepare changes the fake after the member was muted, so the member's own lookup cannot
		// be answered and the bot's standing is the one thing that can still be read.
		prepare func(t *testing.T, env *memberReqEnv, state *mreqLookupState, channel, member int64)
		hangs   bool
		freed   bool
	}
	botIs := func(status string) func(*testing.T, *memberReqEnv, *mreqLookupState, int64, int64) {
		return func(_ *testing.T, _ *memberReqEnv, state *mreqLookupState, channel, member int64) {
			mreqNoMemberLookup(state, channel, member)
			state.setMember(channel, staffTestBotID, mreqChannelMember{Status: status})
		}
	}
	botErrors := func(errs ...error) func(*testing.T, *memberReqEnv, *mreqLookupState, int64, int64) {
		return func(_ *testing.T, _ *memberReqEnv, state *mreqLookupState, channel, member int64) {
			mreqNoMemberLookup(state, channel, member)
			state.scriptMember(channel, staffTestBotID, errs...)
		}
	}
	cases := []inertCase{
		{name: "the bot is a plain member of the channel", prepare: botIs(gotgbot.ChatMemberStatusMember), freed: true},
		{name: "the bot left the channel", prepare: botIs(gotgbot.ChatMemberStatusLeft), freed: true},
		{name: "the bot was kicked from the channel", prepare: botIs(gotgbot.ChatMemberStatusKicked), freed: true},
		{name: "the bot is a restricted member of the channel", prepare: botIs(gotgbot.ChatMemberStatusRestricted), freed: true},
		{name: "the bot's own lookup is a 400", prepare: botErrors(telegram(400)), freed: true},
		{name: "the bot's own lookup is a 403", prepare: botErrors(telegram(403)), freed: true},
		{name: "the member lookup is a 403 and the bot is no administrator", freed: true,
			prepare: func(_ *testing.T, _ *memberReqEnv, state *mreqLookupState, channel, member int64) {
				state.scriptMember(channel, member, telegram(403))
				state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
			}},
		{name: "the group has no linked channel any more", freed: true,
			prepare: func(_ *testing.T, env *memberReqEnv, state *mreqLookupState, _, _ int64) { state.link(env.chat.Id, 0) }},

		{name: "the bot's own lookup is a 429 past the retry", prepare: botErrors(persistent429...)},
		{name: "the bot's own lookup is a 502", prepare: botErrors(telegram(502))},
		{name: "the bot's own lookup is a transport error", prepare: botErrors(errors.New("connection reset by peer"))},
		{name: "the bot's own lookup times out", hangs: true,
			prepare: func(t *testing.T, _ *memberReqEnv, state *mreqLookupState, channel, member int64) {
				mreqNoMemberLookup(state, channel, member)
				t.Cleanup(state.holdKey(mreqChannelKey("getChatMember", channel, staffTestBotID)))
			}},
		{name: "the pacer sheds every lookup",
			prepare: func(t *testing.T, env *memberReqEnv, _ *mreqLookupState, _, _ int64) {
				const blockKey = "fuku:memberreq:lkp:pace:block"
				mreqSetKey(t, env, blockKey, 30*time.Second)
				t.Cleanup(func() { env.redis.Del(blockKey) })
			}},
		{name: "the bot is an administrator of the channel", prepare: botIs(gotgbot.ChatMemberStatusAdministrator)},
		{name: "the bot is the creator of the channel", prepare: botIs(gotgbot.ChatMemberStatusCreator)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, state, member, channel, hold, notice := mreqChannelMuted(t)
			if tc.hangs {
				mreqSwap(t, &memberReqFixedLookupBudget, 300*time.Millisecond)
				mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
				t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })
			}
			tc.prepare(t, env, state, channel, member.Id)
			restrictsBefore, answersBefore := len(env.mreqRestricts(member.Id)), len(env.mreqAnswers())
			started := time.Now()

			if tc.freed {
				env.mreqPressFixed(member, notice)
				mreqWantFixedUnmute(t, env, member, notice, restrictsBefore, answersBefore, defaultUnmutePermissions(),
					mreqAnswer{Text: staffMarker("mreq_fixed_done")},
					mreqFixedNoticeText(t, env.translator(), "mreq_notice_fixed", member))
			} else {
				mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_check_failed")
				mreqWantHeld(t, env, hold)
			}
			if took := time.Since(started); took >= memberReqFixedTimeout {
				t.Errorf("the press took %s, want it inside %s", took, memberReqFixedTimeout)
			}
		})
	}

	t.Run("the bot's own request goes through the lookup pacer and takes no slot of the notice pacer", func(t *testing.T) {
		env, state, member, channel, _, notice := mreqChannelMuted(t)
		mreqNoMemberLookup(state, channel, member.Id)
		state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		const lookupNext, noticeNext = "fuku:memberreq:lkp:pace:next", "fuku:memberreq:pace:next"
		var mu sync.Mutex
		var atMember, atBot string
		var lookupSlotAtBot bool
		state.setOnChannel(func(method string, chat, user int64) {
			if method != "getChatMember" || chat != channel {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			noticeSlot, _ := env.redis.Get(noticeNext)
			switch user {
			case member.Id:
				atMember = noticeSlot
			case staffTestBotID:
				atBot, lookupSlotAtBot = noticeSlot, env.redis.Exists(lookupNext)
			}
		})

		env.mreqPressFixed(member, notice)

		mu.Lock()
		defer mu.Unlock()
		if !lookupSlotAtBot {
			t.Error("no lookup pacer slot existed when the bot's own request arrived: it did not go through the lookup pacer")
		}
		if atBot != atMember {
			t.Errorf("the notice pacer slot moved from %q to %q between the member's request and the bot's own: the lookup took a slot of the wrong pacer", atMember, atBot)
		}
	})

	t.Run("the enforcement job with the same unanswerable channel lookup punishes nobody and makes no hold", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusLeft)
		mreqNoMemberLookup(state, channel, member.Id)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
	})
}

func TestMemberReqFixedNeverLiftsWithLookupRules(t *testing.T) {
	until := func(offset time.Duration) func(int64) *gotgbot.RestrictChatMemberOpts {
		return func(int64) *gotgbot.RestrictChatMemberOpts {
			return &gotgbot.RestrictChatMemberOpts{UntilDate: time.Now().Add(offset).Unix()}
		}
	}
	noEnd := func(int64) *gotgbot.RestrictChatMemberOpts { return nil }
	replace := map[string]func(marker int64) *gotgbot.RestrictChatMemberOpts{
		"an admin's /mute (no end date)":      noEnd,
		"a /tmute of one hour":                until(time.Hour),
		"a staff mute of a day":               until(24 * time.Hour),
		"the captcha's re-mute (no end date)": noEnd,
		"antiflood's mute (no end date)":      noEnd,
		"a warn-limit mute (no end date)":     noEnd,
		"a mute three seconds past the marker": func(marker int64) *gotgbot.RestrictChatMemberOpts {
			return &gotgbot.RestrictChatMemberOpts{UntilDate: marker + 3}
		},
		"a mute three seconds before the marker": func(marker int64) *gotgbot.RestrictChatMemberOpts {
			return &gotgbot.RestrictChatMemberOpts{UntilDate: marker - 3}
		},
	}

	// setup mutes a member for the photo and channel rules, who is also approved, and traces the
	// seams a press could reach, so the test can prove none of them was.
	setup := func(t *testing.T) (*memberReqEnv, gotgbot.User, int64, *models.ChatRequirementHold, staffSentMessage, *mreqPressTrace) {
		t.Helper()
		env, state := mreqWatchLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RulePhoto:   models.RequirementActionMute,
			memberreq.RuleChannel: models.RequirementActionMute,
		})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		state.set(member.Id, mreqPhotoAnswer{})
		notice := env.mreqMuteByMessage(member)
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqRuleMask(t, "ph", "ch") || hold.PunishedAt == nil {
			t.Fatalf("hold = %+v, want a punished mute hold with the photo and channel bits", hold)
		}
		mreqApprove(t, env, member)
		return env, member, channel, hold, notice, mreqTracePress(t, env, state, member)
	}
	// wantUntouched fails when a press reached an approvals read, a settings read or a lookup.
	wantUntouched := func(t *testing.T, env *memberReqEnv, trace *mreqPressTrace, channel, member, trafficBefore int64, chatsBefore int) {
		t.Helper()
		if got := trace.log(); len(got) != 0 {
			t.Errorf("seams reached = %v, want none: the marker check of step 4 comes first", got)
		}
		if n := int64(mreqLookupTraffic(env, channel, member)); n != trafficBefore {
			t.Errorf("lookup requests = %d, want the %d before the presses", n, trafficBefore)
		}
		if n := mreqChatCalls(env, env.chat.Id); n != chatsBefore {
			t.Errorf("getChat requests of the group = %d, want the %d before the presses", n, chatsBefore)
		}
	}

	for name, opts := range replace {
		t.Run(name, func(t *testing.T) {
			env, member, channel, hold, notice, trace := setup(t)
			if _, err := env.bot.RestrictChatMember(env.chat.Id, member.Id, MutedPermissions, opts(hold.MuteUntil)); err != nil {
				t.Fatal(err)
			}
			data := env.mreqFixedData(notice)
			traffic, chats := int64(mreqLookupTraffic(env, channel, member.Id)), mreqChatCalls(env, env.chat.Id)

			mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")
			mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")

			wantUntouched(t, env, trace, channel, member.Id, traffic, chats)
			mreqWantHeld(t, env, hold)
		})
	}

	t.Run("a member an admin unmuted", func(t *testing.T) {
		env, member, channel, hold, notice, trace := setup(t)
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember, IsMember: true, CanSendMessages: true})
		data := env.mreqFixedData(notice)
		traffic, chats := int64(mreqLookupTraffic(env, channel, member.Id)), mreqChatCalls(env, env.chat.Id)

		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")
		mreqPressRefused(t, env, member, member, notice.MessageID, data, "mreq_fixed_not_ours")

		wantUntouched(t, env, trace, channel, member.Id, traffic, chats)
		mreqWantHeld(t, env, hold)
	})

	t.Run("a member who left", func(t *testing.T) {
		env, member, channel, hold, notice, trace := setup(t)
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
		traffic, chats := int64(mreqLookupTraffic(env, channel, member.Id)), mreqChatCalls(env, env.chat.Id)

		mreqPressRefused(t, env, member, member, notice.MessageID, env.mreqFixedData(notice), "mreq_fixed_not_ours")

		wantUntouched(t, env, trace, channel, member.Id, traffic, chats)
		mreqWantHeld(t, env, hold)
	})
}
