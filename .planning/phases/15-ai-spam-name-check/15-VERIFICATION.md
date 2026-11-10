---
phase: 15-ai-spam-name-check
verified: 2026-11-10T12:00:00Z
status: human_needed
score: "13/13 must-haves verified (3 ROADMAP success criteria plus 10 supporting truths); MREQ-14, MREQ-10 and MREQ-01 judged satisfied on automated and code-read evidence only, PLAT-06 judged still satisfied"
covered_files:
  - ".planning/phases/15-ai-spam-name-check/15-01-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-01-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-02-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-02-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-03-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-03-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-04-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-04-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-05-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-05-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-06-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-06-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-07-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-07-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-08-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-08-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-09-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-09-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-10-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-10-SUMMARY.md"
  - ".planning/phases/15-ai-spam-name-check/15-11-PLAN.md"
  - ".planning/phases/15-ai-spam-name-check/15-11-SUMMARY.md"
  - "AGENTS.md"
  - "README.md"
  - "docs/src/content/docs/api-reference/callbacks.md"
  - "docs/src/content/docs/api-reference/environment.md"
  - "docs/src/content/docs/api-reference/permissions.md"
  - "docs/src/content/docs/commands/aispam/index.md"
  - "fuku/db/memberreq/settings.go"
  - "fuku/db/models/member_requirements.go"
  - "fuku/modules/memberreq_ai.go"
  - "fuku/modules/memberreq_ai_breaker.go"
  - "fuku/modules/memberreq_ai_budget.go"
  - "fuku/modules/memberreq_ai_latch.go"
  - "fuku/modules/memberreq_enforce.go"
  - "fuku/modules/memberreq_fixed.go"
  - "fuku/modules/memberreq_held.go"
  - "fuku/modules/memberreq_join.go"
  - "fuku/modules/memberreq_lookup.go"
  - "fuku/modules/memberreq_notice.go"
  - "fuku/modules/memberreq_rules.go"
  - "fuku/modules/memberreq_watch.go"
  - "fuku/modules/settings_mreq.go"
  - "fuku/utils/claude/names.go"
  - "fuku/utils/claude/request.go"
  - "fuku/utils/namecanon/canon.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
  - "migrations/20261109120000_add_member_requirement_ai_rule.sql"
  - "sample.env"
  - "scripts/claude-spike/main.go"
  - "scripts/claude-spike/names.go"
  - "scripts/claude-spike/report.go"
