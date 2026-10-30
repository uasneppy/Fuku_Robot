---
title: Antiraid Commands
description: Complete guide to Antiraid module commands and features
---
<!-- MANUALLY MAINTAINED: do not regenerate -->

# 📦 Antiraid Commands

The old raid mode is retired. Raids are now caught automatically and answered with a lockdown, the same lockdown `/lockdown` starts by hand. Nobody is temp-banned by the bot's raid protection any more.

This page covers what replaced the raid mode, what the old commands do now, and how a group's old settings were carried over. For the lockdown itself see [Lockdown](/commands/lockdown/).

## What replaced the raid mode

Two automatic triggers watch every supergroup. Both are on by default, with conservative thresholds.

| Trigger | Fires when | Default |
|---------|------------|---------|
| Join surge | this many different accounts join within 60 seconds | 15 joins |
| Flood | this many messages, photos, GIFs, stickers or other items arrive within 30 seconds, at least 60% of them from members who joined in the last 24 hours, and from at least 3 such accounts | 30 items |

The windows (60 seconds, 30 seconds), the 60% share and the 3-account minimum are fixed. The thresholds are yours to change, from 3 to 500.

What does not count:

- The bot itself and approved users never count, in either trigger.
- A join does not count when an admin added the user (a live check with Telegram), or when it is only a join request.
- For a flood, admins do not count either (the cached admin list is used), and neither do Telegram's service accounts, commands, edited messages, service messages and posts sent as the group or as a linked channel.
- A join that Telegram reports twice counts once, and an album counts as one item.

When a trigger fires, the bot starts a lockdown exactly as `/lockdown` does (the bot is recorded as the one who started it), and:

- posts a short public notice in the locked group saying it was locked automatically, with the numbers behind it, and that an admin can `/unlockdown`;
- sends the usual lockdown alert to the group's Staff Group and log channel, with a trigger line such as "Join surge: 23 joins in 60 s" or "Flood: 41 messages in 30 s, 87% from new members";
- for a flood, lists the flooding accounts in the alert and in "Ban recent joiners", even if they joined hours earlier.

Several triggers firing together, or on several replicas, still give one lockdown and one alert. The flood is not deleted: the lockdown mutes everyone but the admins, which stops it.

After an admin lifts a lockdown, the automatic triggers stay quiet in that group for 10 minutes, so leftover activity does not lock it again at once. While a lockdown is active the triggers do nothing.

Automatic detection counts in Redis. Without Redis the triggers do nothing, and `/lockdown` still works. Counting runs in supergroups only.

If a raid is detected but the bot cannot lock the group (it is not a supergroup, or the bot is not an admin who can restrict members), nothing is started. The bot warns the group's Staff Group and log channel instead, at most once a day per group.

## Admin Commands:

/autolockdown: Show both triggers, whether each is on, and its threshold (any admin).
/autolockdown joins <N|on|off>: Change the join surge trigger.
/autolockdown flood <N|on|off>: Change the flood trigger.
/lockdown [reason]: Lock the group at once.
/unlockdown: Lift the lockdown.
/lockdownstatus: Show whether the group is locked.
/autoantiraid <N|off>: Alias for the join surge threshold.

A number from 3 to 500 sets the threshold and turns that trigger on. `on` and `off` switch the trigger and keep its threshold. Changing a setting needs the group owner or an admin who can restrict members, checked live with Telegram. Any admin can view the settings. Anonymous admins are asked to confirm who they are first. The change takes effect on the next join or message, on every replica.

## Retired commands

| Command | What it does now |
|---------|------------------|
| `/antiraid` | Replies with a pointer to `/lockdown` and `/autolockdown`. Changes nothing. |
| `/raidtime` | Replies with the same pointer. Changes nothing. |
| `/raidactiontime` | Replies with the same pointer. Changes nothing. |
| `/autoantiraid N` | Still works as an alias: sets the join surge threshold to N (3 to 500) and turns it on. |
| `/autoantiraid off` | Turns the join surge trigger off. The threshold is kept. |
| `/autoantiraid` | Shows both triggers, like `/autolockdown`. |

`/autoantiraid` needs the same live authority as `/autolockdown`: the group owner or an admin who can restrict members. It used to be open to any admin in the cached admin list. Buttons from the old raid mode answer with the pointer and change nothing.

## What happened to old settings

- A group whose old `/autoantiraid` threshold was on keeps that number as its join surge threshold. A number below 3 became 3, and a number above 500 became 500, so a group can never lock on its first join or two. The flood trigger is on at 30 for those groups.
- A group whose old threshold was off has the join surge on at 15, like every other group.
- The old raid duration and ban duration (`/raidtime`, `/raidactiontime`) are not read any more.
- A raid mode that was still switched on when the bot was updated is ignored, and the bot clears the leftover Redis keys once at startup. During a rolling deploy an old copy of the bot can still temp-ban until it stops.
- Backups, `/export`, `/import` and `/reset` do not carry the `/autolockdown` settings. See [Backup](/commands/backup/).

## Required Permissions

Viewing the triggers needs an admin. Changing them, and `/autoantiraid` in any form that changes something, needs the group owner or an admin who can restrict members.
