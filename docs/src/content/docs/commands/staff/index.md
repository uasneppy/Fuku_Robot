---
title: Staff Commands
description: Complete guide to Staff module commands and features
---

# 📦 Staff Commands

A Staff Group is one group of trusted admins that manages your other groups.

### Group owner:
- `/setstaff`: Make this group your Staff Group (group creator only).
- `/unsetstaff`: Remove Staff status and unlink every group (group creator only).
- `/linkstaff [ID]`: Link this group to your Staff Group (group creator only). Add the Staff Group chat ID when you own more than one.
- `/unlinkstaff`: Unlink this group from its Staff Group (creator of both groups only).

### In the Staff Group:
- `/staff`: Show the Staff Group chat ID and its linked groups. Press Unlink next to a group to unlink it (creator of both groups only). Press Recent actions to see past staff actions of this Staff Group and how each group went.

### Staff actions (in the Staff Group):
- `/ban user [duration] [reason]`: Ban the user in every linked group. The user is a numeric ID, an @username I have seen, or a mention. The duration is a number followed by m, h, d or w, and more than 366 days means permanent.
- `/tban user duration [reason]`: Temporary ban. The duration is required.
- `/mute user [duration] [reason]`: Mute the user in every linked group. The duration works like /ban.
- `/tmute user duration [reason]`: Temporary mute. The duration is required.
- `/kick user [reason]`: Remove the user from every linked group. They can rejoin.
- `/unban user`: Lift a ban in every linked group.
- `/unmute user`: Lift a mute in every linked group.

Every action shows a card first. Only the person who sent the command can press Confirm or Cancel, and the card expires after 5 minutes.
I act only in linked groups where you are an admin who can restrict members right now, never against a group's admins or owner, and the result lists every group as done, skipped or failed.
Every finished summary has an "↩ Undo everywhere" button, also offered in Recent actions. Any member of the Staff Group may press it, the person who pressed it confirms, and each group is put back as it was only where that person can restrict members right now. Groups changed since are left alone. A kick cannot be undone.
Each applied action and each undo is posted to the log channel of the group it applied to (admin log category).
/sban, /dban, /skick, /dkick, /smute and /dmute are not used here.
Send these commands as yourself, not anonymously.


## Module Aliases

This module can be accessed using the following aliases:

- `staff`
- `setstaff`
- `unsetstaff`
- `linkstaff`
- `unlinkstaff`

## Available Commands

| Command | Description | Disableable |
|---------|-------------|-------------|
| `/setstaff` | Make this group your Staff Group (group creator only). | ❌ |
| `/staff` | Show the Staff Group chat ID and its linked groups. Press Unlink next to a group to unlink it (creator of both groups only). Press Recent actions to see past staff actions of this Staff Group and how each group went. | ❌ |
| `/unsetstaff` | Remove Staff status and unlink every group (group creator only). | ❌ |

## Usage Examples

### Basic Usage

```text
/setstaff
/staff
/unsetstaff
```

For detailed command usage, refer to the commands table above.

## Required Permissions

Commands in this module are available to all users unless otherwise specified.