covered_digest: "v4:sha256:59c9843ba5db71ef0915db3d55a765fa7417c0dc23a74ffeeb45776eef697efe"
behavior_unverified: 0
overrides_applied: 0
unverified_prohibitions: 13
human_verification:
  - test: "15-SPIKE.md check 1, BEFORE any group sets the AI rule to Mute, Kick or Ban: run `go run ./scripts/claude-spike -names names.csv -repeat 1` with the owner's ANTHROPIC_API_KEY on a labelled file of at least 200 names (half non-Latin, 30 business names, 60 spam, 10 injection names) and read the 0.9-bar summary and the sweep."
    expected: "False-positive rate on legitimate names 0.5% or lower at the 0.9 bar, recall on spam useful. If not, retune the one-line constant memberReqAIBar (cached verdicts are probabilities, so a new bar applies at once) or keep the rule on Warn or Off."
    why_human: "Every test drives the provider through an httptest stand-in. What Haiku really answers for real names, and the bar's false-positive rate, exist only against the live model. The code is safe under every outcome (a verdict below the bar and every unknown punish nobody); this changes the starting bar and the owner's confidence, not safety."
  - test: "15-SPIKE.md check 2 (stability) and check 3 (latency, tokens, cost): the same names file with `-repeat 5`; read the repeat lines, p50/p95/max latency, mean tokens (estimate about 760 in, 20 out) and the cost per 1,000 names with the real list prices passed to -price-in and -price-out."
    expected: "Few or no names cross the bar between repeats; p95 under about 2.5 s so most joins fit the 4 s join lookup budget (a slower call is welcomed and judged at the first message, never punished); a cost the owner accepts."
    why_human: "The default model takes no sampling parameter, so a verdict cannot be made deterministic, and latency, token counts and price are properties of the live service. The 24 h verdict cache (D-11) and the no-re-roll rule (D-23) make a lucky or unlucky first judgement stick, which only the measured spread can price."
  - test: "15-SPIKE.md check 4: a short live names run (`-limit 3`) with the configured ANTHROPIC_MODEL, and `-models-get`."
    expected: "The model answers with probabilities and not HTTP 400: a Haiku-class model accepts the forced tool call with thinking disabled and no sampling parameter (Opus 5.5, Sonnet 5.5 and the Fable 5.1 family answer 400)."
    why_human: "The request shape (one strict forced tool, thinking disabled, no temperature, top_p or top_k) is asserted against httptest and a fake transport. Whether the owner's configured model accepts it is live provider behaviour; if it does not, the breaker reads five 400s as provider failures, pauses the checks and punishes nobody."
  - test: "15-SPIKE.md check 5: include a few adult-content spam names in the file and count the rows whose finish text is `blocked` (stop_reason refusal)."
    expected: "Few or none. A refusal is no verdict: never a breach, never cached, counted as a breaker success."
    why_human: "TestClassifyNames* and the breaker tests prove the code treats a refusal as no verdict. How often the live model refuses on adult-content names is unknowable offline; many refusals would mean those spam accounts are never caught (the safe direction)."
  - test: "15-SPIKE.md check 6, the six-step walk-through in a real test supergroup with the bot as administrator and the key set: (1) AI rule to Warn, join a spam-like account; (2) rename it twice, back and forth; (3) rename to an ordinary name and write; (4) rule to Mute, rename and press I fixed it; (5) unset the key and restart; (6) put a wrong key on the bot."
    expected: "(1) warned, notice names the rule and ends with the remedy; (2) judged once, no second call for a string already judged; (3) the Warn hold is released; (4) the member is unmuted; (5) page says AI is not configured, the AI screen offers [Off] alone, a stored punishment stays visible and inert, a punishing press is refused with the same line; (6) provider 401, every group's name checks pause 5 minutes, one warning, nobody punished."
    why_human: "TestMemberReqAIEndToEnd walks this slice through the real dispatcher with only Telegram and the provider faked, and it passed. A real Telegram (join and rename updates, the 330 day mute marker, the button), a real client and the real provider can only be walked live."
  - test: "15-SPIKE.md check 7: the AI cell (the eighth, the last row of the grid), the AI rule screen with the key set and unset (Off-only form) and the Mute confirmation card in hi, ru and uk on a phone."
    expected: "Each cell on one line (hi `AI स्पैम नाम`, ru `ИИ-спам`, uk `ШІ-спам`), no row wraps, the card says new members are judged when they join or at their first message and members already in the group only after a name change."
    why_human: "TestSettingsLabelLengths, TestSettingsMreqPageFitsEveryLocale and TestSettingsMreqCallbackBytes hold the code point and byte limits (cell at most 20 code points). Truncation and wrapping at phone width, Hindi combining marks included, is client rendering."
  - test: "15-SPIKE.md check 8: two live bot processes over one PostgreSQL, one Redis and one key; send the same spam-like name through both."
    expected: "One string judged once across both replicas (one provider call, the other reads the verdict cache), the hourly budgets (5 per account, 100 per group, 400 in all) counted fleet-wide in Redis, one punishment and not two."
    why_human: "TestMemberReqAIFlight, TestMemberReqAIBudget and the replica tests prove it with two dispatchers over one miniredis and one SQLite. Real processes, a real deployment config and real PostgreSQL (the fuku/modules harness does not run on PostgreSQL) are only checked live."
  - test: "15-SPIKE.md check 9: with the AI rule on Warn, join several spam-like test accounts together and send SIGTERM mid-burst, then restart."
    expected: "The process stops inside the 60 s budget with no new drain, no hold is left half made, nobody is punished for a cut-off call, and a member whose call was cut is judged again at their next message."
    why_human: "TestShutdownEndsMemberReqAI proves the drain's own cancel of memberReqContext() ends a job inside a hung provider call within memberReqStopWait and leaves no hold, notice, write, warn row or cached verdict. A real SIGTERM against a real deployment with docker's stop_grace_period 75s is a live check."
  - test: "15-SPIKE.md check 10: a native speaker of each of es, fr, hi, id, pt, ru and uk reads the ten new keys (mreq_rule_ai, mreq_remedy_ai, settings_mreq_rule_ai, settings_mreq_hint_ai, settings_btn_mreq_ai, settings_mreq_desc_ai, settings_mreq_confirm_ai_kick, settings_mreq_confirm_ai_ban, settings_mreq_confirm_ai_mute, settings_mreq_here_ai), including the three confirm-card keys as reworded by security fix SEC-04 (`when they join or at their first message, or after a name change`)."
    expected: "Each text reads naturally and says what the English says; mreq_rule_ai stays a noun phrase that fills the slot after a colon in ru, uk and hi and does not read as an accusation in a public message; the mute card quotes the I-fixed-it button words verbatim."
    why_human: "make check-translations, TestMemberReqLocaleKeys, TestSettingsLocaleKeys and the fit tests prove presence, placeholders, markup and length only. The seven non-English texts were written from the English source, not by a native speaker."
  - test: "15-SPIKE.md check 11 and CI on HEAD 7e420fe: make lint with a golangci-lint built for Go 1.26, make test and make test-postgres-integrity."
    expected: "All three green; the integrity target runs TestMemberRequirementAIMigrationDefaults (./fuku/db/migrations) for real: ai_action defaults to 0, chk_chat_requirements_ai exists and refuses 5 with SQLSTATE 23514, the statement runs twice without error."
    why_human: "TestMemberRequirementAIMigrationDefaults skips without PostgreSQL by design (scripts/check_test_results allows it). The verifier did not run make test, make lint or the PostgreSQL target; the orchestrator's earlier gate run and the focused tests here are the evidence."
  - test: "Owner review of the 13 judgment-tier and backstop prohibitions of plans 15-01 to 15-11 (see Prohibitions below), the accepted limits AR-01 to AR-18 (15-SECURITY.md), IN-01, IN-02, IN-04 and NEW-01 to NEW-03 (15-REVIEW-DISPOSITION.md), the four Info wording slips under Anti-Patterns Found (already corrected after this verification, wording only, see the note there), and the follow-up decisions of 15-SPIKE.md (D-22 sticky unknowns, D-23 no re-roll, A15-A4 hourly-budget wording on I fixed it, A15-A6 silent Redis outage, A15-A9 public phrase)."
    expected: "The owner accepts the verifier's non-authoritative prohibition verdicts, confirms the accepted limits, and chooses which follow-ups become their own changes."
    why_human: "An autonomous verifier never passes a judgment-tier prohibition silently, and two prohibitions of plan 15-11 are backstop-tier (they name facts only the live checks can settle). The follow-ups are product decisions the phase left open on purpose."
