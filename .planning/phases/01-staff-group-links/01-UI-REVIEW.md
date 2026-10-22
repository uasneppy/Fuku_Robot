# Phase 01 — UI Review

**Audited:** 2026-10-05
**Baseline:** Abstract 6-pillar standards (no UI-SPEC; Telegram bot chat-based UI)
**Screenshots:** Not captured (code-only audit: no dev server, Telegram bot backend)
**Interaction captures:** off (workflow.ui_interaction_capture not set)

---

## Summary

Phase 01 implements the staff group linking system for a Telegram bot. The user-facing surface comprises:
- **Commands:** `/setstaff`, `/unsetstaff`, `/linkstaff`, `/unlinkstaff`, `/staff`
- **Messages:** HTML-formatted Telegram messages with inline keyboard buttons
- **Strings:** 63 locale keys across all 7 languages (en, es, fr, hi, id, pt, ru)
- **Panel:** Live group status display with pagination (8 per page, max 3800 UTF-16), refresh and link management

Audit scope: copywriting, button labels, error messages, visual hierarchy via emoji and HTML formatting, message spacing, and interaction patterns (confirmations, state coverage, live permission checks).

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | Specific, action-oriented copy with clear error reasons; all 7 locales complete; minor: generic "Problem:" prefix in panel error lines. |
| 2. Visuals | 2/4 | Good emoji usage for status and most actions; Unlink button lacks icon (text-only), inconsistent with Add/Refresh/Paging patterns; × bullets in help may not render consistently. |
| 3. Color | 3/4 | Proper use of `<code>` for chat IDs and `<b>` for emphasis; error/warning messages lack visual weight; emoji provide character-based distinction. |
| 4. Typography | 3/4 | Consistent use of code blocks, bold, italic; appropriate message hierarchy; Telegram limitation: no font sizes; hint swap for length cap is reasonable. |
| 5. Spacing | 3/4 | Consistent double-break (`\n\n`) for sections, single-break (`\n`) for subsections; minor inconsistencies between help and header spacing. |
| 6. Experience Design | 3/4 | Good error handling with specific reasons; confirmations for destructive actions; live authority re-checks on every button; missing: no loading indicator for 45s panel build; removed links disappear silently. |

**Overall: 17/24**

---

## Top 3 Priority Fixes

1. **Add emoji icon to Unlink button to match action pattern** — **User impact:** Inconsistent visual language; Unlink breaks the established icon convention (Add ➕, Refresh 🔄, Prev/Next ⬅️➡️ all use emoji; Unlink is text-only "Unlink {group}"). — **Fix:** Prepend a link icon (e.g., 🔗❌ or 🔌) to the label in `staff_panel_unlink_button` locale key and `staffUnlinkButton` rendering code. Verify truncation still works at 64 bytes callback data limit.

2. **Show loading state during live panel checks (up to 45s timeout)** — **User impact:** User presses `/staff` or Refresh button and receives no feedback for up to 45 seconds while the bot checks every linked group's live status; appears to hang. — **Fix:** Send a transient "🔄 Loading Staff Group data..." message immediately, edit it in place once the panel is built (or after a timeout). Alternatively, show a toast "Fetching live status..." via callback answer, though Telegram dismisses it after 5 seconds.

3. **Use visual emphasis (emoji prefix or bold) for error and warning messages** — **User impact:** Refusals and warnings (e.g., "Not linked: this group is already linked...") are plain text, blending with regular info; hard to scan for problems. — **Fix:** Add a prefix emoji (❌ for refusals, ⚠️ for warnings) to locale keys `staff_link_refuse_*`, `staff_link_warn_*`, and `staff_notice_health_*`. Or wrap the problem summary in `<b>` tags. Verify translation consistency in all 7 locales.

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)

**Strengths:**
- All command outcomes are specific and action-oriented, never generic:
  - ✅ "Group linked: {group}\nChat ID: `{id}`" (not "Done" or "OK")
  - ✅ "Are you sure you want to unlink {group}? Staff commands will no longer reach it." (specific consequence)
  - ✅ Refusals state the exact reason: "Not linked: {group} is a basic group. Upgrade it to a supergroup first, then link it again."
- Button labels are clear action verbs with emoji:
  - `staff_panel_add_group_button: "➕ Add group"`
  - `staff_panel_refresh_button: "🔄 Refresh"`
  - `staff_panel_prev: "⬅️ Prev"` / `staff_panel_next: "Next ➡️"`
