---
title: Lockdown Commands
description: Complete guide to Lockdown module commands and features
---

# 📦 Lockdown Commands

**🔒 Lockdown**

A lockdown stops a raid in this group: everyone but the admins is muted until an admin lifts it. It never ends on its own.

**Admin commands**
- `/lockdown [reason]` — locks the group at once, with no confirm step. Only admins can talk, approved members included, and new members are removed until the lift.
- /unlockdown — lifts the lockdown. Every permission goes back exactly as it was, and the people I removed are unbanned, except the accounts banned for good with Ban recent joiners.
- /lockdownstatus — shows whether the group is locked, since when, by whom and why. Any admin can use it.
- `/autolockdown [joins|flood|ai] [N|on|off]` — shows or changes the automatic triggers. Any admin can look; only the group owner or an admin who can restrict members can change them. A number from 3 to 500 sets the threshold and turns the trigger on.

**Automatic lockdown**
I lock the group on my own when a raid starts:
- join surge — 15 people join within 60 seconds;
- flood — 30 messages, photos, GIFs or stickers arrive within 30 seconds, at least 60% of them from at least 3 members who joined in the last 24 hours;
- AI — suspicious text from members who joined in the last 24 hours is checked by TypeSafe, their photos, GIFs, stickers and video thumbnails by Anthropic Claude (if set up). With /aispam on, every message it deletes counts too, from any member. 5 spam verdicts from 3 accounts within 120 s lock the group; one verdict never does.
Those numbers are the defaults; `/autolockdown` shows this group's own. Join surge and flood are on by default. Admins, approved members and commands I handle never count, so approving trusted newcomers keeps them out of it. I post a short notice when I lock the group, and the alert says which trigger fired. For 10 minutes after a lift I do not lock again on my own. If I see a raid but cannot lock, I warn the Staff Group and the log channel, at most once a day.

**Alert buttons**
- 🔓 Lift lockdown — lifts the lockdown like /unlockdown.
- 🚫 Ban recent joiners — bans the accounts of the burst for good, so they cannot rejoin; I cannot undo it. Admins, approved members and people an admin let in are left out. A later press bans only new accounts.
- ⛔ Revoke link — revokes the invite link the raid used, if I created it; revoke other admins' links in Telegram. One revoke per lockdown; if the raid moves to another link I made, revoke it in the group's settings in Telegram.
Lift and Ban need the group owner or an admin who can restrict members, Revoke an admin who can invite users; I check that live, and being in the Staff Group is not enough. Every button asks for a confirmation that only the person who pressed it can give. Press as yourself, not as an anonymous admin or a channel.

**Notes**
- Only the group owner, or an admin who can restrict members, can use these commands, and I check that live. Anonymous admins confirm who they are first.
- I must be an admin who can restrict members, and the group must be a supergroup.
- The bans I place on joiners last 330 days from each join. If a lockdown lasts longer, those bans run out and the people can come back; the lift summary says how many.
- Join requests are declined while the group is locked; people can ask again after the lift.
- When the group is locked, I post an alert to its Staff Group (if linked) and its log channel (if the admin category is on): the trigger, how many joined just before and since, up to 10 of those accounts, and the invite link most of them used. While the group stays locked I post a reminder every hour.

**Example**
`/lockdown spam wave` — locks the group with the reason "spam wave".

## Module Aliases

This module can be accessed using the following aliases:

- `lockdown`
- `unlockdown`
- `lockdownstatus`
- `autolockdown`

