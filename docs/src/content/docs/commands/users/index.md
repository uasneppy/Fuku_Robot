---
title: Users Commands
description: Complete guide to Users module commands and features
---

# 📦 Users Commands

Resident registry. I keep an automatic record of users and chats. There are no commands here: I quietly note every message sender and every chat I am active in.
### What It Tracks:
- User IDs, usernames and display names of every message sender.
- Chat IDs and names of every group I am in.
- Channel IDs, names and usernames of linked channels.
### Rate Limiting:
- User updates are limited to one per UserUpdateInterval.
- Chat updates are limited to one per ChatUpdateInterval.
Other modules, such as /info, depend on the data collected here.

**Background Tracker:**
The Users module is a passive background module with no user-facing commands. I run it quietly on every message I process, tracking every message sender and every chat I am active in.

**What It Tracks:**
- User IDs, usernames and display names of every message sender
- Chat IDs and names of every group I am in
- Channel IDs, names and usernames of linked channels

**Rate Limiting:**
- User updates: limited to one per `UserUpdateInterval`
- Chat updates: limited to one per `ChatUpdateInterval`
- This prevents excessive database writes while keeping the data reasonably current

**Passive Operation:**
- Runs as a message watcher at handler group -1 (fires before all command handlers)
- No user interaction required — fully automatic
- Other modules, such as `/info`, depend on the data this module collects