- Error messages include remediation steps (e.g., "Make me an admin, then send the command again")
- Help text is well-structured with * for bold and × for bullets (alita/modules/staff.go lines 68-87 show proper Markdown-to-HTML conversion)
- **All 7 locales verified complete:** 63 `staff_*` keys present in en.yml, es.yml, fr.yml, hi.yml, id.yml, pt.yml, ru.yml (verified via grep)
- Locale keys are copy-friendly: `staff_panel_chat_id`, `staff_set_done`, `staff_link_done` all interpolate data cleanly

**Issues:**
- Generic button labels without context:
  - `staff_btn_confirm: "Confirm"` and `staff_btn_cancel: "Cancel"` are not action-specific (though they appear in context of a clear prompt, so acceptable)
- Some refusal messages use "Not linked:" prefix (appears in 5+ keys: `staff_link_refuse_already_linked`, `staff_link_refuse_not_supergroup`, etc.), creating repetition
- Error line in panel uses generic "Problem:" prefix:
  - `staff_panel_reason_bot_missing: "Problem: the bot is not in this group."` (technical language; could be "Issue:" or "Help needed:")
- Long error messages might wrap awkwardly on small phone screens (e.g., `staff_link_warn_bot_cannot_restrict` is 97 characters)

**Files audited:**
- `locales/en.yml` lines 2639-2711 (63 staff_* keys)
- `alita/modules/staff.go` lines 62-150 (message handling and reply wording)

---

### Pillar 2: Visuals (2/4)

**Strengths:**
- Status indicators use emoji consistently in panel:
  - ✅ = "Bot is admin" YES
  - ❌ = "Bot can restrict" NO
  - ❔ = "Owner matches" UNKNOWN
  - Defined in `alita/modules/staff_panel.go` lines 65-74
- Action buttons use recognizable emoji icons:
  - ➕ "Add group" (international symbol for add)
  - 🔄 "Refresh" (universal reload symbol)
  - ⬅️➡️ "Prev" / "Next" (directional, clear)
- Panel layout creates clear visual hierarchy:
  - Help/hint at top
  - Chat ID in `<code>` (visually distinct)
  - "Linked groups (N):" header
  - Per-row: title + status line + optional reason line (lines 227-237)
  - Legend and updated time at bottom

**Issues:**
- **BLOCKER-adjacent:** Unlink button lacks icon, inconsistent with pattern:
  - `staff_panel_unlink_button: "Unlink {group}"` is text-only
  - All other action buttons (Add, Refresh, Prev/Next) have emoji prefixes
  - Breaks the established visual convention (alita/modules/staff_panel.go line 385)
- Bullet points in help text use × (multiplication sign), not standard:
  - `staff_help_msg` line 2643-2650 shows "× /setstaff:" style bullets
  - × may render as different characters in some Telegram clients (font-dependent)
  - Should use standard ‣ or • instead
- No visual weight differentiation for error lines:
  - Reason lines in panel are plain text (no bold, no emoji prefix)
  - Hard to visually distinguish "Problem: ..." from regular content

**Code references:**
- `alita/modules/staff_panel.go` lines 65-74 (statusIcon), 308-325 (keyboard layout), 345-388 (button rendering)
- `alita/modules/staff_link.go` (unlink button definition)

---

### Pillar 3: Color (3/4)

**Context:** Telegram messages support only HTML formatting: `<b>`, `<i>`, `<u>`, `<code>`, `<pre>`. "Color" in chat-bot context means visual emphasis via these tags and emoji.

**Strengths:**
- Chat IDs consistently wrapped in `<code>` for visual separation:
  - `"Staff Group chat ID: <code>{chat_id}</code>"` (staff_panel_chat_id)
  - `"Group linked: {group}\nChat ID: <code>{id}</code>"` (staff_link_done)
  - Clear visual distinction from body text
- Bold emphasis for group titles and important data (via Markdown * in help, converted to `<b>` by `formatting.ToTelegramHTML(help)`)
- Emoji provide character-based visual distinction:
  - Status: ✅ ❌ ❔
  - Actions: ➕ 🔄 ⬅️ ➡️
  - Warnings: "Heads-up:" text (could be preceded by ⚠️)

