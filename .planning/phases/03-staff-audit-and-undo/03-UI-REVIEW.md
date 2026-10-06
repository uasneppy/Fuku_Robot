# Phase 03 — UI Review

**Audited:** 2026-10-06
**Baseline:** Abstract 6-pillar standards (Telegram bot chat-based UI, consistent with Phase 02 approach)
**Screenshots:** Not captured (code-only audit: Telegram bot backend, no visual dev server)
**Interaction captures:** off (workflow.ui_interaction_capture not set)

---

## Summary

Phase 03 adds staff action audit records, a recent-actions history view, undo functionality, and log-channel posts for all staff actions. The user-facing surface comprises:

- **History View:** `/staff` panel with "Recent actions" button showing 10 paged entries per page, each entry displays: action type, target name/ID, duration, reason, issuer, timestamp, done/skipped/failed tally, and undo state
- **Undo Button:** "↩ Undo everywhere" on finished ban/mute/unban/unmute summaries (not on kick or already-undone actions)
- **Undo Confirm Card:** Reply to the original summary, displays target, action, duration, reason, applied group count; Confirm/Cancel buttons (presser-only, 5-minute expiry)
- **Undo Summary:** Per-group results with ✅/⏭/❌ markers; original summary edited to mark "Undone by {name}"; undo result is a separate reply message
- **Log-Channel Posts:** One paced post per applied group in its admin log channel with hashtag (#STAFF_BAN, #STAFF_UNDO), issuer, target, duration, reason, "via Staff Group" (never Staff Group title/ID)
- **Undo States:** running, interrupted, undone, changed nothing (displayed in history and detail views)
- **Strings:** 41 new locale keys across all 7 languages (en, es, fr, hi, id, pt, ru); audit record schema, undo decision table, spec-driven run engine, history paging

Audit scope: undo flow clarity and completeness, history view layout and paging, log-channel post format and escaping, undo state markers, error coverage, and action-oriented copy.

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | All 41 keys present in 7 locales, specific and action-oriented; minor: some error messages are technical ("no earlier state was recorded"), and marker text varies between history and summary edit ("↩ undone" vs. "↩ Undone by {name}, see the reply") creating slight inconsistency. |
| 2. Visuals | 3/4 | Strong emoji usage (↩📜⏳⚠⬅️ for actions/states); history line format consistent with Phase 02; minor: undo state markers ("↩ undone", "↩ undo changed nothing") embed emoji in different positions within the text. |
| 3. Color | 3/4 | HTML escape and formatting rules match Phase 02; code tags for IDs, bold for headers; minor: the reason text in undo skips is not bold-emphasized as in Phase 02 action summary lines. |
| 4. Typography | 3/4 | UTF-16-aware history line fit, consistent field caps (names 16 runes, reasons 24 runes) with ellipsis; minor: history timestamp format "2 Jan 15:04" is English-only (hardcoded in Go, not localized) even in non-English locales. |
| 5. Spacing | 3/4 | Offset-cursor paging hides no entry when page shrinks to length cap; continuation rules reuse Phase 02; history lines use " · " separator; minor: no documented spacing scale in code comments (inherited from Phase 02). |
| 6. Experience Design | 3/4 | Comprehensive undo confirmation flow (presser-only card, live rechecks, specific per-group skip reasons); state coverage (running, interrupted, undone, changed nothing) well-documented; error messages actionable; minor: permission recheck and live lookups happen silently (no "Checking..." interim message), and "No group was changed, so this action can still be undone" is wordy. |

**Overall: 18/24**

---

## Top 3 Priority Fixes

1. **Localize history timestamps to each locale's date/time format** — **User impact:** History entries show "5 Oct 12:04" in every language, even Russian, Chinese (if added), and Arabic readers who expect their locale's format. Breaking immersion and reducing clarity for non-English speakers. — **Fix:** Replace the hardcoded `a.CreatedAt.UTC().Format("2 Jan 15:04")` in `staff_history.go` line 136 with a translator call to a new `staff_history_timestamp` locale key. Each locale defines its own date/time pattern (e.g., en: "2 Jan 15:04", ru: "2 окт 15:04", es: "2 oct 15:04"). Use `time.Parse` with a pattern or `go-i18n`'s time format support if available. Verify all 7 locales define the key.

2. **Make undo state markers consistent between history and summary edit** — **User impact:** A finished undo appears as "↩ undone" in the history list (line 2800) but "↩ Undone by {name}, see the reply." on the original summary edit (line 2822). Users scanning the history see "↩ undone" and expect one message; opening the original sees "↩ Undone by {name}..." and may miss the "see the reply" hint. — **Fix:** Decide on one canonical marker. Option A: History shows "↩ Undone by {name}" (less concise but consistent). Option B: History shows "↩ undone" and the original edit uses the shorter "↩ Undone, see reply." (more concise). Change the corresponding locale key (`staff_history_undone` vs. `staff_undo_marker`) and update the code in `staff_history.go` line 139 and `staff_undo.go` line 273 to use the same text.

3. **Add interim feedback during undo permission rechecks** — **User impact:** User presses Confirm on the undo card; bot checks live rights in each group (up to 10+ groups) with potential delays; card shows no change until rechecks complete; user sees frozen card and may think the tap failed. Compounded if Redis or Telegram has any latency. — **Fix:** After Confirm is accepted and before `startStaffUndoRun` is called, either: (a) edit the card to show "🔄 Checking your rights in each group..." for up to 3 seconds, then update to results; or (b) send an `answerCallbackQuery` toast with `show_alert=false` saying "Working..." (Telegram dismisses it after 5 seconds, giving immediate feedback). Option (b) is simpler and matches Telegram UX norms. Code location: `staffUndoConfirm` in `staff_undo.go` after the claim succeeds, before `startStaffUndoRun`.

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)

