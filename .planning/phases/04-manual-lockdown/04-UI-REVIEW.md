# Phase 04 — UI Review

**Audited:** 2026-10-08
**Baseline:** Abstract 6-pillar standards, adapted to a Telegram bot (no UI-SPEC.md). Tone and format compared against `.planning/phases/03-staff-audit-and-undo/03-UI-REVIEW.md`.
**Screenshots:** Not captured. This is a Telegram bot with no browser UI; the "UI" is message text, inline buttons, callback alerts and the /staff panel row. Code-and-copy audit only.
**Interaction captures:** off (workflow.ui_interaction_capture is false)

---

## Scope

Phase 4 adds manual lockdown: `/lockdown`, `/unlockdown`, `/lockdownstatus`, the join guard and the lift tally. The user-facing surface is:

- Lock confirmation, refusals and already-active replies (`lockdown.go`)
- Lift confirmation, restore-failure and tally posts (`lockdown.go`, `lockdown_worker.go`)
- Status view (`lockdown_status.go`)
- Help text (`lockdown_help_msg`)
- Three cross-phase strings: `greetings_join_request_lockdown` (Accept alert), `mutes_unmute_lockdown_note` (/unmute reply), `staff_panel_row_lockdown` (/staff panel)
- 41 lockdown keys, present in all 7 locales (en, es, fr, hi, id, pt, ru), so locale parity holds.

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | Clear and specific copy, but `greetings_join_request_lockdown` tells admins to approve requests "after /unlockdown", which the code and the help text contradict (pending requests are cancelled at the lift). BLOCKER. |
| 2. Visuals | 3/4 | Consistent 🔒 / 🔓 / ⚠️ lead lines and a clear help layout; the status view is a flat stack of unlabeled lines. |
| 3. Color | 3/4 | Emoji is the only status signal. Failure and refusal lines carry no marker, unlike Phase 3's ✅ / ⏭ / ❌ vocabulary. |
| 4. Typography | 3/4 | Plain-text messages with mentions and `<code>` only where Phase 3 already used it. Lockdown help does not style command names, unlike other help pages. |
| 5. Spacing | 3/4 | Blank-line paragraphs in help and single-newline blocks in replies are readable. One sentence is broken mid-line in `lockdown_lock_unknown`, and capitalization differs between the panel row and the status line. |
| 6. Experience Design | 3/4 | Every refusal has a message, every live-check failure says "Nothing changed", and the anonymous-admin proof is covered. Gaps: the Accept alert sends admins to a dead end, and the lock-unknown reply is long. |

**Overall: 18/24**

---

## Top 3 Priority Fixes

1. **Accept alert says to approve pending requests after the lift, but they are cancelled** — Admins who tap Accept during a lockdown read "Approve it after /unlockdown, or from Telegram's own request list." The worker declines requests during the lockdown and `CancelPending` cancels any still pending at the lift (`lockdown_worker.go` ~line 480), and the help text says "people can ask again after the lift." The admin's recovery path leads nowhere. — **Fix:** change `greetings_join_request_lockdown` in all 7 locales to match the behavior, for example: "This group is in lockdown, so I can't approve join requests now. Requests are declined during a lockdown, and people can ask again after /unlockdown." Then run `make check-translations`.

2. **Lock-unknown reply is long and exposes raw Telegram detail** — `lockdown_lock_unknown` is three sentences plus a `{detail}` slot filled from `telegramErrorDetail` (raw Telegram description, HTML-escaped, up to `staffTelegramDetailRunes`). Group members see API wording such as a Telegram error description, and the admin has to parse four instructions. — **Fix:** keep the state in one line ("I couldn't confirm whether the lock took effect."), move the raw `{detail}` to a log line, and add one next step: "Send /lockdownstatus to check, or /unlockdown to restore the permissions now." The same `{detail}` pattern appears in `lockdown_lock_failed`, `lockdown_permissions_unreadable` and `lockdown_restore_failed`; apply the same rule to each.

3. **Status markers are inconsistent with Phase 3 and some outcomes have none** — Phase 3 used ✅ / ⏭ / ❌ per group. Phase 4 uses 🔒 (started, status, panel row), 🔓 (lifted), and ⚠️ (manual change), but `lockdown_lift_tally_failed` ("I couldn't unban {count}:"), `lockdown_restore_failed` and `lockdown_lift_record_failed` have no marker at all, so a failure reads the same as the routine tally. — **Fix:** prefix failure lines with ⚠️ (or ❌ to match Phase 3 per-group failures) and add the same marker to the restore-failed reply. Keep 🔒 / 🔓 for state changes only.

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)