**Issues:**
- Error and warning messages lack visual emphasis:
  - `staff_link_refuse_already_linked: "Not linked: {group} is already linked to a Staff Group. Unlink it first."` — plain text
  - `staff_link_warn_bot_missing: "Warning: I am not in that group. Add me there as an admin so staff actions can work."` — no bold "Warning:" prefix
  - `staff_notice_health_bot_missing: "Heads-up: the bot is no longer in {group}. The link is kept; add the bot back with admin rights to restore it."` — no emoji or weight change
- No visual hierarchy between severity levels:
  - Refusals (action failed) and warnings (degraded state) render identically
  - Could use `<b>` for "Not linked:" or "Warning:" to signal importance
- Panel reason lines not emphasized:
  - `staff_panel_reason_bot_missing: "Problem: the bot is not in this group."` — should be bold or have emoji prefix

**Files audited:**
- `locales/en.yml` staff_* keys (2639-2711)
- `alita/modules/staff_panel.go` lines 191-249 (text composition)
- `alita/modules/staff.go` lines 62-66 (reply function)

---

### Pillar 4: Typography (3/4)

**Context:** Telegram does not support custom fonts or font sizes in regular messages. Typography is achieved via markup entities (`<code>`, `<b>`, `<i>`) and line structure.

**Strengths:**
- Consistent use of code blocks for structured data:
  - Chat IDs: `<code>{chat_id}</code>`
  - Command examples in help: `/setstaff`, `/linkstaff <id>` (plain in help, no markup needed)
- Clear visual hierarchy through message structure:
  - Help text (full or hint) → Chat ID → Links header → Rows → Legend → Time
  - Each section separated by `\n\n` (double line break)
  - Rows separated by `\n\n` as well (lines 227-229 in staff_panel.go)
- Markdown-to-HTML conversion preserves formatting:
  - Help text uses `* *` for bold and is converted via `formatting.ToTelegramHTML(help)`
  - Ensures consistent rendering across clients
- Reasonable fallback for length overflow:
  - When message exceeds 3800 UTF-16 units, help swaps to a hint (staff_panel_help_hint), then rows are dropped with a note
  - Keeps readability intact (alita/modules/staff_panel.go lines 256-272)

**Issues:**
- No variation in font weight beyond bold/normal:
  - All status lines, reason lines, and labels use regular weight
  - Cannot emphasize critical warnings with heavier font (native Telegram limitation)
- Hint message is very short:
  - `staff_panel_help_hint: "The Staff Group commands are listed in /help."` (11 words)
  - When this replaces full help (~150 words), might leave visual whitespace on large screens
- Message structure could use more visual cues:
  - No dividers or separators between sections (could use "---" as a divider, but might be overkill)

**Code references:**
- `alita/modules/staff_panel.go` lines 191-249 (composeStaffPanelText), 256-272 (staffPanelTextFor with length cap)
- `alita/modules/staff.go` line 199 (ToTelegramHTML call)
- `locales/en.yml` lines 2639-2650 (help text structure)

---

### Pillar 5: Spacing (3/4)

**Audit method:** Examined line-break patterns in message composition (alita/modules/staff_panel.go).

**Pattern analysis:**
```
help/hint            ← double break: \n\n
chat ID
header               ← single break: \n
page line (if multi) ← row double break: \n\n
row 1
row 2 (if any)
...
truncated note       ← legend double break: \n\n
legend
time                 ← single break: \n
```

**Strengths:**
- Consistent use of double breaks (`\n\n`) between logical sections:
  - Help to chat ID
  - Header to rows
  - Rows to legend
  - Allows visual grouping
- Consistent use of single breaks (`\n`) within sections:
  - Chat ID line to header (line 216)
  - Page line within header group (line 225)
  - Legend to time (line 248)
- Row rendering maintains internal spacing:
  - Title + code ID on one line (line 165)
  - Status line on next line (line 166)
  - Reason line after break (line 169) — allows visual scanning
- Truncation note uses double-break (line 235), making it stand out

**Issues:**
- Minor inconsistency: header-to-page spacing
  - After `staff_panel_links_header`, add `\n` (single), then page line (single)
  - Expected: double break before page line (to visually separate from chat ID section)
  - Current: treated as part of header section (lines 213-226)
  - Impact: low, but could be clearer
- No documented spacing guideline:
  - No mention in AGENTS.md or staff module comments of the spacing scale or rules
  - Future locale strings risk inconsistent spacing if added
