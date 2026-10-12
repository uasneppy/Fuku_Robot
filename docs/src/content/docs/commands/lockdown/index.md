---
title: Lockdown Commands
description: Complete guide to Lockdown module commands and features
---

# 📦 Lockdown Commands

A lockdown stops a raid in this group: everyone but the admins is muted until an admin lifts it.

- `/lockdown [reason]`: Lock this group at once, with no confirm step. Only admins can talk, approved users included, and new members are removed until the lift.
- `/unlockdown`: Lift the lockdown. Every permission goes back exactly as it was before the lockdown, and the people I removed are unbanned.
- `/lockdownstatus`: Show whether this group is locked, since when, by whom and why (any admin).

Only the group owner, or an admin who can restrict members, can use these commands, and I check that live. I must be an admin who can restrict members, and the group must be a supergroup.
A lockdown never ends on its own. It stays until an admin lifts it.
The bans I place on joiners last 330 days from each join. If a lockdown lasts longer than that, those bans run out by themselves and the people can come back; the lift summary says how many.
Anonymous admins are asked to confirm who they are first.
Join requests are declined while the group is locked; people can ask again after the lift.

When this group is locked, I post an alert to its Staff Group (if it is linked) and to its log channel (if the admin log category is on): the trigger, how many joined just before and since, up to 10 of those accounts, and the invite link most of them used.

🔓 Lift lockdown: lifts the lockdown like `/unlockdown`. Only the group owner or an admin who can restrict members, checked live; being in the Staff Group is not enough.

🚫 Ban recent joiners: bans the accounts of the burst for good, so they can't rejoin, and I can't undo it. Admins, approved users and people an admin let in are left out. A later press bans only new accounts.

⛔ Revoke link: revokes the invite link the raid used, if I created it; other admins' links must be revoked in Telegram. Only the group owner or an admin who can invite users. Only one revoke is offered per lockdown; if the raid moves to another link I made, revoke it in the group's settings in Telegram.

Every button asks for a confirmation that only the person who pressed it can give. Press the buttons as yourself, not as an anonymous admin or a channel. While the group stays locked I post a reminder every hour; reminders stop when the lockdown is lifted.


## Module Aliases

This module can be accessed using the following aliases:

- `lockdown`
- `unlockdown`
- `lockdownstatus`