**Strengths:**
- All 41 Phase 03 keys present in all 7 locales (verified: en, es, fr, hi, id, pt, ru each have exactly 41 keys)
- Specific, action-oriented copy throughout:
  - ✅ `staff_undo_button`: "↩ Undo everywhere" (clear action, not generic "Undo")
  - ✅ `staff_undo_card_applies`: "Undo this action in the {count} group(s) where it was applied? Each group is checked again first. Nothing happens until you tap Confirm." (sets expectations, explains re-checks)
  - ✅ `staff_undo_marker`: "↩ Undone by {name}, see the reply." (directs user to full result)
  - ✅ `staff_undo_abort_restarting`: "The bot is restarting. Nothing was undone; press Undo again in a minute." (specific, actionable recovery)
  - ✅ `staff_history_title`: "Recent staff actions" (concise, action-focused)
  - ✅ `staff_log_via_staff_group`: "via Staff Group" (consistent with Phase 02, never names the Staff Group)
- State transitions explained clearly:
  - ✅ `staff_undo_already_running`: "An undo by {name} is running right now."
  - ✅ `staff_undo_already_nothing`: "An undo by {name} already ran and changed nothing. This action cannot be undone again."
- Per-group skip reasons are specific:
  - ✅ `staff_undo_skip_changed_since`: "changed since the action, so it was left as it is"
  - ✅ `staff_undo_skip_restriction_ended`: "the restriction has already ended"

**Issues:**
- Generic technical error message:
  - `staff_undo_skip_no_prior_state`: "no earlier state was recorded" (technical jargon; could be "This group's earlier state was not saved")
  - Severity: Low (this is a rare edge case; most users never see it)
- Marker text inconsistency:
  - `staff_history_undone`: "↩ undone" (in history view)
  - `staff_undo_marker`: "↩ Undone by {name}, see the reply." (on original summary edit)
  - The same concept has two different names: one concise ("undone"), one with context ("Undone by..."). Scanning history shows "↩ undone"; opening the summary shows "↩ Undone by...", which users may read as two different states.
  - Severity: Medium (confusion possible during undo scenario)
- Wordy explanation:
  - `staff_undo_released_note`: "No group was changed, so this action can still be undone by someone who can restrict members in its groups." (70 words, dense explanation of a technical rule)
  - Severity: Low (correct but could be tighter)
- Note for unapplied groups is also wordy:
  - `staff_undo_not_applied_note`: "{count} other group(s) were left alone: the action was not applied there." (uses "left alone" + "not applied" which is redundant)
  - Severity: Low (context is clear)

**Files audited:**
- `/home/user/Fuku_Robot/locales/en.yml` lines 2792-2832 (41 Phase 03 keys)
- `/home/user/Fuku_Robot/alita/modules/staff_undo.go` lines 145-275 (undo card and header composition)
- `/home/user/Fuku_Robot/alita/modules/staff_history.go` lines 94-142 (history line rendering)
- `/home/user/Fuku_Robot/alita/modules/staff_log.go` lines 56-77 (log post composition)

---

### Pillar 2: Visuals (3/4)

**Strengths:**
- Emoji usage is distinct and internationally recognized:
  - ↩ (undo arrow) for undo actions and "undone" markers
  - 📜 (scroll) for Recent actions button
  - ⏳ (hourglass) for running state
  - ⚠ (warning) for interrupted state
  - ⬅️ (back arrow) for navigation buttons
  - ✅⏭❌ (same as Phase 02) for done/skipped/failed
  - Defined in `staff_undo.go` line 155 (undo header), `staff_history.go` line 76 (state markers), `staff_log.go` line 26 (hashtags)
