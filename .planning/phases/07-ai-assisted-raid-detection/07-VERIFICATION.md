---
phase: 07-ai-assisted-raid-detection
verified: 2026-10-29T22:30:00Z
status: human_needed
score: 4/4 must-haves verified
covered_files:
  - .planning/phases/07-ai-assisted-raid-detection/07-01-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-01-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-02-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-02-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-03-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-03-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-04-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-04-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-05-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-05-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-06-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-06-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-07-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-07-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-08-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-08-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-09-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-09-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-10-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-10-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-11-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-11-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-12-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-12-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-13-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-13-SUMMARY.md
  - .planning/phases/07-ai-assisted-raid-detection/07-14-PLAN.md
  - .planning/phases/07-ai-assisted-raid-detection/07-14-SUMMARY.md
  - alita/config/config.go
  - alita/db/models/lockdown.go
  - alita/db/models/raid_detection.go
  - alita/db/raiddetection/repository.go
  - alita/db/raiddetection/stats.go
  - alita/modules/aispam.go
  - alita/modules/aispam_jev.go
  - alita/modules/autolockdown.go
  - alita/modules/devs.go
  - alita/modules/lockdown.go
  - alita/modules/lockdown_alert.go
  - alita/modules/lockdown_status.go
  - alita/modules/raid_ai.go
  - alita/modules/raid_ai_breaker.go
  - alita/modules/raid_ai_count.go
  - alita/modules/raid_ai_image.go
  - alita/modules/raid_ai_media.go
  - alita/modules/raid_trigger.go
  - alita/modules/shutdown_drains.go
  - alita/utils/gemini/client.go
  - alita/utils/gemini/errors.go
  - alita/utils/gemini/request.go
  - alita/utils/gemini/response.go
  - alita/utils/logredact/logredact.go
  - main.go
  - migrations/20261029120000_add_raid_detection_ai.sql
  - scripts/gemini-spike/main.go
covered_digest: "v2:sha256:9dc81f832ec3dd0e828d15afba79e881304a42dfa5ada3cda604f52466685fdc"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Run the Gemini spike with the owner's key"
    expected: "Put labelled spam and ordinary photos, GIFs and stickers in a folder and run `go run ./scripts/gemini-spike` with GEMINI_API_KEY set (then `-probe` and `-models-get`). The key is accepted, `gemini-3.5-flash-lite` answers in the strict JSON shape, the report shows accuracy at the 0.85 bar, latency and cost, and the model is not LEGACY or DEPRECATED. Follow 07-SPIKE.md to tune `raidAIImageBar`, the thumbnail choice and the model."
    why_human: "Needs the owner's real Gemini key and real sample images. Every test here runs against an httptest server, so the wire shape is proven against the discovery document and a fake, not against Google. No key exists in this environment."
  - test: "A real AI burst in a Telegram supergroup"
    expected: "With Redis running, GEMINI_API_KEY and TYPESAFE_API_KEY set and the bot an admin with can_restrict_members, have 3 fresh accounts post suspicious photos or link texts until 5 spam verdicts land within 120 s. The group locks on its own, one public notice appears, and the Staff Group alert and log copy read 'AI spam: N spam verdicts in 120 s' and list the accounts. One verdict, or five from two accounts, does not lock. A group that ran `/autolockdown ai off` does not lock. Established members' photos and plain texts cause no provider call."
    why_human: "Needs a live Telegram bot, real accounts with real join times, and the real providers. The tests use a real dispatcher with a fake Bot API client, miniredis and fake providers."
  - test: "Two or more replicas share one AI burst"
    expected: "With two replicas on one PostgreSQL and one Redis, verdicts reported by either replica add to the same counter and exactly one chat_lockdowns active row and one alert per destination result. After `/autolockdown ai off` on one replica the other stops counting at once."
    why_human: "Needs two live replicas. The single-row guarantee rests on uk_chat_lockdowns_active, lockdown.Start and BeginAlert (PostgreSQL CI); the cross-replica settings freshness rests on `raid_detection:` in skipLocal and Redis."
  - test: "PostgreSQL CI run (`make test-postgres-integrity`)"
    expected: "TestRaidDetectionSettings passes with the new chk_chat_raid_detection_ai CHECK (3..500) and the six column-targeted upserts, and the migration chain applies 20261029120000 cleanly on PostgreSQL 16 with existing rows taking (true, 5)."
    why_human: "PostgreSQL is never started in this environment. On SQLite the CHECK is built from the GORM model tag and passes, but the real migration file only runs on PostgreSQL."
  - test: "Native-speaker read of the six non-English translations"
    expected: "es, fr, hi, id, pt and ru read naturally for the new lockdown_auto_ai_*, lockdown_trigger_ai_burst, lockdown_alert_counts_ai keys and the AI paragraph of lockdown_help_msg. A real `/lockdown help` still sends in every language."
    why_human: "Translation quality and a real send cannot be judged by a script. make check-translations and TestLockdownHelpFitsOneMessage prove key parity, placeholders and length only. The summaries record that es, pt and id paragraphs were shortened to fit."
  - test: "Read the key out of a live log"
    expected: "Run the bot at DEBUG with the real Gemini key, provoke a rejected key (wrong GEMINI_API_KEY) and a Telegram getFile failure, and grep the full log output and /metrics for the key and for the bot token. Neither appears."
    why_human: "The tests prove this for fabricated keys against httptest servers, with the redaction hook on and off. A real Google error body and the real log pipeline can only be read live."
  - test: "make lint"
    expected: "golangci-lint v2 reports no new issue."
    why_human: "golangci-lint in this environment is built with Go 1.25 and cannot lint the Go 1.26 module (every Phase 7 summary says so). go vet, gofmt and CGO_ENABLED=0 go build ./... were clean in the earlier run and gofmt and the build are clean again here."