**Strengths:**
- `lockdown_started`: "🔒 This group is in lockdown. Only admins can talk, and anyone who joins is removed until an admin runs /unlockdown." States the effect and the exit in one line.
- `lockdown_already_active`, `lockdown_not_active`, `lockdown_already_lifted` and `lockdown_lift_in_progress` each name who acted and when, so repeat taps are not confusing.
- Refusals name the cause and the next step: `lockdown_basic_group` ("Upgrade this group to a supergroup and run /lockdown again") and `lockdown_bot_cannot_restrict` ("I need to be an admin who can restrict members").
- `lockdown_check_failed` and `lockdown_state_failed` both say "Nothing changed", which tells the admin the state is safe to retry.
- `lockdown_help_msg` says the lockdown never ends on its own and that the 330-day ban limit exists, rather than hiding it.

**Issues:**
- **BLOCKER** — `greetings_join_request_lockdown` (en.yml line 1372): "Approve it after /unlockdown, or from Telegram's own request list." Conflicts with `lockdown_worker.go` (`CancelPending` at lift) and with `lockdown_help_msg` ("people can ask again after the lift"). An admin following it finds no request to approve.
- **WARNING** — `lockdown_lock_unknown` (en.yml line 2862) runs to three sentences and a `{detail}` slot. It also tells the admin "In about a minute I'll check the group's live permissions", a timing promise the bot makes only through its worker cycle.
- **WARNING** — `lockdown_status_permissions_unknown` and `lockdown_status_failed` use "right now" and "Try again in a moment." Both are fine on their own, but the Phase 3 reviewer already flagged technical wording; these two read less plainly than the lock-side refusals.
- **INFO** — Term drift: help says "locked", "lockdown" and "the lift"; the status line says "lifting"; the tally says "removed joiners". Settle on "lockdown" and "removed joiners" everywhere.

### Pillar 2: Visuals (3/4)

**Strengths:**
- The lead emoji signals the state at a glance: 🔒 for active, 🔓 for lifted, ⚠️ for a manual permission change (`lockdown_status_manual_change`).
- The help text has a clear structure: one purpose line, one bullet per command (× marker matching the rest of en.yml), then rule paragraphs.
- The lift tally puts the list of failed joiners after the translated lines through a token, so user-controlled names never shift the layout (`lockdown_worker.go` ~line 663).

**Issues:**
- **WARNING** — `lockdown_status_*` lines (`lockdown.go`/`lockdown_status.go`) are concatenated as separate lines with no labels, so "Joiners removed so far: 3" / "Joiners I could not remove: 1" / "Join requests declined: 0" read as a list without a header. The status view has no title, while the Phase 3 history view has "Recent staff actions".
- **INFO** — `lockdown_status_active` leads with 🔒 but `lockdown_status_failed` leads with nothing. A failed read looks like a normal status line with the wrong text.

### Pillar 3: Color (3/4)

Telegram has no colors, so status is carried by emoji and wording only.

**Strengths:**
- State changes have a marker: 🔒 on lock, 🔓 on lift, ⚠️ on a manual change in status and in the lift message.
- `staff_panel_row_lockdown` ("🔒 in lockdown since {since}") gives the /staff panel a single status marker per group.

**Issues:**
- **WARNING** — Failure and partial-failure lines have no marker: `lockdown_lift_tally_failed` ("I couldn't unban {count}:"), `lockdown_restore_failed`, `lockdown_lift_record_failed`. They look like plain text next to 🔓 and 🔒 lines. Phase 3 used ❌ and ⏭ on per-group lines.
- **INFO** — `lockdown_lift_tally` (routine success) has no marker, so the successful lift and its failure tally cannot be told apart by emoji.

### Pillar 4: Typography (3/4)

**Strengths:**
- Names are rendered as mentions through `lockdownMention`, not raw text, and reasons are capped with `lockdownCapRunes`.
- Dynamic text is HTML-escaped (`telegramErrorDetail` escapes the detail it returns).

**Issues:**
- **WARNING** — `lockdown_help_msg` puts command names and placeholders (`/lockdown [reason]`, `/unlockdown`, `/lockdownstatus`) in plain text. Other help pages in en.yml style argument placeholders with `<code>` (for example `/approve <code>...</code>` at line 84). The lockdown help therefore looks unfinished next to them.
- **INFO** — Phase 3 bolded section headers in its views. Phase 4's status view has no header, so there is no bold anchor for it.

### Pillar 5: Spacing (3/4)

**Strengths:**
- `lockdown_help_msg` separates its purpose line, command list and rule paragraphs with blank lines.
- Replies are joined with `\n` and trimmed (`strings.TrimSpace` in `lockdown.go`), so no stray blank lines appear at the ends.
- The tally keeps each failed joiner on its own line and caps the list, with a "…and N more" line (`lockdown_lift_tally_more`).

**Issues:**
- **WARNING** — `lockdown_lock_unknown` (en.yml line 2862) puts a hard line break inside the message and a second sentence after it. In a chat bubble the instruction is hard to find below the detail.
- **INFO** — Capitalization of the status line differs: "In lockdown since {since}" (`lockdown_status_active`) vs "in lockdown since {since}" (`staff_panel_row_lockdown`). Harmless on its own, but the same state is named twice with different case.

