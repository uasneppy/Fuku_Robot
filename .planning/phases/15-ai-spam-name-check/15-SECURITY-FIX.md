---
phase: "15"
slug: "ai-spam-name-check"
status: all_fixed
findings_in_scope: 4
fixed: 4
accepted: 1
skipped: 0
source: 15-SECURITY.md (New Threats Found by the Audit: SEC-01 to SEC-05)
red_commits: "894ae8e, 9a3f7cb, 87988d9"
green_commits: "82e14e1, 27bb07e, 5c44ace, 9743cd8"
created: "2026-11-10"
---

# Phase 15 - Security hardening pass (SEC-01 to SEC-04, SEC-05 accepted)

Four findings of the audit in `15-SECURITY.md` were fixed in the orchestrator's order. SEC-01, SEC-02 and SEC-04 went test
first (a RED commit that failed for the stated reason, then a GREEN `fix(15)` commit); SEC-03 is docs only. SEC-05 is
accepted as AR-17, and the one residual this pass leaves (an account's own allowance does not latch) is AR-18. No
`go.mod`, `go.sum`, migration, Redis key family, handler group, drain or secret changed. Nothing was pushed, no history was
rewritten, and no requirement was ticked or unticked (`REQUIREMENTS.md` untouched, no `phase.complete`).

## SEC-01 (medium): an established member was punished through another account's cached breach

**What changed.** `memberReqAIDecide` (`fuku/modules/memberreq_ai.go`) now answers pass first for a caller that is not
joining and not `IgnoreBaseline`, has no marker and whose baseline equals the current names hash, before it looks at the
verdict cache (nothing is written). The cache is keyed by the canonical string, which is global to the fleet, so a breach
that another account earned could reach a member who never renamed whenever a runner job reached the AI row for another
reason (an expired photo positive, say), while the watcher half stayed satisfied. Every other order is as before: a joiner,
a marker holder, a member whose baseline differs (a rename), an absent baseline and the `IgnoreBaseline` callers (the held
release and "I fixed it") still use the cached verdict and the flight, so a breach cached for a new member or a renamer
still applies.

**Files.** `fuku/modules/memberreq_ai.go` (the function and its comment), `fuku/modules/memberreq_watch.go` (the comment of
`memberReqAIWatchSatisfied`: the gap is now the rule), `AGENTS.md` (the "full order" sentence of the AI rule bullet and the
watcher bullet reworded as the rule of both halves), `15-SPIKE.md` (the follow-up on the baseline).

**Tests** (`fuku/modules/memberreq_ai_sec_test.go`, `TestMemberReqAIEstablishedMemberIgnoresCachedBreach`): an equal baseline
with no marker and a planted 0.97 breach punishes nobody, makes 0 provider requests, writes no marker and leaves the planted
baseline untouched (and the rule itself answers pass); a renamer, a member with no baseline, a marker holder and a joiner
with the same cached breach are all Muted; a held member and the rule with `IgnoreBaseline` keep reading the cached breach
(the hold stays, no punishment, 0 requests).

**RED and GREEN.** RED `894ae8e`: the executor returned Muted with a hold, a restrict call and a marker, and the rule
answered breach; the six control cases already passed. GREEN `82e14e1`. The GREEN commit also corrects one assertion of my
own RED test (the planted baseline has no expiry, so the test checks it is untouched rather than expecting a 180 day TTL).

**Existing tests edited:** none pinned the old order (the whole AI, watcher, held and fixed families passed unchanged). Two
labels in `memberreq_ai_watch_test.go` that called the case "the recorded gap" were reworded to "the rule (SEC-01)"; their
expectations (satisfied in the watcher) are unchanged.

## SEC-02 (medium): a spent allowance, a full gate or a failed counter still cost a job per message

**What changed.** A new in-memory refusal latch (`fuku/modules/memberreq_ai_latch.go`: `memberReqAILatch`,
`memberReqAILatches`, a bounded map of 10,000 chats like the breaker's, per replica, an ended entry forgotten on read and
pruned to make room, one warning when full). The AI row sets it: `memberReqAIReserve` now says which allowance refused
(`memberReqAICapped` for the account, new `memberReqAICappedGroup` and `memberReqAICappedFleet`); a group or fleet refusal
latches the chat until the end of the current UTC hour (`raidNow`, so a test stops the clock), and a gate that stayed full
while the flight's context was live and a failed counter latch it for 60 seconds. An account's refusal sets nothing.
`memberReqAIOnlyPaused` (`memberreq_watch.go`) consults the latch exactly as it consults `memberReqAIPaused`: when the AI
part is the only open reason and the chat is paused or latched the watcher starts no job, makes no live lookup and runs no
clean path (the third outcome, so a hold for an AI breach is not deleted); any other open reason still hands a job. The
executor, the join entry, the held release and "I fixed it" ignore the latch.

**Files.** `memberreq_ai_latch.go` (new), `memberreq_ai_budget.go`, `memberreq_ai.go`, `memberreq_watch.go`, `AGENTS.md` (the
watcher bullet, a latch paragraph in the AI rule bullet beside the breaker's, the test-hygiene lines),
`memberreq_ai_fake_test.go` (`mreqAICleanup` installs an empty latch per test through `mreqSwap`, so a test that latches a
chat restores the previous latch in `t.Cleanup`).

**Tests** (`fuku/modules/memberreq_ai_latch_test.go`): `TestMemberReqWatcherAILatch`, through the real dispatcher and runner:
with the group allowance spent, and with the fleet's, a marker holder's six clean messages after the first cost 0 jobs, 0
live requests and no keyspace change; a full gate and a failed counter latch for 60 seconds (59 s still quiet, 61 s later one
job) and release; an account's own allowance does not latch (the next member's message and the same member's both start a
job); a member held for an AI breach keeps the hold, writes no pass key and is not punished; another open rule (the photo)
still hands a job; after the stopped clock crosses the hour the next message starts a job, is judged (1 provider request) and
gets a baseline. `TestMemberReqAILatch` is the unit test of the type (expiry, never shortening, hour versus minute, the bound
and the one warning).

**RED and GREEN.** RED `9a3f7cb`: 6 of 6 messages were jobs with the group allowance, the fleet allowance, the full gate and
the failed counter, and the live lookups ran; the controls (account allowance, another rule) passed. GREEN `27bb07e`; a
follow-up `6f1ae21` made one condition read one reason at a time after the lint stand-in's QF1001.

**Decisions and consequences.** A fleet refusal latches only the chat whose call met it (the spec's per-chat latch), so every
other chat and replica pays one job before its own latch; a fleet-wide latch would be cheaper but was not asked for. While a
chat is latched a member whose string has a cached breach is not acted on either (the job that would punish from the cache
is not started), the same safe direction as the breaker pause; they are judged by the first message after the latch. Both are
in `AGENTS.md`. AR-18 records the account-allowance residual.

## SEC-03 (low): `sample.env` and the docs described `ANTHROPIC_API_KEY` as image-only

Docs only, `5c44ace`. `sample.env` has two new lines under `ANTHROPIC_API_KEY` (the same key carries a member's first name,
last name and username when the AI spam-name rule is set to a punishment, Off by default; retention and terms are the owner's
Anthropic account's). `docs/src/content/docs/api-reference/environment.md` (a frozen page, marker kept) says the account
terms and retention apply to name checks too, and the README row says the same. `make generate-docs` then `make check-docs`:
no drift. No test.

## SEC-04 (low): the three AI card texts said "when they join" only

**What changed.** `settings_mreq_confirm_ai_kick`, `_ban` and `_mute` now say a member is checked "when they join or at
their first message, or after a name change" in all 8 locale files (each translated in the register of its existing key; the
mute text still quotes the `mreq_btn_fixed` words). `settingsLocaleLimits` ceilings rose by 26 characters for English
(190/180/200 to 216/206/226) and 36 for the other column (266/252/280 to 302/288/316). The seven translations are on the
native-reader list of `15-SPIKE.md` check 10 with the new clause quoted, and `AGENTS.md` records the wording.

**RED and GREEN.** RED `87988d9` changed only `locales/en.yml` with the ceilings untouched: `TestSettingsLocaleKeys/en` failed
(kick 198 over 190, ban 189 over 180, mute 210 over 200). GREEN `9743cd8` (the other seven locales, the ceilings, the spike
list); `32e2484` records the wording in `AGENTS.md`. The rendered-card fit tests in 8 languages
(`TestSettingsMreqPageFitsEveryLocale`, `TestMemberReqScreensFitEveryLocale`), `TestSettingsMreqCard`, the privacy sweep
`TestSettingsStaffPrivacy` and `TestSettingsLabelLengths` stayed green with no Telegram limit touched. The generated docs did
not drift.

## SEC-05 (low): accepted, AR-17

Redis cardinality of `nu` and `nm` is bounded by population in a single-owner deployment. Nothing changed; AR-17 in
`15-SECURITY.md` carries the rationale and the two options (size Redis for the join rate, or clean `nu` at `leftMember`).

## `15-SECURITY.md`

SEC-01 to SEC-04 marked Fixed with commits and tests, SEC-05 accepted (AR-17), AR-18 added, escape items 4, 7 and 9 reworded,
the residual remarks of T-15-09, T-15-13, T-15-19, T-15-25, T-15-31 and the review's WR-04 row updated, frontmatter
`findings_open_nonblocking: 0` (`findings_fixed: 4`, `findings_accepted: 1`), and an audit-trail row for this pass.

## New concerns

- **SEC-NEW-01 (low, recorded, not acted on):** the latch is per replica and per chat, so a rolling fleet pays one job per chat
  per replica per refusal event before each latch is set, and a restart forgets every latch. Bounded by the 32-job runner and
  the pacer; no wrong punishment. Folded into AR-18.
- None other. No new secret, key family, endpoint, log line carrying a name, goroutine or drain.

## Verification (real results)

- `gofmt -l` on every file I touched: clean (the repository's pre-existing unformatted files are not mine).
  `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` ok.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit read directly, output to
  a file: **exit 0**, "no dupl, gocyclo or godox findings on lines added since 8c0c827" (an earlier run reported QF1001 on the
  SEC-02 condition, fixed in `6f1ae21`, then exit 0). gocyclo over the touched production files with `-over 19`: exit 0
  (highest 19, `memberReqLookupCachedWith`, unchanged; `memberReqAIDecide` 8).
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: **exit 0**.
- gosec (`gosec -quiet -no-fail -fmt json -out <scratchpad>/gosec-sec.json ./...`): 17 issues repo-wide, the same as the
  audit's; of the non-test Go files changed since `8c0c827` the only issue is the baseline `fuku/modules/warns.go:36 G115`.
  No identifier I added contains token, secret, password, credential, apikey or api_key.
- `make check-translations`: exit 0, all translations present in all 8 locales. `make generate-docs` then `make check-docs`:
  "No drift detected".
- New tests with `-race -count=5`: ok. Focused runs after each fix: the AI, watcher, fixed, held and shutdown-AI families ok
  (`sec01` 111 s, `sec02` 169 s), the settings page, privacy and screen-fit tests ok.
- CI gate on the changed and new families: `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(TestSettings|TestLoadSettings|TestMemberReq|TestMemberRequirement|TestShutdown|TestChatRequirementsSettings|TestApplyMemberReqAction|TestApplyWarn)' ./fuku/modules ./fuku/db/memberreq ./fuku/i18n ./scripts/claude-spike ./fuku/utils/claude ./fuku/utils/namecanon > <scratchpad>/gate.json`,
  then `go run ./scripts/check_test_results < <scratchpad>/gate.json`: **exit 0** (no covdata fallback was needed). 2959 test
  passes, 0 failures, 0 skips, every package `ok` (`fuku/modules` 749 s, `fuku/i18n` 7.8 s, the others about 1 s).
- The gate was started after the QF refactor `6f1ae21`, so it covers that HEAD (this report and `15-SECURITY.md` are the only
  later changes).

## Not run

`make test` and `make lint` as wholes (the sandbox cannot run them), `make test-postgres-integrity` and the PostgreSQL run of
anything (no migration or `CHECK` changed), the unfiltered `fuku/modules` suite, `node --test scripts/captcha-page/*.test.mjs`
(the page did not change), and the live Anthropic and Telegram checks of `15-SPIKE.md` (the owner's).
