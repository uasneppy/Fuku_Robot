//go:build testtools

package modules

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/raiddetection"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
)

// mreqAIEnvOf builds the environment of an AI rule test: the AI check configured, the Anthropic
// stand-in installed (its default answer is a 0.1 probability), the verdict keys cleaned, and the
// rules stored as given. With join set the greetings module is loaded and Check on join is on
// (mreqJoinEnv); otherwise it is the plain requirements environment.
func mreqAIEnvOf(t *testing.T, rules map[memberreq.Rule]int, join bool) (*memberReqEnv, *mreqAIState) {
	t.Helper()
	withAIConfigured(t)
	ai := mreqInstallAIFake(t)
	mreqAICleanup(t, nil, nil)
	var env *memberReqEnv
	if join {
		env = mreqJoinEnv(t, rules, true)
	} else {
		env = newMemberReqEnv(t)
		env.mreqRules(rules)
	}
	return env, ai
}

// mreqAIRename gives the member names that carry its own user ID, so no two tests share a
// verdict key, and returns the member as the update shows them. last may be empty.
func mreqAIRename(env *memberReqEnv, member gotgbot.User, last string) gotgbot.User {
	first := fmt.Sprintf("Promo%d", member.Id)
	username := fmt.Sprintf("promo%d", member.Id)
	env.fake.setProfile(member.Id, first, last, username)
	member.FirstName, member.LastName, member.Username = first, last, username
	return member
}

// mreqAIMember mints a member of the environment's group whose names carry its user ID.
func mreqAIMember(env *memberReqEnv, last string) gotgbot.User {
	env.t.Helper()
	return mreqAIRename(env, env.mreqMember("Alex", "Smith", "alexsmith"), last)
}

// mreqAIUniqueNames are names no other test shares.
func mreqAIUniqueNames() *memberReqNames {
	id := uniqueLinkOwnerID()
	return &memberReqNames{First: fmt.Sprintf("Promo%d", id), Last: "Casino", Username: fmt.Sprintf("promo%d", id)}
}

// mreqAIAsk calls the AI rule directly with names in the mode.
func mreqAIAsk(env *memberReqEnv, names *memberReqNames, mode memberReqLookupMode) memberReqVerdict {
	mode.Names = names
	return memberReqAIRule(context.Background(), env.bot, env.chat.Id, uniqueLinkOwnerID(), mode)
}

// mreqAIWantNothing is mreqWantNothing and no verdict written: nothing was punished, held,
// noticed or cached for the member.
func mreqAIWantNothing(t *testing.T, env *memberReqEnv, member gotgbot.User, got memberReqResult) {
	t.Helper()
	mreqWantNothing(t, env, member, got)
	if keys := mreqAIVerdictKeys(t); len(keys) != 0 {
		t.Errorf("verdict keys = %v, want none written", keys)
	}
}

// mreqAIWelcomes is how many welcomes the group received.
func mreqAIWelcomes(env *memberReqEnv) int {
	env.t.Helper()
	return mreqCountText(env.mreqSentTexts(), mreqWelcomeWord)
}

// mreqAIKeyOf is the verdict key of names under the model of the tests.
func mreqAIKeyOf(t *testing.T, names *memberReqNames) string {
	t.Helper()
	return memberReqAIVerdictKey(raidAITestModel, mreqAICanon(t, names.First, names.Last, names.Username))
}

// mreqAIFullwidth writes ASCII letters and digits as their fullwidth forms, which NFKC folds back.
func mreqAIFullwidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 0xFF41
		case r >= 'A' && r <= 'Z':
			return r - 'A' + 0xFF21
		case r >= '0' && r <= '9':
			return r - '0' + 0xFF10
		}
		return r
	}, s)
}

func TestMemberReqAIRule(t *testing.T) {
	direct := func(t *testing.T) (*memberReqEnv, *mreqAIState) {
		t.Helper()
		return mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, false)
	}

	t.Run("no names is unknown and asks nobody", func(t *testing.T) {
		env, ai := direct(t)
		if got := mreqAIAsk(env, nil, memberReqLookupMode{}); got != memberReqUnknown {
			t.Errorf("verdict = %v, want unknown", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("requests = %d, want none", n)
		}
	})

	t.Run("names that cannot be judged are inert and ask nobody", func(t *testing.T) {
		for name, names := range map[string]*memberReqNames{
			"all empty":          {},
			"only invisible":     {First: "​", Last: "​", Username: ""},
			"a lone Deleted Acc": {First: "Deleted Account"},
		} {
			env, ai := direct(t)
			if got := mreqAIAsk(env, names, memberReqLookupMode{}); got != memberReqInert {
				t.Errorf("%s: verdict = %v, want inert", name, got)
			}
			if n := ai.count(); n != 0 {
				t.Errorf("%s: requests = %d, want none", name, n)
			}
		}
	})

	t.Run("no Redis, and a Redis that fails, are unknown and ask nobody", func(t *testing.T) {
		env, ai := direct(t)
		names := mreqAIUniqueNames()
		mreqSwap(t, &memberReqRedisClient, func() *redis.Client { return nil })
		if got := mreqAIAsk(env, names, memberReqLookupMode{}); got != memberReqUnknown {
			t.Errorf("no Redis: verdict = %v, want unknown", got)
		}
		memberReqRedisClient = raidRedis
		env.redis.SetError("connection refused")
		t.Cleanup(func() { env.redis.SetError("") })
		if got := mreqAIAsk(env, names, memberReqLookupMode{}); got != memberReqUnknown {
			t.Errorf("a failing Redis: verdict = %v, want unknown", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("requests = %d, want none (the cache cannot be read, so the provider is not asked)", n)
		}
	})

	t.Run("the bar is 0.9 and the probability is cached", func(t *testing.T) {
		for _, tc := range []struct {
			probability float64
			want        memberReqVerdict
			stored      string
		}{
			{0.9, memberReqBreach, "0.9"},
			{0.8999, memberReqPass, "0.8999"},
			{0, memberReqPass, "0"},
			{1, memberReqBreach, "1"},
			{0.97, memberReqBreach, "0.97"},
			{0.3, memberReqPass, "0.3"},
		} {
			env, ai := direct(t)
			ai.setDefault(mreqAIProbability(tc.probability))
			names := mreqAIUniqueNames()

			first := mreqAIAsk(env, names, memberReqLookupMode{})
			second := mreqAIAsk(env, names, memberReqLookupMode{})
			if first != tc.want || second != tc.want {
				t.Errorf("probability %v: verdicts = %v, %v, want %v twice", tc.probability, first, second, tc.want)
			}
			if n := ai.count(); n != 1 {
				t.Errorf("probability %v: requests = %d, want 1 (the second sighting is cached)", tc.probability, n)
			}
			key := mreqAIKeyOf(t, names)
			if value, err := env.redis.Get(key); err != nil || value != tc.stored {
				t.Errorf("probability %v: stored %q (error %v), want the probability %q", tc.probability, value, err, tc.stored)
			}
			if ttl := env.redis.TTL(key); ttl != memberReqAIVerdictTTL {
				t.Errorf("probability %v: ttl = %v, want %v", tc.probability, ttl, memberReqAIVerdictTTL)
			}
			for _, stored := range mreqAIVerdictKeys(t) {
				if strings.Contains(stored, names.First) || strings.Contains(stored, names.Username) {
					t.Errorf("verdict key %q names the member", stored)
				}
			}
		}
	})

	t.Run("a cached probability is judged by the bar without a request", func(t *testing.T) {
		env, ai := direct(t)
		high, low := mreqAIUniqueNames(), mreqAIUniqueNames()
		env.redis.Set(mreqAIKeyOf(t, high), "0.95")
		env.redis.Set(mreqAIKeyOf(t, low), "0.2")
		if got := mreqAIAsk(env, high, memberReqLookupMode{}); got != memberReqBreach {
			t.Errorf("cached 0.95: verdict = %v, want breach", got)
		}
		if got := mreqAIAsk(env, low, memberReqLookupMode{}); got != memberReqPass {
			t.Errorf("cached 0.2: verdict = %v, want pass", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("requests = %d, want none", n)
		}
	})

	t.Run("a stored value that is not a probability is a miss", func(t *testing.T) {
		for _, planted := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "1.5", "-0.1", "abc", ""} {
			env, ai := direct(t)
			names := mreqAIUniqueNames()
			key := mreqAIKeyOf(t, names)
			env.redis.Set(key, planted)
			if got := mreqAIAsk(env, names, memberReqLookupMode{}); got != memberReqPass {
				t.Errorf("planted %q: verdict = %v, want the provider's pass", planted, got)
			}
			if n := ai.count(); n != 1 {
				t.Errorf("planted %q: requests = %d, want 1 (a planted value is a miss, never a verdict)", planted, n)
			}
			if value, err := env.redis.Get(key); err != nil || value != "0.1" {
				t.Errorf("planted %q: stored %q (error %v), want the fresh probability 0.1", planted, value, err)
			}
		}
	})

	t.Run("names that canonicalise alike share one cached verdict", func(t *testing.T) {
		env, ai := direct(t)
		ai.setDefault(mreqAIProbability(0.97))
		id := uniqueLinkOwnerID()
		plain := &memberReqNames{First: fmt.Sprintf("Promo%d", id), Last: "Casino", Username: fmt.Sprintf("promo%d", id)}
		alike := &memberReqNames{
			First:    "  " + mreqAIFullwidth(strings.ToLower(plain.First)) + "  ",
			Last:     "Casino",
			Username: "@" + strings.ToUpper(plain.Username),
		}
		if got := mreqAIAsk(env, plain, memberReqLookupMode{}); got != memberReqBreach {
			t.Fatalf("plain names: verdict = %v, want breach", got)
		}
		if got := mreqAIAsk(env, alike, memberReqLookupMode{}); got != memberReqBreach {
			t.Errorf("alike names: verdict = %v, want breach", got)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("requests = %d, want 1 (the canonical form is the cache key)", n)
		}
	})

	t.Run("another model does not read the first model's verdict", func(t *testing.T) {
		env, ai := direct(t)
		names := mreqAIUniqueNames()
		mreqAIAsk(env, names, memberReqLookupMode{})
		mreqSwap(t, &config.AppConfig.AnthropicModel, "other-test-model")
		mreqAIAsk(env, names, memberReqLookupMode{})
		if n := ai.count(); n != 2 {
			t.Errorf("requests = %d, want 2 (a model change never serves the old model's verdict)", n)
		}
	})

	t.Run("the request carries only the three canonical strings and the key only in its header", func(t *testing.T) {
		env, ai := direct(t)
		names := &memberReqNames{First: fmt.Sprintf("  Promo%d  ", uniqueLinkOwnerID()), Last: mreqAIFullwidth("Casino"), Username: "@PromoName"}
		mreqAIAsk(env, names, memberReqLookupMode{})
		requests := ai.received()
		if len(requests) != 1 {
			t.Fatalf("requests = %d, want 1", len(requests))
		}
		canon := mreqAICanon(t, names.First, names.Last, names.Username)
		got := mreqAINamesOf(t, requests[0].Body)
		if got.First != canon.First || got.Last != canon.Last || got.Username != canon.Username {
			t.Errorf("request names = %+v, want the canonical %q %q %q", got, canon.First, canon.Last, canon.Username)
		}
		decoded := mreqAIDecode(t, requests[0].Body)
		if decoded.Model != raidAITestModel || decoded.System != claude.NamesInstruction || decoded.Tools != 1 {
			t.Errorf("request model %q, tools %d, system matches instruction %t, want the test model, one tool and the code-owned instruction",
				decoded.Model, decoded.Tools, decoded.System == claude.NamesInstruction)
		}
		if requests[0].Key != raidAITestKey {
			t.Errorf("x-api-key header does not carry the configured key")
		}
		if strings.Contains(string(requests[0].Body), raidAITestKey) {
			t.Errorf("the request body carries the key")
		}
	})

	t.Run("Fresh and Classify do not bypass the cache", func(t *testing.T) {
		env, ai := direct(t)
		names := mreqAIUniqueNames()
		mreqAIAsk(env, names, memberReqLookupMode{})
		mreqAIAsk(env, names, memberReqLookupMode{Fresh: true})
		mreqAIAsk(env, names, memberReqLookupMode{Classify: true})
		mreqAIAsk(env, names, memberReqLookupMode{Fresh: true, Classify: true})
		if n := ai.count(); n != 1 {
			t.Errorf("requests = %d, want 1 (the AI row answers from the cache whatever the mode)", n)
		}
	})

	t.Run("concurrent judgements of one string make one request", func(t *testing.T) {
		env, ai := direct(t)
		ai.setDefault(func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
			time.Sleep(150 * time.Millisecond)
			mreqAIProbability(0.97)(w, r, release)
		})
		names := mreqAIUniqueNames()
		var wg sync.WaitGroup
		verdicts := make([]memberReqVerdict, 12)
		for i := range verdicts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				verdicts[i] = mreqAIAsk(env, names, memberReqLookupMode{})
			}()
		}
		wg.Wait()
		for i, got := range verdicts {
			if got != memberReqBreach {
				t.Errorf("caller %d: verdict = %v, want breach", i, got)
			}
		}
		if n := ai.count(); n != 1 {
			t.Errorf("requests = %d, want 1 (one flight per verdict key)", n)
		}
	})
}

func TestMemberReqAIEnforcePunishes(t *testing.T) {
	aiBit := func(t *testing.T) int { return mreqBitOf(t, "ai") }
	joinEnv := func(t *testing.T, action int, probability float64) (*memberReqEnv, *mreqAIState, gotgbot.User) {
		t.Helper()
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: action}, true)
		ai.setDefault(mreqAIProbability(probability))
		return env, ai, mreqAIMember(env, "Casino")
	}
	// joinTwice runs the join and the second delivery of it (the service message).
	joinTwice := func(env *memberReqEnv, member gotgbot.User) {
		env.mreqJoin(member)
		env.mreqServiceJoin(member)
	}
	noticeOf := func(t *testing.T, env *memberReqEnv, key string) staffSentMessage {
		t.Helper()
		texts := env.mreqSentTexts()
		if got := mreqCountText(texts, staffMarker(key)); got != 1 {
			t.Fatalf("%s notices = %d, want exactly 1 (texts %v)", key, got, texts)
		}
		notice := env.mreqNotices()[mreqIndexOfText(texts, staffMarker(key))]
		if !strings.Contains(mreqText(notice), staffMarker("mreq_rule_ai")) {
			t.Errorf("notice %q does not name the AI phrase", mreqText(notice))
		}
		return notice
	}

	t.Run("join Mute mutes once with one notice and no welcome", func(t *testing.T) {
		env, ai, member := joinEnv(t, models.RequirementActionMute, 0.97)
		joinTwice(env, member)

		notice := noticeOf(t, env, "mreq_notice_mute")
		if data := mreqKeyboardData(notice); len(data) != 1 {
			t.Errorf("mute notice buttons = %v, want the one I fixed it button", data)
		}
		if got := mreqAIWelcomes(env); got != 0 {
			t.Errorf("welcomes = %d, want none for a muted joiner", got)
		}
		restricts := env.calls("restrictChatMember")
		if len(restricts) != 1 {
			t.Fatalf("restrictChatMember calls = %d, want the one requirement mute", len(restricts))
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != aiBit(t) || hold.Action != models.RequirementActionMute || hold.PunishedAt == nil || !hold.WelcomeHeld {
			t.Fatalf("hold = %+v, want a punished mute hold with bit %d and welcome_held", hold, aiBit(t))
		}
		if until := staffParamInt(restricts[0].Params, "until_date"); until != hold.MuteUntil {
			t.Errorf("until_date = %d, hold mute_until = %d, want the marker on both", until, hold.MuteUntil)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1 for the two deliveries of one join", n)
		}
	})

	t.Run("join Warn warns once with one notice and then welcomes", func(t *testing.T) {
		env, _, member := joinEnv(t, models.RequirementActionWarn, 0.97)
		joinTwice(env, member)

		noticeOf(t, env, "mreq_notice_warn")
		texts := env.mreqSentTexts()
		if notice, welcome := mreqIndexOfText(texts, staffMarker("mreq_notice_warn")), mreqIndexOfText(texts, mreqWelcomeWord); welcome < notice {
			t.Errorf("welcome at %d before the notice at %d, want the notice first (texts %v)", welcome, notice, texts)
		}
		row := env.mreqWarnRow(member.Id)
		if row == nil || row.NumWarns != 1 || len(row.Reasons) != 1 || !strings.Contains(row.Reasons[0], staffMarker("mreq_rule_ai")) {
			t.Errorf("warns row = %+v, want one warn naming the AI rule", row)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != aiBit(t) || hold.Action != models.RequirementActionWarn {
			t.Errorf("hold = %+v, want a warn hold with the AI bit", hold)
		}
	})

	t.Run("join Kick removes once with one notice", func(t *testing.T) {
		env, _, member := joinEnv(t, models.RequirementActionKick, 0.97)
		joinTwice(env, member)

		noticeOf(t, env, "mreq_notice_kick")
		unbans := env.calls("unbanChatMember")
		if len(unbans) != 1 || staffParamBool(unbans[0].Params, "only_if_banned") {
			t.Errorf("unban calls = %+v, want one member-removing unban", unbans)
		}
		if n := len(env.mreqWrites()); n != 1 {
			t.Errorf("ban, restrict and unban calls = %d, want only the unban", n)
		}
		if got := mreqAIWelcomes(env); got != 0 {
			t.Errorf("welcomes = %d, want none for a removed joiner", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != aiBit(t) || hold.Action != models.RequirementActionKick {
			t.Errorf("hold = %+v, want a kick hold with the AI bit", hold)
		}
	})

	t.Run("join Ban bans once with one notice", func(t *testing.T) {
		env, _, member := joinEnv(t, models.RequirementActionBan, 0.97)
		joinTwice(env, member)

		noticeOf(t, env, "mreq_notice_ban")
		if bans := env.calls("banChatMember"); len(bans) != 1 {
			t.Errorf("ban calls = %d, want 1", len(bans))
		}
		if got := mreqAIWelcomes(env); got != 0 {
			t.Errorf("welcomes = %d, want none for a banned joiner", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != aiBit(t) || hold.Action != models.RequirementActionBan {
			t.Errorf("hold = %+v, want a ban hold with the AI bit", hold)
		}
	})

	t.Run("a joiner rated below the bar is welcomed with no write, hold or notice", func(t *testing.T) {
		for _, action := range []int{models.RequirementActionMute, models.RequirementActionWarn, models.RequirementActionKick, models.RequirementActionBan} {
			t.Run(fmt.Sprint("action ", action), func(t *testing.T) {
				env, ai, member := joinEnv(t, action, 0.3)
				env.mreqJoin(member)
				if got := mreqAIWelcomes(env); got != 1 {
					t.Errorf("welcomes = %d, want 1", got)
				}
				mreqWantNothing(t, env, member, memberReqResult{})
				if n := ai.count(); n != 1 {
					t.Errorf("provider requests = %d, want 1", n)
				}
				if keys := mreqAIVerdictKeys(t); len(keys) != 1 {
					t.Errorf("verdict keys = %v, want the one cached pass", keys)
				}
			})
		}
	})

	t.Run("with the rule Off the provider is never asked", func(t *testing.T) {
		env, ai, member := joinEnv(t, models.RequirementActionOff, 0.99)
		env.mreqJoin(member)
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1", got)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for a rule that is Off", n)
		}
	})

	t.Run("job: the executor judges a message case the same way", func(t *testing.T) {
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, false)
		ai.setDefault(mreqAIProbability(0.97))
		member := mreqAIMember(env, "Casino")

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted || got.Hold == nil {
			t.Fatalf("outcome = %+v, want Muted with the hold", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != aiBit(t) {
			t.Errorf("hold = %+v, want the AI bit", hold)
		}
		noticeOf(t, env, "mreq_notice_mute")
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrictChatMember calls = %d, want 1", n)
		}
		// The second sighting is held and asks nobody.
		if again := env.mreqEnforce(env.mreqCase(member)); again.Outcome != memberReqHeld {
			t.Errorf("second outcome = %+v, want Held", again)
		}
		if n := ai.count(); n != 1 {
			t.Errorf("provider requests = %d, want 1", n)
		}
	})
}

func TestMemberReqAINotConfigured(t *testing.T) {
	t.Run("the condition is the image checks' condition", func(t *testing.T) {
		for _, tc := range []struct {
			enabled bool
			key     string
			want    bool
		}{
			{true, raidAITestKey, true},
			{false, raidAITestKey, false},
			{true, "", false},
			{false, "", false},
		} {
			t.Run(fmt.Sprintf("switch %t key set %t", tc.enabled, tc.key != ""), func(t *testing.T) {
				mreqSwap(t, &config.AppConfig.EnableAISpam, tc.enabled)
				mreqSwap(t, &config.AppConfig.AnthropicAPIKey, tc.key)
				if got := memberReqAIConfigured(); got != tc.want {
					t.Errorf("memberReqAIConfigured = %t, want %t", got, tc.want)
				}
				if got, image := memberReqAIConfigured(), raidAIImageConfigured(); got != image {
					t.Errorf("memberReqAIConfigured = %t but raidAIImageConfigured = %t, want the same answer", got, image)
				}
			})
		}
	})

	for _, tc := range []struct {
		name    string
		enabled bool
		key     string
	}{
		{"the kill switch off with a key", false, raidAITestKey},
		{"the switch on with no key", true, ""},
		{"both missing", false, ""},
	} {
		for _, raidOn := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s, raid ai_enabled %t: inert, no request, no key, nobody punished", tc.name, raidOn), func(t *testing.T) {
				// The last name rule is the weaker punishment: were the AI rule unknown instead of inert,
				// its Ban would defer the member's Warn.
				env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{
					memberreq.RuleLastName: models.RequirementActionWarn,
					memberreq.RuleAI:       models.RequirementActionBan,
				}, true)
				mreqSwap(t, &config.AppConfig.EnableAISpam, tc.enabled)
				mreqSwap(t, &config.AppConfig.AnthropicAPIKey, tc.key)
				raidCleanup(t, env.chat.Id)
				if err := raiddetection.SetAIEnabled(env.chat.Id, raidOn); err != nil {
					t.Fatalf("SetAIEnabled error = %v", err)
				}
				ai.setDefault(mreqAIProbability(0.99))

				clean := mreqAIMember(env, "Casino")
				planted := &memberReqNames{First: clean.FirstName, Last: clean.LastName, Username: clean.Username}
				plantedKey := mreqAIKeyOf(t, planted)
				env.redis.Set(plantedKey, "0.99")

				env.mreqJoin(clean)
				if got := mreqAIWelcomes(env); got != 1 {
					t.Errorf("welcomes = %d, want 1 (a joiner is welcomed when the AI is not configured)", got)
				}
				mreqWantNothing(t, env, clean, memberReqResult{})
				mreqWantNothing(t, env, clean, env.mreqEnforce(env.mreqCase(clean)))

				blank := mreqAIMember(env, "")
				got := env.mreqEnforce(env.mreqCase(blank))
				if got.Outcome != memberReqWarned {
					t.Errorf("blank last name: outcome = %+v, want Warned (an inert AI rule defers nothing)", got)
				}
				if hold := env.mreqHold(blank.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") {
					t.Errorf("hold = %+v, want the last name bit only", hold)
				}
				if n := ai.count(); n != 0 {
					t.Errorf("provider requests = %d, want none", n)
				}
				if keys := mreqAIVerdictKeys(t); len(keys) != 1 || keys[0] != plantedKey {
					t.Errorf("verdict keys = %v, want only the planted one", keys)
				}
			})
		}
	}

	t.Run("configured, the raid ai_enabled switch off does not stop the rule", func(t *testing.T) {
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: models.RequirementActionMute}, true)
		ai.setDefault(mreqAIProbability(0.97))
		raidCleanup(t, env.chat.Id)
		if err := raiddetection.SetAIEnabled(env.chat.Id, false); err != nil {
			t.Fatalf("SetAIEnabled error = %v", err)
		}
		member := mreqAIMember(env, "Casino")
		env.mreqJoin(member)
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ai") {
			t.Errorf("hold = %+v, want the AI hold (the raid switch is not part of the condition)", hold)
		}
	})
}