### Pillar 6: Experience Design (3/4)

**Strengths:**
- Every failure path has a reply: basic group, bot cannot restrict, bot rights check failed, permissions unreadable, lock refused, lock unknown, restore failed, record failed. None is silent.
- Anonymous admins get the proof prompt, and after proof the actor is checked live. The Accept alert refuses rather than approving while a lockdown is active or cannot be read.
- Repeat actions are answered as state, not errors: "already in lockdown", "already lifted", "lift in progress by {name}".
- The lift reports what it did: the restored permissions, the unbanned count, the kept count, the expired count, and failures with a list.

**Issues:**
- **BLOCKER** — the admin recovery path in the Accept alert is a dead end (see Pillar 1). This is the only place a lockdown tells an admin how to handle a request, and the instruction is wrong.
- **WARNING** — `/lockdown` applies immediately with no confirmation, while the other destructive commands in the bot (staff undo, staff actions) use a confirm card. The lockdown is reversible through `/unlockdown`, so this is a judgement call, but it is inconsistent with the staff flow and should be stated in the help text if kept.
- **WARNING** — The lock-unknown path asks the admin to wait "about a minute" and then check `/lockdownstatus` or `/unlockdown`. There is no message that says when the check has completed, so the admin has to poll.
- **INFO** — Joiners who are removed get no explanation from the bot (the guard ends the update with no welcome). This is by design per the phase decisions, and the lockdown_started message says that new joiners are removed.

---

## Consistency with Phase 3

- Tone matches: short imperative sentences, specific next steps, and "Nothing changed" wording on failed checks.
- Marker vocabulary does not match: Phase 3 used ✅ / ⏭ / ❌ per group; Phase 4 uses 🔒 / 🔓 / ⚠️ for states and no marker for failures.
- Phase 3 flagged a timestamp-locale issue and a marker-wording mismatch. Phase 4 does not reuse Phase 3's history markers (`↩`), so there is no conflict there.

---

## Files Audited

- `/home/user/Fuku_Robot/locales/en.yml` (lockdown keys at lines 1137, 1372, 2717, 2836–2884)
- `/home/user/Fuku_Robot/locales/es.yml`, `fr.yml`, `hi.yml`, `id.yml`, `pt.yml`, `ru.yml` (key-count parity only: 41 keys each, same as en)
- `/home/user/Fuku_Robot/alita/modules/lockdown.go` (start, refusal, lift and already-active replies; `lockdownText`; `telegramErrorDetail` use)
- `/home/user/Fuku_Robot/alita/modules/lockdown_status.go` (status view)
- `/home/user/Fuku_Robot/alita/modules/lockdown_worker.go` (lift tally, `CancelPending` at lift, failure list)
- `/home/user/Fuku_Robot/alita/modules/greetings.go` (Accept alert, ~line 972)
- `/home/user/Fuku_Robot/alita/modules/staff_action_run.go` (`telegramErrorDetail`)
- `/home/user/Fuku_Robot/.planning/phases/04-manual-lockdown/04-01-SUMMARY.md` through `04-08-SUMMARY.md` (deviations scanned; no UI deviations recorded)
- `/home/user/Fuku_Robot/.planning/phases/04-manual-lockdown/04-UAT.md` (UAT result: the permissions round trip passed on a real supergroup; no message-copy checks were in scope)
- `/home/user/Fuku_Robot/.planning/phases/03-staff-audit-and-undo/03-UI-REVIEW.md` (tone and marker baseline)

**Not verified:** rendered Telegram output (no client available). Message layout is judged from the source strings and `\n` joins, not from a screen.

---

## Orchestrator Verification (2026-10-08)

**Top fix 1 (the BLOCKER on `greetings_join_request_lockdown`) is a false positive and is withdrawn.**

- An approve card is posted only for a request that arrived **before** the lockdown, because during a lockdown the guard (group -7) ends every `chat_join_request` update, so `pendingJoins` posts no card.
- A request from before the lockdown has no joiner row, so the worker never declines it.
- `lockdown.CancelPending` (`alita/db/lockdown/joiners.go:160`) only sets the state of joiner rows recorded **during** the lockdown to cancelled in the database. It makes no Telegram call, so the pre-lockdown request is still pending in Telegram after the lift.
- "Approve it after /unlockdown, or from Telegram's own request list" is therefore accurate. Both routes were confirmed live in 04-UAT tests 5 and 8.
- The help text's "people can ask again after the lift" refers to requests that arrived during the lockdown, which are declined. It does not contradict the alert.

The Copywriting score stays 3/4 on the remaining findings, which are all below blocker level. Top fixes 2 and 3 and the WARNING items are non-blocking polish, left for a later copy pass.
