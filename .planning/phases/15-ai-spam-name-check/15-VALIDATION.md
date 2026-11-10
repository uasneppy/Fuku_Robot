---
phase: "15"
slug: "ai-spam-name-check"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-11-08"
---

# Phase 15 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Seeded from `15-RESEARCH.md` § Validation Architecture and the eleven plans 15-01 to 15-11; the test names are the plans' contract, and the executors set the status.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, testify assertions in `fuku/db` tests only, miniredis, hand-written `gotgbot.BotClient` fakes (the shared `lockdownFake`/`staffActionFake`, the Phase 14 `mreqLookupFake` and the new Anthropic stand-in `mreqAIState` from 15-04), `httptest` servers for the provider and the spike tool, SQLite via `internal/testdb.Run` (PostgreSQL for the integrity list), no mock libraries |
| Config file | none; `Makefile` targets `test` (30 min per package) and `test-postgres-integrity` |
| Quick run, claude and canon | `go test -tags testtools -race -count=1 ./fuku/utils/claude ./fuku/utils/namecanon ./scripts/claude-spike` |
| Quick run, AI rule, executor and watcher | `go test -tags testtools -race -count=1 -run '^(TestMemberReq)' ./fuku/modules` |
| Quick run, settings page | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (about 4.5 minutes; only at plan end) |
| Quick run, data and locales | `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup ./fuku/i18n` |
| Full suite command | `make test` (never a bare `go test ./...`), then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; CI: `make test-postgres-integrity` |
| CI gate for the new families | `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(<the Phase 15 names>)$' <packages> > file.json` then `go run ./scripts/check_test_results` (D-17; 15-11) |
| Enforcement switch | `memberReqEnforcementLive` stays true by default (Phase 13); tests write it only through `withMemberReqLive`; no test that uses it runs in parallel |
| AI globals | the provider seams (`memberReqAIEndpoint`, `memberReqAIHTTP`, `memberReqAICall`, `memberReqAITake`), the gate and its wait, the three budgets, the breaker instance, the verdict and state TTLs and `raidNow` are package variables: tests swap them through `mreqSwap` or a helper and restore them in `t.Cleanup`; `config.AppConfig` fields are set through `withAIConfigured` / `withAINotConfigured` and restored; a test that calls `StopMemberReqJobs` calls `restoreMemberReqRunner` |

---

## Sampling Rate

- **Per task commit:** the focused `-run '^(...)$'` of the task's tests (seconds each), plus the package run of any `fuku/db/*`, `fuku/utils/*`, `scripts/claude-spike` or `./fuku/i18n` package the task touched.
- **Per plan end:** the plan's family run in its `<verification>` block; the Phase 13 and 14 guarantee tests the plan names are re-run unchanged whenever the plan edited the code that carries them (15-04 executor and table, 15-05 watcher, join and held release, 15-06 rule table, executor and lookup, 15-07 notice and button); the settings family at the end of 15-04 and 15-08.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/... ./fuku/utils/... ./scripts/claude-spike` (about 11 to 15 minutes for `fuku/modules`) plus the lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (an invariant gate: it exits 0 at HEAD and fails only on lines the phase added; it names any `dupl` clone of the image budget, breaker or request builder).
- **Flake discipline:** every new concurrency or timing test runs `-race -count=20` once (`TestMemberReqAIGate`, `TestMemberReqAIFlight`) or `-count=10` (`TestMemberReqAIEnforcePunishes`, `TestMemberReqAINewMember`, `TestMemberReqAICheckOnJoinOff`, `TestMemberReqFixedAI`, `TestShutdownEndsMemberReqAI`, `TestMemberReqAIJoinBudget`, `TestMemberReqAIEndToEnd`); any test that joins, rejoins or sends with a date takes the date before the message at least 2 s apart (`mreqSendAt`); multi-goroutine tests use Mute or Kick, never Warn (SQLite "database is locked" with concurrent warn writers); fresh IDs, names that carry a counter and cleanups per `memberreq_env_test.go` and `memberreq_ai_fake_test.go`.
- **Phase gate:** `make test` (or the documented sandbox fallback of 12-16-SUMMARY and 14-12-SUMMARY), `make lint` (or its fallback), `make check-translations`, `make generate-docs` then `make check-docs`, `git diff --exit-code 8c0c827 -- go.mod go.sum`, CI `make test-postgres-integrity`, and the owner's live checklist `15-SPIKE.md` (deferred like Phases 7 to 14 unless the owner says otherwise; no key exists in this environment).

