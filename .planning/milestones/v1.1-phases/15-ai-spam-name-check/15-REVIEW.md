---
phase: 15-ai-spam-name-check
reviewed: 2026-11-09T00:00:00Z
depth: deep
files_reviewed: 24
files_reviewed_list:
  - migrations/20261109120000_add_member_requirement_ai_rule.sql
  - fuku/db/memberreq/settings.go
  - fuku/db/models/member_requirements.go
  - fuku/utils/claude/names.go
  - fuku/utils/claude/request.go
  - fuku/utils/namecanon/canon.go
  - fuku/modules/memberreq_ai.go
  - fuku/modules/memberreq_ai_budget.go
  - fuku/modules/memberreq_ai_breaker.go
  - fuku/modules/memberreq_rules.go
  - fuku/modules/memberreq_enforce.go
  - fuku/modules/memberreq_lookup.go
  - fuku/modules/memberreq_watch.go
  - fuku/modules/memberreq_held.go
  - fuku/modules/memberreq_join.go
  - fuku/modules/memberreq_notice.go
  - fuku/modules/memberreq_fixed.go
  - fuku/modules/settings_mreq.go
  - scripts/claude-spike/main.go
  - scripts/claude-spike/names.go
  - scripts/claude-spike/report.go
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
  - locales/uk.yml
findings:
  critical: 0
  warning: 4
  info: 4
  total: 8
status: issues_found
---

# Phase 15: Code Review Report

**Reviewed:** 2026-11-09
**Depth:** deep
**Files Reviewed:** 24 production files plus the 10 new locale keys in all 8 locale files (the diff `10bdf99..HEAD`)
**Status:** issues_found

## Summary

I read every file in scope in full (the shared files as the phase diff against `10bdf99`), traced the
chains lookup row -> flight -> gate -> budget -> call -> record -> executor -> claim, the watcher's cached
read, the held release, the join entry, "I fixed it" and the settings gate, and compared them with
`15-CONTEXT.md` (D-01..D-33), research S-03/S-04 and `15-UI-SPEC.md`. `CGO_ENABLED=0 go build ./...` is clean,
`go vet -tags testtools` is clean on `namecanon`, `claude` and the spike, and `go list -deps` shows the spike
imports only `fuku/utils/claude` and `fuku/utils/namecanon` (no `fuku/config` or `fuku/db`). The existing
families `TestMemberReqAI*`, `TestMemberReqFixed*`, `TestMemberReqHeld*` and `TestSettingsMreq*` pass
(`ok ... 44s`). I proved the findings below with a throwaway test in `fuku/modules` (a `zz_review_tmp_test.go`
built on the phase's own fixtures `mreqAIEnvOf`, `mreqAIJoinEnv`, `mreqAIPlant` helpers). **It has been
deleted; `git status` is clean.** No other file was touched.

The central safety property holds. I found no path from an unanswerable AI answer, a paused breaker, a
refused budget, a full gate, a cancelled context, a null, refusal, out-of-range or NaN value, a Redis error or
a stale cache to bit 128. The only producer of `memberReqBreach` for the AI row is `memberReqAIVerdictOf`, fed
by a finite probability in 0..1 from either `parseVerdict` (checked) or `memberReqAIParseProbability` (checked
for NaN, Inf and range), and the comparison is `>=` the 0.9 constant. Exempt members never reach the provider on
the punishing path. There are no Critical findings.

The real problems are about what the rule does when its Redis state is missing or the provider is down, which
is exactly where this rule differs from Phases 13 and 14:

1. **WR-01.** A hold that carries bit 128 is released, and a mute freed by "I fixed it", with no judgement of
   the still-spam names whenever Redis has lost `nm` and `nu` (flush, eviction, 30 days of silence). The durable
   hold row is ignored in favour of Redis-only absence. Proven.
2. **WR-02.** The baseline never refreshes, so every member's baseline lapses 30 days after the first
   sighting and the next sighting is a no-call re-baseline of whatever names they carry then. A dormant or
   hijacked account that was renamed to an advert is never judged. Proven.
3. **WR-03.** A Redis outage (or any single read error) is a plain unknown, so it suspends every weaker name
   punishment in every group whose AI action is stricter. D-22 removed exactly this coupling for the provider
   states but not for Redis. Proven.
4. **WR-04.** While the breaker is paused (a rejected key, billing, a long 429) every message of every member
   who still carries a join marker is a runner job plus a paced live `getChatMember`: Phase 14's WR-01 cost
   class, on the pacer and runner that Phase 13 punishments share. Proven.

## Warnings

### WR-01: A durable AI hold is released, and "I fixed it" lifts the mute, with no judgement when Redis has lost the baseline and the marker

**Files:** `fuku/modules/memberreq_ai.go:323-336` (`memberReqAIDecide`) and `:350-353` (`baselineOnly` branch of
`memberReqAIRule`); `fuku/modules/memberreq_held.go:76-93` (`memberReqHeldLookupsClear`, mode has no `Joining`);
`fuku/modules/memberreq_fixed.go:247-250` (`memberReqFixedJudge`, same mode).

**Issue.** The marker `nu` exists so "a baseline that expired during a long hold can never launder a member"
(file header, `:43-46`). It does not cover the loss of both keys. The marker has the same 30 day TTL as the
baseline and is only refreshed by a new breach, and Redis can lose both at any time. With `nv`, `nm` and `nu`
all absent, `memberReqAIDecide` reads the member as an established one it has never seen: `!joining && !marker`
and `baseline == ""` gives `baselineOnly`, which writes `nm = h` and answers `memberReqPass`. The two callers
that act on a durable record of a breach then trust that pass:

- `memberReqHeldLookupsClear` returns true, `ClearHoldOnClean` deletes the Warn hold, and the spam names are now
  the member's baseline (the watcher then keeps them satisfied for ever).
- `memberReqFixedJudge` returns nil for a muted member with unchanged spam names, so the mute is lifted and the
  hold taken (by reading; the held-release variant is proven).

The hold row in PostgreSQL carries bit 128 and is the authoritative proof that the names were judged spam.

**Failure scenario (reproduced).** AI rule on Warn. Member `M` with names "Promo<id> Casino" has a marker, is
judged 0.97 and warned (hold `Rules = 128`). Redis is flushed (restart without persistence, an eviction, or `M`
is silent for 30 days so both keys expire). `M` writes again with the same names. Result in the throwaway test
`TestZZReviewHeldReleaseAfterRedisLoss`: `outcome = 0, new provider requests = 0, hold = <nil>`, log line
`the hold of user ... was released: the member now meets what it was for`. No call was made and no name changed.

**Fix.** A hold that carries bit 128 must never be released by a baseline-only step. In
`memberReqHeldLookupsClear` and `memberReqFixedJudge` pass a mode that makes the AI row judge regardless of the
baseline (reuse `Joining: true`, or add `IgnoreBaseline bool` so "joining" keeps its meaning for the join entry).
Only the verdict cache or a real call may pass such a member. Cost: one call after a Redis loss per held member,
bounded by the budgets.

### WR-02: The baseline never refreshes, so any member silent for 30 days is re-baselined without judgement, whatever names they carry

**Files:** `fuku/modules/memberreq_ai.go:87` (`memberReqAIStateTTLDefault`), `:327-334` (`memberReqAIDecide`:
baseline equal is `memberReqAIFinal(memberReqPass)`, no write), `:371-398` (`memberReqAIRecord`: writes only on a
definite pass); `fuku/modules/memberreq_watch.go:240-245` (`memberReqAIWatchSatisfied`: baseline equal means no job).

**Issue.** `nm` is set once (SET NX at first sight, or by a pass) with a 30 day TTL. Every later sighting with an
equal baseline is satisfied in the watcher, so no job runs and nothing can extend the TTL (the phase's own test
asserts "baseline ttl ... want it not rewritten"). Thirty days after the first record the key is gone whatever
the member's activity. The next sighting then has no baseline and no marker and is recorded by `baselineOnly`
with the names the member has at that moment, with no call. D-02 accepted that "a spammer who was already in
the group when the rule was switched on" is first judged at a rename. It did not accept that the same hole
reopens for every member every 30 days.

**Failure scenario (reproduced).** An old account that joined the group months ago and has been silent for over
30 days is sold and renamed to an advert ("Promo<id> Casino", `@promo<id>`). Its baseline expired. It posts the
ad. Throwaway test `TestZZReviewBaselineLapse` with AI = Ban and a 0.99 provider: `outcome = 0, provider
requests = 0, hold = nil`. The hijacked-dormant-account pattern is precisely the common spam profile. An active
member's gap is only the seconds between expiry and their next message, so the practical exposure is the dormant
and resold account, but it is every member of every group.

**Fix.** Refresh the baseline where the job already reads it (the equal-baseline pass can `EXPIRE nm` in the job;
the watcher must stay read-only, so make the watcher not-satisfied when the baseline's remaining TTL is under
a margin, or simply use a TTL that comfortably outlives dormant accounts, 180 days or more, and say so in the
rulebook). Alternatively judge, instead of baselining, a member who has a Redis record of ever having been
sighted (for example `nm` tombstone) but a lapsed baseline.

### WR-03: A Redis outage or read error is a plain unknown, so it suspends every weaker name punishment in groups whose AI action is stricter

**Files:** `fuku/modules/memberreq_ai.go:312-315` (`memberReqAIPrepare`: state read error returns
`memberReqUnknown`, not sticky), `:457-460` (`memberReqAIJudge` read error returns `memberReqAIAnswer{}`),
`:530-533` (counter fault returns a non-sticky answer); `fuku/modules/memberreq_enforce.go:309-310`;
`fuku/modules/memberreq_rules.go:~268` (`memberReqUnknownDefers`).

**Issue.** D-22 made a paused breaker, a used-up budget and a full gate sticky so "an AI outage ... never
suspends the weaker name punishments in every group". A Redis outage is the same kind of system state and is
more common, but the AI row returns it as a non-sticky unknown (and a missing Redis client is the same). Redis is
required for the row to say anything at all (cache, baseline, marker, budgets), so with Redis down the AI row is
unknown for every member, always, and `memberReqUnknownDefers` then drops any weaker punishment of a member who
broke a name rule. Phase 14's rules do not behave like this (their positive-only caches degrade to "ask
Telegram").

**Failure scenario (reproduced).** Rules: last name Warn, AI Ban. Member with a blank last name. Results of the
throwaway tests: Redis up, Warn is applied (`outcome=2 hold=true`); Redis down with last name Warn alone, applied;
Redis down with last name Warn and AI Warn, applied (equal severity); **Redis down with last name Warn and AI
Ban: `outcome=0 hold=false`**. The member is judged again at every message and is never warned for as long as
Redis is down, in every group that chose a stricter AI action.

**Fix.** Mark `no Redis`, a failed state read, a failed cache re-read and a budget counter fault as sticky (call
`memberReqNoteSticky` for them: they are states of the system, not faults of one member's answer), or make
`memberReqUnknownDefers` ignore the AI bit whenever `memberReqRedisClient() == nil`. Keep the verdict unknown so
the held release and "I fixed it" still refuse.

### WR-04: While the breaker is paused every message of every member with a join marker is a job and a paced live lookup

**Files:** `fuku/modules/memberreq_watch.go:240-245`, `:262-295` (`memberReqAIWatchSatisfied`,
`memberReqLookupCached`: a marker with no cached verdict is never satisfied); `fuku/modules/memberreq_ai.go:501-504`
(`memberReqAIAcquire`: a paused breaker returns a sticky unknown); `fuku/modules/memberreq_join.go:103-105`
(every joiner gets a marker, 30 days); `fuku/modules/memberreq_enforce.go:635-690` (the job's live `getChatMember`).

**Issue.** The marker is deleted only by a definite pass. While the provider answers nothing (a revoked key or
exhausted credit pauses every group for 5 minutes and the pause repeats at each retry; a long 429; a per-group
trip), every member who joined while the rule was live, that is a large share of the active members for 30 days,
stays marked and unjudged. The watcher cannot see the breaker, so each of their messages starts a runner job
(approvals read, captcha read, fresh hold query, Redis pause read, one live `getChatMember` on the fleet-wide
`memberReqPacer`, settings read, state read) that ends in `memberReqAIPaused` and does nothing. That is the cost
class Phase 14's WR-01 documented, on the runner (32 jobs per replica) and the pacer (about 10 calls a second
fleet-wide) that Phase 13 punishments and mute notices use, so real name-rule violators are shed behind them.

**Failure scenario (reproduced).** AI rule on Mute, provider answers 401, member with a marker writes six clean
messages: `TestZZReviewMarkerHolderCostsAJobPerMessage...` reports `provider requests = 1, live getChatMember
lookups = 6`. The first call trips the 5 minute pause (log: `AI name checks paused for every group`); the other
five messages each cost a paced live lookup and a job.

**Fix.** `memberReqAIPaused(chatID)` is in-memory and pure, so the watcher may consult it: when the AI part is the
only reason a member is not satisfied and `memberReqAIPaused(chatID)` (or the provider-wide pause) holds, treat
the AI part as satisfied for this message (nothing can be judged now; the marker stays and the first message
after the pause is judged). This keeps "the watcher writes nothing, calls nobody" true.

## Info

### IN-01: The verdict key and the baseline ignore case, but the text sent to the model keeps it

**File:** `fuku/utils/namecanon/canon.go:86-95` (`namesHash` lower-cases the first and last name),
`:58-72` (`Of` keeps their case in `Canon.First` / `Canon.Last`, which `claude.Names` sends).

**Issue.** "FREE CRYPTO" and "free crypto" are one baseline and one `nv` key but two different strings for the
model. Whichever variant is judged first decides the verdict of both for 24 hours, and a rename that only
changes case is not a rename (baseline equal, no judgement), although the research itself calls all caps "a spam
signal" and keeps the case for that reason. Exploiting it needs the model to score two case variants of one
string on opposite sides of 0.9, so it is a consistency defect, not a framing attack.

**Fix.** Either hash the case-preserved first and last name (a case change is then a rename and a separate
verdict) or send the lower-cased text. Bump `mrn1` and `mrv1` with the change, as the file demands.

### IN-02: A refusal is never cached and counts as unknown, so it costs a call per message and defers weaker punishments

**Files:** `fuku/modules/memberreq_ai.go:477-481` (error path returns `memberReqAIAnswer{}`),
`:541-553` (`memberReqAIAfterFailure` records a refusal as a breaker success and caches nothing).

**Issue.** Names that make the provider refuse (adult-content adverts are the research's own example) are
re-asked at every trigger until the 5 per account per hour cap, and each answer is a non-sticky unknown, so a
stricter AI action also defers the member's weaker name punishment on those messages
(`memberReqUnknownDefers`). The spend is bounded by the account budget (5 per hour), so this is waste and a
short-lived deferral, not a bypass.

**Fix.** Cache a refusal for a short time as its own marker (for example `nv` value `refused`, read as unknown
without a call) or treat it as sticky.

### IN-03: The spike's names CSV writes attacker-controlled names unescaped

**File:** `scripts/claude-spike/names.go:288-318` (`writeNamesCSV`, columns `first_name`, `last_name`, `username`).

**Issue.** The names come from the owner's labelled list, which is expected to hold real spam names. A name
beginning with `=`, `+`, `-` or `@` is written as the cell text and is executed as a formula when the owner opens
the output in a spreadsheet (CSV/DDE injection). The key is never printed and the names appear only in the owner's
own file, as designed; this is the one place a spam name can act.

**Fix.** Prefix a `'` (or a space) to any cell that starts with `= + - @`, tab or carriage return, or write the
names only when asked (`-csv-names`).

### IN-04: One flight serves every chat, so one chat's breaker, budget or cap decides the answer of another chat's caller

**Files:** `fuku/modules/memberreq_ai.go:358-360` (`memberReqFlightValue` keyed by verdict only, leader's
`chatID` / `userID` passed to `memberReqAIJudge`), `:500-515`, `:520-535`.

**Issue.** Concurrent callers of one string in different chats share the leader's guards. If the leader's group
is paused or over its group budget (or the leader's account is over its 5 per hour), the answer, sticky or not, is
returned to a follower in a healthy chat that would have had a unit; conversely a follower is never charged. It is
transient (the next message re-judges) and only happens for the same canonical string at the same moment (a
botnet template, a common name), so it is a coupling note, not a bug.

**Fix.** Key the flight by verdict key and chat, or let a follower re-enter once with its own guards when the
answer is a sticky unknown.

## Checked and found sound

1. **Never punish on a guess (focus 1).** Bit 128 has one producer: `memberReqAIVerdictOf` (`>= memberReqAIBar`).
   Inputs are `parseVerdict` (stop reason `tool_use`, tool name, finite 0..1) and `memberReqAIParseProbability`
   (NaN and Inf rejected before the range test). A refusal is `ErrBlocked` (unknown, not cached). A missing
   client, a Redis error, a cancelled context (`memberReqFlightValue` returns no value, the zero verdict is
   unknown), a paused breaker, a refused or failed budget, a full gate and a recovered panic (the flight
   recovers inside the function, the gate is freed by `defer`) are all unknown and cache nothing. `nv` stores the
   probability, so a retuned bar applies at once, and an unparsable cached value is a miss.
2. **Exemptions (focus 2).** `memberReqNeverChecked`, approvals and pending captcha are judged before the hold read;
   the AI lookup in `memberReqLiveViolation` runs after the live `getChatMember` eligibility check and the fresh
   settings read; the re-read judges the exemptions again (SEC-02). The join entry sets only the marker for an
   exempt joiner (no call). The held release runs after the exemptions and can only delete a hold, never punish;
   an administrator promoted while held can cost one call, as AGENTS.md states, and `memberReqDeleteHeld` still
   protects their messages. The watcher's filters (supergroup, no `sender_chat`, no service account) still run first.
3. **D-21 algorithm (focus 3).** Order matches S-03: names nil unknown, not configured inert, not judgeable inert,
   no Redis or a read error unknown, one pipelined `GET nv, nm, nu` with every command read on its own (so a first
   `redis.Nil` cannot hide a later error), cache decides first, baseline step, then the flight. A pass is one
   `TxPipeline` (`SET nm`, `DEL nu`); `SET NX` for the baseline-only step; a breach sets the marker; unknown
   writes nothing. A rename between the read and the write is safe (an old-hash pass leaves a baseline that
   differs from the new names, so the member is judged again). Two messages of one member share one flight; two
   replicas can make two calls at most, bounded by the budgets. A rejoiner is judged (`Joining`). The marker
   survives a long hold (S-03, and `TestMemberReqAIMarkerSurvivesLongHold`), with the Redis-loss exception in
   WR-01. The watcher's cached read queues three `GET`s, writes nothing, calls nobody and is recovered by the
   existing `ext.ContinueGroups` default; the accepted watcher gap is as documented.
4. **Flight, gate, order (focus 4).** The order is cache re-read, breaker, gate (3 s bounded wait, released by
   `defer` right after a successful acquire), budget (account, group, fleet, a refusal stops the chain), call,
   breaker record, cache write. A gate that cannot be had spends no unit. The flight context is
   `memberReqContext()` plus the 15 s bound (gate 3 s plus the client's 8 s fits). Sticky is noted by each
   caller from the flight's value, not by the leader.
5. **Canonical form and hashes (focus 5).** Bidi, Cc, Cf, default-ignorable runes and U+2800 are dropped before
   and after NFKC; whitespace collapsed; rune cuts 64/64/32; length-prefixed fields so no field slides; lone
   "Deleted Account" is inert; the verdict key carries the model and `mrv1`. TTLs are 30 days (`nm`, `nu`) and 24 h
   (`nv`). Collisions between people are by design (the verdict is about the string).
6. **Privacy and secrets (focus 6).** No name, hash, probability, instruction or key is logged: the AI log lines
   carry chat and user IDs, status and error type; `HTTPError.Error()` never includes the server message;
   `raidAIScrub` runs over the key. Keys and Redis values hold hashes and a probability. The request is the
   forced strict tool call with the system instruction and one JSON user block of the three canonical strings;
   `json.Marshal` escapes quotes, backslashes and angle brackets. The Anthropic key is registered with `logredact`.
7. **D-22 and D-29 (focus 7).** `unknownMask &^= touch.sticky` removes only the AI bit when the unknown was
   sticky; any other unknown still defers (WR-03 and IN-02 are the gaps). The stale-names guard drops bit 128
   when the re-read record's names hash differs, and the name rules judge the re-read names. I found no path where
   a changed name is punished on an old verdict through the executor.
8. **Settings page and gate (focus 8).** `AINotConfigured` is set at every return through the wrapper
   `settingsMreqReadStatus`; the Off-only form, the `[Off]`-alone shape, the slot priority and the page lines match
   UI-SPEC M15/M16 and the composition table. `settingsMreqAIGate` is pure (no Redis, no Telegram, no network),
   never refuses Off, runs after the same-value read in `settingsMreqChoose` and after the expiry check in
   `settingsMreqYes`, so an expired Yes writes nothing. No callback grew; the card consequence comes from
   `ConfirmKeys`.
9. **Notice tails and "I fixed it" (focus 9).** `memberReqNoticeRemedies` appends photo then AI after translation
   for the plain warn, kick and mute notices only. "I fixed it" reads the cache for unchanged names and makes one
   call only for changed ones (D-23), under the 5 s budget; an unknown or paused answer is
   `mreq_fixed_check_failed` and changes nothing; approved members and foreign mutes are untouched by this phase.
   (The Redis-loss exception is WR-01.)
10. **Spike (focus 10).** The key is read from the environment only and never printed (`describeError` scrubs it);
    the endpoint check is the existing one (https except loopback); names go only to stdout or the `-csv` file
    (see IN-03); stderr messages carry row numbers and counts; the imports are `claude` and `namecanon` only.
11. **Shutdown (focus 11).** No new drain, goroutine set or queue: the call's context derives from
    `memberReqContext()`, `StopMemberReqJobs` ends it, and the budget counter is bounded by `raidRedisTimeout`.
    `main.go` and the shutdown files are untouched, so the 65 s worst case is unchanged.
12. **Locale texts (focus 12).** All 10 keys exist in all 8 files with placeholder parity with English
    (`{state}` on the cell key, `{rule}` on `settings_mreq_here_ai`, none on the others). The statements about
    what is sent, who is checked and when match the code (new members at join or first message, others after a
    rename); the remedy and card sentences are consistent with the approval exemption and the mute button. I
    found nothing that would mislead an owner or a member.
13. **Data layer.** The migration is one idempotent `ADD COLUMN IF NOT EXISTS ... CHECK` with no top-level
    constraint, no transaction control and a name that sorts after the Phase 14 file; the model, `Rules`,
    `ruleColumns`, `actionField` and `SetRuleAction` stay column-targeted with `cache.DeleteCache`; a cached
    value of the previous build decodes `ai_action` as 0 (off).

## Summary of counts

Critical 0, Warning 4, Info 4, total 8.

**Top three risks:**

1. WR-01: a held spam name is washed by any Redis loss, and "I fixed it" has the same hole.
2. WR-02: the 30 day absolute baseline reopens the "established member" hole for dormant and resold accounts.
3. WR-03 and WR-04 together: Redis or provider trouble either suspends weaker punishments (stricter AI action) or
   floods the shared runner and pacer that Phase 13 punishments depend on.

---

_Reviewed: 2026-11-09_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