func TestMemberReqAISparesExempt(t *testing.T) {
	env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{
		memberreq.RuleLastName: models.RequirementActionWarn,
		memberreq.RuleAI:       models.RequirementActionMute,
	}, true)
	ai.setDefault(mreqAIProbability(0.99))

	cases := []struct {
		name string
		make func(t *testing.T) gotgbot.User
	}{
		{"an approved member", func(t *testing.T) gotgbot.User {
			member := mreqAIMember(env, "Casino")
			if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
				t.Fatal(err)
			}
			return member
		}},
		{"a live administrator", func(*testing.T) gotgbot.User {
			member := mreqAIMember(env, "Casino")
			env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
			return member
		}},
		{"the bot itself", func(*testing.T) gotgbot.User {
			return gotgbot.User{Id: env.bot.Id, FirstName: "Promo", LastName: "Casino", Username: "promobot"}
		}},
		{"another bot", func(*testing.T) gotgbot.User {
			member := mreqAIMember(env, "Casino")
			member.IsBot = true
			return member
		}},
		{"a Telegram service account", func(*testing.T) gotgbot.User {
			return gotgbot.User{Id: 777000, FirstName: "Promo", LastName: "Casino", Username: "promoservice"}
		}},
		{"a member with a pending captcha attempt", func(t *testing.T) gotgbot.User {
			member := mreqAIMember(env, "Casino")
			if _, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, env.chat.Id, "answer", 5); err != nil {
				t.Fatal(err)
			}
			dbcache.DeleteCache(dbcache.CacheKey("captcha_pending", env.chat.Id))
			return member
		}},
	}
	for _, tc := range cases {
		t.Run("job: "+tc.name, func(t *testing.T) {
			member := tc.make(t)
			mreqAIWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))
			if n := ai.count(); n != 0 {
				t.Errorf("provider requests = %d, want none", n)
			}
		})
		t.Run("join: "+tc.name, func(t *testing.T) {
			member := tc.make(t)
			env.mreqJoin(member)
			mreqAIWantNothing(t, env, member, memberReqResult{})
			if n := ai.count(); n != 0 {
				t.Errorf("provider requests = %d, want none", n)
			}
		})
	}

	// The hold carries the last name rule's bit and never the AI bit, so this case proves that a
	// held member's message makes no lookup (D-03): a hold that carries the AI bit is the held
	// release's own case, added by a later plan.
	t.Run("job: a member whose message arrives while they hold a hold", func(t *testing.T) {
		member := mreqAIMember(env, "")
		mreqSeedHeld(t, env, member, mreqBitOf(t, "ln"), models.RequirementActionWarn, 10*time.Minute)
		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqHeld {
			t.Errorf("outcome = %+v, want Held", got)
		}
		if n := len(env.mreqWrites()); n != 0 {
			t.Errorf("ban, restrict and unban requests = %d, want none", n)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none", row)
		}
		if n := ai.count(); n != 0 {
			t.Errorf("provider requests = %d, want none for a held member", n)
		}
		if keys := mreqAIVerdictKeys(t); len(keys) != 0 {
			t.Errorf("verdict keys = %v, want none", keys)
		}
	})
}