---

# Phase 7: AI-Assisted Raid Detection Verification Report

**Phase Goal:** As a group admin, I want to have AI flag borderline spam text and images, so that subtler raids still trigger a lockdown.
**Verified:** 2026-10-29
**Status:** human_needed
**Re-verification:** No, initial verification

All four ROADMAP success criteria hold in the code and in tests I ran myself. Nothing is FAILED. The status is `human_needed` only because the live checks (a real Gemini call with the owner's key, a real AI burst, two replicas, PostgreSQL CI, a native-speaker read of the translations, `make lint`) cannot run in this environment.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Borderline text goes to the existing TypeSafe check; borderline photos, GIFs and stickers go to Gemini (`gemini-3.5-flash-lite`, owner's key); ordinary messages go to no AI | VERIFIED | **Text:** `raidAIWatchText` (`alita/modules/raid_ai.go:218`) sends only a new member's text that carries a link, is a forward, or repeats one of the sender's last 5 texts (`:253-255`), after the carve-outs (`aispamCarvedOut`, `:257`). `raidAIProcessText` (`:552`) calls the existing `aispamJevDecide` with `raidAIStateOf`. Where `/aispam` will judge the text, `aispamWillJudge` (`:92`) makes the raid path send nothing, so at most one TypeSafe call per message. **Images:** `raidAIWatchImage` (`:282`) takes only a new member's media (`raidAICheckCandidate` with an empty hash, `:304`); `raidAIProcessImage` (`raid_ai_image.go:~118`) builds a `gemini.Client` from `config.AppConfig.GeminiAPIKey` / `GeminiModel`; `gemini.DefaultModel = "gemini-3.5-flash-lite"` (`alita/utils/gemini/client.go:28`). The frame is the photo size nearest 1024 px, a static sticker's WebP, or Telegram's own thumbnail for a GIF or video (`raid_ai_media.go`). **Not sent:** `raidFloodSenderCounts` (`raid_flood.go:199`) drops `sender_chat`, automatic forwards, service IDs and the bot; `raidAIWatch` (`:193`) drops non-supergroups; `aispamCarvedOut` drops pending captcha, admins and approved users; `!isNew` drops established members. Tests: `TestRaidAICandidateText`, `TestRaidAIMedia`, `TestRaidAIKillSwitch`, `TestRaidAITextRequestIsMinimal`, `TestRaidAINoRedis` pass. |
| 2 | A burst of AI spam verdicts in a short window locks the group and the alert names the AI trigger and its count; on by default; an admin can turn it off or change its threshold | VERIFIED | Every spam verdict reaches `raidAIRecordVerdict` (`raid_ai_count.go:162`) from three sources: `/aispam` delete verdicts (`aispam.go:326`), raid text jobs (`raid_ai.go:596`) and raid image jobs (`raid_ai_image.go:208`). The Lua script `raidAIVerdictScript` adds one item per message or album (`ZADD NX`) in a 120 s window. `raidAICounts` (`:116`) fires at threshold items from at least 3 accounts, and `raidAIFireAsync` (`:143`) calls the shared `raidFire` (`raid_trigger.go:222`, which goes through `startLockdownRow`) with `Kind: models.LockdownTriggerAIBurst`, `Count`, `Window` 120. Alert surfaces: `lockdownTriggerSentence`, `raidPostNotice`, `lockdownAlertTriggerLine`, `lockdownRenderSample` (`lockdown_alert.go:225, 508, 593`; `raid_trigger.go:281, 355, 388`; `lockdown_status.go:112`); `TestLockdownTriggerKindsAllSurfaces` renders every kind in `models.LockdownTriggerKinds()` on all of them. **Default on:** migration `20261029120000_add_raid_detection_ai.sql` adds `ai_enabled BOOLEAN NOT NULL DEFAULT TRUE` and `ai_threshold INTEGER NOT NULL DEFAULT 5` with `chk_chat_raid_detection_ai` (3..500); a missing row reads as `raiddetection.Defaults` (`DefaultAIThreshold = 5`, `repository.go:31`). **Admin control:** `/autolockdown ai [N|on|off]` (`autolockdown.go:25, 91, 148-152`) calls `raiddetection.SetAIThreshold` / `SetAIEnabled` (`repository.go:176, 184`), each a column-targeted upsert followed by `cache.DeleteCache` (`:133`); the key is in `skipLocal` (`alita/db/cache/local.go:47`); authority is the same live `getChatMember` check as `joins` and `flood`. Every verdict re-reads the chat's settings, so the next burst follows the new setting. Tests: `TestRaidAIBurstFires`, `TestRaidAIBurstSettings`, `TestRaidAIAlert`, `TestRaidAIVerdictScript`, `TestRaidAIImageBurst` (end to end through the real dispatcher, three new members, fake Gemini, `ai_burst` lockdown, notice and alert name the trigger), `TestAutoLockdown*`, `TestRaidDetectionSettings` (SQLite) pass. |
| 3 | AI never decides alone: a single verdict does not lock; if a provider is slow, down or not configured, the rule-based triggers keep working and nothing waits on AI | VERIFIED | **Rule decides:** a verdict only adds one item; `raidAICounts` needs `items >= threshold`, `accounts >= 3` and `threshold >= MinThreshold`. Tests cover one verdict, four verdicts from three accounts, five from two accounts, a message reported twice and keep verdicts: none lock (`TestRaidAIBurstFires`, `TestRaidAINeverActs`). **Never acts:** nothing on the raid AI path calls delete, ban, restrict or post; `TestRaidAINeverActs` asserts no Telegram write of any kind. **Nothing waits:** the group `2` watcher returns `ext.ContinueGroups` on every path and only enqueues (`raidAIEnqueue`, `raid_ai.go:456`) to a pool of 4 workers behind a 128-job queue, shedding when full or draining; the lockdown start runs on its own tracked, recovered goroutine (`raidAIFireAsync`), so no worker waits on Telegram. Each provider call derives from `raidAIContext()` (`:356`), cancelled by `StopRaidAI` (`:607`), registered in `main.go:226` after `RegisterRaidPostsDrain` and before the inline `DrainAISpamChecks`. Gemini has an 8 s timeout, one retry only when the wait is at most 2 s, a per-chat breaker (5 failures pause 5 min), a provider-wide pause for a rejected key or a long 429, hourly budgets (20 per account, 150 per group, 500 fleet), a verdict cache, an album gate and a fire-latch skip (`raid_ai_breaker.go`, `raid_ai_image.go`). **Not configured:** `raidAITextConfigured` and `raidAIImageConfigured` both require `ENABLE_AISPAM` (global kill switch) plus the key; without Redis the counters do nothing. The join and flood triggers do not import any AI code path. Tests: `TestRaidAIFailOpen` (slow, failing, rate limited, unreadable, unconfigured, paused and shed providers all return promptly with no verdict, and the join surge and flood triggers still lock), `TestRaidAIPool`, `TestRaidAIBreaker`, `TestRaidAIBudget`, `TestRaidAINoRedis`, `TestRaidAIKillSwitch`, `TestRaidJoinSurge`, `TestRaidFloodBurst`, `TestRaidTriggersSingleLockdown`, `TestRaidTriggersConcurrent` pass. |
| 4 | The Gemini API key never appears in logs, including errors and debug output | VERIFIED | `GEMINI_API_KEY` is read in `alita/config/config.go:344` and passed to `logredact.RegisterSecret` with the other secrets (`:481-488`); `logredact` also scrubs any `AIza[0-9A-Za-z_-]{35}` string even if never registered (`logredact.go:70-75`). The key travels only in the `x-goog-api-key` header (`gemini/client.go:35`), never a URL; `gemini/errors.go` rebuilds transport errors from `url.Error.Err` and replaces the key in every non-2xx body; the package imports the standard library only (`go list -deps` shows no module outside std). An invalid `GEMINI_MODEL` is never logged verbatim (only its length; IN-02). The Telegram file URL, which holds the bot token, is also never logged or wrapped (`raidAIDownload`). Tests: `TestGeminiErrorsNeverContainKey`, `TestGeminiConfig`, `TestScrubGoogleAPIKeyShape`, and `TestRaidAISecretsNeverLogged`, which runs at debug level and captures both the log output and a logrus hook, with a Gemini 400 that echoes the key, a 500 whose body is the key, and a redirect, and asserts neither the key, the bot token nor `/file/bot` appears in any returned error or log line. All pass. |

