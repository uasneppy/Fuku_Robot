---
title: Commandcenter Commands
description: Complete guide to Commandcenter module commands and features
---

# 📦 Commandcenter Commands

**Command Center**

Turn an admin group into a control room: ban, unban, kick, mute or unmute a user across every chat connected to it.

**Setup**
 - /ccsetup <name>: make the current chat a command center (chat admins).
 - /ccconnect <id>: run in another chat to put it under that command center (owner only).
 - /ccdisconnect: detach the current chat.
 - /ccdelete: delete the command center (owner only).
 - /cclist: show the chats being managed.

**Moderation**
Run these inside the command center chat, replying to a user or naming them:
 - /ccban, /ccunban, /cckick, /ccmute, /ccunmute

Admins of the connected chats are never targeted, and the bot must be an admin with restrict rights in each chat.

## Available Commands

| Command | Description | Disableable |
|---------|-------------|-------------|
| `/ccconnect` | run in another chat to put it under that command center (owner only). | ❌ |
| `/ccdelete` | delete the command center (owner only). | ❌ |
| `/ccdisconnect` | detach the current chat. | ❌ |
| `/cclist` | show the chats being managed. | ❌ |
| `/ccsetup` | make the current chat a command center (chat admins). | ❌ |

## Usage Examples

### Basic Usage

```text
/ccconnect
/ccdelete
/ccdisconnect
```

For detailed command usage, refer to the commands table above.

## Required Permissions

Commands in this module are available to all users unless otherwise specified.

