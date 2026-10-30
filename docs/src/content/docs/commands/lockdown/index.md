---
title: Lockdown Commands
description: Complete guide to Lockdown module commands and features
---

# 📦 Lockdown Commands

A lockdown stops a raid in this group: everyone but the admins is muted until an admin lifts it.

- `/lockdown [reason]`: Lock this group at once, with no confirm step. Only admins can talk, approved users included, and new members are removed until the lift.
- `/unlockdown`: Lift the lockdown. Every permission goes back exactly as it was before the lockdown, and the people I removed are unbanned, except the accounts banned for good with Ban recent joiners.
- `/lockdownstatus`: Show whether this group is locked, since when, by whom and why (any admin).
- `/autolockdown [joins|flood|ai] [N|on|off]`: Show or change the automatic triggers. Any admin can look; only the group owner or an admin who can restrict members can change them. A number from 3 to 500 sets the threshold and turns the trigger on.

Only the group owner, or an admin who can restrict members, can use these commands, and I check that live. I must be an admin who can restrict members, and the group must be a supergroup.
A lockdown never ends on its own. It stays until an admin lifts it.
The bans I place on joiners last 330 days from each join. If a lockdown lasts longer than that, those bans run out by themselves and the people can come back; the lift summary says how many.
Anonymous admins are asked to confirm who they are first.
Join requests are declined while the group is locked; people can ask again after the lift.

When this group is locked, I post an alert to its Staff Group (if it is linked) and to its log channel (if the admin log category is on): the trigger, how many joined just before and since, up to 10 of those accounts, and the invite link most of them used.

I also lock this group on my own when a raid starts: when 15 people join within 60 seconds (join surge), or when 30 messages, photos, GIFs or stickers arrive within 30 seconds, at least 60% of them from at least 3 members who joined in the last 24 hours (flood). Those numbers are the defaults; `/autolockdown` shows this group's own numbers. Both triggers are on by default. Admins, approved users and commands I handle never count, so approving trusted newcomers keeps them out of it. I post a short notice when I lock the group, and the alert says which trigger fired. For 10 minutes after a lift I don't lock again on my own. If I see a raid but can't lock (I must be an admin who can restrict members, in a supergroup), I warn the Staff Group and the log channel, at most once a day. These settings are not saved in backups.

AI trigger: suspicious text from members who joined in the last 24 hours is checked by TypeSafe, their photos, GIFs and stickers by Anthropic Claude (if set up). 5 spam verdicts from 3 accounts within 120 s lock the group; one verdict never does, and AI never deletes or bans.

🔓 Lift lockdown: lifts the lockdown like `/unlockdown`. Only the group owner or an admin who can restrict members, checked live; being in the Staff Group is not enough.

🚫 Ban recent joiners: bans the accounts of the burst for good, so they can't rejoin, and I can't undo it. Admins, approved users and people an admin let in are left out. A later press bans only new accounts. Only the group owner or an admin who can restrict members, checked live.

⛔ Revoke link: revokes the invite link the raid used, if I created it; other admins' links must be revoked in Telegram. Only the group owner or an admin who can invite users. Only one revoke is offered per lockdown; if the raid moves to another link I made, revoke it in the group's settings in Telegram.

Every button asks for a confirmation that only the person who pressed it can give. Press the buttons as yourself, not as an anonymous admin or a channel. While the group stays locked I post a reminder every hour; reminders stop when the lockdown is lifted.


## Module Aliases

This module can be accessed using the following aliases:

- `lockdown`
- `unlockdown`
- `lockdownstatus`
- `autolockdown`