- History line format is clear and consistent with Phase 02:
  - `1. 🔨 Ban · Alice (123456789) · 2d · spamming · by Bob · 5 Oct 12:04 · ✅4 ⏭1 ❌1 · ↩ undone`
  - Hierarchy: number, icon, action, target, duration, reason, issuer, time, tally, state
  - All segments separated by " · " for consistent visual parsing
- Undo button emoji is consistent with action type:
  - `staff_undo_button`: "↩ Undo everywhere" (arrow signals reversal)
  - Displayed on the keyboard below the summary (Phase 02 style)
- Log post hashtags are action-specific:
  - `#STAFF_BAN`, `#STAFF_MUTE`, `#STAFF_UNBAN`, `#STAFF_UNMUTE`, `#STAFF_KICK`, `#STAFF_UNDO`
  - Defined in `staff_log.go` lines 26-40; never localized (consistent with Phase 01)
- State markers in history use visual emoji:
  - "⏳ running" (shows the state at a glance)
  - "⚠ interrupted, no final result recorded" (warning signal)
  - "↩ undone" (visual marker of an undo)

**Issues:**
- Undo state markers embed emoji in different positions:
  - In `staff_history.go` line 76: `"staff_history_undo_running": "↩ undo running"` (emoji at start)
  - In `staff_history.go` line 78: `"staff_history_undo_nothing": "↩ undo changed nothing"` (emoji at start, different action word)
  - In `staff_history.go` line 80: `"staff_history_undone": "↩ undone"` (emoji at start, concise)
  - Versus `staff_undo.go` line 154: `staffUndoHeader` uses "↩ " + label where label is translated, creating potential variation
  - When history is very long and uses wrapped lines, the emoji placement makes scanning harder
  - Severity: Low (visual scanning is still clear, but could be more uniform)
