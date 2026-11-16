---
title: Users Commands
description: Complete guide to Users module commands and features
---

# 📦 Users Commands

Resident registry. An automatic background tracker of users and chats. This module has no commands: it quietly records every message sender and every chat the bot is active in.
### What It Tracks:
- User IDs, usernames and display names of every message sender.
- Chat IDs and names of every group the bot is in.
- Channel IDs, names and usernames of linked channels.
### Rate Limiting:
- User updates are limited to one per UserUpdateInterval.
- Chat updates are limited to one per ChatUpdateInterval.
Other modules, such as /info, depend on the data this module collects.

**Background Tracker:**
The Users module is a passive background module with no user-facing commands. It runs quietly on every message the bot processes, tracking every message sender and every chat the bot is active in.

**What It Tracks:**
- User IDs, usernames and display names of every message sender
- Chat IDs and names of every group the bot is in
- Channel IDs, names and usernames of linked channels

**Rate Limiting:**
- User updates: limited to one per `UserUpdateInterval`
- Chat updates: limited to one per `ChatUpdateInterval`
- This prevents excessive database writes while keeping the data reasonably current

**Passive Operation:**
- Runs as a message watcher at handler group -1 (fires before all command handlers)
- No user interaction required — fully automatic
- Other modules, such as `/info`, depend on the data this module collects