- Panel help swap could introduce jarring whitespace:
  - Full help (~150 words, ~400 chars) → hint (~11 words, ~50 chars)
  - Row below hint might appear far down screen on small displays

**Code references:**
- `alita/modules/staff_panel.go` lines 191-249 (spacing constants: `\n` = "\\n", `\n\n` = "\\n\\n")

---

### Pillar 6: Experience Design (3/4)

**Audit method:** Analyzed error handling, state coverage, confirmations, and interaction patterns.

**Coverage:**

| State | Coverage | Evidence |
|-------|----------|----------|
| **Loading** | Partial | 45-second timeout without visual feedback; no skeleton or progress message |
| **Error** | Good | Specific errors with remediation (e.g., "Upgrade to supergroup") |
| **Empty** | Good | "No groups are linked yet." with Add and Refresh buttons available |
| **Success** | Good | Specific confirmations (e.g., "Group linked: ...") with chat ID |
| **Disabled** | Good | Buttons only shown when applicable (Unlink only for link owner, paging only if >8 groups) |
| **Confirmation** | Good | "Are you sure?" for destructive actions (unlink, unset staff) |

**Strengths:**
- Specific error messages with remediation:
  - ✅ `staff_refuse_bot_not_admin: "I need to be an administrator of this group first. Make me an admin, then send the command again."` (not just "Admin required")
  - ✅ `staff_link_refuse_not_supergroup: "Not linked: {group} is a basic group. Upgrade it to a supergroup first, then link it again."` (action required)
- Confirmation flow for destructive actions:
  - ✅ Unlink: "Are you sure you want to unlink {group}? Staff commands will no longer reach it." (clear consequence)
  - ✅ Unset: "Are you sure? Removing Staff status will unlink {count} group(s) from this Staff Group."
  - Both with Confirm/Cancel buttons (alita/modules/staff.go lines 212-226)
- Live permission re-checks on every button press:
  - ✅ Refresh callback checks "Staff Group member" via `IsUserInChatWithError` (not cached admin list)
  - ✅ Unlink callback re-checks owner status live before acting
  - ✅ Page callback checks membership live (alita/modules/staff_panel.go test assertions)
  - Excellent security pattern: authority cannot be stale
- Auto-delete of command messages:
  - ✅ `/linkstaff` and `/unlinkstaff` commands deleted when bot has permission (alita/modules/staff_link.go)
  - ✅ Keeps linked group quiet (D-07 requirement)
- Pagination and button availability:
  - ✅ 8 groups per page, Prev/Next only shown when needed
  - ✅ Out-of-range pages clamped (page = min(max(page, 0), pages-1))
  - ✅ Unlink buttons only on displayed rows
- Callback query answered once:
  - ✅ `answerStaffCallback` called exactly once per action

**Issues:**
- **No loading indicator for panel build:**
  - `/staff` and Refresh pressed → live checks start (up to 45s timeout)
  - No message shown to user; appears to hang
  - Expected behavior: send "🔄 Loading..." message, edit once checks complete
  - Severity: High (UX feels broken), but timeouts are rare with small groups
  - Evidence: `alita/modules/staff_panel.go` line 30: `staffPanelBuildTimeout = 45 * time.Second`

- **Removed links disappear silently:**
  - When panel is built and a link is found to have a changed owner, it's removed and notice posted to Staff Group
  - User viewing the panel doesn't see that the link was removed — it just isn't shown
  - Could be highlighted: "This group was just unlinked (owner changed). Refresh to update."
  - Severity: Medium (Security is sound, but UX is opaque)

- **Health change notices only in Staff Group:**
  - If bot loses rights in a linked group, "Heads-up: the bot can no longer..." notice only goes to Staff Group
  - If the staff member is not in the Staff Group at that moment, they won't see the status change
  - Next `/staff` opens will show it as broken, but there's no alert
  - Severity: Low (design choice per D-14)

- **Failed panel render shown as toast, not inline:**
  - If `buildStaffPanel` fails with an error (e.g., timeout or Telegram API error), user sees "I could not verify..." in a reply
  - Panel is not shown at all; authority of linked groups is unknown
  - Expected: show a degraded panel with "⚠️ Could not verify status" for each group
  - Severity: Medium (rare, but UX is poor when it happens)
  - Evidence: `alita/modules/staff.go` lines 160-164

