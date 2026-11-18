---
title: Federations Commands
description: Complete guide to Federations module commands and features
---

# 🛡️ Federations Commands

**🏛 Federations**

A federation shares one ban list across the groups you own. One ban, every facility.

**Federation owner commands (in private)**
- /newfed `<name>` — creates a federation (one per user).
- /renamefed `<name>` — renames your federation.
- /delfed — deletes your federation, after a confirmation.
- /fedpromote and /feddemote — manage the federation's admins.
- /fedreason `on/off` — requires a reason for fbans.
- /fednotif `on/off` — tells the owner about fbans in private.
- /subfed, /unsubfed and /fedsubs — subscribe to another federation (up to 5).
- /fbanlist `[csv|json|minicsv]` — exports the fbans.
- /importfbans — imports fbans from the CSV or JSON file you replied to.
- /unsetfedlog — clears the federation log chat.

**Federation log chat**
- /setfedlog — in a group, makes that group the federation log; you must be an admin there.
- For a channel, post `/setfedlog <fed_id>` in it (I must be an admin there), then within 1 hour forward that post to me in private or to a group, from the federation owner's account.

**Group owner commands**
- /joinfed `<fed_id>` — joins this group to a federation.
- /leavefed — takes the group out of the federation.
- /quietfed `on/off` — hides passive fban notices.

**Federation admin commands**
- /fban, /unfban and /funban — ban and unban in the federation.
- /fedstat and /fbanstat — show a user's fbans.

**Commands for everyone**
- /fedinfo, /fedadmins, /chatfed and /myfeds — show information about federations.

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
| `/delfed` | No description available | ❌ |
| `/fban` | No description available | ❌ |
| `/fbanlist` | No description available | ❌ |
| `/fbanstat` | No description available | ❌ |
| `/fedadmins` | No description available | ✅ |
| `/feddemote` | No description available | ❌ |
| `/feddemoteme` | No description available | ❌ |
| `/fedinfo` | No description available | ✅ |
| `/fednotif` | No description available | ❌ |
| `/fedpromote` | No description available | ❌ |
| `/fedreason` | No description available | ❌ |
| `/fedstat` | No description available | ✅ |
| `/fedsubs` | No description available | ✅ |
| `/funban` | No description available (Aliases: `unfban, funban`) | ❌ |
| `/importfbans` | No description available | ❌ |
| `/joinfed` | No description available | ❌ |
| `/leavefed` | No description available | ❌ |
| `/myfeds` | No description available | ❌ |
| `/newfed` | No description available | ❌ |
| `/quietfed` | No description available | ❌ |
| `/renamefed` | No description available | ❌ |
| `/setfedlog` | No description available | ❌ |
| `/subfed` | No description available | ❌ |
| `/unfban` | No description available (Aliases: `unfban, funban`) | ❌ |
| `/unsetfedlog` | No description available | ❌ |
| `/unsubfed` | No description available | ❌ |

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