---

# Phase 15: AI Spam-Name Check Verification Report

**Phase Goal:** As a group owner, I want accounts whose names look like spam flagged by AI, so that advertising accounts are screened when they join, with no message text leaving the server.
**Mode:** mvp (the goal is a user story)
**Verified:** 2026-11-10T12:00:00Z at HEAD 7e420fe (branch claude/ecstatic-goldberg-qsss05, clean working tree)
**Status:** human_needed
**Re-verification:** No, initial verification

I started from the assumption that the goal was missed and tried to break it from the code: what actually leaves the server, what happens with the AI off, what happens with every kind of unknown, whether an established member can cause a burst of calls, whether the budgets and the cache are real, and whether the shutdown path grew. I found no path where an unanswered or over-budget AI punishes anyone, no path where text beyond the three canonical names is sent, and no new shutdown drain. I found no BLOCKER and no WARNING. I found four Info wording slips in documentation and comments that describe the global AI switch without the new name check (details under Anti-Patterns Found).

The status is `human_needed`, not `passed`, by the owner's decision: every live check of 15-SPIKE.md needs the owner's Anthropic key or a real Telegram group and was deferred ("defer live checks, keep going"). The code is safe under every outcome those checks can have.

## User Flow Coverage

User story: As a group owner, I want accounts whose names look like spam flagged by AI, so that advertising accounts are screened when they join, with no message text leaving the server.

