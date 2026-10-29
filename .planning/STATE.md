---
gsd_state_version: "1.0"
current_phase: 7
current_phase_name: AI-Assisted Raid Detection
status: planning
stopped_at: Phase 6 complete, ready to plan Phase 7
last_updated: "2026-10-29T15:40:47.600Z"
last_activity: 2026-10-29
last_activity_desc: Phase 6 complete, transitioned to Phase 7
state_head: 6654315d203a0f9d3a662cc659163893848197b2
progress:
  total_phases: 9
  completed_phases: 6
  total_plans: 67
  completed_plans: 67
  percent: 67
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-10-18)

**Core value:** My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.
**Current focus:** Phase 6 — Automatic Raid Detection

## Current Position

Phase: 7 — AI-Assisted Raid Detection
Plan: Not started
Status: Ready to plan
Last activity: 2026-10-29 — Phase 6 complete, transitioned to Phase 7

Progress: [███████░░░] 67%

## Performance Metrics

**Velocity:**
- Total plans completed: 67
- Average duration: -
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 10 | - | - |
| 02 | 10 | - | - |
| 03 | 12 | - | - |
| 04 | 8 | - | - |
| 5 | 14 | - | - |
| 6 | 13 | - | - |

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
- [Phase 5]: Alerts are planned once by the conditional `lockdown.BeginAlert` and posted, refreshed, reminded and closed by the lockdown worker from `chat_lockdown_alerts` rows. Phase 6's automatic triggers should call the same alert path with their own `trigger_kind` and counts, and may reuse the `alita:lockdown:recent:<chat>` 10-minute join tracker.
- [Phase 5]: Alert buttons (`lockdown|v1`) are stateless and judge every press by a live `getChatMember` of the locked chat; Lift goes through `liftLockdownRow`, shared with `/unlockdown`.
- [Phase 5]: "Ban N recent joiners" bans are permanent and resumable (`chat_lockdown_ban_runs`, result delivery since migration 20261011160000); the lift keeps them.
- [Phase 6]: Automatic triggers call `startLockdownRow`, the start path `/lockdown` uses; one lockdown and one alert come from the PostgreSQL index and conditional writes, never from the Redis latch. Settings live in `chat_raid_detection` (`raiddetection.GetSettings`, listed in `skipLocal`). Phase 7's AI-verdict trigger should add its own counter and setting beside the join and flood ones, and fire through `raidFire`.
- [Phase 6]: The flood counter is two watchers (group `-4` and group `12`) sharing `raidFloodCountUpdate`; group `12` must stay after every other handler group. The old `/antiraid` mode is a pointer command and `alita:antiraid:*` keys are swept at startup.

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
- [Cross-phase]: Some requirements are only partly checkable in their own phase, and a later phase finishes them. The `/staff` in-lockdown status (SETUP-08) shipped in Phase 4; the alert's Lift button (LOCK-06) and listing removed joiners on the alert (LOCK-01) shipped in Phase 5. The AI-trigger toggle (RAID-06) lands in Phase 7.
- [Phase 5]: Live Telegram UAT (Lift from both copies, real Ban N run, revoke refusal texts, hourly reminders) and `make test-postgres-integrity` in CI were accepted by the owner without being run here. Code review WR-01..WR-03 and IN-01..IN-09 were all fixed (`05-REVIEW-FIX.md`). By decision D-04/D-20 the alert text is not refreshed and a log channel with the admin category off gets no copy.
- [Quick 261008-j3b]: Shutdown follow-ups not fixed: `DrainAISpamChecks` waits up to 30 s but keeps the 10 s allowance; in polling mode the 60 s budget can still cut the staff drain short; `render.yaml`, `app.json`/Procfile and `railway.toml` keep their platforms' 30 s stop limits.
- [Quick 261008-j3a]: Follow-up candidate: an anonymous admin's ban-family command also gets a stray `chat_status_restrict_cmd_error` reply before the proof tap (pre-existing).
- [Phase 6]: The owner accepted the seven live checks without running them ("All good, continue"): a real join surge, a real flood, two replicas racing, `/autolockdown` with anonymous-admin proof, the retired `/antiraid`, and `TestRaidDetectionCarryOver` / `TestRaidDetectionSettings` through `make test-postgres-integrity` in CI. Code review WR-01..WR-03 and IN-01..IN-05 were all fixed (`06-REVIEW-FIX.md`).
- [Phase 6]: The six non-English translations of the new lockdown and antiraid texts have not had a native-speaker review. `lockdown_help_msg` is near Telegram's 4096-character limit in es (margin 44) and pt (margin 31); `TestLockdownHelpFitsOneMessage` guards it.
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

Last session: 2026-10-18T05:45:00.000Z
Stopped at: Phase 6 complete, ready to plan Phase 7
Resume file: None