func TestMemberReqAILookupOrder(t *testing.T) {
	t.Run("the AI row is asked after the photo and channel rows and all three bits are on the hold", func(t *testing.T) {
		withAIConfigured(t)
		ai := mreqInstallAIFake(t)
		mreqAICleanup(t, nil, nil)
		env, state := mreqLookupEnv(t)
		env.mreqRules(map[memberreq.Rule]int{
			memberreq.RulePhoto:   models.RequirementActionMute,
			memberreq.RuleChannel: models.RequirementActionMute,
			memberreq.RuleAI:      models.RequirementActionMute,
		})
		ai.setDefault(mreqAIProbability(0.97))
		channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		member := mreqAIRename(env, mreqLookupMember(t, env, state, mreqPhotoAnswer{}), "Casino")
		state.setMember(channel, member.Id, mreqChannelMember{Status: gotgbot.ChatMemberStatusLeft})
		probe := mreqProbeLookups(env, state)
		ai.setOnRequest(func() { probe.add("ai") })

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		events := probe.log()
		if len(events) < 3 || events[0] != "photo" || events[len(events)-1] != "ai" {
			t.Fatalf("lookup requests in order = %v, want the photo first, the channel requests next and the AI request last", events)
		}
		for _, event := range events[:len(events)-1] {
			if event == "ai" {
				t.Errorf("lookup requests in order = %v, want one AI request at the end", events)
			}
		}
		want := mreqBitOf(t, "ph") | mreqBitOf(t, "ch") | mreqBitOf(t, "ai")
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != want {
			t.Errorf("hold = %+v, want the three lookup bits %d", hold, want)
		}
		text := mreqText(env.mreqNotices()[0])
		photo, ch, aiPhrase := strings.Index(text, staffMarker("mreq_rule_photo")), strings.Index(text, staffMarker("mreq_rule_channel")), strings.Index(text, staffMarker("mreq_rule_ai"))
		if photo < 0 || ch < photo || aiPhrase < ch {
			t.Errorf("notice %q should name the photo rule, the channel rule and then the AI rule", text)
		}
	})

	t.Run("an unanswerable AI never blocks an established name breach when it is equal or weaker", func(t *testing.T) {
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionMute,
			memberreq.RuleAI:       models.RequirementActionMute,
		}, false)
		ai.setDefault(mreqAIStatus(400, "invalid_request_error", ""))
		member := mreqAIMember(env, "")

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted for the name breach", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") {
			t.Errorf("hold = %+v, want only the last name bit", hold)
		}
		if text := mreqText(env.mreqNotices()[0]); strings.Contains(text, staffMarker("mreq_rule_ai")) {
			t.Errorf("notice %q names the AI rule, which was not established", text)
		}
		if n := ai.count(); n == 0 {
			t.Errorf("provider requests = 0, want the AI row asked")
		}
	})

	t.Run("an unknown AI stricter than the name punishment defers it, and the next sighting punishes", func(t *testing.T) {
		env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{
			memberreq.RuleLastName: models.RequirementActionWarn,
			memberreq.RuleAI:       models.RequirementActionBan,
		}, false)
		ai.setDefault(mreqAIStatus(400, "invalid_request_error", ""))
		member := mreqAIMember(env, "")

		mreqAIWantNothing(t, env, member, env.mreqEnforce(env.mreqCase(member)))

		ai.setDefault(mreqAIProbability(0.1))
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("second outcome = %+v, want Warned once the AI answers", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.Rules != mreqBitOf(t, "ln") || hold.Action != models.RequirementActionWarn {
			t.Errorf("hold = %+v, want the warn hold of the name rule", hold)
		}
	})
}