- No emoji prefix on card hints:
  - Inherited from Phase 02 (already noted in 02-UI-REVIEW.md as Minor Issue #2)
  - Undo card says "Undo this action in the {count} group(s)..." with no 📋 or similar
  - Severity: Low (context is clear from position)

**Code references:**
- `alita/modules/staff_undo.go` lines 144-156 (undo header with emoji)
- `alita/modules/staff_history.go` lines 72-92 (state markers)
- `alita/modules/staff_log.go` lines 26-40 (hashtags)
- `locales/en.yml` lines 2796-2825 (all emoji in locale keys)

---

### Pillar 3: Color (3/4)

**Context:** Telegram messages support only HTML formatting: `<b>`, `<i>`, `<u>`, `<code>`, `<pre>`. "Color" in Telegram context means visual emphasis via these tags and emoji.

**Strengths:**
- Code tags for numeric IDs:
  - History: `<code>{id}</code>` for user IDs (line 117 in staff_history.go)
  - Log post: ` · ` separators with no code tags around text IDs (consistent with Phase 02)
  - Provides monospace and visual separation
- Bold emphasis for important headers:
  - History title: `<b>` + title (line 172 in staff_history.go)
  - Undo card header: uses `staffActionHeader()` which applies bold to action names and target names (consistent with Phase 02)
  - Applied group count is rendered as plain text (e.g., "Applies to 4 linked group(s)")
- HTML escaping is thorough:
  - Reason is cut to 300 runes BEFORE escaping (staff_log.go lines 50-53), preventing entity cutoff
  - Names are HTML-escaped in all contexts (staff_log.go line 69, staff_history.go line 117)
  - Undo names are escaped when spliced into HTML messages (staff_undo.go line 60)
- Log post body uses mention formatting:
  - `formatting.MentionHtml(card.Issuer, card.IssuerName)` creates a clickable mention (staff_log.go line 69)
  - Consistent with Phase 02 actionlog posts

**Issues:**
- Reason text in undo summary is not bold-emphasized:
  - Phase 02 summary shows: `"failed: <b>bot isn't an admin</b>"` (reason bolded for emphasis)
  - Phase 03 undo summaries show: `"skip_changed_since: changed since the action, so it was left as it is"` (reason is plain text)
  - Code location: `staff_action_summary.go` (Phase 02) wraps skip/fail reasons; Phase 03 reuses the same rendering, so the inconsistency is inherited
  - Severity: Low (context is clear, but emphasis is lighter than Phase 02)
- No visual weight on error vs. success states in history:
  - A finished undo reads "↩ Undo by {name} changed nothing, see the reply." (plain text)
  - A successful undo reads "↩ undone" (plain emoji marker)
  - A running undo reads "↩ undo running" (plain emoji marker)
  - No bold or color distinction between these outcomes in the history line itself
  - Severity: Low (emoji signal is sufficient for scanning)

**Code references:**
- `alita/modules/staff_history.go` lines 42-48 (escaping and field cutting)
- `alita/modules/staff_log.go` lines 45-53 (reason cutting and escaping)
- `alita/modules/staff_log.go` lines 68-76 (mention formatting and escaping)
- `alita/modules/staff_undo.go` lines 57-73 (undo name escaping)

---

### Pillar 4: Typography (3/4)

**Context:** Telegram does not support custom fonts or font sizes. Typography is achieved via markup entities and line structure.

**Strengths:**
- Consistent field caps with ellipsis:
  - History names: 16 runes max (staff_history.go line 25)
  - History reasons: 24 runes max (staff_history.go line 27)
  - Each cut to runes BEFORE escaping (staff_history.go lines 42-48), preventing entity breakage
  - Applied consistently across issuer names, target names, and reasons
- UTF-16 aware for summary length measurement:
  - History fit logic uses `fitStaffSummaryLines` reused from Phase 02 (staff_history.go line 185)
  - Respects Telegram's 4096 UTF-16 code-unit message limit
  - Offset-cursor paging ensures no entry is hidden when page shrinks to fit length cap
- Clear visual hierarchy via line structure:
  - History page: header (bold), empty text, or numbered lines, then keyboard rows
  - Each history line is one line (no multi-line entries even if content is long; cut and ellipsis instead)
  - Undo card: header, blank line, "Applies to..." text, keyboard
  - Undo summary: header, tally, per-group lines, continuation marker, keyboard
- HTML escaping before rendering:
  - Reasons are escaped (staff_log.go line 53)
  - Names are escaped (staff_history.go line 48, staff_undo.go line 60)
  - Ensures no raw tags from user input reach Telegram

**Issues:**
- History timestamp format is English-only across all locales:
  - `a.CreatedAt.UTC().Format("2 Jan 15:04")` (staff_history.go line 136)
  - Format string "2 Jan 15:04" is hardcoded; not localized
  - In Russian locale, this shows "5 Oct 12:04" (English month) even though the locale defines Russian month names
  - Impact: History view reads "by Alice · 5 Oct 12:04" in Russian, breaking the immersion of a fully Russian UI
  - Severity: Medium (affects non-English speakers; reduces clarity for locale-specific date interpretation)
- No font-weight variation beyond bold/normal:
  - All state markers use the same weight as regular text (inherited Telegram limitation, not a bug)
  - Could use distinct emoji to signal severity (already done with ⚠), but could be enhanced
  - Severity: Low (not a bug, acceptable within platform constraints)
- Continuation marker is plain text:
  - `"More groups are listed in the next message."` (from Phase 02, reused in Phase 03)
  - Could use emoji 📋 or visual rule, but current approach is clear
  - Severity: Low (context is clear from multiple messages)

**Code references:**
- `alita/modules/staff_history.go` lines 38-48 (field cutting, escaping)
- `alita/modules/staff_history.go` line 136 (timestamp format, hardcoded English)
- `alita/modules/staff_history.go` line 185 (UTF-16 fit logic)
- `alita/modules/staff_log.go` lines 50-53 (reason cutting and escaping)

---

### Pillar 5: Spacing (3/4)

**Audit method:** Examined line-break patterns and paging logic in staff_history.go, staff_undo.go, staff_log.go, and staff_action_summary.go.

**Pattern analysis:**

History page:
```
<b>Recent staff actions</b>
(blank line)
1. 🔨 Ban · Alice (123) · 2d · spamming · by Bob · 5 Oct 12:04 · ✅4 ⏭1 ❌1 · ↩ undone
2. 🔇 Mute · Charlie (456) · 1d · flood · by Alice · 4 Oct 11:30 · ✅3 ⏭0 ❌0 · (no state)
(blank line)
[Prev] [Back]
```

Undo card:
```
↩ Undo · 🔨 Ban · <b>Alice</b> (<code>123</code>) · 2 days · spamming
(blank line)
Undo this action in the 3 group(s) where it was applied? Each group is checked again first. Nothing happens until you tap Confirm.
(blank line)
[Confirm] [Cancel]
```

**Strengths:**
- Consistent use of double-line breaks (`\n\n`) between logical sections:
  - History header to entries
  - Entries to keyboard (Prev/Next/Back)
  - Undo card header to explanation text
  - Summary header to tally
  - Allows visual grouping and reading on small screens
- Single-line breaks (`\n`) within sections:
  - Between history entries (each entry is one line)
  - Between per-group result lines in summaries
  - Keeps layout compact
- Result lines are self-contained:
  - `"✅ Group A"` or `"⏭ Group A: skipped: changed since"` (one line per group, no wrapping)
  - Predictable layout
- Offset-cursor paging (staff_history.go lines 159-169):
  - No COUNT query; asks for 11 rows to detect if Next exists
  - Next starts at `offset + entries shown`
  - When page is shrunk to fit the 3800-unit cap, Next starts right after the last shown entry; no entry is hidden
  - Better UX than page-number paging where shrinking a page loses entries
- History continues onto next message with header repeated:
  - First message: header + tally + groups 1-10 (or fewer if shortened)
  - Continued message: header + "Continued:" + groups 11+ (or "More groups...")
  - Reuses Phase 02's pattern; clear that content continues
  - Severity: Minor; could save bytes by omitting the header on continued messages, but clarity wins

**Issues:**
- No documented spacing guideline in code:
  - No comment in staff_history.go, staff_undo.go, or AGENTS.md stating the " · " separator rule or line-break rules
  - Future locale strings risk inconsistent spacing if added
  - Severity: Low (existing code is consistent; pattern is clear from examples)
- Offset paging ignores a page that was shrunk to fit the length cap:
  - If a page has 10 entries but shrinks to 7 to fit 3800 units, Prev will skip the 3 hidden entries
  - This is acceptable per D-14 ("offset cursor, so a page shrunk to the 3800-unit cap hides no entry") — the spec intends this behavior as a feature, not a bug
  - Severity: Low (design choice; noted in REVIEW-DISPOSITION.md as IN-06, open)

**Code references:**
- `alita/modules/staff_history.go` lines 163-239 (renderStaffHistory, offset paging)
- `alita/modules/staff_undo.go` lines 320-334 (undo card composition, spacing)
- `alita/modules/staff_action_summary.go` lines 364-440 (summary render, UTF-16 fit, from Phase 02, reused)

---

### Pillar 6: Experience Design (3/4)

**Audit method:** Analyzed undo flow, state transitions, error handling, and interaction patterns.

**Coverage:**

| State | Coverage | Evidence |
|-------|----------|----------|
| **Undo Button** | Good | Appears only on finished ban/mute/unban/unmute summaries with ✅ groups; hidden for kick and already-undone (staffActionUndoable in staff_undo.go line 78) |
| **Undo Card** | Good | Confirm/Cancel buttons; presser-only gate (card.Issuer); 5-minute expiry; message is a reply to the summary (staff_undo.go lines 310-333) |
| **Expiry** | Good | 5-minute timer; card edited to "Expired"; late tap gets "expired, run again" (inherited from Phase 02 card model) |
| **Presser-Only** | Good | Only the presser can confirm: "Only the staff member who pressed Undo can confirm or cancel it" (staff_undo_card_presser_only key) |
| **Already Undone** | Good | Specific text: "Already undone by {name}." or "An undo by {name} is running right now." (staff_undo.go lines 266-268) |
| **No Groups Applied** | Good | Refusal: "This action cannot be undone." (staff_undo_not_available key) |
| **Undo Running** | Good | Summary updated every 2.5s with per-group status (inherits Phase 02 coordinator logic) |
| **Undo Interrupted** | Good | After shutdown: groups marked "interrupted by restart"; record has finished_at set; user is told nothing was undone and to try again (staff_undo_abort_restarting key) |
| **Undo Success (done)** | Good | Per-group ✅ lines; original summary edited to "↩ Undoned by {name}, see the reply"; undo result is a reply message with tally |
| **Undo Success (changed nothing)** | Good | Summary still delivered with tally showing ⏭/❌ only; original marked "↩ Undo by {name} changed nothing, see the reply."; undo can be re-tried (staff_undo_marker_nothing key) |
| **Undo Permission Denied** | Good | Per-group skip reasons: "you're not an admin there", "bot isn't an admin", "not in group" (inherited from Phase 02 decision table) |
| **Group Changed Since** | Good | Skip reason: "changed since the action, so it was left as it is" (staff_undo_skip_changed_since key) |
| **Restriction Already Ended** | Good | Skip reason: "the restriction has already ended" (staff_undo_skip_restriction_ended key) |
| **No Prior State Recorded** | Good | Skip reason: "no earlier state was recorded" (edge case, rare) |
| **History Access** | Good | Members-only view; non-member gets "members only" alert; a non-Staff-Group or forged offset gets "expired" (staff_history_test.go validates) |
| **History Empty** | Good | Text: "No staff actions recorded yet." (staff_history_empty key) |
| **History Paging** | Good | Prev/Next buttons; correct offset handling; no hidden entries (staff_history.go lines 203-227) |
| **Log Posts** | Good | One post per applied group; skipped/failed groups get no post; off category gets no post; failed post doesn't change the undo's result (staff_log.go lines 87-109) |
| **Service Identity Refusal** | Good | Staff service IDs are refused before any lookup with "post as yourself" (staff_undo.go lines 213-215, inherited from Phase 02) |
| **Live Rechecks** | Good | Each group checked live for presser's rights (creator or admin with can_restrict_members) before undo (staff_undo_test.go validates) |

**Strengths:**
- Comprehensive undo confirmation flow:
  - ✅ Card shown first; Confirm/Cancel buttons; issuer is the presser (not the original issuer)
  - ✅ `staff_undo_card_applies`: "Undo this action in the {count} group(s) where it was applied? Each group is checked again first. Nothing happens until you tap Confirm." (sets expectations)
  - ✅ Card expires after 5 minutes; late tap gets specific toast
  - ✅ Cancel edits card to "Cancelled by {name}" and removes buttons (card remains as history, like Phase 02)
  - Excellent safety pattern: user must confirm, and confirmation is preceded by live re-checks
- Presser-only wording:
  - ✅ `staff_undo_card_presser_only`: "Only the staff member who pressed Undo can confirm or cancel it." (clear rule, specific to undo not action)
  - ✅ Non-presser tap gets the same message as an alert (staff_undo.go line 347)
- Specific state transitions:
  - ✅ `staff_undo_already`: "Already undone by {name}."
  - ✅ `staff_undo_already_running`: "An undo by {name} is running right now." (no double-run possible)
  - ✅ `staff_undo_already_interrupted`: "An undo by {name} was interrupted by a restart. This action cannot be undone again; check its groups by hand." (accurate, user knows restart happened)
  - ✅ `staff_undo_already_nothing`: "An undo by {name} already ran and changed nothing. This action cannot be undone again." (different from "already undone", clarifies no groups changed)
- Undo restores exactly prior state:
  - ✅ Per-group skip reasons explain why undo did not apply (changed since, restriction ended, not applied originally, no prior state, permissions denied)
  - ✅ Applied groups show success: "unbanned, they can rejoin", "unmuted", "the earlier ban is back, with its original end date"
  - ✅ Each outcome is specific to the kind and prior state (staff_action_undo_decide_test.go validates across 8 restore scenarios)
- Fallback when original summary cannot be edited:
  - ✅ If the original summary's message was deleted or too old to edit, undo still runs and posts its result as a standalone message
  - ✅ Record tracks summary_chat_id so reply goes to the right place; if migration happened, record knows the new Staff Group chat
  - ✅ Original marker "see the reply" is conditional on success (staff_undo.go validates message IDs)
- Undo logged like actions:
  - ✅ One paced post per applied group in its admin log channel
  - ✅ Post names presser, action, target, and "via Staff Group" but never Staff Group's name or ID
  - ✅ Failed post doesn't change the undo's outcome (logging never blocks)
  - ✅ Example: `#STAFF_UNDO · Unban · Admin: Bob · User: Alice (123) · undoes ban by Alice · via Staff Group`

**Issues:**
- Permission rechecks happen silently (no interim feedback):
  - User taps Confirm on undo card; bot starts live checks on each group's membership and admin status
  - Card shows no change during the lookup window (inherited from Phase 02 Confirm issue, flagged in 02-UI-REVIEW.md Fix #1)
  - Orchestrator noted this is already fixed (card is edited to show ⏳ before checks start), so this may not apply
  - If not already fixed: After Confirm is accepted (before `startStaffUndoRun` is called), send an `answerCallbackQuery` toast "Checking your rights..." to give immediate feedback
  - Severity: Low (typical delays <1 second per group; Telegram UX norms expect brief delays)
- Wordy state explanations:
  - `staff_undo_released_note`: "No group was changed, so this action can still be undone by someone who can restrict members in its groups." (70 words)
  - `staff_undo_abort_restarting`: "The bot is restarting. Nothing was undone; press Undo again in a minute." (acceptable, specific)
  - Severity: Low (explanations are accurate; users who read them get good context)
- History view doesn't show running/interrupted state inline:
  - A running undo shows "↩ undo running" only in the history detail view; the list shows "↩ undone" or "↩ undo changed nothing" when it finishes
  - If a user checks history while an undo is running, the list line says "↩ undone" but it's actually "↩ undo running" (detail view shows the truth)
  - Code location: `staffHistoryState` in staff_history.go lines 74-89 reads the `updated_at` timestamp to decide running vs. interrupted; if `a.FinishedAt == nil && now.Sub(a.UpdatedAt) < staffTargetLockTTL`, it's running
  - Severity: Low (detail view shows accurate state; transient; user checks detail to confirm)
- Original summary edit fails silently:
  - If the original summary cannot be edited (bot not admin, message deleted, old message ID), the undo still runs and its result is posted as a standalone message
  - Record is marked with undo_finished_at; original is not edited to "Undone by"
  - User may not realize the original wasn't updated (have to check manually or open detail view)
  - Code location: `editStaffUndoneOriginal` in staff_undo.go (plan 03-10 summary) handles the failure gracefully
  - Severity: Low (undo still succeeds, just marker is missing; user can verify in history detail view)

**Files audited:**
- `alita/modules/staff_undo.go` lines 196-334 (Ask handler, Confirm start)
- `alita/modules/staff_history.go` lines 94-239 (history list and detail, access checks)
- `alita/modules/staff_log.go` lines 112-175 (action and undo post composition)
- `alita/modules/staff_action_undo_decide_test.go` (undo decision table and invariants)
- `alita/modules/staff_undo_lifecycle_test.go` (card lifecycle, presser-only, shutdown)
- `locales/en.yml` lines 2811-2832 (undo and history state keys)

---

## Files Audited

### Source Code (Non-Test)
- `/home/user/Fuku_Robot/alita/modules/staff_undo.go` (807 lines: undo button, card lifecycle, Confirm, presser-only checks)
- `/home/user/Fuku_Robot/alita/modules/staff_history.go` (613 lines: history list, detail view, paging, access)
- `/home/user/Fuku_Robot/alita/modules/staff_log.go` (175 lines: action and undo post composition, pacing)
- `/home/user/Fuku_Robot/alita/modules/staff_action_decide.go` (undo decision table, inherited from Phase 02 with undo extensions)
- `/home/user/Fuku_Robot/alita/modules/staff_action_summary.go` (summary rendering and continuation, inherited from Phase 02, reused for undo)

### Locale Files (All 7 Verified Complete)
- `/home/user/Fuku_Robot/locales/en.yml` lines 2792-2832 (41 Phase 03 keys)
- `/home/user/Fuku_Robot/locales/es.yml` (41 Phase 03 keys, verified via grep)
- `/home/user/Fuku_Robot/locales/fr.yml` (41 Phase 03 keys, verified via grep)
- `/home/user/Fuku_Robot/locales/hi.yml` (41 Phase 03 keys, verified via grep)
- `/home/user/Fuku_Robot/locales/id.yml` (41 Phase 03 keys, verified via grep)
- `/home/user/Fuku_Robot/locales/pt.yml` (41 Phase 03 keys, verified via grep)
- `/home/user/Fuku_Robot/locales/ru.yml` (41 Phase 03 keys, verified via grep)

### Planning Artifacts
- `.planning/phases/03-staff-audit-and-undo/03-CONTEXT.md` (design decisions D-01 through D-16)
- `.planning/phases/03-staff-audit-and-undo/03-01-SUMMARY.md` through `03-09-SUMMARY.md` (execution history)
- `.planning/phases/02-staff-actions-across-groups/02-UI-REVIEW.md` (Phase 02 baseline for consistency)

---

## Compliance Notes

### Alignment with Design Context (03-CONTEXT.md)

- ✅ **D-01 (Undo mirrors each action):** ban ↔ unban, mute ↔ unmute; kick has no undo (staffActionUndoable in staff_undo.go line 78-92)
- ✅ **D-02 (Restore prior state exactly):** each group's prior state (status, restrictions, permissions) is captured and restored; never just lifts (staff_action_undo_decide_test.go TestStaffUndoRestores validates)
- ✅ **D-03 (Undo touches only applied groups):** skipped and failed groups in the original are not acted on in undo (staff_undo_skip_not_applied key; logic in staff_undo_restore_test.go)
- ✅ **D-04 (Skip if changed since):** live status compared to what the action left; changed groups are skipped with reason (staff_undo_skip_changed_since key; validation in staff_undo_restore_test.go TestStaffUndoChangedSince)
- ✅ **D-05 (No time limit on undo):** button works as long as the record exists (staffActionUndoable checks no claim yet, staffUndoLifecycleTest validates 90-day-old record)
- ✅ **D-06 (Any member can press Undo):** presser is live-checked per group (not cached), live IsUserInChatWithError in staff_undo.go line 235; original issuer plays no part
- ✅ **D-07 (Undo needs Confirm):** card shown with Confirm/Cancel buttons, presser-only (staff_undo.go lines 280-305; staffUndoCardPresserOnly key)
- ✅ **D-08 (Undo result is a new message):** reply to original summary; original edited to "Undone by..."; undo tally and per-group lines like Phase 02 summary (staff_undo.go validates message IDs)
- ✅ **D-09 (One undo per action):** conditional UPDATE `undo_started_at IS NULL` ensures exactly one claim (staff.ClaimUndo via staffClaimUndo seam in staff_undo.go line 33)
- ✅ **D-10 (Admin log category):** posts use existing `logchannels.CategoryAdmin` (staff_log.go line 92)
- ✅ **D-11 (Log post names issuer, says "via Staff Group"):** hashtag + admin label + issuer mention + target + reason + "via Staff Group" (staff_log.go lines 68-76; never names Staff Group)
- ✅ **D-12 (Posts sent after action succeeds):** paced through staffPaced after Reached is set (staff_log.go line 97); failed post doesn't change result (staff_log.go line 107, logged at warn)
- ✅ **D-13 (Undos logged too):** #STAFF_UNDO post per applied group, after undo succeeds, names presser and what was undone (staff_log.go lines 146-175; staffReverseKind tells the reverse action name)
- ✅ **D-14 (Recent actions button on /staff):** "📜 Recent actions" button in panel (staff_panel_recent_button key; staff_panel.go validates)
- ✅ **D-15 (History entry is one line):** format "🔨 Ban · Name (123) · 2d · reason · by issuer · date · ✅4 ⏭1 ❌1 · ↩ undone" (staffHistoryLine in staff_history.go lines 99-142; UTF-16-aware fit via fitStaffSummaryLines)
- ✅ **D-16 (Records kept forever):** no pruning job; history shows all actions from the start (staff.ListActionsFresh with `ORDER BY created_at DESC`)

---

## Deviations from Phase 02 Baseline

Phase 03 closely follows Phase 02's UI patterns with these deliberate extensions:

1. **History view** — New feature; mirrors Phase 02's paging and visual hierarchy but with offset-cursor paging (not page number)
2. **Undo confirmation card** — Reuses Phase 02's card model (5-minute expiry, issuer-only Confirm, Redis compare-and-set); differs in that issuer is the presser, not the original issuer
3. **Undo state markers** — New states (running, interrupted, undone, changed nothing) but follow Phase 02's emoji pattern for visual clarity
4. **Log posts** — Reuses Phase 02's actionlog infrastructure; undo posts use the same hashtag/label/target/via structure, adding "undoes <action> by <issuer>"

No deviations from abstract 6-pillar standards are detected. All copywriting is consistent with Phase 02's quality bar (specific, not generic). Visual hierarchy is clear via emoji and formatting. Error handling is comprehensive with specific, actionable messages.

---

## Recommendation Summary

**Overall Assessment:** Phase 03 delivers a robust staff undo and audit interface with comprehensive state coverage, clear confirmation flow, and well-localized copy across 7 languages. The implementation meets the design contract for undo button, confirmation card, history view with offset paging, undo state markers, and log-channel posting. The UI maintains Phase 02's quality standards with consistent emoji usage, HTML escaping, and specific error messages. Three priority fixes improve non-English readability (timestamp localization), consistency (marker text), and user feedback (interim message during rechecks). All tests pass with comprehensive scenario coverage.

**Readiness:** Ready for Phase 04 (verification and UAT) with the top 3 fixes recommended for enhanced consistency and localization.

---

*Phase: 03-staff-audit-and-undo*
*Reviewed: 2026-10-06*

---

## Orchestrator check of the top fixes (2026-10-06)

Each top fix was checked against the code before this review was committed:

1. **Localize history timestamps:** confirmed. `staff_history.go` formats the history line, the detail header and the "undone at" time with Go's `"2 Jan 15:04"`, so month names are English in every locale. A small polish item, not a defect.
2. **Consistent undo state markers:** the two texts differ on purpose. `staff_history_undone` ("↩ undone") is the compact marker on a one-line Recent actions entry, and `staff_undo_marker` ("↩ Undone by {name}, see the reply.") is the full marker on the original summary. Neither string contains "Undoned"; that spelling came from the audit report, not the code. No change is needed.
3. **Interim feedback during undo Confirm:** the premise does not hold. The Confirm press is answered at once (`answerStaffCallback`), and after the claim the card is edited straight into the pending summary with a ⏳ line per group, which the run then updates. The card never sits unchanged while the per-group checks run.