**Score:** 4/4 truths verified, 0 present but behavior-unverified. Every behavior-dependent truth (burst rule, never-acts, fail-open, shutdown drain, no key in logs) has a named passing test that exercises it. Live and PostgreSQL behavior is listed under human verification, not counted as a gap.

### Design decisions honoured (07-CONTEXT.md D-01..D-22)

| Decision | Status | Evidence |
|----------|--------|----------|
| D-01, D-02 (candidate rules, exclusions, supergroups only, cheapest first) | HONOURED | `raidAIWatch`, `raidAIWatchText`, `raidAIWatchImage`; `TestRaidAICandidateText` |
| D-03, D-19 (`/aispam` unchanged, its delete verdicts feed the counter, no second TypeSafe call) | HONOURED | `aispam.go:326` is the only addition; `aispamWillJudge`; `TestRaidAISpamSharesVerdict` |
| D-04 (bars: `/aispam`'s for text, 0.85 for images, failure is no verdict) | HONOURED | `raidAIImageBar = 0.85` (`raid_ai_image.go:30`); `aispamIsDelete` for text |
| D-05..D-09 (5 verdicts, 3 accounts, 120 s, Redis only, `ai_burst`, alert line, spammer snapshot) | HONOURED | `raid_ai_count.go`; WR-04 narrowed the snapshot to recent joiners (established spammers still count, are never listed or banned) |
| D-10, D-11 (plain REST, no new dependency, header key, `RegisterSecret`) | HONOURED | `go.mod`/`go.sum` last changed 2026-09-25 (commit 976c787, before Phase 7); the diff since the Phase 6 close touches neither |
| D-12, D-13 (media rules, 5 MB, request shape, 8 s, one retry, breaker) | HONOURED | `raid_ai_media.go`, `gemini/request.go`, `raid_ai_breaker.go` |
| D-14 (`/autolockdown ai`, migration, not part of backup) | HONOURED | `autolockdown.go`, migration `20261029120000`, `skipLocal` |
| D-15 (counters in `/stats`, no text logged, docs say what goes to Google) | HONOURED | `raidAIStatsSection` in `devs.go`, `LoadRaidDetectionStats` AI-off count; `TestRaidAIStats`, `TestRaidAIStatsDevsOnly` |
| D-16, D-17 (swappable seams, spike kit instead of the spike) | HONOURED | `scripts/gemini-spike/` (tests pass, endpoint validated, exits 1 when every request failed); `07-SPIKE.md` |
| D-18 (own pool, breakers, drain) | HONOURED | `raid_ai.go`, `StopRaidAI`, `RegisterRaidAIDrain` |
| D-20 (spend guard) | HONOURED, stricter | 150 per group and 500 fleet as specified, plus a per-account cap of 20 (WR-01) |
| D-21, D-22 (paid-tier docs note, no AI-SPEC) | HONOURED | `sample.env`, `docs/.../environment.md`, `.../commands/aispam/index.md` |

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| `alita/modules/raid_ai_count.go` (284 lines) | VERIFIED | Script, rule, async fire, snapshot; wired from three call sites |
| `alita/modules/raid_ai.go` (621 lines) | VERIFIED | Watcher at group 2 (`lockdown.go:560`), pool, text job, `StopRaidAI` |
| `alita/modules/raid_ai_image.go`, `raid_ai_media.go`, `raid_ai_breaker.go` | VERIFIED | Image path with all cost guards, token-safe download, breakers |
| `alita/utils/gemini/{client,errors,request,response}.go` | VERIFIED | Stdlib-only leaf package, strict verdict parse |
| `alita/config/config.go`, `alita/utils/logredact/logredact.go` | VERIFIED | Key and model config, secret registration, `AIza` pattern |
| `alita/db/raiddetection/repository.go`, `stats.go`, `alita/db/models/raid_detection.go` | VERIFIED | AI columns, defaults, normalisation of stale cache entries, six upserts |
| `migrations/20261029120000_add_raid_detection_ai.sql` | VERIFIED | Newest timestamp, idempotent, named CHECK 3..500 |
| `alita/modules/autolockdown.go`, `lockdown_alert.go`, `lockdown_status.go`, `raid_trigger.go`, `devs.go`, `shutdown_drains.go` | VERIFIED | `ai` word, all trigger-kind surfaces, stats, drain |
| `scripts/gemini-spike/` and `07-SPIKE.md` | VERIFIED | Kit present, network-free tests pass |
| `main.go:226` `RegisterRaidAIDrain` | VERIFIED | After DB-close, before the inline AI spam drain |

### Key Link Verification

| From | To | Status |
|------|----|--------|
| `processAISpam` delete branch (`aispam.go:326`) | `raidAIRecordVerdict` | WIRED |
| Raid text and image jobs | `raidAIRecordVerdict` (`raid_ai.go:596`, `raid_ai_image.go:208`) | WIRED |
| `raidAIRecordVerdict` | `raidFire` via `raidAIFireAsync` (`kind ai_burst`) | WIRED; `raidFire` calls the same `startLockdownRow` as `/lockdown` |
| `LoadLockdown` | `raidAIWatcher` at group 2 (`lockdown.go:560`) | WIRED |
| `raidAIProcessImage` | `gemini.Client` from `config.AppConfig` | WIRED |
| `writeAutoLockdown` | `raiddetection.SetAIThreshold` / `SetAIEnabled` then `cache.DeleteCache` | WIRED |
| `config.registerSecrets` | `logredact.RegisterSecret(cfg.GeminiAPIKey)` | WIRED |
| `main.go` | `RegisterRaidAIDrain` | WIRED |
| `lockdownBurstSnapshot` | `raidAISpammersLookup` for `ai_burst` | WIRED |

### Data-Flow Trace (Level 4)

Verdict counts come from Redis Lua scripts keyed `alita:raid:{chat}:av` and `:aa`, thresholds from `chat_raid_detection` through the cache, trigger numbers are stored in the `chat_lockdowns.trigger_*` columns and rendered by the alert, and the spammer sample comes from the `:aispammers` snapshot saved by `BeginAlert`. Provider verdicts come from TypeSafe and Gemini answers, not from static values. FLOWING.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Raid AI, burst, autolockdown, flood, join surge, antiraid, aispam and stats tests | `CGO_ENABLED=1 go test -tags testtools -race -count=1 -v -run 'RaidAI\|AIBurst\|AIAlert\|TriggerKinds\|AutoLockdown\|Autolockdown\|Raid\|Flood\|JoinSurge\|AntiRaid\|LockdownHelp\|AISpam\|Aispam\|Stats' ./alita/modules` | ok, 87 top-level PASS, 0 FAIL, 0 SKIP | PASS |
| Gemini client, settings, config, redaction, migrations, locale, devs, spike | `go test -tags testtools -race -count=1 ./alita/utils/gemini ./alita/db/raiddetection ./scripts/gemini-spike ./alita/config ./alita/utils/logredact ./alita/db/migrations ./alita/i18n ./alita/db/devs` | ok for all eight | PASS |
| Phase 6 regression (join surge, flood, single lockdown) | included in the first run: `TestRaidJoinSurge*`, `TestRaidFlood*`, `TestRaidTriggersSingleLockdown`, `TestRaidTriggersConcurrent`, `TestAntiRaid*` | PASS | PASS |
| Compile and format | `CGO_ENABLED=0 go build ./...`, `gofmt -l alita scripts main.go` | build ok, no files listed | PASS |
| Translations | `make check-translations` | all keys present in 7 of 7 locales | PASS |
| Docs drift | `make check-docs` | no drift | PASS |
| API coverage matrix | `gsd-tools check api-coverage.verify-pre` | passed, 23 capabilities (10 integrate, 13 opt-out) | PASS |
| New dependencies | `git log -- go.mod go.sum` | last change 2026-09-25, before this phase | PASS |
| Skipped tests | grep `t.Skip` in every test file this phase touched | none | PASS |
| PostgreSQL-only guarantees | `TestRaidDetectionSettings` (CHECK on PostgreSQL), migration chain | not run by rule | human (CI) |

I did not run the full `make test` (63 packages ok, exit 0 per the orchestrator) or `make lint` (cannot lint Go 1.26 here).

### Probe Execution

No probes declared for this phase. SKIPPED.

### Requirements Coverage

Requirement IDs from the 14 PLAN frontmatters: RAID-03, RAID-04, RAID-05, PLAT-02. All four appear in `REQUIREMENTS.md` and in the ROADMAP's Phase 7 list. REQUIREMENTS.md maps no other ID to Phase 7, so there are no orphans.

| Requirement | Plans | Status | Evidence |
|-------------|-------|--------|----------|
| RAID-03 (a burst of AI spam verdicts in a short window triggers a lockdown) | 07-01, 04, 06, 07, 10, 11, 12, 13, 14 | SATISFIED | Truth 2; `TestRaidAIBurstFires`, `TestRaidAIImageBurst` |
| RAID-04 (text by TypeSafe, photos, GIFs and stickers by `gemini-3.5-flash-lite` with the owner's key) | 07-02, 03, 05, 06, 08, 09, 10, 11, 12, 13 | SATISFIED in code; live Gemini call is a human check | Truth 1 |
| RAID-05 (rules, not AI, decide when to lock) | 07-01, 06, 08, 09, 12, 13, 14 | SATISFIED | Truth 3; prohibition tests `TestRaidAINeverActs`, `TestRaidAIBurstFires` |
| PLAT-02 (new secrets never appear in logs) | 07-02, 03, 08, 12, 13 | SATISFIED for the Gemini key; the Turnstile half is Phase 8 | Truth 4; ROADMAP assigns the Turnstile secret's `RegisterSecret` to Phase 8, "like the Gemini key" |

The REQUIREMENTS.md checkboxes and traceability rows for these four IDs still read Pending. The orchestrator updates them when the phase closes; PLAT-02 should stay open for the Turnstile half until Phase 8.

### Anti-Patterns Found

None blocking. No TBD, FIXME, XXX, TODO or HACK in the Phase 7 production files. No `t.Skip` in the new tests. The 14 review findings (5 warnings, 9 info) are marked fixed in `07-REVIEW-FIX.md`, and I confirmed in the code the ones that bear on the success criteria:
- WR-01: the per-account budget `raidAIUserHourlyBudget = 20` (`raid_ai_image.go`), tested by `TestRaidAIBudget`.
- WR-02: `ENABLE_AISPAM` gates `raidAITextConfigured` and `raidAIImageConfigured` (`raid_ai.go:72-80`), tested by `TestRaidAIKillSwitch`.
- WR-03: raid-only text requests use `raidAIState`, which has no field for description, rules or warn count (`raid_ai.go:502`), tested by `TestRaidAITextRequestIsMinimal`.
- WR-04: `raidAISpammersScript` saves only accounts with a `:joined` score within 24 h (`raid_ai_count.go:211`).
- WR-05: the test read helpers wait for the lockdown starts (`raidAIFires`) as well as the jobs and posts.

### Human Verification Required

See `human_verification` in the frontmatter (7 items). In short: a live Gemini spike with the owner's key, a real AI burst in a group, two replicas, `make test-postgres-integrity`, a native-speaker read of the translations (es, pt and id AI paragraphs were shortened), a live log grep for the key, and `make lint` on a toolchain that can lint Go 1.26.

### Gaps Summary

No gaps. Notes for the owner:
- In a group that has switched `/aispam` on, `/aispam` already sends every message to TypeSafe; that is existing opt-in behaviour (D-03) and is unchanged. The new raid path sends nothing extra to any AI in such a group, and elsewhere it sends only a new member's suspicious item.
- The image bar of 0.85, the thumbnail choice for GIFs and video stickers, and the model were never measured against Google (the spike was deferred by the owner, D-17). The kit and `07-SPIKE.md` are the way to do it.
- Rolling-deploy caveat (documented in AGENTS.md): an old-build replica can re-cache a settings row without the AI fields, so `/autolockdown ai off` may read as on on new replicas for up to 30 minutes in a mixed fleet.

---

_Verified: 2026-10-29_
_Verifier: Claude (gsd-verifier)_