// mreqAIShape is one way the provider's answer cannot be turned into a probability.
type mreqAIShape struct {
	name string
	step mreqAIStep
	// hangs makes the budgets and the flight timeout short, because the request never answers.
	hangs bool
	// stops cancels the jobs' root context before the member is seen.
	stops bool
}

func mreqAIShapes() []mreqAIShape {
	return []mreqAIShape{
		{name: "a 400", step: mreqAIStatus(400, "invalid_request_error", "")},
		{name: "a 401", step: mreqAIStatus(401, "authentication_error", "")},
		{name: "a 403", step: mreqAIStatus(403, "permission_error", "")},
		{name: "a 429 with a short retry-after", step: mreqAIStatus(429, "rate_limit_error", "1")},
		{name: "a 429 with a long retry-after", step: mreqAIStatus(429, "rate_limit_error", "30")},
		{name: "a 500", step: mreqAIStatus(500, "api_error", "")},
		{name: "a refusal stop", step: mreqAIBody(raidAIClaudeRefusal())},
		{name: "an end_turn stop", step: mreqAIAnswerWithStop("end_turn", `[{"type":"text","text":"ordinary"}]`)},
		{name: "a missing tool call", step: mreqAIAnswerWithStop("tool_use", `[{"type":"text","text":"ordinary"}]`)},
		{name: "a probability of 1.5", step: mreqAIProbability(1.5)},
		{name: "a probability of -0.1", step: mreqAIProbability(-0.1)},
		{name: "a null body", step: mreqAIBody("null")},
		{name: "a truncated JSON body", step: mreqAIBody(`{"id":"msg_test","content":[{"type":"tool_use","name":"`)},
		{name: "a dropped connection", step: mreqAIDrop()},
		{name: "a request that outlives the budget", step: mreqAIHang(), hangs: true},
		{name: "the shutdown root cancelled", step: mreqAIProbability(0.97), stops: true},
	}
}

