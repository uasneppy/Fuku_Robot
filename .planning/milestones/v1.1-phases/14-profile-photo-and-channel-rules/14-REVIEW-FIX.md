---
phase: 14-profile-photo-and-channel-rules
review: 14-REVIEW.md
status: all_fixed
findings_in_scope: 6
fixed: 6
skipped: 0
fixed_at: 2026-11-08
---

# Phase 14: Review Fix Report

Fix pass for the findings WR-01 to WR-05 and IN-03 of `14-REVIEW.md`, run sequentially on branch
`claude/ecstatic-goldberg-qsss05` in the order WR-01, WR-02, WR-03, WR-04, WR-05 with IN-03. Every finding has a RED
`test(14)` commit and a GREEN `fix(14)` commit. Out of scope and untouched (the orchestrator dispositions them): IN-01, IN-02,
IN-04, IN-05.

## Commits

| Finding | RED (test) | GREEN (fix) |
|---------|------------|-------------|
| WR-01 | b3413c2 | 48d87cf |
| WR-02 | ba98914 | 812f945 |
| WR-03 | 0bece09 | 79eeb53 |
| WR-04 | 497d779 | 71a472a |
| WR-05, IN-03 | 9cf8e19 | 5b87552 |
| spike document | | cc5e7ce (`docs(14)`) |

Each RED commit failed its new assertions for the stated reason (a missing key, a punishment that happened, a read count of 1
where 2 was wanted), not on a compile error. No test is skipped (`t.Skip` is not used anywhere in the new files).

## WR-01: a channel the bot cannot read

