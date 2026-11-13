---
phase: "14"
slug: "profile-photo-and-channel-rules"
status: all_fixed
findings_in_scope: 2
fixed: 2
skipped: 0
source: 14-SECURITY.md (New Threats Found by the Audit: SEC-02, SEC-03)
red_commit: ec6f839
green_commit: 7f7b0c7
created: "2026-11-08"
---

# Phase 14 - Security hardening pass (SEC-02, SEC-03)

Two findings of the audit in `14-SECURITY.md` were fixed test first: a RED `test(14)` commit that failed its new
assertions for the stated reason, then one GREEN `fix(14)` commit. SEC-01 (the channel ID in Debug logs, AR-10) and SEC-04
(press and runner load, AR-07) are accepted risks and were not touched. No `go.mod`, `go.sum`, migration, locale key,
handler group, Redis key family or shutdown drain changed.

## SEC-03 (medium): a demoted bot was still read as a channel administrator for up to 2 minutes

**What changed.** `memberReqGateBreach` (`fuku/modules/memberreq_lookup.go`), the D-22 gate that turns a not-subscribed
candidate (left, kicked, restricted without `is_member`) into a breach, now forces `mode.Fresh = true` before it calls
`memberReqBotChannelStatus`. The gate therefore never believes the cached `ba` administrator standing: it reads the bot's
status through the existing coalesced `f:b:<channel>` flight behind the lookup pacer (bounded by the flight timeout), the
same shape as WR-05's fresh link re-read. A definite administrator answer caches `ba` again and the breach stands. A
definite non-administrator (a member, restricted, left or kicked status, or a 400 or 403 by code) makes the candidate
inert, writes the channel-level `cna` fact and deletes the stale `ba`. An answer that cannot be established (429, pacer
shed, 5xx, transport error, timeout, null, unrecognised status) is unknown and never a breach. Only breach candidates pay
the one extra request: a subscribed member never reaches the gate, and a `Fresh` caller (the tap, "I fixed it") already
read fresh, so it pays no second read. No new function was needed.

**Files.** `fuku/modules/memberreq_lookup.go` (gate and its doc comment), `AGENTS.md` (the `ba` key bullet and the D-22
gate bullet now say the gate reads fresh).

**Tests** (`fuku/modules/memberreq_sec_test.go`, `TestMemberReqDemotedBotNeverBreaches`): a bot demoted while `ba` is cached
positive makes none of `left`, `kicked`, `restricted` (not a member) a breach (no hold, no write, no notice), writes `cna`,
deletes `ba` and costs exactly one bot-status request; a bot still an administrator keeps the breach (Ban lands, `ba`
cached again, no `cna`) at exactly one extra fresh request; seven unanswerable fresh shapes (429 that keeps coming, 500,
transport error, null answer, unrecognised status, timeout, pacer shed that only hits the bot read) are unknown and write
no `cna`; a subscribed member costs no bot-status request and a `Fresh` caller exactly one.

**RED and GREEN.** RED `ec6f839`: the demoted-bot cases, the unanswerable cases and the one-extra-request case failed
(a Ban landed and no bot-status request was made behind the cached `ba`). GREEN `7f7b0c7`.

**Existing tests whose expectation legitimately changed** (both in `fuku/modules/memberreq_lookup_test.go`, family
`TestMemberReqChannelBreachNeedsBotAdmin`): "an administrator bot is cached for 2 minutes and the second breach asks nothing
about the bot" now expects 2 bot-status requests (one fresh read per candidate; the breach and the 2-minute `ba` TTL
assertions are unchanged), and "Fresh bypasses the ba cache and drops a stale positive" now expects the stale positive to be
inert and dropped without `Fresh` too, then re-seeds it and keeps its original Fresh assertions. Both premises were the
exposure SEC-03 names.

**Decisions.** Fresh in the gate rather than a separate fresh-check function, so the existing flight, pacer, classifier and
cache writes are the single path; the fresh read replaces the `ba` read on the breach path only (the page's status line and
an unanswerable member lookup still read through the positive cache, which is the inert-direction use).

## SEC-02 (low): the approved and captcha-pending exemptions were read once, before the lookup phase

**What changed.** The WR-03 re-read branch of `memberReqLiveViolation` (`fuku/modules/memberreq_enforce.go`), taken when
something is broken and the lookup phase made or waited for a request or ran over `memberReqLookupStaleAfter` (1 s), moved
into a new helper `memberReqRereadAfterLookups` (kept under gocyclo 20). It first runs `memberReqExempt` again (approvals
through the tri-state reader, then the pending captcha: tri-state, an unknown is exempt), and only then makes the live
`getChatMember` re-read and the eligibility and names judgement as before. An exempt result or a read error punishes
nobody. A phase of cache hits that was fast makes neither the extra approvals read nor the extra live read.

**Files.** `fuku/modules/memberreq_enforce.go`, `AGENTS.md` (the WR-03 sentence of the executor bullet).

**Tests** (`fuku/modules/memberreq_sec_test.go`, `TestMemberReqExemptionsReadAfterLookups`, over Ban, Kick, Mute and Warn):
a member approved during the lookup phase, a member whose captcha attempt starts during it, and a member whose approvals
read fails after it are never punished (no hold, no warn row, no write, no notice); a lookup phase that made a request
costs exactly one more approvals read (2 in all) and a breach still punishes; an all-cache fast phase costs no extra
approvals read (1 in all). The tests swap the approvals and captcha seams once before the run, with an atomic flag the
lookup hook flips, so no package variable is written while a flight runs.

**RED and GREEN.** RED `ec6f839` (every "becomes exempt" case punished; the "two reads" case saw one). GREEN `7f7b0c7`.

**Decisions.** The exemptions are re-read before the live re-read (database reads are cheaper than a paced Telegram
request and stop a lost cause earlier); the join entry gets the same protection because it goes through the same executor
(4 s lookup budget).

## Verification (real results)

- `gofmt -l` on the touched files: clean. `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: ok.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit code read directly
  (output redirected to a file, not a pipe): 0, "no dupl, gocyclo or godox findings on lines added since 8c0c827".
- New and changed tests (`TestMemberReqDemotedBotNeverBreaches`, `TestMemberReqExemptionsReadAfterLookups`,
  `TestMemberReqChannelBreachNeedsBotAdmin`) with `-tags testtools -race -count=20`: ok (269 s).
- The whole `TestMemberReq` family before the two test edits: the only two failures were the two tests named above.
- CI-style gate: `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(TestSettings|TestLoadSettings|TestMemberReq|TestMemberRequirement|TestShutdown|TestChatRequirementsSettings|TestApplyMemberReqAction|TestApplyWarn)' ./fuku/modules ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup ./fuku/i18n`,
  JSON saved, then `go run ./scripts/check_test_results` on it: exit 0. 2412 test passes, 0 failures, every package `ok`
  (`fuku/modules` 762 s) and one skip, `TestMemberRequirementLookupMigrationDefaults`, the PostgreSQL-only test that the
  checker allows and that plan 14-12 proved on PostgreSQL 16. No new test skips; none was written.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.

## Not run

`make test` and `make lint` as a whole (the sandbox cannot run them; the family gate above is the CI gate on the Phase 13
and 14 tests), `make check-translations`, the PostgreSQL integrity target and the live Telegram checks of `14-SPIKE.md`
(the owner's). No locale file, generated doc or migration changed.
