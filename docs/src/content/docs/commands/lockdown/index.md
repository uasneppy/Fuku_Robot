---
title: Lockdown Commands
description: Complete guide to Lockdown module commands and features
---

# 📦 Lockdown Commands

A lockdown stops a raid in this group: everyone but the admins is muted until an admin lifts it.

- `/lockdown [reason]`: Lock this group. Only admins can talk, approved users included, and new members are removed until the lift.
- `/unlockdown`: Lift the lockdown. Every permission goes back exactly as it was before the lockdown, and the people I removed are unbanned.
- `/lockdownstatus`: Show whether this group is locked, since when, by whom and why (any admin).

Only the group owner, or an admin who can restrict members, can use these commands, and I check that live. I must be an admin who can restrict members, and the group must be a supergroup.
A lockdown never ends on its own. It stays until an admin lifts it.
The bans I place on joiners last 330 days from each join. If a lockdown lasts longer than that, those bans run out by themselves and the people can come back; the lift summary says how many.
Anonymous admins are asked to confirm who they are first.
Join requests are declined while the group is locked; people can ask again after the lift.


## Module Aliases

This module can be accessed using the following aliases:

- `lockdown`
- `unlockdown`
- `lockdownstatus`