- **Callback data length not validated visually:**
  - If a group title is very long (e.g., 1000 runes), the button will be silently dropped with a log warning
  - User sees fewer Unlink buttons than groups shown, but no explanation
  - Severity: Low (happens only with pathological titles; documented in code)
  - Evidence: `alita/modules/staff_panel.go` lines 374-387 (button generation)

**Files audited:**
- `alita/modules/staff.go` lines 62-300 (command handlers, confirmation flow)
- `alita/modules/staff_panel.go` lines 282-388 (panel rendering, button logic)
- `alita/modules/staff_unlink.go` (unlink action flow)
- `alita/modules/staff_sweeper.go` (health check and removal)
- `locales/en.yml` staff_* keys (error, success, confirmation messages)

---

## Files Audited

### Source Code
- `/home/user/Fuku_Robot/alita/modules/staff.go` (lines 1-300+)
- `/home/user/Fuku_Robot/alita/modules/staff_panel.go` (lines 1-541)
- `/home/user/Fuku_Robot/alita/modules/staff_link.go` (staff picking and linking flow)
- `/home/user/Fuku_Robot/alita/modules/staff_unlink.go` (unlink action)
- `/home/user/Fuku_Robot/alita/modules/staff_sweeper.go` (background checks and notices)

### Locale Files (All 7 Verified Complete)
- `/home/user/Fuku_Robot/locales/en.yml` lines 2639-2711 (63 keys)
- `/home/user/Fuku_Robot/locales/es.yml` (63 staff_* keys, verified via grep)
- `/home/user/Fuku_Robot/locales/fr.yml` (63 staff_* keys, verified via grep)
- `/home/user/Fuku_Robot/locales/hi.yml` (63 staff_* keys, verified via grep)
- `/home/user/Fuku_Robot/locales/id.yml` (63 staff_* keys, verified via grep)
- `/home/user/Fuku_Robot/locales/pt.yml` (63 staff_* keys, verified via grep)
- `/home/user/Fuku_Robot/locales/ru.yml` (63 staff_* keys, verified via grep)

### Planning Artifacts
- `.planning/phases/01-staff-group-links/01-CONTEXT.md` (requirements and design decisions D-01 through D-20)
- `.planning/phases/01-staff-group-links/01-01-SUMMARY.md` through `01-10-SUMMARY.md` (execution history)

---

## Compliance Notes

### Alignment with Design Context (01-CONTEXT.md)
- ✅ **D-01 (Add group via picker):** Implemented via `staffAddGroupURL` which builds `t.me/<bot>?startgroup=stf_` deep link. Button labeled "➕ Add group".
- ✅ **D-02 (/linkstaff fallback):** Implemented with typed `/linkstaff [id]` command. Handled in `linkStaff` function.
- ✅ **D-05 (Unlink button):** Rendered in panel with confirmation prompt "Are you sure you want to unlink {group}?" before deletion.
- ✅ **D-06 (Notices to Staff Group only):** Notices (`staff_notice_*` keys) only posted to Staff Group, never to linked group.
- ✅ **D-16 (/staff works only in Staff Group):** `staffPanel` checks `staff.GetStaffGroup(c.Chat.Id)` and returns silently if not a Staff Group.
- ✅ **D-18 (Live status on /staff open and Refresh):** `buildStaffPanel` calls `buildStaffPanelRows` which runs `recheckLink` and `FetchBotMember` on every call.
- ✅ **D-19 (Paging at 8):** `staffPanelPageSize = 8` defined in staff_panel.go line 33.
- ✅ **PLAT-03 (All 7 locales):** All 63 staff_* keys present in all 7 locale files.

### Deviations from Contract
- No deviations from copywriting, visual, color, or typography contracts detected.
- Experience Design: Loading state missing (fix #2 in top 3).
- Visuals: Unlink button icon missing (fix #1 in top 3).

---

## Recommendation Summary

**Overall Assessment:** Phase 01 delivers a functional, well-localized Telegram staff management interface with clear error messages and good permission patterns. The implementation meets the design contract for linking, unlinking, pagination, and live status checks. Minor UX gaps (no loading indicator, inconsistent button icons, weak error emphasis) degrade the visual polish but do not block functionality.

**Readiness:** Ready for Phase 2 (staff moderation actions) with the top 3 fixes recommended for better UX.

---

*Phase: 01-staff-group-links*
*Reviewed: 2026-10-05*
