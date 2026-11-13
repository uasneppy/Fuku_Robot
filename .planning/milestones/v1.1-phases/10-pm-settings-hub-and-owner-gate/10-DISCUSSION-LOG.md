# Phase 10: PM Settings Hub and Owner Gate - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-11-03
**Phase:** 10-PM Settings Hub and Owner Gate
**Areas discussed:** Group list contents, List freshness, The admin switch, Refusals and no owner

---

## Group list contents

### What each row shows

| Option | Description | Selected |
|--------|-------------|----------|
| Title only (Recommended) | One button per group with just its name, like the GroupHelp screenshot. Simplest; the button carries only the group ID. | ✓ |
| Title + role marker | A marker for groups you own versus groups you can open as an allowed admin. | |
| Title + attention marker | A marker when something needs fixing (bot not admin, lockdown active). Needs an extra read per row. | |

**User's choice:** Title only

### Groups where the bot is not an admin

| Option | Description | Selected |
|--------|-------------|----------|
| Hide until admin (Recommended) | Only groups the bot has verified appear; the empty-list hint says to make the bot an admin and run /admincache. | ✓ |
| Show as unavailable | Show the group greyed out with a "make the bot an admin" note. Needs another way to learn the owner. | |

**User's choice:** Hide until admin

### A user with exactly one manageable group

| Option | Description | Selected |
|--------|-------------|----------|
| Always show the list (Recommended) | Same behaviour for everyone, one extra tap, confirms which group is about to change. | ✓ |
| Open it directly | Skip the list and show the group's main menu. | |

**User's choice:** Always show the list

### List order (eight per page)

| Option | Description | Selected |
|--------|-------------|----------|
| Alphabetical (Recommended) | By group title; a group is always in the same place. | ✓ |
| Last opened first | Needs the bot to remember what each person opened. | |
| Newest group first | Groups the bot joined most recently first. | |

**User's choice:** Alphabetical
**Notes:** The user chose "Next area" at the first continue check.

---

## List freshness

### How the list fills after deploy

| Option | Description | Selected |
|--------|-------------|----------|
| Startup backfill (Recommended) | Once at startup, load the admin list of every group the bot is in where it is an admin, paced. | |
| Lazy only | A group is recorded the first time its admin list loads. | |
| Backfill + hourly sweep | As the backfill, plus an hourly recheck like the Staff Group sweeper. | ✓ |

**User's choice:** Backfill + hourly sweep

### Repair path for a missing group

| Option | Description | Selected |
|--------|-------------|----------|
| /admincache only (Recommended) | The roadmap's wording; nothing new to learn or document. | ✓ |
| Add /reload too | GroupHelp's wording; /reload becomes an alias of /admincache. | |
| Refresh button | A Refresh button on the PM list; cannot find a group the bot hasn't recorded. | |

**User's choice:** /admincache only
**Notes:** The user chose "Next area" at the continue check.

---

## The admin switch

### Where the switch lives

| Option | Description | Selected |
|--------|-------------|----------|
| Its own Access page (Recommended) | An "Access" entry on the main menu; admins never see it; room to grow. | ✓ |
| A button on the main menu | One toggle on the main page, owner only; crowds the grid, no room to explain. | |

**User's choice:** Its own Access page

### Confirming ON

| Option | Description | Selected |
|--------|-------------|----------|
| Confirm card for ON only (Recommended) | ON asks first (stateless card, 5-minute expiry); OFF is one tap because it only narrows access. | ✓ |
| Single tap both ways | Faster, but an accidental tap hands every admin the menu. | |
| Confirm both ways | Most cautious; OFF being harmless makes the extra tap pointless. | |

**User's choice:** Confirm card for ON only

### An admin's open menu when the switch goes OFF

| Option | Description | Selected |
|--------|-------------|----------|
| Refuse, leave menu as is (Recommended) | Short alert, nothing edited; every press already re-checks live. | ✓ |
| Refuse and close the menu | Same alert, and the menu message is edited to a closed note. | |

**User's choice:** Refuse, leave menu as is

### Announcing a flip

| Option | Description | Selected |
|--------|-------------|----------|
| Nothing (Recommended) | The page shows the state; matches other menu settings; log-channel posts were deferred. | ✓ |
| Notice in the group | A short public line on every flip. | |

**User's choice:** Nothing
**Notes:** The user chose "Next area" at the continue check.

---

## Refusals and no owner

### Public reply to a gated admin typing /settings in a group

| Option | Description | Selected |
|--------|-------------|----------|
| Refusal plus the fix (Recommended) | "Only the group owner can open settings. The owner can let admins in from the Access page." | |
| Plain refusal | "Only the group owner can open settings." | ✓ |

**User's choice:** Plain refusal (differs from the recommendation)

### Empty private list for a gated admin

| Option | Description | Selected |
|--------|-------------|----------|
| One neutral text for all (Recommended) | Same words for everyone, so it reveals which groups exist to no one. | ✓ |
| Tailored for gated admins | Tell a known admin of a gated group that only the owner can change settings. | |

**User's choice:** One neutral text for all

### A group with no live owner

| Option | Description | Selected |
|--------|-------------|----------|
| Admins allowed (Recommended) | Live administrators get the menu, still checked on every press. | ✓ |
| Nobody until an owner exists | Strictest reading; the menu is closed to everyone in that group. | |
| Only admins with full rights | Admins holding every right stand in for the owner. | |

**User's choice:** Admins allowed
**Notes:** The user chose "Next area" at the continue check, then "I'm ready for context".

---

## Claude's Discretion

- Button labels, emoji, page controls and where the Access entry sits in the grid (UI-SPEC).
- Table, column and namespace names, cache policy, the extraction of the shared authorization step.
- Pacing, lock and timing of the startup backfill and hourly sweep, and their drain.
- Exact wording of the deep-link refusal for a gated admin and of in-menu refusal alerts.

## Deferred Ideas

- Role and attention markers on list rows; showing bot-not-admin groups as unavailable; opening a single group directly.
- `/reload` alias and a Refresh button; last-opened or newest-first ordering.
- Announcing a switch flip (group notice or log channel).
- Closing an open admin menu when the switch goes OFF; tailored empty-list text for gated admins; a full-rights stand-in for a missing owner.
