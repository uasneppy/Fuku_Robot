---
title: Federations Commands
description: Complete guide to Federations module commands and features
---

# 🛡️ Federations Commands

Federations share one ban list across the groups you own. One ban, every facility.

### Owner (in private):
- `/newfed <name>`: Create a federation (one per user).
- `/renamefed <name>`: Rename your federation.
- `/delfed`: Delete your federation (confirm).
- `/fedpromote /feddemote`: Manage fed admins.
- `/fedreason on/off`: Require a reason for fbans.
- `/fednotif on/off`: Tell the owner about fbans in private.
- `/subfed /unsubfed /fedsubs`: Subscribe to another fed (max 5).
- `/fbanlist [csv|json|minicsv]`: Export fbans.
- `/importfbans`: Import from a replied CSV/JSON file.
- `/unsetfedlog`: Clear the federation log chat.

### Federation log chat (owner):
- `/setfedlog`: In a group, make that group the federation log (you must be an admin there).
- For a channel: post /setfedlog <fed_id> in the channel (I must be an admin there), then forward that post to me in private or to a group, from the owner account, within 1 hour.

### Group owner:
- `/joinfed <fed_id>`: Join this chat to a federation.
- `/leavefed`: Leave the federation.
- `/quietfed on/off`: Suppress passive fban notices.

### Fed admins:
- `/fban /unfban /funban`: Federation ban/unban.
- `/fedstat /fbanstat`: Check a user's fban.

### Anyone:
- `/fedinfo /fedadmins /chatfed /myfeds`


## Module Aliases

This module can be accessed using the following aliases:

- `fed`
- `feds`
- `fban`
- `unfban`
- `newfed`
- `joinfed`

## Available Commands

| Command | Description | Disableable |
|---------|-------------|-------------|
| `/chatfed` | No description available | ❌ |
| `/delfed` | Delete your federation (confirm). | ❌ |
| `/fban` | Federation ban/unban. | ❌ |
| `/fbanlist` | Export fbans. | ❌ |
| `/fbanstat` | Check a user's fban. | ❌ |
| `/fedadmins` | No description available | ✅ |
| `/feddemote` | Manage fed admins. | ❌ |
| `/feddemoteme` | No description available | ❌ |
| `/fedinfo` | No description available | ✅ |
| `/fednotif` | Tell the owner about fbans in private. | ❌ |
| `/fedpromote` | Manage fed admins. | ❌ |
| `/fedreason` | Require a reason for fbans. | ❌ |
| `/fedstat` | Check a user's fban. | ✅ |
| `/fedsubs` | Subscribe to another fed (max 5). | ✅ |
| `/funban` | Federation ban/unban. (Aliases: `unfban, funban`) | ❌ |
| `/importfbans` | Import from a replied CSV/JSON file. | ❌ |
| `/joinfed` | Join this chat to a federation. | ❌ |
| `/leavefed` | Leave the federation. | ❌ |
| `/myfeds` | No description available | ❌ |
| `/newfed` | Create a federation (one per user). | ❌ |
| `/quietfed` | Suppress passive fban notices. | ❌ |
| `/renamefed` | Rename your federation. | ❌ |
| `/setfedlog` | In a group, make that group the federation log (you must be an admin there). | ❌ |
| `/subfed` | Subscribe to another fed (max 5). | ❌ |
| `/unfban` | Federation ban/unban. (Aliases: `unfban, funban`) | ❌ |
| `/unsetfedlog` | Clear the federation log chat. | ❌ |
| `/unsubfed` | Subscribe to another fed (max 5). | ❌ |

## Usage Examples

### Basic Usage

```text
/chatfed
/delfed
/fban
```

For detailed command usage, refer to the commands table above.

## Required Permissions

Most commands in this module require **admin permissions** in the group.

**Bot Permissions Required:**

- Delete messages
- Ban users
- Restrict users
- Pin messages (if applicable)

