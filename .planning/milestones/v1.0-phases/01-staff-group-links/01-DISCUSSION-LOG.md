# Phase 1: Staff Group Links - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md. This log keeps the alternatives that were considered.

**Date:** 2026-10-04
**Phase:** 01-staff-group-links
**Areas discussed:** Linking flow, Staff Group rules, When links break, /staff panel

---

## Linking flow

**How should an owner link a group to the Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Group picker button (Recommended) | An "Add group" button in `/staff` opens Telegram's group picker. The bot checks you own both groups, then links. It can also request bot rights. | ✓ |
| One-time code | `/staff` shows a short code that expires. You send `/linkstaff CODE` in the group. | |
| Command with group ID | `/linkstaff <staff group ID>` in the group, like `/joinfed`. | |

**Should there also be a typed command for linking?**

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, `/linkstaff` too (Recommended) | A typed fallback in the group being linked, with the same live checks. | ✓ |
| Picker only | One way to link. | |

**How does `/linkstaff` know which Staff Group to link to?**

| Option | Description | Selected |
|--------|-------------|----------|
| Auto when obvious (Recommended) | No argument if you own exactly one Staff Group; otherwise give its ID, which `/staff` shows. | ✓ |
| Always the Staff Group ID | Explicit every time. | |
| One-time code | A short expiring code from `/staff`. | |

**Can a group be linked if the bot isn't an admin there yet, or lacks restrict rights?**

| Option | Description | Selected |
|--------|-------------|----------|
| Link, but warn (Recommended) | Link anyway; show a warning now and in `/staff`. | ✓ |
| Refuse until fixed | Refuse until the bot is an admin with restrict rights. | |

**How should an owner unlink a group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Both ways (Recommended) | An Unlink button in `/staff` (owner only, asks to confirm) plus `/unlinkstaff`. | ✓ |
| Button in /staff only | All link management in one place. | |
| `/unlinkstaff` only | Typed in the linked group. | |

**When a group is linked or unlinked, who sees a message about it?**

| Option | Description | Selected |
|--------|-------------|----------|
| Both groups (Recommended) | A notice in the Staff Group and in the linked group. | |
| Staff Group only | Staff are told; nothing is posted in the linked group. | ✓ |
| Just the reply | Only the person who did it gets a confirmation. | |

**Linking from inside the group leaves a visible message there (the picker's `/start@bot …`, or a typed `/linkstaff`). What happens to it?**

| Option | Description | Selected |
|--------|-------------|----------|
| Delete it, confirm in Staff (Recommended) | Delete the command message; confirm, or give the refusal reason, in the Staff Group. | ✓ |
| Short reply in the group | Reply "Linked" in the group and also tell the Staff Group. | |

**User's choices:** Picker first, with `/linkstaff` as a fallback. Linked groups stay quiet: notices go to the Staff Group only, and command messages are deleted.

---

## Staff Group rules

**Does the bot need to be an admin in the Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, require admin (Recommended) | Needed for `chat_member` updates, so ownership changes are seen right away. | ✓ |
| Member is enough | Only live checks catch Staff Group ownership changes. | |

**Can one owner have more than one Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, several (Recommended) | e.g. separate rooms for separate communities. | ✓ |
| One per owner | Simpler; `/linkstaff` never needs an argument. | |

**Can a Staff Group also be a linked group?**

| Option | Description | Selected |
|--------|-------------|----------|
| No, keep roles separate (Recommended) | No chains, and authority stays clear. | ✓ |
| Yes, allow it | Head staff over moderator rooms. | |

**Basic (non-super) groups?**

| Option | Description | Selected |
|--------|-------------|----------|
| Allow, with a warning (Recommended) | Basic groups allowed everywhere; warn that mute doesn't work. | |
| Linked groups must be super | The Staff Group may be basic; linking a basic group is refused. | ✓ |
| Supergroups only | Both must be supergroups. | |

**User's choices:** The bot is an admin in the Staff Group. Several Staff Groups per owner are allowed. The two roles never overlap. Linked groups must be supergroups.

---

## When links break

**When a link is removed automatically because ownership changed, who gets told?**

| Option | Description | Selected |
|--------|-------------|----------|
| Staff Group notice (Recommended) | Post in the Staff Group. | ✓ |
| Staff Group + old owner DM | Also message the person who made the link privately. | |
| Silent | The link just disappears. | |

**If both groups are transferred to the same new owner, does the link survive?**

| Option | Description | Selected |
|--------|-------------|----------|
| No, new owner relinks (Recommended) | The link belongs to whoever made it. | ✓ |
| Yes, keep it | One person still owns both. | |

**What happens when the bot is removed from a linked group, or loses admin there?**

| Option | Description | Selected |
|--------|-------------|----------|
| Keep it, show broken (Recommended) | Only an ownership change breaks a link; the Staff Group gets one heads-up. | ✓ |
| Unlink if the bot is removed | Drop the link if the bot is kicked; keep it if the bot is only demoted. | |

**How quickly should a stale link disappear (background re-check)?**

| Option | Description | Selected |
|--------|-------------|----------|
| Within about an hour (Recommended) | Hourly re-check, spread out; live checks still guard every use. | ✓ |
| Within about 10 minutes | Faster, about 6× more API calls. | |
| Daily | Fewest calls. | |

**User's choices:** Links belong to the person who made them, and only an ownership change breaks one. The Staff Group is notified. A background re-check runs about every hour.

---

## /staff panel

**Who in the Staff Group can open `/staff`?**

| Option | Description | Selected |
|--------|-------------|----------|
| Any member (Recommended) | Everyone sees coverage; buttons that change links are for the owner only. | ✓ |
| Staff Group admins only | A tighter view. | |

**How many groups per Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Under 10 | One message, no pages. | ✓ |
| 10 to 30 | Compact lines; page after about 15. | |
| 30 or more | Paged panel; spread-out checks. | |

**How fresh should the status be?**

| Option | Description | Selected |
|--------|-------------|----------|
| Live on open + Refresh (Recommended) | Live checks each open, and a Refresh button that updates in place. | ✓ |
| From the hourly re-check | Instant, but up to an hour out of date. | |

**What should `/staff` do outside the Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Private: list my Staff Groups (Recommended) | In a private chat, list the Staff Groups you own; stay quiet in linked groups. | |
| Staff Group only | Works only inside a Staff Group. | ✓ |
| Also in linked groups | Show "linked to X" to admins of a linked group. | |

**User's choices:** Any member can open it. Built for under 10 groups. Live status with Refresh. Works in the Staff Group only.

---

## Claude's Discretion

- Command names and aliases (`/setstaff`, `/unsetstaff`, `/linkstaff`, `/unlinkstaff`, `/staff`)
- A confirm step and the notice wording for removing Staff status
- What happens when the Staff Group itself changes owner (default: its links break, the status stays)
- The bot removed from the Staff Group or demoted there (default: keep the status and links)
- How anonymous admins are handled (refuse, or the "prove it" button); the live creator check is never skipped
- Where refusals go for people who don't own the Staff Group they name (never posted into that Staff Group)
- Picker payload format and expiry
- Pacing of the hourly re-check, with a single replica running it
- Status icons and message wording, in all 7 locales
- Re-keying a linked group whose chat ID changes, done defensively

## Deferred Ideas

- `/staff` in a private chat or in linked groups (owner chose "Staff Group only"; revisit with the settings menu)
- A private message to the person who made a link when it breaks
- Nested Staff Groups