// mreqAIShapeEnv builds the environment of one shape: the rule stored at action, the stand-in
// answering the shape, and, for a hang, budgets short enough that the test does not wait for them.
func mreqAIShapeEnv(t *testing.T, shape mreqAIShape, action int, join bool) (*memberReqEnv, *mreqAIState, gotgbot.User) {
	t.Helper()
	env, ai := mreqAIEnvOf(t, map[memberreq.Rule]int{memberreq.RuleAI: action}, join)
	ai.setDefault(shape.step)
	if shape.hangs {
		mreqSwap(t, &memberReqLookupJobBudget, 150*time.Millisecond)
		mreqSwap(t, &memberReqLookupJoinBudget, 150*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
		// Runs before the row cleanups: the abandoned flight has ended by its own timeout.
		t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })
	}
	if shape.stops {
		restoreMemberReqRunner(t)
		StopMemberReqJobs()
	}
	return env, ai, mreqAIMember(env, "Casino")
}

func TestMemberReqAIUnanswerable(t *testing.T) {
	actions := []int{models.RequirementActionMute, models.RequirementActionWarn, models.RequirementActionKick, models.RequirementActionBan}
	secondSighting := func(t *testing.T, shape mreqAIShape, env *memberReqEnv, ai *mreqAIState, member gotgbot.User) {
		t.Helper()
		ai.setDefault(mreqAIProbability(0.97))
		if shape.hangs {
			time.Sleep(700 * time.Millisecond)
		}
		if shape.stops {
			mreqAIRestartRoot()
		}
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome == memberReqNone {
			t.Errorf("second sighting with a working answer: outcome = %+v, want a punishment (the member is judged again)", got)
		}
	}
	for i, shape := range mreqAIShapes() {
		action := actions[i%len(actions)]
		t.Run("job: "+shape.name, func(t *testing.T) {
			env, ai, member := mreqAIShapeEnv(t, shape, action, false)
			started := time.Now()
			got := env.mreqEnforce(env.mreqCase(member))
			if shape.hangs {
				if took := time.Since(started); took > 3*time.Second {
					t.Errorf("a hung request held the job for %s, want about the 150 ms budget", took)
				}
			}
			mreqAIWantNothing(t, env, member, got)
			secondSighting(t, shape, env, ai, member)
		})
		t.Run("join: "+shape.name, func(t *testing.T) {
			env, ai, member := mreqAIShapeEnv(t, shape, action, true)
			started := time.Now()
			env.mreqJoin(member)
			if shape.hangs {
				if took := time.Since(started); took > 3*time.Second {
					t.Errorf("a hung request held the join entry for %s, want about the 150 ms budget", took)
				}
			}
			mreqAIWantNothing(t, env, member, memberReqResult{})
			if got := mreqAIWelcomes(env); got != 1 {
				t.Errorf("welcomes = %d, want 1 (a joiner whose names cannot be judged is welcomed)", got)
			}
			secondSighting(t, shape, env, ai, member)
		})
	}
}
