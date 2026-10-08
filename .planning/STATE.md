---
gsd_state_version: "1.0"
current_phase: 5
current_phase_name: Lockdown Alerts and Response
status: planning
stopped_at: Phase 04 complete, ready to plan Phase 5
last_updated: "2026-10-08T13:41:04.687Z"
last_activity: 2026-10-08
last_activity_desc: Phase 04 complete, transitioned to Phase 5
state_head: 14600f8313d8006bef3f8de82390e7f3aa4a7cfa
progress:
  total_phases: 9
  completed_phases: 4
  total_plans: 40
  completed_plans: 40
  percent: 44
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-10-08)

**Core value:** My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.
**Current focus:** Phase 5 — Lockdown Alerts and Response

## Current Position

Phase: 5 — Lockdown Alerts and Response
Plan: Not started
Status: Ready to plan
Last activity: 2026-10-08 — Phase 04 complete, transitioned to Phase 5

Progress: [████░░░░░░] 44%

## Performance Metrics

**Velocity:**
- Total plans completed: 40
- Average duration: -
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 10 | - | - |
| 02 | 10 | - | - |
| 03 | 12 | - | - |
| 04 | 8 | - | - |

**Recent Trend:**
- Last 5 plans: -
- Trend: -

*Updated after each plan completion*
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 03 P01 | 15min | 3 tasks | 16 files |
| Phase 03 P02 | 8min | 2 tasks | 2 files |
| Phase 03 P03 | 11min | 2 tasks | 12 files |
| Phase 03 P04 | 12min | 2 tasks | 15 files |
| Phase 03 P05 | 14min | 2 tasks | 13 files |
| Phase 03 P06 | 24min | 2 tasks | 16 files |
| Phase 03 P07 | 11min | 2 tasks | 11 files |
| Phase 03 P08 | 13min | 2 tasks | 13 files |
| Phase 03 P09 | 20min | 2 tasks | 11 files |

## Accumulated Context

### Decisions

