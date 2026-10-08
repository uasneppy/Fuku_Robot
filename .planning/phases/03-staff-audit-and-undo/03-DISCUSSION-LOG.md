# Phase 3: Staff Audit and Undo - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-10-05
**Phase:** 03-staff-audit-and-undo
**Areas discussed:** Undo semantics, Undo flow & summary, Log-channel posts, Recent-actions list

---

## Undo semantics

**What should Undo do for each action type?**

| Option | Description | Selected |
|--------|-------------|----------|
| Mirror all | ban→unban, mute→unmute, unban→re-ban, unmute→re-mute; kick has no Undo button | ✓ |
| Only restrictive actions | Undo only on ban and mute | |
| Mirror all incl. kick | Kick's Undo is a no-op reporting "nothing to undo" | |

**A permanent staff ban upgraded an existing 1-day ban. What does Undo do there?**

| Option | Description | Selected |
|--------|-------------|----------|
| Restore prior | Record pre-action status per group and put exactly that back | ✓ |
| Lift fully | Unban/unmute completely, dropping the earlier ban | |
| Skip that group | Skip groups with a prior restriction | |

**Target's status changed since the action?**

| Option | Description | Selected |
|--------|-------------|----------|
| Skip if changed | Compare live status with what the action left; skip "changed since" | ✓ |
| Reverse anyway | Apply the reverse call regardless (still via decideStaffAction) | |

**How long does the Undo button stay usable?**

| Option | Description | Selected |
|--------|-------------|----------|
| 24 hours | Short window | |
| 7 days | Longer window | |
| No limit | Works as long as the record exists | ✓ |

**User's choice:** Mirror all; restore prior; skip if changed; no time limit.

---

## Undo flow & summary

**Who may press "Undo everywhere"?**

| Option | Description | Selected |
|--------|-------------|----------|
| Any staff member | Any live Staff Group member; per-group live restrict check | ✓ |
| Issuer only | Only the original issuer | |

**Should Undo need a Confirm tap?**

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, confirm | Confirm/Cancel card, only the presser confirms | ✓ |
| No, act at once | One tap reverses immediately | |

**Where does the undo result appear?**

| Option | Description | Selected |
|--------|-------------|----------|
| New reply message | New summary replying to the original; original gets "Undone by" line | ✓ |
| Edit original summary | Original lines flip to show the undo result | |

**After an undo has run, what happens to the button?**

| Option | Description | Selected |
|--------|-------------|----------|
| Gone once run | One undo per action; second press says "already undone" | ✓ |
| Retry skipped groups | Button stays as "Retry undo" for skipped/failed groups | |

**User's choice:** Any staff member; Confirm required; new reply message; one undo per action.

---

## Log-channel posts

**Which log category?**

| Option | Description | Selected |
|--------|-------------|----------|
| Existing 'admin' | Same toggle as local admin actions; no migration | ✓ |
| New 'staff' category | Separate toggle; migration + locale labels | |
| Always post | Ignore toggles | |

**How much does the post reveal about the origin?**

| Option | Description | Selected |
|--------|-------------|----------|
| Issuer + 'via Staff Group' | Names the issuer, not the Staff Group chat | ✓ |
| Issuer + Staff Group name | Also shows the Staff Group title | |
| Same as local /ban | Plain #BAN, no staff marker | |

**When is each group's post sent?**

| Option | Description | Selected |
|--------|-------------|----------|
| As each group succeeds | Right after ✅, via shared pacer; failed post never changes the line | ✓ |
| After the whole run | Batch posts once the summary is final | |

**Are undos logged?**

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, same rules | #STAFF_UNDO post per group where the undo applied | ✓ |
| No | Only the original action is logged | |

**User's choice:** admin category; issuer + "via Staff Group"; as each group succeeds; undos logged.

---

## Recent-actions list

**Where does the list live?**

| Option | Description | Selected |
|--------|-------------|----------|
| 'Recent actions' button | /staff button switches to a paged history view with Back | ✓ |
| Inline below links | Last few actions appended to /staff | |
| Separate command | e.g. /staffhistory | |

**How does each entry show per-group outcomes?**

| Option | Description | Selected |
|--------|-------------|----------|
| Tally + tap for detail | Compact line with counts; button opens full per-group list (+ Undo) | ✓ |
| Full list inline | Every entry lists all its groups | |
| Tally only | Counts only | |

**How many entries, how far back?**

| Option | Description | Selected |
|--------|-------------|----------|
| 10 per page, paged | Newest first, Prev/Next through everything stored | ✓ |
| Last 20 only | Most recent only | |

**How long are records kept?**

| Option | Description | Selected |
|--------|-------------|----------|
| Forever | No pruning; matches unlimited undo | ✓ |
| 90 days | Periodic pruning | |
| 1 year | Periodic pruning | |

**User's choice:** "Recent actions" button; tally + detail; 10 per page, paged; kept forever.

---

## Claude's Discretion

- Audit schema shape, write timing, and how records link to summary messages (incl. Staff Group migration)
- Callback token/ID scheme for records
- Display of unlinked/renamed groups in history; undo skips no-longer-linked groups
- No Undo for pre-Phase-3 actions; Undo only on final summaries
- Undo takes the per-target fan-out lock
- Locale wording; reason length cap and truncation

## Deferred Ideas

None.