**Name reconciliation (the plans use these, not the research's proposals):** `TestMemberReqAIRequestIsMinimal` (UI-SPEC) is the research's `TestMemberReqAIRequestCarriesOnlyNames`; the log sweep is the existing `TestMemberReqLookupLogsCarryIDsOnly` extended in place (UI-SPEC check 13), not a new `TestMemberReqAILogsCarryIDsOnly`; the page tests are `TestSettingsMreqAIStatus` and `TestSettingsMreqAIGate` (the research's `TestSettingsMreqAIUnconfigured` is `TestSettingsMreqAIStatus`); the status field is `AINotConfigured` (UI-SPEC, the research wrote `AIUnconfigured`); the plan order differs from the research's (locales before the table row, the baseline before the cost guards).

---

## Per-Requirement Verification Map

Every command is `go test -tags testtools -race -count=1` with the given `-run` and package. "Completes" names the one plan that completes the requirement.

| Requirement | Behavior | Plan | Test Type | Automated Command | File Exists | Status |
|---|---|---|---|---|---|---|
| MREQ-14 (completes: 15-11) | names are canonicalised and hashed as specified (case, NFKC, whitespace, bidi, zero-width, fillers, bounds, boundary slide, missing fields, judgeable); the request holds only the three strings; every unreadable answer is no verdict | 15-02 | unit (httptest) | `./fuku/utils/namecanon -run '^TestCanon'` and `./fuku/utils/claude -run '^TestClassifyNames'` | ✅ | ✅ green |
| MREQ-14 | a probability at or above 0.9 punishes a joiner through the real join entry (each punishment), below it welcomes; the verdict is cached by the names; exempt members are never judged; the AI row runs after the photo and channel rows | 15-04 | integration (miniredis, httptest) | `-run '^(TestMemberReqAIRule\|TestMemberReqAIEnforcePunishes\|TestMemberReqAISparesExempt\|TestMemberReqAILookupOrder)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-14 | an established member's first message is baseline-only; a new joiner is judged at join or at the first message; a rename is judged; Check on join off judges at the first message; a breach marker survives an expired baseline | 15-05 | integration | `-run '^(TestMemberReqAIBaseline\|TestMemberReqAINewMember\|TestMemberReqAIRename\|TestMemberReqAICheckOnJoinOff\|TestMemberReqAIMarkerSurvivesLongHold)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-14 | the watcher reads verdict, baseline and marker in one round, writes nothing, calls nobody, starts one job on a miss; switching the rule on in a big group causes no provider call; a held member who renamed is released | 15-05 | integration | `-run '^(TestMemberReqWatcherAI\|TestMemberReqWatcherAIBurst\|TestMemberReqPassCacheWithAI\|TestMemberReqHeldReleaseAI)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-14 | hourly budgets 5, 100 and 400 account first, gate of four, one call for 100 callers, the 24 h verdict cache, a changed name is never punished for the old one | 15-06 | integration, concurrency | `-run '^(TestMemberReqAIBudget\|TestMemberReqAIGate\|TestMemberReqAIFlight\|TestMemberReqAIVerdictCache\|TestMemberReqAINamesChangedDuringLookup)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-14 | the notice carries the AI phrase and remedy in rule order; I fixed it judges the live names (cache first, one call for new names), frees a renamed member, names the AI phrase while still broken | 15-07 | integration | `-run '^(TestMemberReqNoticeAIRemedy\|TestMemberReqFixedAI\|TestMemberReqStillBrokenText)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-14 | only the three names and the instruction are in the request; no name, hash, probability or key in any log | 15-10 | integration | `-run '^(TestMemberReqAIRequestIsMinimal\|TestMemberReqLookupLogsCarryIDsOnly)$' ./fuku/modules` | ✅ extended | ✅ green |
| MREQ-14 | the owner's names spike measures what the bot sends (counts, bar sweep, repeats, key never printed) | 15-09 | unit (httptest) | `./scripts/claude-spike -run '^(TestRunNamesMode\|TestNamesFileParse\|TestNamesSummaryAtBar\|TestNamesRepeatStability\|TestNamesModeKeyNeverPrinted\|TestNamesModeExclusiveFlags)$'` | ✅ | ✅ green |
| MREQ-14 | the whole slice through the real dispatcher: page, join, notice, rename, button, established member, unconfigured, rejected key | 15-11 | end to end | `-run '^TestMemberReqAIEndToEnd$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-10 (AI line; completes: 15-11) | not configured: the four combinations equal `raidAIImageConfigured`, inert, no call, no job; the page says so, the screen is Off-only, a punishing press is refused with one answer and no write | 15-04, 15-08 | integration | `-run '^(TestMemberReqAINotConfigured\|TestSettingsMreqAIStatus\|TestSettingsMreqAIGate)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-10 | every unanswerable shape (400, 401, 403, 429, 500, refusal, no tool call, out of range, null, garbage, transport, timeout, cancelled, paused, over budget, gate full, Redis error, flight panic) punishes nobody and leaves no hold, notice, write or cached verdict; a sticky unknown never defers a weaker punishment; an ordinary unknown still does | 15-04, 15-06 | integration, table | `-run '^(TestMemberReqAIUnanswerable\|TestMemberReqAIUnknownDefers\|TestMemberReqAIBreaker)$' ./fuku/modules` | ✅ | ✅ green |
| MREQ-10 | at the button an unanswerable AI is `mreq_fixed_check_failed` and changes nothing; the refused-press alert fits 200 in 8 languages | 15-07, 15-10 | integration | `-run '^(TestMemberReqFixedAI\|TestMemberReqLookupAlertsFitEveryLocale)$' ./fuku/modules` | ✅ extended | ✅ green |
| MREQ-01 (all eight; completes: 15-11) | CHECK 0..4 on `ai_action`, a zero default for an old row, no clobbering, backup exclusion; migration statement, idempotence, named constraint | 15-01 | repository (SQLite and PostgreSQL), unit | `./fuku/db/memberreq -run '^TestChatRequirementsSettings$'`, `./fuku/db/migrations -run '^(TestMemberRequirementAIMigrationStatements\|TestMemberRequirementAIMigrationDefaults)$'`, `./fuku/db/backup -run '^TestMemberRequirementsStayOutOfBackup$'` (CI also runs the PostgreSQL pair) | ✅ extended | ✅ green |
| MREQ-01 | the eight-row table, bit 128, the fingerprint of eight actions, the eight-cell grid, six rows, callback bytes, the card's own sentence, the table-driven drift and authority tests | 15-04, 15-08 | unit and integration | `-run '^(TestMemberReqRuleTable\|TestMemberReqPassValue\|TestMemberReqRuleKeysExist\|TestSettingsMreqRowBudget\|TestSettingsMreqCallbackBytes\|TestSettingsMreqCard\|TestSettingsDriftMreq\|TestSettingsAuthorityMreq\|TestApplyMemberReqAction)$' ./fuku/modules` | ✅ (edited) | ✅ green |
| MREQ-01, MREQ-10 (UI / locale; PLAT-05 binds) | 255 `settings_` and 31 `mreq_` keys in 8 locales, ceilings, notice cap 520 with both tails, still-broken with the four longest of eight, warn reason 300, the cell at most 20, the mute card quotes the button | 15-03 | locale | `-run '^(TestMemberReqLocaleKeys\|TestMemberReqNoProfileInReworkedKeys\|TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsHelpFitsOneMessage)$' ./fuku/i18n` | ✅ (extended) | ✅ green |
| PLAT-06 (cross-cutting, already ticked; binds Phases 15 to 17) | one call for 100 callers, at most four at once, the slot released on a panic and a cancel; the call ends at `StopMemberReqJobs` inside the existing budget; a join returns at its 4 s budget while the flight finishes; nothing shared with the pacers | 15-06, 15-10 | integration, concurrency | `-run '^(TestMemberReqAIFlight\|TestMemberReqAIGate\|TestShutdownEndsMemberReqAI\|TestMemberReqAIJoinBudget\|TestMemberReqAIPacersAreIndependent)$' ./fuku/modules` | ✅ | ✅ green |
| Privacy (D-01, D-32) | no AI screen, alert or card names the Staff Group, the channel, a member or a count; no AI log line holds a name | 15-08, 15-10 | integration | `-run '^(TestSettingsStaffPrivacy\|TestMemberReqLookupLogsCarryIDsOnly)$' ./fuku/modules` | ✅ (extended) | ✅ green |
| Phase 13 and 14 guarantees (re-asserted) | one punishment across replicas, held path, a clean message ends a warn hold, never lifts another mute, cap and pause, join behaviour, shutdown drain, the lookups' unanswerable matrix | 15-04 to 15-07 | integration | the anchored re-run lists of each plan's Task 3 | ✅ | ✅ green |

---

## Per-Task Verification Map

| Task | Plan | Wave | Requirement | Test(s) | Automated Command | Status |
|------|------|------|-------------|---------|-------------------|--------|
| 15-01-T1 (tracer) | 01 | 1 | MREQ-01, MREQ-14 | `TestChatRequirementsSettings` | `-run '^TestChatRequirementsSettings$' ./fuku/db/memberreq` | ✅ green |
| 15-01-T2 | 01 | 1 | MREQ-01, MREQ-14 | `TestMemberRequirementAIMigrationStatements`, `TestMemberRequirementAIMigrationDefaults` (PostgreSQL) | `-run '^TestMemberRequirementAIMigrationStatements$' ./fuku/db/migrations` | ✅ green |
| 15-01-T3 | 01 | 1 | MREQ-01, MREQ-14 | `TestMemberRequirementsStayOutOfBackup`; the PostgreSQL pair on a throwaway cluster; `make -n test-postgres-integrity` | `make -n test-postgres-integrity \| grep -c TestMemberRequirementAIMigrationDefaults` | ✅ green |
| 15-02-T1 (tracer) | 02 | 2 | MREQ-14 | `TestClassifyNames`, the unedited `TestClaudeRequestShape` | `-run '^TestClassifyNames$' ./fuku/utils/claude` | ✅ green |
| 15-02-T2 | 02 | 2 | MREQ-14 | `TestCanonForm`, `TestCanonBounds`, `TestCanonHash`, `TestCanonJudgeable`, `TestCanonVerdictKey`, `TestCanonPackageIsLeaf` | `-run '^TestCanon' ./fuku/utils/namecanon` | ✅ green |
| 15-02-T3 | 02 | 2 | MREQ-14 | `TestClassifyNamesVerdicts`, `TestClassifyNamesInjection`, `TestClassifyNamesErrors`, `TestClassifyNamesSpikeOptions` | `-run '^TestClassifyNames' ./fuku/utils/claude` | ✅ green |
| 15-03-T1 (tracer) | 03 | 3 | MREQ-14 | `TestMemberReqLocaleKeys`, `TestMemberReqNoProfileInReworkedKeys` | `-run '^(TestMemberReqLocaleKeys\|TestMemberReqNoProfileInReworkedKeys)$' ./fuku/i18n` | ✅ green |
| 15-03-T2 | 03 | 3 | MREQ-10, MREQ-14 | `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`, `TestSettingsHelpFitsOneMessage` | `-run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsHelpFitsOneMessage)$' ./fuku/i18n` | ✅ green |
| 15-03-T3 | 03 | 3 | MREQ-10, MREQ-14 | `make check-translations`, `make check-docs`, the whole `./fuku/i18n` | `make check-translations` | ✅ green |
| 15-04-T1 (tracer) | 04 | 4 | MREQ-10, MREQ-14 | `TestMemberReqAIRule`, `TestMemberReqAIEnforcePunishes`, `TestMemberReqAINotConfigured`, `TestMemberReqAISparesExempt`, `TestMemberReqAILookupOrder`, `TestMemberReqAIUnanswerable` | `-run '^(TestMemberReqAIRule\|TestMemberReqAIEnforcePunishes\|TestMemberReqAIUnanswerable)$' ./fuku/modules` | ✅ green |
| 15-04-T2 | 04 | 4 | MREQ-01 | `TestMemberReqRuleTable`, `TestMemberReqPassValue`, `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes` | `-run '^(TestMemberReqRuleTable\|TestMemberReqPassValue\|TestSettingsMreqRowBudget)$' ./fuku/modules` | ✅ green |
| 15-04-T3 | 04 | 4 | MREQ-01, MREQ-10, MREQ-14 | the whole `TestMemberReq` family, the settings family once, the Phase 13 and 14 guarantees | `-run '^(TestMemberReq)' ./fuku/modules` | ✅ green |
| 15-05-T1 (tracer) | 05 | 5 | MREQ-14 | `TestMemberReqAIBaseline`, `TestMemberReqAIRename`, `TestMemberReqAINewMember`, `TestMemberReqAICheckOnJoinOff`, `TestMemberReqAIMarkerSurvivesLongHold`; the edited tracer tests | `-run '^(TestMemberReqAIBaseline\|TestMemberReqAINewMember)$' ./fuku/modules` | ✅ green |
| 15-05-T2 | 05 | 5 | MREQ-10, MREQ-14 | `TestMemberReqWatcherAI`, `TestMemberReqWatcherAIBurst`, `TestMemberReqPassCacheWithAI`, `TestMemberReqHeldReleaseAI` | `-run '^(TestMemberReqWatcherAI\|TestMemberReqHeldReleaseAI)$' ./fuku/modules` | ✅ green |
| 15-05-T3 | 05 | 5 | MREQ-10 | the Phase 13 and 14 watcher, join, held, captcha-guard and shutdown guarantees | the anchored list of the task | ✅ green |
| 15-06-T1 (tracer) | 06 | 6 | MREQ-14, PLAT-06 | `TestMemberReqAIBudget`, `TestMemberReqAIGate`, `TestMemberReqAIFlight` | `-run '^(TestMemberReqAIBudget\|TestMemberReqAIGate\|TestMemberReqAIFlight)$' ./fuku/modules` | ✅ green |
| 15-06-T2 | 06 | 6 | MREQ-10 | `TestMemberReqAIBreaker`, `TestMemberReqAIUnknownDefers`, `TestMemberReqAIUnanswerable` (more shapes) | `-run '^(TestMemberReqAIBreaker\|TestMemberReqAIUnknownDefers\|TestMemberReqAIUnanswerable)$' ./fuku/modules` | ✅ green |
| 15-06-T3 | 06 | 6 | MREQ-10, MREQ-14 | `TestMemberReqAINamesChangedDuringLookup`, `TestMemberReqAIVerdictCache`; the Phase 7, 13 and 14 guarantees | `-run '^(TestMemberReqAINamesChangedDuringLookup\|TestMemberReqAIVerdictCache)$' ./fuku/modules` | ✅ green |
| 15-07-T1 (tracer) | 07 | 7 | MREQ-14 | `TestMemberReqNoticeAIRemedy` (first subtests), `TestMemberReqFixedAI` (frees a renamed member) | `-run '^(TestMemberReqNoticeAIRemedy\|TestMemberReqFixedAI)$' ./fuku/modules` | ✅ green |
| 15-07-T2 | 07 | 7 | MREQ-10, MREQ-14 | `TestMemberReqNoticeAIRemedy`, `TestMemberReqFixedAI`, `TestMemberReqStillBrokenText` | `-run '^(TestMemberReqNoticeAIRemedy\|TestMemberReqFixedAI\|TestMemberReqStillBrokenText)$' ./fuku/modules` | ✅ green |
| 15-07-T3 | 07 | 7 | MREQ-10 | `TestMemberReqScreensFitEveryLocale`, `TestMemberReqNoticePhotoRemedy` (edited), the button guarantees | `-run '^(TestMemberReqScreensFitEveryLocale\|TestMemberReqNoticePhotoRemedy)$' ./fuku/modules` | ✅ green |
| 15-08-T1 (tracer) | 08 | 8 | MREQ-10 | `TestSettingsMreqAIStatus` | `-run '^TestSettingsMreqAIStatus$' ./fuku/modules` | ✅ green |
| 15-08-T2 | 08 | 8 | MREQ-01, MREQ-10 | `TestSettingsMreqAIGate`, `TestSettingsMreqCard`, `TestSettingsMreqCallbackBytes`, `TestApplyMemberReqAction`, `TestSettingsDriftMreq`, `TestSettingsAuthorityMreq` | `-run '^(TestSettingsMreqAIGate\|TestSettingsMreqCard)$' ./fuku/modules` | ✅ green |
| 15-08-T3 | 08 | 8 | MREQ-01, MREQ-10 | `TestSettingsMreqPageFitsEveryLocale`, `TestSettingsMainPageFitsEveryLocale`, `TestSettingsMreqOpenCost`, `TestSettingsMreqMainState`, `TestSettingsStaffPrivacy`; the settings family once | `-run '^(TestSettingsMreqPageFitsEveryLocale\|TestSettingsMreqOpenCost\|TestSettingsStaffPrivacy)$' ./fuku/modules` | ✅ green |
| 15-09-T1 (tracer) | 09 | 9 | MREQ-14 | `TestRunNamesMode` | `-run '^TestRunNamesMode$' ./scripts/claude-spike` | ✅ green |
| 15-09-T2 | 09 | 9 | MREQ-14 | `TestNamesFileParse`, `TestNamesSummaryAtBar`, `TestNamesRepeatStability`, `TestNamesModeKeyNeverPrinted`, `TestNamesModeExclusiveFlags` | `-run '^TestNames' ./scripts/claude-spike` | ✅ green |
| 15-09-T3 | 09 | 9 | MREQ-14 | `15-SPIKE.md` and the AGENTS.md lines, the whole spike package | `go test -tags testtools -race -count=1 ./scripts/claude-spike` | ✅ green |
| 15-10-T1 (tracer) | 10 | 10 | PLAT-06 | `TestShutdownEndsMemberReqAI`, `TestMemberReqAIJoinBudget` | `-run '^(TestShutdownEndsMemberReqAI\|TestMemberReqAIJoinBudget)$' ./fuku/modules` | ✅ green |
| 15-10-T2 | 10 | 10 | MREQ-14 | `TestMemberReqLookupLogsCarryIDsOnly` (extended), `TestMemberReqAIRequestIsMinimal` | `-run '^(TestMemberReqLookupLogsCarryIDsOnly\|TestMemberReqAIRequestIsMinimal)$' ./fuku/modules` | ✅ green |
| 15-10-T3 | 10 | 10 | PLAT-06, MREQ-10 | `TestMemberReqAIPacersAreIndependent`, `TestMemberReqLookupAlertsFitEveryLocale` (extended) | `-run '^(TestMemberReqAIPacersAreIndependent\|TestMemberReqLookupAlertsFitEveryLocale)$' ./fuku/modules` | ✅ green |
| 15-11-T1 (tracer) | 11 | 11 | all | `TestMemberReqAIEndToEnd` | `-run '^TestMemberReqAIEndToEnd$' ./fuku/modules` | ✅ green |
| 15-11-T2 | 11 | 11 | MREQ-01, MREQ-10, MREQ-14 | docs, README, rulebook | `make generate-docs && make check-docs` and the greps of the task | ✅ green |
| 15-11-T3 | 11 | 11 | all | the phase gate, `15-SPIKE.md`, the requirement ticks | `make test` (or the recorded fallback), `make check-translations`, `git diff --exit-code 8c0c827 -- go.mod go.sum` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `fuku/utils/namecanon/canon_test.go` and `leaf_test.go` (imports: the standard library and `golang.org/x/text` only) and `fuku/utils/claude/names_test.go` (a request-body decoder and the answer table over `httptest`) (15-02).
- [x] `fuku/modules/memberreq_ai_fake_test.go`: the Anthropic stand-in (`mreqAIState`: a probability, a refusal, a missing tool call, an out-of-range value, null, garbage, HTTP statuses with `retry-after`, a hang that honours the request context, a delay, recorded decoded bodies, a call count and a maximum-in-flight counter), `mreqInstallAIFake`, `withAIConfigured` (15-04), `withAINotConfigured` (15-08), `mreqAICleanup` (verdict, baseline, marker and budget keys, extended by 15-05 and 15-06), `mreqAIMarkNew` (15-05), `withAIGate` and `withAIBudgets` (15-06), `mreqAIUnique` (15-10).
- [x] Test seams in production code, each read by the code and replaced by a test: `memberReqAIEndpoint` and `memberReqAIHTTP` (15-04), `memberReqAIVerdictTTL` (15-04), `memberReqAIStateTTL` (15-05), `memberReqAIGate` and `memberReqAIGateWait`, the three budget variables, `memberReqAITake` and `memberReqAICall` (15-06); the breaker's clock is `raidNow` and `memberReqRedisClient` exists for the Redis-down tests.
- [x] `TestChatRequirementsSettings` extended to eight columns and `TestMemberRequirementAIMigrationDefaults` in the anchored `-run` list of `test-postgres-integrity`, both proved on a throwaway PostgreSQL 16 (15-01).
- [x] The `fuku/i18n` limits tables extended (+2 `mreq_` rows, +8 `settings_` rows, eight phrases, multi-tail renders, notice cap 520 as a rendered check, eight cells) before the keys exist in code (15-03).
- [x] Every Phase 13 and 14 test that counted to seven edited to eight in 15-04, 15-07 and 15-08 (the table, the fingerprint, the page helpers, grid, rows, bytes, card, privacy, notice and button tests), none re-recorded; each edited test named in its plan's SUMMARY.
- [x] AGENTS.md lines in the same commit as each rule change (15-01 to 15-10), read whole once and finished in 15-11.

---

## Manual-Only Verifications

Covered by the owner's live checklist `15-SPIKE.md` (written by plan 15-09, checked by 15-11; deferred like Phases 7 to 14 unless the owner says otherwise). None blocks the phase, and the code is safe under every outcome of each (an unknown, a verdict below the bar, an unconfigured AI, a missing Redis and an unreachable provider punish nobody).

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Haiku's accuracy at the 0.9 bar on a labelled list (at least 200 names, half non-Latin, 30 business names, 10 injection names): false-positive rate, recall, the sweep | MREQ-14 | Needs the owner's key and real names (no key exists here) | `go run ./scripts/claude-spike -names names.csv -repeat 5 -csv out.csv`; retune `memberReqAIBar` from the sweep |
| Non-determinism between repeats and whether the 24 h stickiness and "no re-roll" stay | MREQ-14 | Live model behaviour | read the repeat report of the same run |
| Latency (p95 under about 2.5 s so most joins fit the 4 s budget), tokens (about 760 in, 20 out) and cost per 1,000 names | MREQ-14, PLAT-06 | Live API | read the summary of the same run |
| The configured model accepts a forced tool call and `thinking: disabled`; Opus 5.5, Sonnet 5.5 and Fable 5.1 do not | MREQ-14, MREQ-10 | Live API behaviour | one short `-names` run with the configured model |
| Refusal stops on adult-content spam names are no verdict | MREQ-10 | Live model behaviour | count blocked rows in the run |
| In a test supergroup with the key on the bot: Warn on a joining spam-like name, a rename judged once, the hold released on an ordinary name, Mute then rename then "I fixed it", key removed (page says AI not configured, the cell offers Off only, a punishing press refused), a wrong key (401 pauses the checks, one warning, nobody punished) | MREQ-14, MREQ-10 | Real Telegram and the real provider | the six steps of check 6 of `15-SPIKE.md` |
| The new cell and the Off-only screen in hi, ru and uk on a phone are not ellipsized | MREQ-01 | Real-client rendering | open the page on Android, iOS and Desktop |
| Two live replicas over one PostgreSQL and one Redis: the budgets and the verdict cache are shared, one string is judged once across both | PLAT-06, MREQ-14 | Two real replicas | run two bot instances and join two accounts with one spam-like name |
| A restart (SIGTERM) during a burst of joins leaves no hold and punishes nobody | PLAT-06, MREQ-10 | A real SIGTERM | burst joins, restart the bot mid-burst, read the log |
| Native-speaker read of the ten new keys in es, fr, hi, id, pt, ru and uk | MREQ-10, MREQ-14 | Wording quality is human | read each key listed in `15-SPIKE.md` check 10 |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency: a focused `-run` under 60 s; the settings family (about 4.5 minutes) and the `fuku/modules` package (about 11 to 15 minutes) only at plan end or wave merge
- [x] `nyquist_compliant: true` set in frontmatter (by validate-phase, after the plans ran)

**Approval:** validated 2026-11-10 (orchestrator: every named test exists and passes through CI's own result gate; the one skip is the PostgreSQL-only migration test that `make test-postgres-integrity` runs in CI)

## Validation Audit 2026-11-10

| Metric | Count |
|---|---|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |
