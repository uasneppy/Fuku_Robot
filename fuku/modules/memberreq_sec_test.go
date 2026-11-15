//go:build testtools

package modules

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqSeedBotAdmin stores the cached administrator standing of the bot in channel, as an earlier
// lookup would have left it.
func mreqSeedBotAdmin(t *testing.T, env *memberReqEnv, channel int64) {
	t.Helper()
	mreqSetKey(t, env, memberReqBotAdminKey(channel), memberReqBotAdminTTL)
}

// TestMemberReqDemotedBotNeverBreaches is SEC-03: a not-subscribed candidate of the channel is a
// breach only on a FRESH answer that the bot administers the channel, so a bot demoted while its
// administrator standing is still cached (up to two minutes) punishes nobody.
func TestMemberReqDemotedBotNeverBreaches(t *testing.T) {
	t.Run("a bot demoted while its standing is cached makes no breach of any not-subscribed status", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			status string
		}{
			{"left", gotgbot.ChatMemberStatusLeft},
			{"kicked", gotgbot.ChatMemberStatusKicked},
			{"restricted and not a member", gotgbot.ChatMemberStatusRestricted},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
				member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusMember, tc.status)
				mreqSeedLink(t, env, channel, time.Minute)
				mreqSeedBotAdmin(t, env, channel)

				mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
				if !env.redis.Exists(mreqCnaKey(channel)) {
					t.Errorf("%s was not written after a fresh answer showed the bot is no administrator", mreqCnaKey(channel))
				}
				if env.redis.Exists(memberReqBotAdminKey(channel)) {
					t.Error("the stale administrator standing survived a fresh answer that showed the demotion")
				}
				if n := mreqMemberCalls(env, channel, staffTestBotID); n != 1 {
					t.Errorf("bot status requests = %d, want exactly 1 fresh one", n)
				}
			})
		}
	})

	t.Run("a bot that is still an administrator keeps the breach at the cost of one fresh bot status request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
		mreqSeedLink(t, env, channel, time.Minute)
		mreqSeedBotAdmin(t, env, channel)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 1 {
			t.Errorf("bot status requests = %d, want exactly 1 (the fresh read behind the cached standing)", n)
		}
		if !env.redis.Exists(memberReqBotAdminKey(channel)) {
			t.Error("the administrator standing was not cached again after the fresh answer")
		}
		if env.redis.Exists(mreqCnaKey(channel)) {
			t.Errorf("%s was written for a bot that administers the channel", mreqCnaKey(channel))
		}
	})

	t.Run("a fresh bot status that cannot be established is unknown and punishes nobody", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			setup func(t *testing.T, env *memberReqEnv, state *mreqLookupState, channel int64)
		}{
			{"a 429 that keeps coming", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFake429(1), staffFake429(1))
			}},
			{"a 500", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, staffFakeError(500, "Internal Server Error"), staffFakeError(500, "Internal Server Error"))
			}},
			{"a transport error", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c int64) {
				s.scriptMember(c, staffTestBotID, errors.New("connection reset by peer"), errors.New("connection reset by peer"))
			}},
			{"a null answer", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Null: true})
			}},
			{"an unrecognised status", func(_ *testing.T, _ *memberReqEnv, s *mreqLookupState, c int64) {
				s.setMember(c, staffTestBotID, mreqChannelMember{Status: "something_new"})
			}},
			{"a timeout", func(t *testing.T, _ *memberReqEnv, s *mreqLookupState, c int64) {
				mreqSwap(t, &memberReqLookupFlightTimeout, 100*time.Millisecond)
				t.Cleanup(s.holdKey(mreqChannelKey("getChatMember", c, staffTestBotID)))
			}},
			{"a pacer shed", func(t *testing.T, env *memberReqEnv, s *mreqLookupState, _ int64) {
				// The link is cached and the member lookup is made first; the fresh link re-read is the
				// request that arrives right before the bot's status, so a long block set when it arrives
				// sheds the status read and nothing before it.
				var blocked atomic.Bool
				s.setOnChannel(func(method string, chat, _ int64) {
					if method == "getChat" && chat == env.chat.Id && blocked.CompareAndSwap(false, true) {
						if err := env.redis.Set("fuku:memberreq:lkp:pace:block", "1"); err != nil {
							t.Errorf("set the pacer block: %v", err)
						}
						env.redis.SetTTL("fuku:memberreq:lkp:pace:block", time.Minute)
					}
				})
				t.Cleanup(func() { env.redis.Del("fuku:memberreq:lkp:pace:block") })
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, state := mreqLookupEnv(t)
				env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
				member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusLeft)
				mreqSeedLink(t, env, channel, time.Minute)
				mreqSeedBotAdmin(t, env, channel)
				tc.setup(t, env, state, channel)

				mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
				if env.redis.Exists(mreqCnaKey(channel)) {
					t.Errorf("%s was written for a standing that could not be established", mreqCnaKey(channel))
				}
			})
		}
	})

	t.Run("a subscribed member and a Fresh caller cost no extra bot status request", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleChannel: models.RequirementActionBan})
		member, channel := mreqChannelUser(t, env, state, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember)
		mreqSeedLink(t, env, channel, time.Minute)
		mreqSeedBotAdmin(t, env, channel)

		mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 0 {
			t.Errorf("bot status requests for a subscribed member = %d, want none", n)
		}

		leaver := mreqLookupMember(t, env, state, mreqHasPhoto)
		state.setMember(channel, leaver.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		if got := mreqChannelVerdict(env, leaver.Id, memberReqLookupMode{Fresh: true}); got != memberReqBreach {
			t.Fatalf("Fresh verdict = %d, want breach", got)
		}
		if n := mreqMemberCalls(env, channel, staffTestBotID); n != 1 {
			t.Errorf("bot status requests for a Fresh caller = %d, want exactly 1 (no second read)", n)
		}
	})
}