Decisions are logged in the PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [Roadmap]: Image classification uses Gemini `gemini-3.5-flash-lite` (owner's decision). This replaces the research STACK's Claude Haiku / `anthropic-sdk-go` recommendation, so Phase 7 research must target Gemini.
- [Roadmap]: Raid auto-triggers are on by default with conservative thresholds (owner's decision), not "off until configured" as the research suggested.
- [Roadmap]: Multiple replicas are the deployment target, so fan-out pacing and detection counters go through Redis, not an in-process limiter. This closes research gap 5.
- [Roadmap]: PLAT-03 is mapped to Phase 1, PLAT-01 to Phase 2 and PLAT-02 to Phase 7, where each first matters. All three still apply to every later phase (see ROADMAP Cross-Cutting Constraints).
- [Roadmap]: LOCK-10 (one lockdown and one alert per raid) is in Phase 6, where concurrent auto-triggers first exist.
- [Phase 1]: Role exclusivity is enforced by a PostgreSQL trigger with per-chat advisory locks (owner's pick), on top of in-transaction app checks.
- [Phase 1]: Only a definite owner mismatch removes a link; Telegram errors are unknown and change nothing. Phase 2's pre-action owner recheck should reuse `staff.*Fresh` + `chat_status.CheckOwner` the same way.
- [Phase 1]: Exactly-once Staff Group notices come from conditional writes (RowsAffected == 1 posts), not locks.
- [Phase 2]: One decision table (`decideStaffAction`) picks each group's Telegram call from the target's live status, so no staff action lifts a ban. Phase 3's "Undo everywhere" must reuse it rather than calling Telegram directly.
- [Phase 2]: Staff fan-out calls go through the fleet-wide Redis pacer (`staffPaced`); a slot more than 60 s away fails at once as "rate limited". One run per target is held by a Redis lock renewed every 10 minutes.
- [Phase 3]: Undo decisions live in `decideStaffUndo`, beside `decideStaffAction`. One undo per action is the conditional `ClaimUndo`; `ReleaseUndo` gives it back only when no group reached its Telegram write (owner's UAT choice).
- [Phase 3]: Undo state everywhere (Recent actions, detail view, Ask/Confirm, the original summary) comes from stored per-group undo outcomes through `staffUndoStateOf`, never from the claim alone.
- [Phase 3]: The staff audit record (`staff_actions`, `staff_action_groups`) is read fresh, never cached, kept forever and outside backup/export/import/reset. Every staff fan-out, action or undo, goes through `startStaffRun` with a `staffRunSpec`; later fan-outs (Phase 5's "Ban N recent joiners") should reuse it.
- [Phase 4]: Lockdown state is only in PostgreSQL (`chat_lockdowns`, `chat_lockdown_joiners`, read fresh). The join guard (group -7) records and a DB-driven worker on every replica makes every Telegram write through the fleet-wide `lockdownPacer`. Phase 5's alert buttons and "Ban N recent joiners" should read the joiner rows and lift through `lockdownBeginLift` only.
- [Phase 4]: The lift restores the raw `getChat` snapshot first, then records it with the conditional `BeginLift`; it unbans only joiners whose live ban carries the row's 330-day `ban_until` marker. Authority is a live `getChatMember` (`requireLockdownAuthority`), never the admin cache.
- [Phase 4]: Every unmute goes through `resolveUnmutePermissions`, which returns the stored pre-lockdown permissions during a lockdown (D-23).

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 7]: Spike gate. Evaluate Gemini on spam images, GIFs and stickers, then run `/gsd-ai-integration-phase`.
- [Phase 8]: Spike gate. Test Direct Link Mini App launch from a group, and Turnstile rendering in Telegram WebViews. The owner must provide a public HTTPS hostname, a BotFather Mini App and Turnstile keys.
- [Phase 1]: The live ownership-transfer check (does `chat_owner_changed`/`chat_member` arrive, with the bot as admin and as a member) was never run; UAT tests 3-4 were deferred. The hourly sweep removes stale links whatever Telegram sends.
- [Phase 1]: 10 of 11 live-Telegram UAT checks were deferred without being run (`phases/01-staff-group-links/01-UAT.md` Deferred Follow-Ups). CI had not run on PR #1 at phase close; the PostgreSQL trigger test passed locally against PostgreSQL 16.
- [Phase 1]: Low, non-blocking security flag TF-01-06: a stranger's `/unlinkstaff` reply reveals whether the group is linked (`01-SECURITY.md`).
- [Phase 2]: Open code-review finding WR-03 (warning): at a `retry_after` of exactly 60 s, all but the first concurrent paced call fail as "rate limited" instead of waiting. Those groups are still reported. Nine info notes are open too (`02-REVIEW-DISPOSITION.md`).
- [Phase 3]: Open code-review warnings WR-03 (a retry after an ambiguous `ReleaseUndo` error can run `FinalizeUndo` on a released record) and WR-04 (a history page press is answered before the data is read, so a DB error is a silent no-op), plus 6 info notes (`03-REVIEW-DISPOSITION.md`). The owner accepted WR-01, WR-02 and IN-02 in UAT.
- [Phase 4]: Spike 1 answered by the owner: Telegram greys out a per-user "Send messages" exception once the default is locked, so approved users are muted during a lockdown like everyone else (LOCK-03 fallback, 04-CONTEXT D-01/D-02). Spike 2 (join paths) is designed away, with live join delivery checked in UAT.
- [Phase 4]: Code review info notes IN-01..IN-04 are open (`04-REVIEW-DISPOSITION.md`); CR-01, WR-01 and WR-02 were fixed before verification. UI review copy fixes landed in quick task 261008-j3d.
- [Cross-phase]: Some requirements are only partly checkable in their own phase, and a later phase finishes them. The `/staff` in-lockdown status (SETUP-08) shipped in Phase 4. The alert's Lift button (LOCK-06), and listing removed joiners on the alert (LOCK-01), land in Phase 5. The AI-trigger toggle (RAID-06) lands in Phase 7.
- [Quick 261008-j3b]: Shutdown follow-ups not fixed: `DrainAISpamChecks` waits up to 30 s but keeps the 10 s allowance; in polling mode the 60 s budget can still cut the staff drain short; `render.yaml`, `app.json`/Procfile and `railway.toml` keep their platforms' 30 s stop limits.
- [Quick 261008-j3a]: Follow-up candidate: an anonymous admin's ban-family command also gets a stray `chat_status_restrict_cmd_error` reply before the proof tap (pre-existing).
- [Phase 4]: Intermittent data race between the lockdown worker (`lockdown.ListJoinMsgsToDeleteFresh`) and `TestPostInitSetsCommandsAndStartupMessage` in `main_test.go`, seen only under heavy parallel load (did not reproduce in 6 runs or in the final `make test`). Test-harness race, not yet fixed.
- [Tooling]: `make lint` needs a golangci-lint built with Go 1.26; the sandbox's is built with Go 1.25, so the quick batch was linted only with `go vet` and `gofmt`.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 261004-w9v | Load AGENTS.md from .claude/CLAUDE.md and correct the locale list in CLAUDE.md and the codebase map | 2026-10-04 | c85d648 | [261004-w9v-load-agents-md-from-claude-claude-md-and](./quick/261004-w9v-load-agents-md-from-claude-claude-md-and/) |
| 261008-j3a | Fix anonymous-admin commands doing nothing after the proof tap | 2026-10-08 | 686f960 | .planning/quick/261008-j3a-fix-the-pre-existing-anonymous-admin-bug-after-the-anonymous |
| 261008-j3b | Give the staff and lockdown shutdown drains their full wait | 2026-10-08 | 6f7da31 | .planning/quick/261008-j3b-close-the-shutdown-timing-gap-the-shutdown-manager-gives-eac |
| 261008-j3c | Make TestStaffActionNonStaffUnchanged robust to second boundaries | 2026-10-08 | 077ee2e | .planning/quick/261008-j3c-make-the-flaky-test-teststaffactionnonstaffunchanged-tmute-a |
| 261008-j3d | Lockdown copy polish: no raw Telegram errors, warning markers, code-formatted help | 2026-10-08 | 924b5e6 | .planning/quick/261008-j3d-lockdown-copy-polish-from-planning-phases-04-manual-lockdown |

## Deferred Items

Items acknowledged and deferred at milestone close, most recent first:

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| *(none)* | | | | |

## Session Continuity

Last session: 2026-10-08T13:45:00.000Z
Stopped at: Phase 04 complete, ready to plan Phase 5
Resume file: None
