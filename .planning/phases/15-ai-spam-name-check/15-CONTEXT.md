# Phase 15: AI Spam-Name Check - Context

**Gathered:** 2026-11-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 15 adds the last member requirement, the AI spam-name rule, on top of Phase 13's rules engine, punisher and page and Phase 14's lookup layer:

- **AI spam-name rule** (MREQ-14): a member whose first name, last name and username Claude Haiku judges to be spam (an advertising or scam account) gets the rule's punishment, when they join and when their names change. Only those three strings leave the server, never message text, a photo, the bio, the group or an ID.
- **Established members** are first recorded without a call, so switching the rule on in a large group causes no burst of calls. Verdicts are cached by the names and hourly budgets cap the cost.
- **Not configured and unanswerable** (MREQ-10, completed here): with `ENABLE_AISPAM=false` or no Anthropic key the rule is off and the page says "AI not configured". A slow, failing, rate-limited, over-budget, paused or shut-down AI gives no verdict and punishes nobody.
- **MREQ-01 completes** with this rule: the page then offers all eight rules, each with its own punishment, all Off by default.
- **PLAT-06** for the new background work: the AI call shares nothing it can starve, recovers from a crash, never holds up the rest of the bot and ends within the existing shutdown budget.

The five name rules, the photo and channel rules, the hold and its one-punishment guarantee, the marker mute, "I fixed it", the watcher at group -2, the cap and pause, the lookup pacer and flights and the shutdown drain are Phases 13 and 14's and are extended, not rebuilt. Deleting requirement notices on a timer is Phase 16. The raid AI image and text checks (Phase 7) are not touched.

</domain>

<decisions>
## Implementation Decisions

### What is sent, when it runs, who is checked (accepted, area 1)
- **D-01:** Only the member's first name, last name and username are sent, nothing else: no bio, photo, message text, group title, chat ID or user ID. The code-owned system instruction tells the model that the names are data to classify and never instructions (as the raid image check does), and the forced one-tool answer is a single `spam_probability` number. The names are bounded in length before they are sent, and they are never logged, never put in a Redis key or value and never stored (only a hash of them).
- **D-02:** The names are judged at join (the join entry's synchronous call, under the 4 s join budget) and when a member's names change, seen at the member's next message (the enforcement job, under the 20 s job budget). An established member's first sight only records a baseline and makes no call (research decision 17, confirmed): without it, turning the rule on in a large group would be a burst of calls. A spammer who was already in the group when the rule was switched on is therefore first judged when they rename; this is accepted and stated in the rulebook.
- **D-03:** Exemptions are exactly the other rules': the bot, bots, Telegram service accounts, admins, approved users and members with a pending captcha are never checked, and a held member's message makes no lookup (the held path). The AI check sits after the exemptions, the hold read, the pause and the live `getChatMember`, and before the cap and the claim, only while the AI rule's action is not Off.
- **D-04:** The baseline is Redis only: `fuku:memberreq:nm:<user>`, a hash of the normalised three names (never the names), 30 days. Without Redis, or on a Redis error, the rule is inert (no baseline, no call, no punishment), so it can never act on a guess; the page's status line says so only through the "AI not configured" line when the key is missing, and a Redis outage is the unknown case. A name change is a different hash than the baseline.

### Verdict, punishment, model, and no answer (accepted, area 2)
- **D-05:** A verdict is a breach only at or above 0.9 (`memberReqAIBar`, a code constant). Names are short and ambiguous and a wrong punishment lands on a real person, so the bar is higher than the raid image check's 0.85 (the owner's question is settled here; retune after the live spike).
- **D-06:** The rule is Off by default like every rule and offers the same five punishments through the existing two-tap card for Kick, Ban and Mute. The rule screen's hint recommends Warn or Mute and not Kick or Ban, because the verdict is probabilistic and a member can change their name. The page never preselects a punishment.
- **D-07:** The model is Claude Haiku through the owner's `ANTHROPIC_API_KEY` and `ANTHROPIC_MODEL` (the key and model the raid image checks use, `claude.DefaultModel` fallback), through `fuku/utils/claude` (a text request beside the image one; the standard library only; no sampling parameter is ever sent; the key travels only in the `x-api-key` header).
- **D-08:** A slow, failing, rate-limited, over-budget, paused, shed, cancelled or unreadable answer (a refusal stop, a missing tool call, a value outside 0 to 1) is unknown: nobody is punished, nothing is cached, no hold, notice or write is left, and the member is judged again at their next trigger. As in Phase 14, an unanswered AI rule whose punishment is stricter than the strictest broken rule's defers that weaker punishment (`memberReqUnknownDefers`), never silently dropping the stricter rule.