| Step | Expected | Evidence | Status |
| ---- | -------- | -------- | ------ |
| Owner opens Requirements and finds the AI spam name cell, eighth and last | Eight cells, two per row, four full rows, 6 rows in all | `memberreq_rules.go:139-146` the `ai` row (bit 128); `settings_mreq.go` rule grid ranges `memberReqRules`; `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes` | VERIFIED (automated) |
| Owner with no key sees that AI is not configured | One status line, `[Off]` alone, a stored punishment visible and inert, a punishing press refused | `settings_mreq.go:205-213` (`AINotConfigured` from `Configured()`), `:495-518` slot and Off-only form, `:683-690` `settingsMreqAIGate`; `TestSettingsMreqAIStatus`, `TestSettingsMreqAIGate` (run, passed) | VERIFIED (automated) |
| Owner with a key sets Warn, or Kick, Ban or Mute behind the card | Written through `applyMemberReqAction`; the card says its own consequence | `settings_mreq.go:614-639`; `memberreq_rules.go:142-146` `ConfirmKeys`; `settings_mreq_confirm_ai_*` | VERIFIED (automated) |
| A spam-named member joins | Judged once through one flight, punished once, one notice with the AI remedy; a mute carries I fixed it | `memberreq_join.go:105-129` (marker, then synchronous executor with `Joining`); `memberreq_enforce.go:288-290`; `memberreq_ai.go:366-392`; `TestMemberReqAIEndToEnd` (run, passed) | VERIFIED (automated) |
| A member changes their names | Baseline differs, so one judgement of the new string | `memberreq_ai.go:346-357` (`memberReqAIDecide`); `memberreq_watch.go:240-245` | VERIFIED (automated) |
| Rule switched on in a large group | Established members recorded with no call | `memberreq_ai.go:353-355` `baselineOnly`, `:374-377` SET NX and pass; `TestMemberReqWatcherAIBurst`, `TestMemberReqAIBaseline` (run, passed) | VERIFIED (automated) |
| The AI is slow, failing, rate limited or over budget | Nobody punished | `memberreq_ai.go:480-517`; `TestMemberReqAIUnanswerable` (run, passed, 19 s) | VERIFIED (automated) |

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| - | ----- | ------ | -------- |
| 1 | ROADMAP SC1: a member whose names the AI judges spam gets the rule's punishment when they join or change their names | VERIFIED (automated evidence only) | Joiner: `memberreq_join.go:105-128` sets the new-member marker then runs `memberReqEnforce` with `Source == memberReqFromJoin`; the executor's lookup phase passes `Names: &names, Joining: true` (`memberreq_enforce.go:289`) and a probability at or above `memberReqAIBar` 0.9 is `memberReqBreach` (`memberreq_ai.go:192-198`), which `memberReqLookupMasks` turns into bit 128 (`memberreq_rules.go:241-243`) and the strictest-punishment path applies. Rename: the watcher hands a member whose baseline differs to a job (`memberreq_watch.go:240-245`), the job judges the new string (`memberreq_ai.go:346-357` step 4). Live model accuracy at 0.9 is human item 1 |
| 2 | ROADMAP SC1 (second half): only first name, last name and username are sent, never message text | VERIFIED | `names.go:60-84`: the only user content of the request is `json.Marshal(namesUser{First, Last, Username})`, each cut by runes (64, 64, 32); `ClassifyNames` takes a `claude.Names` of three strings and nothing else (`names.go:94`); the flight builds it from `plan.canon.First/Last/Username` only (`memberreq_ai.go:501`), and `memberReqAIRule` receives no message, chat title or ID in its mode (`memberreq_lookup.go:53-67`, `Names *memberReqNames`). The instruction is the code-owned constant `NamesInstruction` and says the strings are data. `TestMemberReqAIRequestIsMinimal` (run, passed: body is exactly the fixed shape, no title, message, chat or user ID) and `TestClassifyNames*` (claude package, passed) |
| 3 | ROADMAP SC2 (first half): an established member's names are first recorded without a call, so switching the rule on in a large group causes no burst | VERIFIED | `memberReqAIDecide` step 3 (`memberreq_ai.go:353-355`): not joining, no marker, no baseline, no readable verdict is `baselineOnly`; `memberReqAIRule` writes `nm` with `SET NX` (`:451-461`) and returns pass with no flight and no call (`:374-377`). The watcher's next message for that member is satisfied with no job (`memberreq_watch.go:244`). `TestMemberReqWatcherAIBurst` (run, passed: one baseline-only job per distinct poster, zero provider calls, next message costs no job) |
| 4 | ROADMAP SC2 (second half): names already judged are answered from the cache and hourly budgets cap the cost | VERIFIED | Cache: `memberReqAIPrepare` reads `nv:<hash>` first (`:316-333`), a parsable probability decides before any call (`:350-352`), the flight reads it once more (`:481-489`), the verdict is written after a definite answer (`:509`, 24 h, `memberReqAIVerdictTTL`); the key is a hash of prompt tag, model and names hash (`memberReqAIVerdictKey`). Budgets: `memberreq_ai_budget.go:33-40` 5 per account, 100 per group, 400 in all, reserved account first, then group, then fleet through `raidAIBudgetTake` (`:79-104`), stopping at the first refusal or counter error, before the call (`memberreq_ai.go:498-500`). Four-call gate (`memberReqAIGateSize = 4`, `:135`) is taken before the budget. `TestMemberReqAIVerdictCache`, `TestMemberReqAIBudget`, `TestMemberReqAIGate`, `TestMemberReqAIFlight` (run, passed) |
| 5 | ROADMAP SC3 (first half): with ENABLE_AISPAM=false or no Anthropic key the rule is off and the page says AI is not configured | VERIFIED | `memberReqAIConfigured` is `EnableAISpam && AnthropicAPIKey != ""` (`memberreq_ai.go:143-145`) and nothing else (no Redis, no raid switch). With it false: `memberReqAIPrepare` returns inert before any Redis read or provider call (`:320-322`), `memberReqAIClient` returns nil without a key (`:149-153`), `memberReqAILive` is false so the watcher reads no AI key and the join entry sets no marker (`:235-237`, `memberreq_join.go:105`), and a stored non-Off value is inert, not unknown, so it defers nothing. Page: `settingsMreqReadStatus` sets `AINotConfigured` from the rule's `Configured()` at every return (`settings_mreq.go:205-213`), the screen is `[Off]` alone with the rule's own line (`:495-518`), and a Warn or card Yes is refused by `settingsMreqAIGate` with the status line's words, writing and drawing nothing (`:683-690`). `TestMemberReqAINotConfigured`, `TestSettingsMreqAIStatus`, `TestSettingsMreqAIGate` (run, passed) |
| 6 | ROADMAP SC3 (second half): a slow, failing, rate-limited or over-budget AI gives no verdict and punishes nobody | VERIFIED | Only `memberReqAIVerdictOf(p) == memberReqBreach` can punish and `p` comes from a probability read from the cache or returned by `ClassifyNames` (`memberreq_ai.go:192-198`, `:350-352`, `:509-516`). Every other outcome is the zero verdict (unknown): breaker pause, gate not obtained, budget spent or counter fault (sticky unknown, `:494-500`, `:550-570`), HTTP error, refusal, no tool call, out-of-range value, transport failure, cancelled context (`:502-506`, `:572-588`). `memberReqAIParseProbability` rejects NaN, infinity and values outside 0..1 (`:173-179`). The executor adds a bit only on an explicit breach (`memberreq_rules.go:241-245`). `TestMemberReqAIUnanswerable` (run, passed), `TestMemberReqAIBreaker`, `TestMemberReqAILatch` |
| 7 | A baseline never launders a judged member, and another account's cached breach never reaches a member who never renamed | VERIFIED | `memberReqAIDecide` order (`memberreq_ai.go:346-357`): equal baseline with no marker and not joining passes first, with the cache ignored (SEC-01); a marker or `skipBaseline` (Joining, IgnoreBaseline) falls through to the cache and the call. Held release and I fixed it set `IgnoreBaseline` (`memberreq_held.go:88`, `memberreq_fixed.go:251`). A breach sets the marker (`memberreq_ai.go:400-402`). `TestMemberReqAIEstablishedMemberIgnoresCachedBreach` (run, passed), `TestMemberReqAIMarkerSurvivesLongHold`, `TestMemberReqAIBaseline` |
| 8 | A rename during the provider call never punishes the old string | VERIFIED | `NamesBound` on the AI row (`memberreq_rules.go:140`); the executor computes `judged := memberReqAINamesHash(names)` before the lookups (`memberreq_enforce.go:288`) and, in the re-read branch, drops the names-bound bit when the re-read record's hash differs (`:341-343`). The AI flight always counts as touched, so the re-read runs when something is broken. `TestMemberReqAINamesChangedDuringLookup` (in the validated set, passed in the orchestrator's gate run) |
| 9 | No new shutdown drain, pool or queue; the provider call ends with `StopMemberReqJobs` | VERIFIED | `git diff aa68b34 HEAD` over `main.go`, `fuku/modules/shutdown_drains.go`, `fuku/modules/runner.go`, `fuku/db/migrations/runner.go`, `scripts/migrate_psql.sh`, `settings_pages.go`, `raid_ai_breaker.go`, `raid_ai_image.go`, `go.mod`, `go.sum` is empty. The call runs inside `memberReqFlightValue` on a context derived from `memberReqContext()` and bounded by `memberReqLookupFlightTimeout` (`memberreq_ai.go:382-384`), the flight recovers internally (AGENTS.md Go rules, `memberReqFlightValue`), the gate slot is released by `defer` (`:497`). The 65 s worst case of AGENTS.md is unchanged. `TestShutdownEndsMemberReqAI` (run, passed in 0.27 s: a job inside a hung call ends at the drain's cancel, leaves no hold, notice, write, warn or cached verdict, is not a breaker failure, releases its slot, a later message starts no job) |
| 10 | The AI key is registered and never logged; logs carry IDs, status and error type only | VERIFIED | `config.go:609-618` `registerSecrets` includes `cfg.AnthropicAPIKey` (`logredact.RegisterSecret`); the key travels only as `Client.APIKey` in the `x-api-key` header; `memberReqAILogFailure` logs chat and user IDs, HTTP status and error type and scrubs through `raidAIScrub(err.Error(), key)` (`memberreq_ai.go:595-610`); a cancelled flight logs nothing. `TestMemberReqLookupLogsCarryIDsOnly` (extended; passed in the orchestrator's gate run) and `TestMemberReqAIRequestIsMinimal` (key in its header alone) |
| 11 | The eighth rule is wired end to end into every path: table, repository, migration, watcher, executor, held release, I fixed it, notice, page | VERIFIED | Migration `20261109120000_add_member_requirement_ai_rule.sql`: one `ADD COLUMN IF NOT EXISTS ai_action SMALLINT NOT NULL DEFAULT 0 CONSTRAINT chk_chat_requirements_ai CHECK (ai_action BETWEEN 0 AND 4)`, newest file in `migrations/`, no `BEGIN`, no `ADD CONSTRAINT`; model has no `default:` tag (`grep -c 'gorm:"default'` over `member_requirements.go` is 0); `memberreq.RuleAI` appended to `Rules`; table row `ai` bit 128 with `Lookup`, `NamesBound`, `RemedyKey`, `Configured`, `ConfirmKeys`; the executor, watcher (`memberreq_watch.go:186-245`), held release (`memberreq_held.go:78-95`), I fixed it (`memberreq_fixed.go:240-251`) and the notice remedy all read it through the table. `TestChatRequirementsSettings` (passed), `TestMemberRequirementAIMigrationStatements` (database-free half) |
| 12 | The refusal latch and the watcher's paused check stop a marker holder costing a job and a paced `getChatMember` per message, without releasing a hold | VERIFIED | `memberReqAIOnlyPaused` (`memberreq_watch.go:326-345`) returns no job only when the AI part is the sole open reason and the breaker or latch holds; it never takes the clean path (so never `ClearHoldOnClean`). `TestMemberReqWatcherAIPaused`, `TestMemberReqWatcherAILatch` (in the validated set) |
| 13 | New strings exist in all 8 locale files; no Go reference to a missing key; the AGENTS.md citations resolve | VERIFIED | `git diff aa68b34 HEAD -- locales/en.yml` adds exactly the 10 keys (`mreq_rule_ai`, `mreq_remedy_ai`, `settings_mreq_rule_ai`, `settings_mreq_hint_ai`, `settings_btn_mreq_ai`, `settings_mreq_desc_ai`, `settings_mreq_confirm_ai_kick/_ban/_mute`, `settings_mreq_here_ai`) and changes no existing key; each of the 7 other locale files also gains 10 lines; counts are 255 `settings_` and 31 `mreq_` keys in `en.yml` and 286 `settings_`+`mreq_` keys in each of the eight files, matching AGENTS.md; `make check-translations` and the locale tests were clean in the orchestrator's gate run. Every `Test*` name AGENTS.md cites (script over `AGENTS.md` against all `func Test` declarations) resolves; the only misses are the English words "Testing" and "Tests" |

**Score:** 13/13 truths verified (0 present, behavior-unverified). Truths 1, 3 to 8 and 12 are behaviour-dependent; each rests on a named test that exercises the transition (the ones I re-ran are marked "run, passed"; the others were in the orchestrator's 77-test gate run, exit 0 through `scripts/check_test_results`).

### Prohibitions (judgment tier, non-authoritative verdicts)

`unverified-prohibition: human review recommended` for each. None is passed silently; the verdict is my own code read or a passing test, and the owner should confirm. Plan 15-11's first two are `backstop` tier: they name facts only live checks settle, so no verdict is given beyond "not assumed by any test".

| Plan | Prohibition (short) | Verdict | Evidence |
| ---- | ------------------- | ------- | -------- |
| 15-01 | No gorm default tag, no `ADD CONSTRAINT` or `NOT VALID`, neither applier changed | holds | `grep -c 'gorm:"default' fuku/db/models/member_requirements.go` is 0; migration is one `ADD COLUMN IF NOT EXISTS` with the CHECK inline; empty `git diff aa68b34 HEAD` over `runner.go` and `migrate_psql.sh` |
| 15-02 | `claude` imports stdlib only and never `namecanon`; `ClassifyNames` takes canonical names; no logging; no name or key in an error | holds | `names.go` imports only `context`, `encoding/json`, `errors`, `net/http`; errors are fixed strings (`names.go:75`, `:111`); `TestClaudeIsLeaf`-style `leaf_test.go` and `TestCanonPackageIsLeaf` pass (claude and namecanon packages passed) |
| 15-03 | No existing locale key changes text; no key beyond the ten; no code outside the two i18n tests and locale files | holds (locale part) | the en.yml diff above adds 10 keys and removes none; the other files' diffs are 10 lines each |
| 15-04 | No pool, queue or drain; the call is inside the flight on `memberReqContext()`; no name, hash, probability, instruction or key logged or stored in a key or error; never Breach without a probability | holds | `memberreq_ai.go:382-384`, `:509-516`, log lines `:420`, `:435`, `:459`, `:595-610` carry IDs and error text only; Breach only from `memberReqAIVerdictOf` |
| 15-05 | The watcher never writes baseline, marker or verdict and never calls Telegram or the provider | holds | the only `Set` in `memberreq_watch.go` is the existing pass key (`:147`); the AI keys are read through a pipeline of `GET`s (`:196-229`) |
| 15-06 | The name check shares no state with the raid image checks; only the stateless `raidAIBudgetTake`, `raidAIHourStamp`, `raidAIScrub`; `raid_ai_breaker.go` and `raid_ai_image.go` untouched; no new drain | holds | grep of `memberreq_ai*.go` shows only those three helpers plus `raidNow` and `raidRedisTimeout` (clock and timeout constants); empty diff over both raid files |
| 15-07 | I fixed it never lifts a mute it did not place, never reads names from callback data, never calls the provider for an approved member; the AI row never bypasses the cache for a press | holds | `memberreq_fixed.go:140-156` marker then approvals before judging; names from the live record (`:240`); the AI row ignores `Fresh` (`memberreq_ai.go:314-316`); `TestMemberReqFixedAI` |
| 15-08 | The page and the gate read no Redis and make no Telegram or provider call for the AI status; no renderer tests the AI rule's identity; `settings_pages.go` untouched; no new callback field, action or namespace; no screen names the Staff Group, a member, a name, a probability or a count | holds | `settingsMreqRuleUnconfigured` tests `rule.Configured != nil` (`settings_mreq.go:298-300`); `Configured` reads `config.AppConfig` only; empty diff over `settings_pages.go`; `TestSettingsStaffPrivacy` (extended) |
| 15-09 | The spike tool imports only `claude` and `namecanon`, prints no key or name on stderr, copies no image-mode summary code | not independently re-read | `scripts/claude-spike/names_test.go` (`TestNamesModeKeyNeverPrinted` and friends) passed in the orchestrator's gate run; I did not read `scripts/claude-spike/names.go` line by line |
| 15-10 | Tests and an AGENTS.md paragraph only; no production change unless a test proves a defect, as its own fix commit | not independently re-read | the SUMMARY and review-fix files describe the fix commits; I did not re-walk the commit list |
| 15-11 (backstop) | Model accuracy, latency, cost, stability, live behaviour of a rejected key and a rate limit, and the wording of ten keys rest on the live checks and are not assumed by any test | holds | no test asserts any of them; they are human items 1 to 5 and 10 |
| 15-11 (backstop) | A spammer already in the group when the rule is switched on is first judged when they rename; with no Redis, a failed marker write or a join the bot never saw, a real new member is baselined and unjudged until they rename | holds as stated (accepted limit) | `memberreq_ai.go:353-355`, `:444-446`; matches 15-SPIKE.md "Accepted limits" and AGENTS.md |
| 15-11 | No production change except a fix its own end-to-end test proves; no spike result assumed; no requirement ticked before the end-to-end test and the gate pass; no earlier plan's note contradicted | holds on the evidence available | `TestMemberReqAIEndToEnd` ran and passed here; REQUIREMENTS.md already ticks MREQ-01, MREQ-10, MREQ-14 and PLAT-06 |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `migrations/20261109120000_add_member_requirement_ai_rule.sql` | idempotent column with named CHECK | VERIFIED | 22 lines, one statement |
| `fuku/db/memberreq/settings.go`, `fuku/db/models/member_requirements.go` | `RuleAI`, `ai_action` field and CHECK tag | VERIFIED | `TestChatRequirementsSettings` passed (`./fuku/db/memberreq` ok) |
| `fuku/utils/claude/names.go`, `request.go` | names request, shared `newRequest`, image request unchanged | VERIFIED | diff of `request.go` is a pure refactor (`requestSpec`); image tests in `claude_test.go` pass |
| `fuku/utils/namecanon/canon.go` | canonical names, hash tag `mrn1`, verdict key tag `mrv1` | VERIFIED | `./fuku/utils/namecanon` ok, leaf test passes |
| `fuku/modules/memberreq_ai.go`, `_budget.go`, `_breaker.go`, `_latch.go` | rule, budgets, own breaker, refusal latch | VERIFIED | 610, 145, 223 and 123 lines; read in full (`_ai.go`, `_budget.go`) or in part (`_breaker.go`); tests passed |
| `memberreq_watch.go`, `_held.go`, `_fixed.go`, `_join.go`, `_notice.go`, `_rules.go`, `_enforce.go`, `_lookup.go`, `settings_mreq.go` | the AI rule wired into every path | VERIFIED | grep and reads above |
| `scripts/claude-spike/names.go` | names mode for the owner's live run | VERIFIED (existence, tests passed in the gate run) | 440 lines; not read line by line |
| `locales/*.yml` | ten keys in 8 files | VERIFIED | see truth 13 |
| `sample.env`, `README.md`, `docs/.../environment.md`, `docs/.../aispam/index.md` | owner-facing description | VERIFIED with four Info slips | see Anti-Patterns Found |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| rule table row `ai` | `memberReqAIRule` | `Lookup` field (`memberreq_rules.go:140`) | WIRED | called by `memberReqLookupMasks` only while the stored action is not Off |
| executor | the rule | `memberReqLookupMask{Names, Joining}` (`memberreq_enforce.go:289`) | WIRED | names come from the live `getChatMember` record, never from the update |
| join entry | marker | `memberReqAIMarkJoin` before the Check-on-join branch (`memberreq_join.go:105-107`) | WIRED | gated by `memberReqAILive` |
| watcher | cached AI keys | `memberReqAIWatchQueue` in the same pipelined round (`memberreq_watch.go:196`) | WIRED | read-only |
| held release, I fixed it | the rule | `IgnoreBaseline: true` (`memberreq_held.go:88`, `memberreq_fixed.go:251`) | WIRED | |
| settings page and press gate | `Configured()` | `settingsMreqReadStatus`, `settingsMreqAIGate` | WIRED | process config only |
| `ANTHROPIC_API_KEY` | log redactor | `registerSecrets` (`config.go:609-618`) | WIRED | |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `memberReqAIJudge` | probability | `ClassifyNames` -> `parseVerdict` (finite 0..1, `tool_use` stop only) | yes (a live provider in production, an httptest server in tests) | FLOWING |
| AI cell and screen | stored action, `AINotConfigured` | `memberreq.GetSettings` (cached DB row) and `config.AppConfig` | yes | FLOWING |
| notice remedy | `RemedyKey` of the broken rules | table row | yes | FLOWING |
| verdict, baseline, marker | Redis keys `nv`, `nm`, `nu` | written only by `memberReqAIJudge`, `memberReqAIRecord*`, `memberReqAIBaseline`, `memberReqAIMarkJoin` | yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| 15 focused tests of the AI rule, budgets, gate, flight, verdict cache, baseline, burst, not-configured, request shape, shutdown drain, end-to-end, page status and press gate | `CGO_ENABLED=1 go test -tags testtools -race -count=1 -v -run '^(TestMemberReqAIRule\|TestMemberReqAIBudget\|TestMemberReqAIRequestIsMinimal\|TestMemberReqAINotConfigured\|TestMemberReqAIEstablishedMemberIgnoresCachedBreach\|TestMemberReqAIUnanswerable\|TestShutdownEndsMemberReqAI\|TestMemberReqAIEndToEnd\|TestSettingsMreqAIGate\|TestSettingsMreqAIStatus\|TestMemberReqAIBaseline\|TestMemberReqWatcherAIBurst\|TestMemberReqAIVerdictCache\|TestMemberReqAIFlight\|TestMemberReqAIGate)$' ./fuku/modules` | all 15 PASS, `ok ... 69.463s` | PASS |
| claude client, canonical names, repository | `go test -tags testtools -race -count=1 ./fuku/utils/claude ./fuku/utils/namecanon ./fuku/db/memberreq` | three `ok` | PASS |
| Debt markers in non-test files changed by the phase | grep `TBD\|FIXME\|XXX` over every changed `.go`, `.sql`, `.yml`, `sample.env` | none | PASS |
| AGENTS.md test citations | script over `AGENTS.md` against `func Test*` declarations | all resolve | PASS |
| Locale key counts | `grep -cE '^(settings_\|mreq_)'` per file | 286 in each of the 8 files; 255 and 31 in `en.yml` | PASS |

### Probe Execution

Step 7c: SKIPPED. The phase declares no `probe-*.sh`; none exists under `scripts/*/tests/`. The spike tool is the owner's manual live step by design.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| MREQ-14 | 15-01 to 15-11 | Only names (first, last, username) go to Claude Haiku, on join and on name change, never message text; established members' names first recorded without a call; verdicts cached, hourly budgets cap the cost; ENABLE_AISPAM=false or no key turns the rule off; no verdict means no punishment | SATISFIED on code read plus automated evidence only | truths 1 to 8. The one word that automated evidence cannot reach is "Claude Haiku": that the configured model accepts the forced tool call and judges names at the 0.9 bar rests on human items 1 to 4 |
| MREQ-10 | 14-*, 15-08 | A check that cannot be answered never punishes; the page shows when a rule cannot work (no linked channel, bot not admin, AI not configured) | SATISFIED on automated evidence only | the AI line, Off-only form and press gate (truth 5, `TestSettingsMreqAIStatus`, `TestSettingsMreqAIGate`) complete the three cases; the first two are Phase 14's and still pass |
| MREQ-01 | 13-*, 14-*, 15-* | Eight rules, each its own punishment off/warn/kick/ban/mute, all off by default | SATISFIED on automated evidence only | eighth row and cell (truth 11); migration default 0 with a named CHECK; every existing group reads the rule Off. The PostgreSQL proof of the CHECK is in CI's integrity target (human item 11) |
| PLAT-06 | cross-cutting | Background work shares rate budgets across replicas, recovers from crashes, drains within the existing shutdown budget | SATISFIED, remains ticked | name checks add no drain (truth 9), share fleet-wide Redis budgets (truth 4), flights recover from panics, `TestShutdownEndsMemberReqAI`. Names use no Telegram pacer (the call is HTTPS to Anthropic; only punishment and notice use `memberReqPaced`), as 15-SPIKE.md states |

No orphaned requirement: REQUIREMENTS.md maps MREQ-14 to Phase 15 and MREQ-01 and MREQ-10 to Phases 13, 14 and 15; all three appear in the plans.

### Accepted limits found (listed, not re-litigated)

- 15-SECURITY.md AR-01 to AR-18, including AR-18 (an account's own hourly allowance latches nothing, so one job per message of that member until their hour ends) and the SEC-05 acceptance.
- 15-REVIEW-DISPOSITION.md IN-01, IN-02, IN-04, NEW-01, NEW-02, NEW-03.
- 15-SPIKE.md "Accepted limits": a member already in the group is judged only after a rename; a baseline lasts 180 days and is never refreshed, so a member dormant for longer is first-sighted again and baselined with no call; a stale verdict stays 24 hours; the AI can be wrong (freed by a rename, an approval, or I fixed it for a mute); a Redis outage is silent on the page; Check on join off judges at the first message; a replica on older code answers a `t=ai` press as invalid.
- Follow-ups left open on purpose: D-22 (sticky unknowns defer nothing), D-23 (no re-roll on I fixed it or the held release), A15-A4 (an I fixed it tap after the member's 5 hourly units are spent answers a "try again in a moment" text), A15-A6, A15-A9.

### Anti-Patterns Found

No stub, hollow prop, empty implementation, debt marker or console-only handler in any file of the phase. Four Info items, all wording that predates or ignores the name check; none changes behaviour:

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `AGENTS.md` | 2657 | "Defaults that surprise: `ENABLE_AISPAM=true` (the global AI kill switch, text and images; inert without `TYPESAFE_API_KEY`)" omits names, while the `/autolockdown` paragraph of the same file and `environment.md:47-53` say the switch also turns the name rule off | Info | A rulebook reader could miss that `false` stops name checks too |
| `sample.env` | 120-125 and 143-147 | The `ENABLE_AISPAM` comment lists "/aispam, the raid text checks (TypeSafe) and the raid image checks" and the `ANTHROPIC_MODEL` comment says "Claude model for the image checks"; neither mentions the AI spam-name rule, although the comment at 133-138 between them does for the key | Info | An owner configuring from `sample.env` is told the model and switch affect images only |
| `README.md` | 804 | `ENABLE_AISPAM`: "Global kill switch for every AI check (text, images and names). Chats still have to opt in with `/aispam on`." The opt-in sentence is false for names (the opt-in is the owner choosing a punishment for the AI rule, not `/aispam on`) | Info | Could lead an owner to think `/aispam on` is needed for the name rule |
| `docs/src/content/docs/commands/aispam/index.md` | 149 (and `fuku/config/config.go:198-206`, comment) | The `ENABLE_AISPAM` row says "every AI check, `/aispam` and the raid checks, text and images", and the `config.go` comments on `EnableAISpam` and the Anthropic fields mention text and images only | Info | Same omission as above; the paragraph at lines 108-113 of the same page does say names are covered |

**Corrected after this verification (orchestrator, wording only, no behaviour change):** all four Info items now name the AI spam-name rule next to text and images: the `AGENTS.md` defaults line, the `sample.env` `ENABLE_AISPAM` and `ANTHROPIC_MODEL` comments, the README `ENABLE_AISPAM` row (the `/aispam on` opt-in no longer claims to cover the name rule, whose opt-in is the owner choosing a punishment), the `aispam/index.md` row and the two `config.go` comments (comment-only). `make check-docs`, `make check-translations`, `go build` and `gofmt` are clean.

### Human Verification Required

Eleven items, listed in the frontmatter (`human_verification`): the live Haiku accuracy at the 0.9 bar, stability, latency and cost, request acceptance by the configured model, refusal behaviour, the six-step test-group walk-through, phone-width cells in hi, ru and uk, two live replicas, SIGTERM during a burst, the native-speaker read of the ten new keys (the three confirm-card keys as reworded by SEC-04 included), CI on HEAD (lint, `make test`, the PostgreSQL integrity target), and the owner's review of the prohibitions, accepted limits, wording slips and follow-ups.

### Gaps Summary

No gaps. All three ROADMAP success criteria and every supporting truth are delivered and wired in the code, and each behaviour-dependent truth has a passing test that drives the real handlers through the hand-written Telegram fake, an `httptest` provider, miniredis and SQLite. I re-ran 15 named tests plus three packages and read the rule, budget, join, held, watcher and page code myself. The phase status is `human_needed` solely because the owner deferred the live checks of 15-SPIKE.md, the native-speaker read and CI on HEAD, and because judgment-tier prohibitions need the owner's eyes. MREQ-14, MREQ-10 and MREQ-01 are satisfied on code and automated evidence only; the live model's accuracy and the live Telegram behaviour are not proven by anything in this repository. For every outcome those checks can have, the code punishes nobody on an unknown, a paused breaker, a spent budget or an unconfigured AI.

---

_Verified: 2026-11-10T12:00:00Z_
_Verifier: Claude (gsd-verifier)_