**What changed.** A channel-level negative cache for the bot-status fact only: `fuku:memberreq:cna:<channel>`
(`memberReqChanNotAdminKey`, 60 s `memberReqChanNotAdminTTL`, fixed marker `1`, keyed by the channel ID alone, never a user
verdict, never a breach). `memberReqBotChannelStatus`'s flight writes it on a definite not-administrator (a member, restricted,
left or kicked status, or a 400 or 403 by `cache.IsBotGoneError`, the classifier of `ba`) and deletes it on a definite
administrator; a 429, a shed, a 5xx, a timeout and a null or unrecognised answer write nothing. An unanswerable member lookup
now asks the bot's standing on every path (it used to only under `Classify`), stays unknown for that one call without
`Classify` and leaves the fact behind. It is read at the top of `memberReqChannelRule` (inert at once, no Telegram call, not
for a Fresh lookup) and in the watcher's cached read (`memberReqLookupCached`, only after the subscription key misses, so the
common path costs the same), where it makes the channel part satisfied and starts no job. The runner-full shed line is Debug.
AGENTS.md states the D-24 amendment next to the lookup bullets (D-24's per-user rule stands) and the cost sentences now say the
cost is bounded by the 60 s cache after the first message.

**Files.** `fuku/modules/memberreq_lookup.go`, `memberreq_watch.go`, `AGENTS.md`.

**Tests.** `TestMemberReqChannelNotAdminCache` (cached for the channel and the next member costs no channel request, an unknown
member lookup writes it too, an unestablished standing writes nothing for a 429, a 500 and a transport error, an administrator
bot never writes it, a cached fact is inert and a Fresh lookup bypasses it and drops it, the watcher's read and the dispatcher
start no job), `TestMemberReqShedLogIsDebug`. Existing tests whose expectation legitimately changed: `TestMemberReqChannelRule`
(the "never reads the bot's status" cases now read it once, except a pacer shed which makes no request, and the "without
Classify" case now stays unknown but leaves the fact), `TestMemberReqLookupCacheKeys` (the inert step leaves `cna`, which the
test drops before the lost lookup it checks), and the four tests that count shed lines (`mreqCountJobStarts` and the three in
`memberreq_watch_lookup_test.go` and `memberreq_watch_test.go`) now take the hook from the new `mreqShedHook`, which raises the
log level for the test.

**Decisions.** The per-(chat, user) job gate and the reserved runner lane stay deferred, as the orchestrator decided.

## WR-02: a partial episode

**What changed.** `memberReqLookupMasks` returns the unknown verdicts as a mask (`memberReqLookupBroken` keeps its signature).
After the lookup phase `memberReqUnknownDefers` makes `memberReqLiveViolation` claim nothing (one Info line with the chat and
user IDs) when a rule that could not be answered has a punishment stricter by `memberReqSeverity` than the strictest broken
rule. An unknown that is equal or weaker, an unknown with no established breach and a rule that is Off change nothing.

**Files.** `memberreq_rules.go`, `memberreq_enforce.go`, `AGENTS.md`.

**Tests.** `TestMemberReqPartialEpisode`: the exact review scenario (last-name Warn broken, channel Ban lookup shed: no warn,
no hold, then the lookup answers a breach and the member is banned once with both bits), the same through a stricter photo
rule, last-name Ban with channel Warn unknown (Ban applied), Ban/Ban, Mute/Mute and Kick/Mute, no breach, and a rule that is Off.
Existing tests changed: `TestMemberReqLookupDoesNotBlockNameBreach` (the channel case) and `TestMemberReqPacersAreIndependent`
(the name-breach case) now use a Mute channel rule, weaker than the name Kick, because a stricter shed channel rule defers by
design. Their intent (an unanswerable lookup never delays or blocks a same-or-weaker rule) is unchanged.

## WR-03: a stale live record

**What changed.** A `memberReqLookupTouch` marker in the lookup context (set by `memberReqFlightValue` for every caller that
starts or joins a flight) tells the executor whether the lookup phase went beyond the cache. When something is broken and the
phase did, or took over 1 s (`memberReqLookupStaleAfter`), the live record is read once more through `memberReqLiveMember`;
eligibility, the names, the mute-over-restriction check and the marker adoption all use it, and a failed read punishes nobody.
A phase of cache hits costs no extra request.

**Files.** `memberreq_enforce.go`, `memberreq_lookup.go`, `AGENTS.md`.

**Tests.** `TestMemberReqLiveRecordAfterLookups`: an administrator promoted during the lookup phase is never banned, kicked or
muted, a member an admin restricted meanwhile is not muted over, a re-read that fails punishes nobody, the re-read is counted
(2 live requests after a lookup request, 1 for an all-cache phase).

## WR-04: "I fixed it" gap

**What changed.** `memberReqFixedJudge` reports whether a lookup went beyond the cache. When it did, `memberReqFixedRun` reads
the live record once more right before `memberReqFixedTake` and answers `mreq_fixed_not_ours` (the existing key) when the marker
is gone, `mreq_fixed_check_failed` when the read fails, changing nothing either way. The worst case of the press is counted in
the code comment and AGENTS.md; the 10 s budget and the single answer are unchanged.

**Files.** `memberreq_fixed.go`, `AGENTS.md`.

**Tests.** `TestMemberReqFixedMarkerRecheck`: an admin `/mute` with another `until_date` landing during the fresh photo lookup
is not undone and the hold stays, a normal tap still frees (with 2 live reads), a name-only press costs 1, and a failed read
before the take changes nothing.

## WR-05 and IN-03: a stale link and the flights

**What changed.** A positive `lc:<chat>` is cached for 60 s (`memberReqLinkedChannelTTL`); none keeps 10 minutes. A
not-subscribed candidate reads the link fresh first (`memberReqLinkUnchanged`, `getChat` through the lookup pacer in the
bounded `f:l:` flight, skipped for a Fresh caller): a changed, removed or unreadable link is unknown, never a breach, and the
fresh read refreshes `lc`. An unchanged link keeps the breach at the cost of one `getChat`; a subscribed member costs none.
The logic sits in `memberReqChannelOutcome` so `memberReqChannelRule` stays under the cyclomatic limit (the lint stand-in
flagged it at 20 first). IN-03: `memberReqLinkedChannel` and `memberReqBotChannelStatus` read their cache once more inside the
flight.

**Files.** `memberreq_lookup.go`, `AGENTS.md`.

**Tests.** `TestMemberReqStaleLink` (the lifetimes, relink, unlink, unreadable link, unchanged link with exactly one `getChat`,
subscribed member with none) and `TestMemberReqFlightsReadCacheAgain` (the first Redis read answers "no Redis", the in-flight
read finds the key: no request). Existing tests changed: `TestMemberReqLinkedChannel` and `TestMemberReqLookupCacheKeys` now
expect 60 s for a positive link.

## Other documents

`14-SPIKE.md`: the accepted limits and the owner's link-freshness check now state the bounded unreadable-channel cost, the
fixed partial episode and the fixed stale link. AGENTS.md was updated in the same commit as each rule it states.

## Verification

- `gofmt -l` on every touched file: clean. `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: ok.
- Lint stand-in `lint-local.py 8c0c827`, exit code read directly: 0, after the one gocyclo finding it raised on
  `memberReqChannelRule` (20) was fixed.
- CI's own gate: `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(TestSettings|TestLoadSettings|TestMemberReq|TestMemberRequirement|TestShutdown|TestChatRequirementsSettings|TestApplyMemberReqAction|TestApplyWarn)' ./fuku/modules ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup ./fuku/i18n`
  exited 0 (`fuku/modules` ok in 686 s), and `go run ./scripts/check_test_results` on its JSON exited 0: no failure and no
  unexpected skip (the one skip event is `TestMemberRequirementLookupMigrationDefaults` in `fuku/db/migrations`, the
  PostgreSQL-only test the checker exempts).
- The new tests (`TestMemberReqChannelNotAdminCache`, `TestMemberReqShedLogIsDebug`, `TestMemberReqPartialEpisode`, `TestMemberReqLiveRecordAfterLookups`, `TestMemberReqFixedMarkerRecheck`, `TestMemberReqStaleLink`, `TestMemberReqFlightsReadCacheAgain`) with `-race -count=20 -timeout 30m`: ok, exit 0.
- Not run: `make check-translations` and `make check-docs` (no locale file or generated-doc source was touched), a PostgreSQL
  proof (no migration or SQL changed), full `make test` and `make lint` (cannot run here, see the brief).

## Concerns noted, not fixed

- A stricter lookup rule that stays unanswerable (a persistent 5xx or timeout on the channel) holds a weaker name punishment
  back for as long as it does (WR-02 by the orchestrator's decision, no bound). A bot that is no administrator is inert through
  `cna` and defers nothing after the first message; on that first message of a 60 s window the unknown deferral applies once.
- The touch marker over-triggers on a flight whose in-flight cache re-read hits, which only costs the safe extra read.
- The breach path now costs one fresh `getChat` per not-subscribed candidate whose punishment is refused or backed off.