### Cost, concurrency and shutdown (accepted, area 3)
- **D-09:** The AI call is an eighth lookup row (`ai`, bit 128, `memberreq.RuleAI`) of the Phase 14 lookup layer, run inside the existing enforcement job and join entry through `memberReqLookupMasks`, with one flight per names hash (`n:<hash>`, Fresh mode behind `f:`), a per-replica in-flight gate (4 calls) and the lookup budgets. There is no new queue, pool or goroutine set and no new shutdown drain: every call derives from `memberReqContext()`, which `StopMemberReqJobs` cancels, so the 65 s worst case of the drain list is unchanged. Research confirms this against the two pool options of research decision 8 (a separate small pool of 64 queued and 2 workers; a third job kind in the raid AI pool of 128 and 4) and records a load note.
- **D-10:** Hourly budgets in their own Redis key family, `fuku:memberreq:aib:*`, separate from the raid image budgets so a name burst can never spend the image allowance and the reverse: 5 calls per account, 100 per group and 400 in all (`memberReqAIUserHourlyBudget`, `memberReqAIChatHourlyBudget`, `memberReqAIGlobalHourlyBudget`, package variables only so a test can shrink them), counted `INCR` plus a 2 h expiry in one transaction on the UTC hour from `raidNow`, the per-account budget reserved first, as the image budgets are. A cached verdict is answered before any budget and spends none. Over budget is no verdict. A Redis error on any key is no verdict (never an unbounded call).
- **D-11:** The verdict cache is keyed by a hash of the normalised names, `fuku:memberreq:nv:<hash>`, 24 hours, and caches a pass and a breach alike, because the verdict is about a string and not a person. An unknown is never cached. This amends Phase 14's "positive only" rule (D-12 there) for this one string-level fact and nothing else: a verdict about a user is still never cached. A `Fresh` lookup ("I fixed it") bypasses it and refreshes it on a definite answer.
- **D-12:** The AI check has its own breaker, separate from the raid image breaker, with the same thresholds (five failures in one group pause that group's name checks for 5 minutes; a rejected key or account (401, 402, 403) and a 429 whose `retry-after` is over 2 s pause every group, for at most 5 minutes; a success clears the count; the breaker map is bounded). A paused check is no verdict and no spent unit, and an image failure never pauses name checks.

### Page, notices, "AI not configured", "I fixed it" (accepted, area 4)
- **D-13:** The page gets an eighth rule cell (`ai`, the punishment icon plus a short name, `memberReqRules` row 8, a `CellKey` and a `HintKey`), so the rule grid is four rows of two with no odd last cell. The page keeps its six rows and no callback grows past 53 bytes (`a=mo&t=ai`, `st` with `v`). A pass cache fingerprint covers eight actions (`memberReqPassValue`: one harmless pass-cache miss at the deploy).
- **D-14:** "AI not configured" is the one process-level fact the page states in words (`settings_mreq_here_ai`, a code-side 🚫 prefix, never an error): `ENABLE_AISPAM=false` or no `ANTHROPIC_API_KEY` (the same condition as `raidAIImageConfigured`'s configuration half). The page body's cannot-work block and the AI rule's status slot carry it; the rule screen is `[Off]` alone, two rows, with the choose and actions hints omitted, and a stored non-Off value stays visible and inert; a punishing press (`st` with `v` 1 to 4, a card's Yes) is refused with the same text as the alert, nothing written and no card, through a pure check with no network call (the AI counterpart of `settingsMreqChannelGate`, judged after the whole press pipeline). Off is never refused. This completes MREQ-10.
- **D-15:** The notice names the rule with a phrase like the other six (`mreq_rule_ai`, one of the lookup phrases after the name phrases) and the plain warn, kick and mute notices end with an AI remedy tail (`mreq_remedy_ai`) whenever the AI rule is among the broken rules, appended after translation like the photo remedy, inside the 520 code point cap in every language. "I fixed it" re-judges the names with one fresh call (bypassing the verdict cache, under the 5 s sub-budget and the member's per-account budget); an unknown, a paused check or an over-budget answer is `mreq_fixed_check_failed` and changes nothing. A rule still broken is the definite answer; the count answer covers up to eight rules (`mreq_fixed_still_broken_many` for five or more, as now).
- **D-16:** The AI rule's screen hint says that only the first name, last name and username are sent to Anthropic's Claude and never messages (`settings_mreq_hint_ai`); no other help text changes, and the `/lockdown` help text is untouched.

### Verification, spikes and tests
- **D-17:** Every new test keeps the Phase 14 rules: no `t.Skip` ever, fresh IDs, `memberReqCleanup`/`mreqLookupCleanup` style cleanups, no `t.Parallel` with the enforcement switch, the AI provider tested only against `httptest` servers or a fake transport (no real provider is ever called in a test), and the new families proved through CI's own gate (`go test -json` into `scripts/check_test_results`).
- **D-18:** The live Haiku spike (accuracy, latency and cost on a labelled set of real names, run with the owner's key through `scripts/claude-spike`) cannot run in this build (no key) and is deferred like the live Telegram checks. The plan writes the owner's checklist (`15-SPIKE.md`) and extends `scripts/claude-spike` with a names mode so the owner can run it; the code is safe under every outcome (a verdict below the bar, an unknown, or an unreachable provider never punishes), and the bar, the budgets and the model are one-line constants and environment settings to retune.

### Claude's Discretion
- The exact normalisation of the names for the hash (case, Unicode form, whitespace, a missing last name or username) and the length bound before sending, within D-01 and D-04.
- The shape of the watcher's cached read for the AI rule (the pass is satisfied when the baseline equals the current names hash, or the names hash has a cached pass verdict; the watcher stays Redis only, makes no Telegram call and writes no verdict) and where the baseline is written (in the job, never in the watcher).
- The system instruction's wording, the tool schema and the request's token cap, within the claude package's existing rules.
- Whether `claude.Client` gains a `ClassifyText` method or a name-specific method, and the migration's file name (a timestamp after `20261108120000`), within the Phase 14 migration pattern (`ADD COLUMN IF NOT EXISTS ai_action SMALLINT NOT NULL DEFAULT 0` with an inline named CHECK `chk_chat_requirements_ai`, so both appliers run it unchanged).
- Locale wording of the new keys in all 8 locale files within the existing length ceilings.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `fuku/modules/memberreq_lookup.go`, `memberreq_rules.go`: the verdict type (`memberReqUnknown` zero value, Pass, Breach, Inert), the lookup pacer, the singleflight flights (`memberReqFlightValue`, `memberReqFlight`, Fresh mode behind `f:`), `memberReqLookupMasks` and `memberReqLookupBroken`, the seven-row `memberReqRules` table (`Lookup` function on a row, nil `Broken`), the lookup budgets (`memberReqLookupJobBudget` 20 s, `memberReqLookupJoinBudget` 4 s, `memberReqFixedLookupBudget` 5 s) and `memberReqLookupTouch`.
- `fuku/modules/memberreq_enforce.go`, `memberreq_join.go`, `memberreq_fixed.go`, `memberreq_held.go`, `memberreq_watch.go`: the executor order (exemptions, hold, pause, live `getChatMember`, fresh settings, name rules and lookup rules, `memberReqUnknownDefers`, re-read after a slow phase, mute-over-restriction, cap, claim, punish, notice), the join entry, "I fixed it" step 5, the held release and the watcher's cached lookup read.
- `fuku/modules/raid_ai_image.go`, `raid_ai_breaker.go`, `raid_ai.go`: the hourly budget helpers (per account, per group, global; `INCR` plus expiry), the provider breaker pattern (per-chat failures, a provider-wide key pause, a bounded map) and `raidAIImageConfigured`; the new rule copies these shapes with its own keys and constants and imports none of the image state.
- `fuku/utils/claude` (`client.go`, `request.go`, `response.go`, `errors.go`): the Messages client (forced one-tool call, no sampling parameters, `x-api-key` only, `HTTPError.KeyRejected()` and `RetryDelay`), today an image request only.
- `fuku/modules/settings_mreq.go`, `memberreq_notice.go`: the rule grid, `settingsMreqChannelGate` (the model of the AI press gate), `settingsMreqReadStatus`, the status block and `settingsMreqOffOnly` for the Off-only screen, the notice phrases and `mreq_remedy_photo` remedy append.
- `fuku/db/memberreq/settings.go`, `fuku/db/models/member_requirements.go`, `migrations/20261108120000_add_member_requirement_lookup_rules.sql`: the rule IDs, the seven action columns and the additive migration pattern.
- `scripts/claude-spike`: the owner's manual spike tool (needs `ANTHROPIC_API_KEY`, refuses a non-https endpoint, exits 1 when every request failed).

### Established Patterns
- A rule is one row of `memberReqRules` plus a `memberreq.Rules` ID and bit, a column in `chat_requirements`, a cell, a screen and strings in all 8 locales; the page is registered from its own file and `settings_pages.go` is not edited.
- Unknown and inert are "not broken"; only an explicit breach adds a rule's bit; a lookup the process cannot answer punishes nobody and leaves no hold, notice or write.
- Every lookup derives from `memberReqContext()`; no lookup has a drain; the 65 s drain worst case in AGENTS.md is unchanged.
- Logs carry chat, user IDs and outcomes only, a provider error by its status code and error type only, never a name, a title, a text or the key (`TestMemberReqLookupLogsCarryIDsOnly`).
- AGENTS.md is updated in the same commit as every rule change; the locale tables in `fuku/i18n` limit tests are extended before the keys exist.

### Integration Points
- A new migration adds `ai_action` to `chat_requirements` (CHECK 0..4) and `memberreq.RuleAI` joins `memberreq.Rules`; `memberReqPassValue` fingerprints eight actions.
- A new text request in `fuku/utils/claude`, a name-judgement file in `fuku/modules` (`memberreq_ai.go`: the lookup row, the verdict cache, the baseline, the budgets, the breaker, the in-flight gate), the watcher's cached read and the join entry and executor through the existing lookup fold.
- `settings_mreq.go`: the eighth cell, the AI status line, the Off-only screen and the press gate; locale keys in all 8 files.
- `scripts/claude-spike`: a names mode for the owner's live check.

</code_context>

<specifics>
## Specific Ideas

- The page says plainly what is sent ("only the first name, last name and username go to Anthropic's Claude, never messages"), and what AI not configured means, in words, never as an error.
- "Never punish on a guess" is the invariant: no key, no Redis, a paused breaker, an over-budget hour, a slow answer, an unreadable answer and a verdict under the bar all leave a member alone.
- Warn or Mute is the recommended punishment in the rule's hint; Ban is never suggested.

</specifics>

<deferred>
## Deferred Ideas

- Judging established members at first sight (a burst of calls when the rule is switched on): rejected for this phase; a one-time sweep with its own pacing would be a later feature.
- A per-group custom list of allowed or blocked names, or a custom instruction to the AI: a future phase.
- Sending the group title or description as context to the model: rejected for privacy; revisit only if the live spike shows names alone are not enough.
- Using the raid AI image pool or its breaker for names: rejected here; revisit only if the load note shows the in-flight gate is not enough.
- Deleting requirement notices on a timer: Phase 16.

</deferred>
