//go:build testtools

package modules

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
)

// The cross-cutting tests of the AI spam-name rule (plan 15-10): the join never waits longer than
// its lookup budget for a slow provider, the request holds exactly the three names and the
// instruction, and nothing the call shares can starve it or be starved by it. The shutdown proof is
// TestShutdownEndsMemberReqAI (shutdown_drains_test.go) and the log sweep is the AI part of
// TestMemberReqLookupLogsCarryIDsOnly (memberreq_lookup_budget_test.go). No real provider is called.

// mreqAIWaitVerdictKey waits until the stand-in's answer has been cached and fails after five seconds.
func mreqAIWaitVerdictKey(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(mreqAIVerdictKeys(t)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the flight never cached its verdict")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestMemberReqAIJoinBudget: the join entry never parks a dispatcher goroutine longer than the join's
// lookup budget. A provider call slower than the budget leaves the member welcomed, unjudged and
// marked, the flight finishes on its own and caches the verdict, and the member's first message then
// judges from the cache with no second request. A slow photo lookup that spends the whole budget
// leaves the AI row unasked, and a message's job on a hung call never delays the update.
func TestMemberReqAIJoinBudget(t *testing.T) {
	t.Run("a call slower than the budget returns at the budget and the flight finishes by itself", func(t *testing.T) {
		env, ai := mreqAIJoinEnv(t, models.RequirementActionMute, true)
		mreqSwap(t, &memberReqLookupJoinBudget, 200*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 4*time.Second)
		const delay = 1500 * time.Millisecond
		ai.setDefault(mreqAIDelayed(delay, mreqAIProbability(0.97)))
		member := mreqAIMember(env, "Casino")

		started := time.Now()
		env.mreqJoin(member)
		took := time.Since(started)

		if took >= delay {
			t.Errorf("the join took %s, want it back at the 200 ms budget and not after the %s call", took, delay)
		}
		if keys := mreqAIVerdictKeys(t); len(keys) != 0 {
			t.Errorf("verdict keys = %v right after the join, want none: the call was still running when the join returned", keys)
		}
		mreqAIWantNothing(t, env, member, memberReqResult{})
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1 (a joiner whose call outlived the budget is welcomed)", got)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)

		// The flight outlived its caller: it finishes and caches the verdict on its own.
		mreqAIWaitVerdictKey(t)
		if got := ai.count(); got != 1 {
			t.Errorf("provider requests = %d, want 1", got)
		}

		// The first message judges from the cache: no second request, and the spam verdict punishes.
		env.mreqSend(member, "buy now")
		memberReqJobs.wait()
		if got := ai.count(); got != 1 {
			t.Errorf("provider requests after the first message = %d, want 1: the verdict was cached", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.Rules != mreqBitOf(t, "ai") {
			t.Errorf("hold = %+v, want a hold with the AI bit", hold)
		}
		if n := env.mreqCallCount("restrictChatMember"); n != 1 {
			t.Errorf("restrict calls = %d, want 1 (the mute)", n)
		}
	})

	t.Run("a photo lookup that spends the whole budget leaves the AI row unasked", func(t *testing.T) {
		withAIConfigured(t)
		ai := mreqInstallAIFake(t)
		mreqAICleanup(t, nil, nil)
		env := mreqJoinEnv(t, map[memberreq.Rule]int{
			memberreq.RulePhoto: models.RequirementActionMute,
			memberreq.RuleAI:    models.RequirementActionMute,
		}, true)
		withFastLookupPacer(t)
		state := newMreqLookupState()
		mreqInstallLookupFake(env, state)
		ai.setDefault(mreqAIProbability(0.97))
		mreqSwap(t, &memberReqLookupJoinBudget, 200*time.Millisecond)
		mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
		member := mreqAIMember(env, "Casino")
		mreqLookupCleanup(t, []int64{member.Id}, nil)
		state.set(member.Id, mreqPhotoAnswer{})
		release := state.hold(member.Id)
		t.Cleanup(release)
		// Runs before the row cleanups: the abandoned photo flight has ended by its own timeout.
		t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })

		started := time.Now()
		env.mreqJoin(member)

		if took := time.Since(started); took > 3*time.Second {
			t.Errorf("the join took %s, want it back at about the 200 ms budget", took)
		}
		if got := ai.count(); got != 0 {
			t.Errorf("provider requests = %d, want none: the photo lookup spent the whole join budget", got)
		}
		mreqAIWantNothing(t, env, member, memberReqResult{})
		if got := mreqAIWelcomes(env); got != 1 {
			t.Errorf("welcomes = %d, want 1 (a joiner whose lookups could not finish is welcomed)", got)
		}
		mreqAIWantMarker(t, env.chat.Id, member.Id, true)
	})

	t.Run("a message job on a hung call never delays the update", func(t *testing.T) {
		env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
		ai.setDefault(mreqAIHang())
		member := mreqAIMember(env, "Casino")
		mreqAIMarkNew(t, env.chat.Id, member.Id)

		returned := make(chan struct{})
		go func() {
			defer close(returned)
			env.mreqSend(member, "buy now")
		}()
		select {
		case <-returned:
		case <-time.After(3 * time.Second):
			t.Fatal("the update waited for the provider: the watcher must hand the call to a job and return")
		}
		ai.waitCount(t, 1)
		if hold := env.mreqHold(member.Id); hold != nil {
			t.Errorf("hold = %+v while the call is held, want none", hold)
		}

		// Release the hung request: it answers nothing, which is unknown, and nobody is punished.
		ai.releaseHangs()
		memberReqJobs.wait()
		mreqAIWantNothing(t, env, member, memberReqResult{})
	})
}

// mreqAISentinelMember mints a member of the environment's group whose names carry the sentinel and a
// counter, never the member's ID, so a test can look for either one in a body or a log line.
func mreqAISentinelMember(env *memberReqEnv) gotgbot.User {
	env.t.Helper()
	n := mreqAIUnique()
	return env.mreqMember(mreqAINameSentinel+"First"+n, mreqAINameSentinel+"Last"+n, strings.ToLower(mreqAINameSentinel)+n)
}

// TestMemberReqAIRequestIsMinimal: only the three bounded canonical names and the code-owned
// instruction reach the provider. The body the stand-in receives for a judgement made in a group with
// a distinctive title, by a member who also sent a distinctive message, holds no chat or user ID, no
// title, no message text and no field beyond the fixed request shape, and the key travels in its
// header alone (D-01, D-16: the settings page says only the first name, last name and username are
// sent).
func TestMemberReqAIRequestIsMinimal(t *testing.T) {
	env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
	ai.setDefault(mreqAIProbability(0.97))
	const titleSentinel, messageSentinel = "ZZGroupTitleSentinel", "ZZMessageSentinelText"
	env.chat.Title = titleSentinel
	member := mreqAISentinelMember(env)
	mreqAIMarkNew(t, env.chat.Id, member.Id)

	env.mreqSend(member, messageSentinel+" buy now")
	memberReqJobs.wait()

	if got := ai.count(); got != 1 {
		t.Fatalf("provider requests = %d, want 1", got)
	}
	request := ai.received()[0]
	raw := string(request.Body)

	for name, leak := range map[string]string{
		"the group's title":    titleSentinel,
		"the member's message": messageSentinel,
		"the chat ID":          strconv.FormatInt(env.chat.Id, 10),
		"the chat ID's digits": strconv.FormatInt(-env.chat.Id, 10),
		"the user ID":          strconv.FormatInt(member.Id, 10),
		"the API key":          raidAITestKey,
		"the group's default":  "Test Chat",
	} {
		if strings.Contains(raw, leak) {
			t.Errorf("the request body carries %s (%q): %s", name, leak, raw)
		}
	}
	if request.Key != raidAITestKey {
		t.Errorf("x-api-key = %q, want the configured key in the header", request.Key)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(request.Body, &fields); err != nil {
		t.Fatalf("decode the request: %v", err)
	}
	want := map[string]bool{"model": true, "max_tokens": true, "system": true, "thinking": true, "tools": true, "tool_choice": true, "messages": true}
	for key := range fields {
		if !want[key] {
			t.Errorf("the request has the field %q: only %v may be sent (no sampling parameter, no chat or user data)", key, want)
		}
	}
	for key := range want {
		if _, ok := fields[key]; !ok {
			t.Errorf("the request lacks the field %q", key)
		}
	}
	var maxTokens int
	if err := json.Unmarshal(fields["max_tokens"], &maxTokens); err != nil || maxTokens != 128 {
		t.Errorf("max_tokens = %s, want 128", fields["max_tokens"])
	}
	var thinking struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(fields["thinking"], &thinking); err != nil || thinking.Type != "disabled" {
		t.Errorf("thinking = %s, want disabled", fields["thinking"])
	}
	var choice struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(fields["tool_choice"], &choice); err != nil || choice.Type != "tool" || choice.Name != claude.ToolName {
		t.Errorf("tool_choice = %s, want the forced %s tool", fields["tool_choice"], claude.ToolName)
	}
	decoded := mreqAIDecode(t, request.Body)
	if decoded.System != claude.NamesInstruction {
		t.Errorf("the system text is not the code-owned instruction: %q", decoded.System)
	}
	if decoded.Tools != 1 {
		t.Errorf("tools = %d, want the one forced tool", decoded.Tools)
	}
	got := mreqAINamesOf(t, request.Body)
	canon := mreqAICanon(t, member.FirstName, member.LastName, member.Username)
	if got.First != canon.First || got.Last != canon.Last || got.Username != canon.Username {
		t.Errorf("the names sent = %+v, want exactly the canonical %+v", got, canon)
	}
}

// The paths of the AI log sweep. Each runs one failure shape of the provider (mreqAIShapes) or of the
// guards (mreqAIStateShapes) through a place that calls the rule and returns what the lines it wrote
// must not hold about the member it used: the hashes of their names.

// mreqAILogJob is the message job: the executor for a member recorded as new.
func mreqAILogJob(t *testing.T, shape mreqAIShape, action int) []string {
	t.Helper()
	env, _, _ := mreqAIShapeEnv(t, shape, action, false)
	member := mreqAISentinelMember(env)
	mreqAIMarkNew(t, env.chat.Id, member.Id)
	env.mreqEnforce(env.mreqCase(member))
	return mreqAIForbiddenOf(t, member)
}

// mreqAILogJoin is the join entry with Check on join on.
func mreqAILogJoin(t *testing.T, shape mreqAIShape, action int) []string {
	t.Helper()
	env, _, _ := mreqAIShapeEnv(t, shape, action, true)
	member := mreqAISentinelMember(env)
	env.mreqJoin(member)
	return mreqAIForbiddenOf(t, member)
}

// mreqAILogHeld is the held release: a member under a Warn hold that carries the AI bit sends a
// message, and the release asks the rule with their current names.
func mreqAILogHeld(t *testing.T, shape mreqAIShape) []string {
	t.Helper()
	env, _, _ := mreqAIShapeEnv(t, shape, models.RequirementActionWarn, false)
	member := mreqAISentinelMember(env)
	mreqSeedHeld(t, env, member, mreqBitOf(t, "ai"), models.RequirementActionWarn, 10*time.Minute)
	mreqAIMarkNew(t, env.chat.Id, member.Id)
	env.mreqEnforce(env.mreqCase(member))
	return mreqAIForbiddenOf(t, member)
}

// mreqAILogButton is the "I fixed it" press of a member muted for the AI rule who has since renamed
// to names the stand-in has no verdict for: the press asks the rule for the new names. The shape is
// applied after the mute, so the mute itself is an ordinary successful judgement.
func mreqAILogButton(t *testing.T, shape mreqAIShape) []string {
	t.Helper()
	env, ai := mreqAIWatchEnv(t, models.RequirementActionMute)
	member, _, notice := mreqAIMuteMember(t, env, ai, "Casino", mreqBitOf(t, "ai"))
	forbidden := mreqAIForbiddenOf(t, member)
	renamed := mreqAIOrdinaryRename(env, member)
	forbidden = append(forbidden, mreqAIForbiddenOf(t, renamed)...)

	ai.setDefault(shape.step)
	if shape.hangs {
		mreqSwap(t, &memberReqLookupFlightTimeout, 600*time.Millisecond)
		// Runs before the row cleanups: the abandoned flight has ended by its own timeout.
		t.Cleanup(func() { time.Sleep(700 * time.Millisecond) })
	}
	if shape.stops {
		restoreMemberReqRunner(t)
		StopMemberReqJobs()
	}
	if shape.setup != nil {
		shape.setup(t, env.chat.Id)
	}
	env.mreqPressFixed(renamed, notice)
	return forbidden
}
