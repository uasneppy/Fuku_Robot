---
phase: "7"
slug: "ai-assisted-raid-detection"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-10-29"
---

# Phase 7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` with build tag `testtools`, testify assertions only; real fixtures: `internal/testdb.Run` (SQLite), miniredis, hand-written `gotgbot.BotClient` fakes (`moduleBotClient`, `lockdownFake`), `httptest` servers standing in for Gemini, TypeSafe and Telegram file downloads |
| **Config file** | none; `-tags testtools` is mandatory (AGENTS.md). Harness: `alita/modules/test_harness_test.go`, `alita/modules/lockdown_fake_test.go` |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^(TestRaidAI|TestGemini|TestAutolockdown)' ./alita/modules ./alita/utils/gemini ./alita/db/raiddetection` |
| **Full suite command** | `make test` |
| **Estimated runtime** | ~60 seconds quick, ~6 minutes full |

---

## Sampling Rate

- **After every task commit:** Run the quick run command above
- **After every plan wave:** Run `make test` plus `go vet -tags testtools ./...` and `gofmt -l` (golangci-lint cannot lint Go 1.26 in this sandbox)
- **Before `/gsd-verify-work`:** `make test`, `make check-translations`, `make check-docs` and `CGO_ENABLED=0 go build ./...` green
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

Seeded from the research's requirement-to-test map. The planner and executors replace each row with the real task IDs; the Nyquist audit fills the status.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 07-XX-XX | TBD | TBD | RAID-04 | T-07 | Gemini request has `x-goog-api-key`, no key in the URL, and a verdict parse that treats every unreadable or blocked answer as no verdict | unit (httptest) | `go test -tags testtools -race -count=1 -run '^(TestGeminiRequestShape|TestGeminiVerdictParse|TestGeminiRetryAndErrors)$' ./alita/utils/gemini` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-04 | T-07 | Only suspicious items from new members are sent; established members, admins, approved users, anonymous senders and bots never are | integration | `... -run '^(TestRaidAICandidateText|TestRaidAIMedia)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-04 | T-07 | `/aispam` on: one TypeSafe call per text and its delete verdict feeds the counter; off: raid-only call, nothing deleted | integration | `... -run '^TestRaidAISpamSharesVerdict$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-03 | T-07 | Counter script counts one verdict per message or album and distinct accounts; N verdicts from at least 3 accounts fires `ai_burst`; one verdict or one account never does | script and end-to-end (miniredis) | `... -run '^(TestRaidAIVerdictScript|TestRaidAIBurstFires)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-03 | T-07 | The alert names the AI trigger and count, the spammers are in the sample and the Ban run; every surface renders `join_surge`, `flood` and `ai_burst` distinctly | integration | `... -run '^(TestRaidAIAlert|TestLockdownTriggerKindsAllSurfaces)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-05 | T-07 | The raid AI path never deletes, bans, restricts or posts; provider slow, down, unreadable, unconfigured, breaker open or queue full means no verdict and the update returns promptly while join and flood triggers still fire | integration | `... -run '^(TestRaidAINeverActs|TestRaidAIFailOpen|TestRaidAIBreaker|TestRaidAINoRedis)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-03 | T-07 | `/autolockdown ai [N\|on\|off]`: live authority, anonymous-admin proof, range 3..500, column-targeted writes with `DeleteCache`, stale cached settings normalise to the defaults, "not set up" notice | integration and repo | `... -run '^(TestAutolockdownAI|TestRaidDetectionStaleCacheDefaults)$' ./alita/modules ./alita/db/raiddetection` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | PLAT-02 | T-07 | The Gemini key and the Telegram file URL (bot token) never reach a log or a returned error, including debug output; the client is a leaf package with no `alita/config` or `alita/db` import | unit and integration | `... -run '^(TestScrubGoogleAPIKeyShape|TestRaidAISecretsNeverLogged|TestGeminiErrorsNeverContainKey|TestGeminiPackageIsLeaf)$' ./alita/utils/logredact ./alita/modules ./alita/utils/gemini` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-04 | T-07 | Per-chat and global hourly budgets stop provider calls; a cached verdict (same `file_unique_id`) costs no budget; the pool is bounded and drains on shutdown | unit | `... -run '^(TestRaidAIBudget|TestRaidAIPool)$' ./alita/modules` | ❌ W0 | ⬜ pending |
| 07-XX-XX | TBD | TBD | RAID-03 | T-07 | Migration adds `ai_enabled` and `ai_threshold` with defaults and the 3..500 CHECK; the carry-over and settings guarantees still hold on PostgreSQL | PostgreSQL only (CI) | extend `TestRaidDetectionSettings` and keep `TestRaidDetectionCarryOver` in `make test-postgres-integrity` (never run locally) | ✅ extend | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `alita/utils/gemini/*_test.go` — client request shape, verdict parse, retry and error tests, and the leaf-import test, with an `httptest` helper
- [ ] `alita/modules/raid_ai_*_test.go` — candidate, media, counter, fire, alert, fail-open, budget and pool tests, with an `httptest` fake for Gemini, TypeSafe and Telegram file downloads
- [ ] A `getFile` response on `lockdownFake` or `moduleBotClient` (the backup tests already set `client.responses["getFile"]`)
- [ ] A new top-level PostgreSQL-only test function, if one is added, is listed in the `-run` pattern of `make test-postgres-integrity` with its package; extending `TestRaidDetectionSettings` avoids this
- [ ] No harness `AutoMigrate` list change is needed (the `ChatRaidDetection` model is already in every list)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Gemini accepts the exact request (model id, `thinkingLevel`, `responseJsonSchema`, `mediaResolution`, `image/webp`, thumbnails) and its spam accuracy at the 0.85 bar, latency, token use, cost, safety-block rate and quota | RAID-04 | Needs the owner's `GEMINI_API_KEY` and real spam samples; the suite never calls Gemini | Run the spike kit (`scripts/gemini-spike/`, `07-SPIKE.md`) with a folder of labelled images, including saved thumbnails of real raid GIFs and video stickers |
| A real image burst from new accounts locks a real group and the alert names "AI spam" | RAID-03 | Needs a live bot, Redis, a supergroup and many real accounts | Send suspicious images from three or more accounts that joined in the last 24 h with the key set; watch the lockdown and the alert |
| With no key set, `/autolockdown` says AI checks are not set up and no image leaves the server | RAID-04, PLAT-02 | Needs a running bot | Start without `GEMINI_API_KEY`, send images from a new account, check the notice and the absence of outbound calls |
| Native-speaker read of the new strings in es, fr, hi, id, pt and ru | RAID-03 | Translation quality | Review the new `lockdown_*ai*` and `autolockdown_*ai*` keys |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
