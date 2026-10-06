# Phase 4: Manual Lockdown - Discussion Log

> **Audit trail only.** Do not use this file as input to planning, research or execution agents.
> The decisions are in 04-CONTEXT.md. This log keeps the alternatives that were considered.

**Date:** 2026-10-06
**Phase:** 04-manual-lockdown
**Areas discussed:** Approved users, Removing joiners, Lock & lift commands, Hand edits & restore

---

## Approved users

### If Telegram can't exempt approved users from a locked group, what should a lockdown do with them?

| Option | Description | Selected |
|--------|-------------|----------|
| Muted too (Recommended) | Approved users are muted like everyone else and only admins talk. The notice says so. Telegram enforces it on its own. | ✓ |
| Bot deletes instead | Defaults stay unlocked, and the bot deletes messages from anyone who isn't an admin or approved. Raid messages show briefly, every delete costs a call, and nothing is blocked while the bot is down. | |
| Hybrid | Media, stickers, polls and invites are blocked for everyone. Text is allowed, and the bot deletes text from anyone who isn't approved. | |

**User's choice:** Muted too.

### How should the two roadmap spikes happen?

| Option | Description | Selected |
|--------|-------------|----------|
| Design both away (Recommended) | The owner does a 2-minute app check for spike 1. The bot handles all three join paths, so spike 2 doesn't gate planning. Live behaviour is checked in UAT. | ✓ |
| Checkpoint in plan 1 | The first plan is a probe build run in a throwaway supergroup, and later plans branch on the result. | |
| Skip spike 1 too | Don't test exemptions; approved users are always muted. | |

**User's choice:** Design both away.

### Record the spike 1 result now?

| Option | Description | Selected |
|--------|-------------|----------|
| Can't — it's locked | The toggle is greyed out or has no effect, so the fallback applies. | |
| Yes, it works | The user can talk, so approved users get a per-user exception. | |
| Not checked yet | Open item. The owner reports the result before planning, and CONTEXT.md describes both branches. | ✓ |

**User's choice:** Not checked yet.
**Notes:** CONTEXT.md D-02 adds a warning for the "works" branch: if per-user permissions can override a locked default, every member with a per-user restriction that allows sending would also talk. The lockdown would then leak, so the owner must be consulted before planning on that branch.

---

## Removing joiners

### How should the bot remove someone who joins during a lockdown?

| Option | Description | Selected |
|--------|-------------|----------|
| Ban until lift (Recommended) | Banned on joining, and unbanned at the lift with pacing. Any unban that fails is reported. | ✓ |
| Kick | Ban then unban at once. Raid bots can loop. | |
| Short timed ban | Telegram lifts the ban after a few minutes, and raiders retry. | |

**User's choice:** Ban until lift.

### At the lift, what about a joiner deliberately banned during the lockdown?

| Option | Description | Selected |
|--------|-------------|----------|
| Stays banned (Recommended) | The lift unbans only joiners whose ban is still the lockdown's own. | ✓ |
| Unban every joiner | Simpler, but deliberate bans are lifted too. | |

**User's choice:** Stays banned.

### Join requests during a lockdown?

| Option | Description | Selected |
|--------|-------------|----------|
| Decline (Recommended) | Declined at once. The person can request again after the lift, and auto-approve never admits anyone. | ✓ |
| Hold pending | No calls during the raid, but admins clear the queue by hand afterwards. | |

**User's choice:** Decline.

### Who may still join during a lockdown? (multi-select)

| Option | Description | Selected |
|--------|-------------|----------|
| Added by an admin | A user an admin adds directly, not through an invite link. | ✓ |
| Bots added by admin | A bot an admin adds. | |
| Approved users | An approved user who rejoins. | |

**User's choice:** Added by an admin only.

### Bot added by an admin during a lockdown?

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, ban bots too | No bot gets in during a lockdown, whoever adds it. | ✓ |
| No, admin-added bots stay | Treat it like any user an admin adds. | |

**User's choice:** Ban bots too.

### People still on their captcha when a lockdown starts?

| Option | Description | Selected |
|--------|-------------|----------|
| Leave them (Recommended) | They keep their attempt, and a pass leaves them muted until the lift. | ✓ |
| Remove them too | Treat them as joiners: banned until the lift. | |

**User's choice:** Leave them.

---

## Lock & lift commands

### Who can run /lockdown and /unlockdown?

| Option | Description | Selected |
|--------|-------------|----------|
| Restrict right, both (Recommended) | The owner, or an admin with can_restrict_members, checked live, for both commands. | ✓ |
| Any admin, both | Any admin can lock or lift. | |
| Lock: any, lift: restrict | Any admin can lock, but lifting needs the restrict right. | |

