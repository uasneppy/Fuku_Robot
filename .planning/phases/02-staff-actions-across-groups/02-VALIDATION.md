---
phase: "2"
slug: "staff-actions-across-groups"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-10-05"
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify assertions; real fixtures (SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes) |
| **Config file** | none; every test file starts with `//go:build testtools` |
| **Quick run command** | `go test -tags testtools -race -count=1 -run '^TestStaffAction' ./alita/modules` |
| **Full suite command** | `make test` (sandbox: `go test -tags testtools -race -count=1 ./alita/modules ./alita/utils/ratelimit ./alita/utils/extraction ./alita/db/user ./alita/i18n`) |
| **Estimated runtime** | ~60 seconds (quick run ~15 s) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command for the touched package
- **After every plan wave:** Run the full suite command
- **Before `/gsd-verify-work`:** `make test`, `make lint`, `make check-translations`, `make check-docs` must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 2-01-01 | 01 | 1 | STAFF-XX | T-2-01 / — | {filled by planner} | unit | `{command}` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Stateful `gotgbot.BotClient` fake for staff actions (per-`(chat,user)` status, `until_date`, `is_member`, `can_send_messages`, `can_restrict_members`; ban/restrict/unban as status transitions; scripted 429 sequences; `editMessageText` failure and "not modified" injection; call-order recording)
- [ ] miniredis helper for staff-action tests (`cache.SetRedisClientForTest` exists)
- [ ] `extraction.ParseDurationToken` plus tests
- [ ] `user.FindUsersByUsername` plus tests in `alita/db/user`
- [ ] Real-dispatcher harness test loading `LoadBans`, `LoadMutes` and `LoadStaffActions` in registry order
- Framework install: none

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Mute fan-out never lifts a real ban (research flag; tdlib's final status-replacement step is inferred) | STAFF-04 | Requires a live Telegram group; fakes model the inferred server semantics only | In a throwaway group, ban a test account, run a staff `/mute` and `/unmute` from the Staff Group, and confirm the account is still banned |
| Behavior under real Telegram flood control across many groups | STAFF-12, PLAT-01 | Real rate-limit numbers are unpublished; tests script 429 responses | Link several groups, run a staff `/ban`, and confirm the summary finishes with every group marked and no group dropped |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
