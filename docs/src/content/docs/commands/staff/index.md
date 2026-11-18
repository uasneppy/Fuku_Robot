---
title: Staff Commands
description: Complete guide to Staff module commands and features
---

# 📦 Staff Commands

**👥 Staff Group**

A Staff Group is one group of trusted admins that manages your other groups.

**Group owner commands**
- /setstaff — makes this group your Staff Group (group creator only).
- /unsetstaff — removes Staff status and unlinks every group (group creator only).
- /linkstaff `[ID]` — links this group to your Staff Group (group creator only). Add the Staff Group's chat ID when you own more than one.
- /unlinkstaff — unlinks this group from its Staff Group (creator of both groups only).

**In the Staff Group**
- /staff — shows the Staff Group's chat ID and its linked groups. Unlink next to a group unlinks it (creator of both groups only). Recent actions shows past staff actions of this Staff Group and how each group went. A 🔒 line marks a linked group in lockdown; its admins can see why with /lockdownstatus there.
- When a linked group is locked down, its alert and hourly reminders appear here, with Lift, Ban recent joiners and Revoke link buttons that only that group's admins can use.

**Staff actions (in the Staff Group)**
- /ban `<user> [duration] [reason]` — bans the user in every linked group.
- /tban `<user> <duration> [reason]` — a temporary ban; the duration is required.
- /mute `<user> [duration] [reason]` — mutes the user in every linked group.
- /tmute `<user> <duration> [reason]` — a temporary mute; the duration is required.
- /kick `<user> [reason]` — removes the user from every linked group; they can rejoin.
- /unban `<user>` — lifts a ban in every linked group.
- /unmute `<user>` — lifts a mute in every linked group.

**Forward blocklist (in the Staff Group)**
- /addfwdblock `<source>` — blocks forwards from a channel (ID or @username) in every linked group: I delete them and apply the list's action to the member. Only the Staff Group's creator changes the list.
- /rmfwdblock `<ID>` — unblocks a source.
- /fwdblocklist — shows the blocked sources.
- /fwdblockaction `[action]` — shows or sets the action: `none` (delete only), `warn`, `mute`, `kick` or `ban`.
A forward from a group is caught only when an anonymous admin posted it.

**Notes**
- The user is a numeric ID, an @username I have seen, or a mention. The duration is a number followed by `m`, `h`, `d` or `w`; more than 366 days means permanent.
- Every action shows a card first. Only the person who sent the command can press Confirm or Cancel, and the card expires after 5 minutes.
- I act only in linked groups where you are an admin who can restrict members right now, never against a group's admins or owner, and the result lists every group as done, skipped or failed.
- Every finished summary has an "↩ Undo everywhere" button, also offered in Recent actions. Any member of the Staff Group may press it, the person who pressed it confirms, and each group is put back as it was only where that person can restrict members right now. Groups changed since are left alone. A kick cannot be undone. If an undo cannot change any group, nothing is used up and someone who can may press Undo again. Once an undo has tried to change a group, the action cannot be undone again.
- Each applied action and each undo is posted to the log channel of the group it applied to (the `admin` category).
- /sban, /dban, /skick, /dkick, /smute and /dmute are not used here. Send these commands as yourself, not anonymously.

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
| `/setstaff` | No description available | ❌ |
| `/staff` | No description available | ❌ |
| `/unsetstaff` | No description available | ❌ |

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