**User's choice:** Restrict right, both.

### Anonymous admin sends /lockdown or /unlockdown?

| Option | Description | Selected |
|--------|-------------|----------|
| Prove-admin button (Recommended) | The existing flow: whoever taps is checked live and named. AnonAdmin mode never skips the check. | ✓ |
| Refuse | Reply "post as yourself". | |
| Follow AnonAdmin mode | Trust the anonymous post when AnonAdmin mode is on. | |

**User's choice:** Prove-admin button.

### What do members see on lock and lift?

| Option | Description | Selected |
|--------|-------------|----------|
| Notice with reason (Recommended) | A public notice naming who acted and the optional reason, and on lift the count of joiners unbanned. | ✓ |
| Notice, no reason | Members see only that the group is locked or unlocked. | |
| Silent | No message in the group. | |

**User's choice:** Notice with reason.

### How does an admin check status?

| Option | Description | Selected |
|--------|-------------|----------|
| /lockdownstatus (Recommended) | A separate read-only command for any admin. | ✓ |
| /lockdown status | A subcommand; "status" becomes a reserved first word. | |
| Status for everyone | A read-only command any member can run. | |

**User's choice:** /lockdownstatus.

### Confirm tap before /lockdown or /unlockdown?

| Option | Description | Selected |
|--------|-------------|----------|
| Neither (Recommended) | Both act at once. | ✓ |
| Lift only | /unlockdown asks for confirmation from the admin who ran it. | |
| Both | Both show a Confirm card first. | |

**User's choice:** Neither.

### How /staff shows a locked group

| Option | Description | Selected |
|--------|-------------|----------|
| 🔒 + since (Recommended) | One marker on the row. | ✓ |
| 🔒 + since + who + reason | Longer rows. | |
| 🔒 only | Icon only. | |

**User's choice:** 🔒 + since.

---

## Hand edits & restore

### The lift when permissions were edited by hand during the lockdown

| Option | Description | Selected |
|--------|-------------|----------|
| Restore, and say so (Recommended) | Always put back the pre-lockdown permissions, and report that a manual change was replaced. | ✓ |
| Keep the hand edit | Leave the permissions alone and only end the lockdown. | |
| Ask the admin | Show the difference and ask which to keep. | |

**User's choice:** Restore, and say so.

### An admin reopens the group by hand while the lockdown is on

| Option | Description | Selected |
|--------|-------------|----------|
| Stay on, don't fight (Recommended) | The lockdown stays active until /unlockdown, and the status points out the manual change. | ✓ |
| Re-lock | Set the locked permissions again whenever the bot notices. | |
| Treat as a lift | End the lockdown when the bot notices. | |

**User's choice:** Stay on, don't fight.

### /lockdown when the bot lacks rights

| Option | Description | Selected |
|--------|-------------|----------|
| Refuse unless it can lock (Recommended) | Refuse and record nothing. A missing delete right doesn't block. | ✓ |
| Record and do what it can | Record the lockdown and report the parts that aren't working. | |

**User's choice:** Refuse unless it can lock.

### /unlockdown when restoring permissions fails

| Option | Description | Selected |
|--------|-------------|----------|
| Stays locked, retry (Recommended) | Stays active, and nobody is unbanned until Telegram confirms the restore. | ✓ |
| Mark lifted anyway | End the lockdown and ask the admin to fix permissions by hand. | |

**User's choice:** Stays locked, retry.
**Notes:** At the area check the owner accepted Claude's default for a bot removed from a locked group: the lockdown stays recorded as active, /staff shows the group as locked with the bot missing, and re-adding the bot then /unlockdown ends it cleanly.

---

## Claude's Discretion

- Schema: snapshot fidelity, the stored locked set, one active lockdown per group, and a trigger-kind field.
- How "the lockdown's own ban" is recognised at the lift.
- Never ban a joiner without recording them first, and resume the lift after a restart.
- Fleet-wide pacing for bans, unbans and declines, and behaviour with Redis down.
- The join guard's handler group number, and how it coexists with /antiraid until Phase 6.
- Deduping joins across paths, suppressing goodbye messages, and how lockdown state is read on each join (cache or fresh).
- Basic groups, and /lockdown inside a Staff Group.
- Wording in all 7 locale files, the reason cap, the time format, help text and docs, and the /unmute reply during a lockdown.

## Deferred Ideas

None came up. Cross-phase notes for Phases 5 and 6 are in 04-CONTEXT.md under Deferred Ideas.