// mreqExemptionFlips counts the approvals reads and, once flipped, answers the exemption the
// test is about. It swaps the approvals and the captcha seams for the test; the flag is atomic
// because the lookup hook runs on the fake's own goroutine.
type mreqExemptionFlips struct {
	approvalReads atomic.Int64
	flipped       atomic.Bool
}

// installApproved makes the approvals read answer approved once flipped.
func (f *mreqExemptionFlips) installApproved(t *testing.T) {
	t.Helper()
	mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
		f.approvalReads.Add(1)
		return f.flipped.Load(), nil
	})
}

// installApprovalError makes the approvals read fail once flipped.
func (f *mreqExemptionFlips) installApprovalError(t *testing.T) {
	t.Helper()
	mreqSwap(t, &memberReqApprovedLookup, func(context.Context, int64, int64) (bool, error) {
		f.approvalReads.Add(1)
		if f.flipped.Load() {
			return false, context.DeadlineExceeded
		}
		return false, nil
	})
}

// installPending makes a pending captcha attempt appear once flipped.
func (f *mreqExemptionFlips) installPending(t *testing.T) {
	t.Helper()
	mreqSwap(t, &memberReqCaptchaPendingLookup, func(context.Context, int64) (bool, error) {
		return f.flipped.Load(), nil
	})
	mreqSwap(t, &memberReqCaptchaAttemptLookup, func(int64, int64) (*models.CaptchaAttempts, error) {
		if f.flipped.Load() {
			return &models.CaptchaAttempts{}, nil
		}
		return nil, nil
	})
}

// TestMemberReqExemptionsReadAfterLookups is SEC-02: the approved and captcha-pending exemptions
// are judged again after a lookup phase that made a request or ran long, so a member an admin
// approves, or whose captcha attempt starts, during that phase is never punished, and a read that
// fails then punishes nobody.
func TestMemberReqExemptionsReadAfterLookups(t *testing.T) {
	for _, tc := range []struct {
		name    string
		install func(f *mreqExemptionFlips, t *testing.T)
	}{
		{"approved", (*mreqExemptionFlips).installApproved},
		{"captcha pending", (*mreqExemptionFlips).installPending},
		{"an approvals read error", (*mreqExemptionFlips).installApprovalError},
	} {
		t.Run("a member who becomes "+tc.name+" during the lookup phase is never punished", func(t *testing.T) {
			for _, action := range []struct {
				name  string
				value int
			}{
				{"Ban", models.RequirementActionBan},
				{"Kick", models.RequirementActionKick},
				{"Mute", models.RequirementActionMute},
				{"Warn", models.RequirementActionWarn},
			} {
				t.Run(action.name, func(t *testing.T) {
					env, state := mreqLookupEnv(t)
					member := mreqNoPhotoMember(t, env, state, action.value)
					flips := &mreqExemptionFlips{}
					tc.install(flips, t)
					state.setOnPhoto(func(user int64) {
						if user == member.Id {
							flips.flipped.Store(true)
						}
					})

					mreqWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
				})
			}
		})
	}

	t.Run("a lookup phase that made a request costs one more approvals read and a breach still punishes", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		member := mreqNoPhotoMember(t, env, state, models.RequirementActionBan)
		flips := &mreqExemptionFlips{}
		flips.installApproved(t)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		if n := flips.approvalReads.Load(); n != 2 {
			t.Errorf("approvals reads = %d, want 2 (before the lookup phase and again before the claim)", n)
		}
	})

	t.Run("a lookup phase that was all cache hits and fast costs no extra approvals read", func(t *testing.T) {
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RulePhoto:    models.RequirementActionBan,
		})
		member := env.mreqMember("Alex", "", "alexsmith")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqHasPhoto)
		mreqSetKey(t, env, memberReqPhotoKey(member.Id), memberReqPositiveTTL)
		flips := &mreqExemptionFlips{}
		flips.installApproved(t)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted by the name rule", got)
		}
		if n := flips.approvalReads.Load(); n != 1 {
			t.Errorf("approvals reads = %d, want 1: a cached lookup phase needs no second read", n)
		}
	})
}
