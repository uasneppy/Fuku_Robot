---
title: Callback Queries
description: Complete reference of inline button callback handlers
---
<!-- MANUALLY MAINTAINED: do not regenerate -->

# Callback Queries

This page documents all inline button callback handlers in Fuku Robot.

## Overview

- **Total Callbacks**: 27
- **Modules with Callbacks**: 19

## Callback Data Format

Callbacks use a versioned codec with URL-encoded fields:

```
<namespace>|v1|<url-encoded-fields>
```

For example: `restrict|v1|a=ban&uid=123456789` routes to the `restrict` namespace handler.

When the payload has no fields, `_` is used as placeholder: `helpq|v1|_`.

**Maximum length**: 64 bytes (Telegram's `callback_data` limit).

### Strict Decoding

Handlers accept only the versioned format above. Malformed payloads, unsupported
versions, and legacy unversioned dot-notation payloads are rejected.

## All Callbacks

| Module | Prefix | Handler |
|--------|--------|----------|
| Backup | `backup` | backupCallbackHandler |
| Bans | `restrict` | restrictButtonHandler |
| Bans | `unrestrict` | unrestrictButtonHandler |
| Blacklists | `rmAllBlacklist` | buttonHandler |
| Bot Updates | `anon_admin` | verifyAnonymousAdmin |
| AntiRaid | `antiraid` | antiraidRetiredCallback |
| Approvals | `rmAllApprovals` | unapproveAllCallback |
| Captcha | `captcha_refresh` | captchaRefreshCallback |
| Captcha | `captcha_verify` | captchaVerifyCallback |
| Connections | `connbtns` | connectionButtons |
| Filters | `filters_overwrite` | filterOverWriteHandler |
| Filters | `rmAllFilters` | filtersButtonHandler |
| Formatting | `formatting` | formattingHandler |
| Greetings | `join_request` | joinRequestHandler |
| Help | `about` | about |
| Help | `configuration` | botConfig |
| Help | `helpq` | helpButtonHandler |
| Languages | `change_language` | langBtnHandler |
| Member requirements | `mreq` | memberReqFixedCallback |
| Notes | `notes.overwrite` | noteOverWriteHandler |
| Notes | `rmAllNotes` | notesButtonHandler |
| Pins | `unpinallbtn` | unpinallCallback |
| Purges | `deleteMsg` | deleteButtonHandler |
| Reactions | `reactions_help` | reactionsHelpHandler |
| Reports | `report` | markResolvedButtonHandler |
| Settings | `settings` | settingsCallback |
| Warns | `rmAllChatWarns` | warnsButtonHandler |
| Warns | `rmWarn` | rmWarnButton |

## Callbacks by Module

### Backup

#### `backup`

- **Handler**: `backupCallbackHandler`
- **Source**: `backup.go`

Handles backup import/export operations via inline buttons.

### Bans

#### `restrict`

- **Handler**: `restrictButtonHandler`
- **Source**: `bans.go`

#### `unrestrict`

- **Handler**: `unrestrictButtonHandler`
- **Source**: `bans.go`

### Blacklists

#### `rmAllBlacklist`

- **Handler**: `buttonHandler`
- **Source**: `blacklists.go`

### Bot Updates

#### `anon_admin`

- **Handler**: `verifyAnonymousAdmin`
- **Source**: `bot_updates.go`

Handles anonymous admin verification callbacks for channel messages.

### AntiRaid

#### `antiraid`

- **Handler**: `antiraidRetiredCallback`
- **Source**: `antiraid.go`

Retired. The old raid mode is gone, so a press on one of its buttons is answered once with a pointer to `/lockdown` and `/autolockdown` and changes nothing. The raid mode's buttons are not offered any more; this handler only answers old messages still showing them.

### Approvals

#### `rmAllApprovals`

- **Handler**: `unapproveAllCallback`
- **Source**: `approvals.go`

Handles confirmation callback for removing all approved users.

### Captcha

#### `captcha_refresh`

- **Handler**: `captchaRefreshCallback`
- **Source**: `captcha.go`

#### `captcha_verify`

- **Handler**: `captchaVerifyCallback`
- **Source**: `captcha.go`

### Connections

#### `connbtns`

- **Handler**: `connectionButtons`
- **Source**: `connections.go`

### Filters

#### `filters_overwrite`

- **Handler**: `filterOverWriteHandler`
- **Source**: `filters.go`

#### `rmAllFilters`

- **Handler**: `filtersButtonHandler`
- **Source**: `filters.go`

### Formatting

#### `formatting`

- **Handler**: `formattingHandler`
- **Source**: `formatting.go`

### Greetings

#### `join_request`

- **Handler**: `joinRequestHandler`
- **Source**: `greetings.go`

### Help

#### `about`

- **Handler**: `about`
- **Source**: `help.go`

#### `configuration`

- **Handler**: `botConfig`
- **Source**: `help.go`

#### `helpq`

- **Handler**: `helpButtonHandler`
- **Source**: `help.go`

### Languages

#### `change_language`

- **Handler**: `langBtnHandler`
- **Source**: `language.go`

### Member requirements

#### `mreq`

- **Handler**: `memberReqFixedCallback`
- **Source**: `memberreq_fixed.go`

The I fixed it button of a requirement mute notice. Fields: exactly `a=f`, `h` the hold row ID and `u` the held member,
positive whole numbers of at most 19 digits; anything else is an invalid request, refused before any lookup. Only the
member `u` may press it, and an administrator who presses it is refused (admins lift a mute with `/unmute`). The hold is
matched by its ID, the chat and the member, the member's live record must carry the requirements' own mute marker (an
admin's, staff, captcha, antiflood or warn-limit mute is never lifted), the live names are judged again against the
group's current settings, and the member is unmuted with the group's own permissions (the stored pre-lockdown
permissions during a lockdown). The query is answered exactly once, in the group's language.

The order of a press is fixed and each step refuses before the next runs. After the live marker check (step 4) comes
step 4b: the approvals are read, and an approved member is judged clean for every rule, so they are freed without a
settings read or a lookup (an unreadable approval is a "could not check" answer and changes nothing). Otherwise step 5
judges every enabled rule again: the name rules on the live record, and the profile photo and channel rules with fresh
lookups that bypass the positive caches, under a 5 second budget of the press's 10 seconds. The AI rule is judged with
the live names too but always answers from its verdict cache: for unchanged names the press gives the same answer every
time with no request, and only a renamed member costs one request, through the rule's breaker, gate and hourly budgets. An
AI that is not configured is inert, and an answer the AI cannot give is "could not check". A rule still broken beats a
lookup that cannot be answered: one to four broken rules are named in the answer, five or more (up to eight) are counted,
and with nothing broken an unknown lookup answers "could not check" and changes nothing. A channel lookup that cannot be answered
for a bot that is definitely not an administrator of the channel makes the channel rule inert (not broken).

### Notes

#### `notes.overwrite`

- **Handler**: `noteOverWriteHandler`
- **Source**: `notes.go`

#### `rmAllNotes`

- **Handler**: `notesButtonHandler`
- **Source**: `notes.go`

### Pins

#### `unpinallbtn`

- **Handler**: `unpinallCallback`
- **Source**: `pins.go`

### Purges

#### `deleteMsg`

- **Handler**: `deleteButtonHandler`
- **Source**: `purges.go`

### Reactions

#### `reactions_help`

- **Handler**: `reactionsHelpHandler`
- **Source**: `reactions.go`

Displays help information for reaction commands.

### Reports

#### `report`

- **Handler**: `markResolvedButtonHandler`
- **Source**: `reports.go`

### Settings

#### `settings`

- **Handler**: `settingsCallback`
- **Source**: `settings_menu.go`, `settings_pages.go`, `settings_edit.go`, `settings_edit_actions.go`, `settings_edit_preview.go`

The settings menu's buttons. Fields: `a` the action, `c` the group's chat ID (a menu opened in private) or `u` the opener's
user ID (a menu opened in the group), never both; `p` the page; `t` the control or the part; `v` the absolute target
value. A press with an unknown field is refused before any lookup.

The message editor's pages and codes:

| Page (`p`) | Part codes (`t`) | Controls (`t`) | Set | Remove | Cancel | Preview | Full preview |
|------------|------------------|----------------|-----|--------|--------|---------|--------------|
| `w` Welcome | `x` text, `m` media, `b` buttons | `e` on/off, `d` delete last, `o` greet once, `u` media under text | `ws` | `wr` | `wc` | `wv` | `wf` |
| `g` Goodbye | `x` text, `m` media, `b` buttons | `e` on/off, `d` delete last, `u` media under text | `gs` | `gr` | `gc` | `gv` | `gf` |
| `r` Rules | `x` text, `m` media, `b` buttons | `u` media under text | `rs` | `rr` | `rc` | `rv` | `rf` |

Set, Remove, Cancel and Preview carry exactly one field, `t`, a part code of the page (a missing, unknown or extra field is
an invalid request); Full preview carries none. Set works only from a private menu and waits for the owner's next private
message; in a menu opened in the group Set, Remove and Cancel write nothing and point to the private chat. A preview is
always sent to the presser's own private chat.

The Member requirements page (`m`, the Requirements page of the menu) has these controls and actions:

| Item | Code | Fields | Notes |
|------|------|--------|-------|
| Rule screen | `a=mo` | `t` a rule code (`ln` last name, `un` username, `ru` Russian, `ar` Arabic, `zh` Chinese, `ph` profile photo, `ch` channel, `ai` AI spam name) | opens the screen of one rule |
| Rule controls | `ln` `un` `ru` `ar` `zh` `ph` `ch` `ai` | `a=st`, `t`, `v` one digit `0` to `4` | `st` only (a `tg` on one is an invalid request); `0` Off, `1` Warn, `2` Kick, `3` Ban, `4` Mute |
| Confirm card | `a=my` | `t`, `v` (`2` to `4`), `e` expiry in unix minutes | the Yes of the card a Kick, Ban or Mute press draws; Off and Warn are written at once |
| Switches | `j` Check on join, `d` Delete messages | `a=tg` or `a=st`, `t`, `v` `0` or `1` | any other `v` is an invalid request |

Each control declares the actions it accepts (the `Actions` allow-list of the control), and a press with any other code is
refused before any lookup.

The page has eight rule cells in four rows, a switch row and the navigation row. Its body ends with a status block, a
list of lines saying when a rule cannot work in this group: a basic group (rules act on supergroups only), a channel rule
with no linked channel, or a linked channel whose bot is not an administrator, and an AI rule while the AI is not
configured (`ENABLE_AISPAM` off or no Anthropic key on the server); a check that could not be made shows the
unknown mark and never an error. The channel rule's screen offers `Off` alone while the group has no linked channel. A
punishing press of the channel rule (`a=st` with `v` `1` to `4`, and the card's Yes `a=my`) makes one live link check
after every other step: a group with no linked channel answers an alert, writes nothing and redraws the Off-only screen, and
a link that cannot be read answers a "could not check" alert and changes nothing. `Off` never makes the check.

The AI rule (`ai`) has a gate of its own, a pure check of the server's configuration that runs after the same steps: while
the AI is not configured its screen offers `Off` alone (with the AI line in the status slot and the stored value still
shown), and a punishing press (`a=st` with `v` `1` to `4`, or the card's Yes `a=my`) is refused with an alert that names
the rule, writes nothing and redraws the Off-only screen. `Off` is always accepted, so an administrator can clear a stored
value. The AI card says its own consequence for Kick, Ban and Mute: the rule judges a member when they join or write for
the first time and again after a name change, and members who are already in the group are judged only when they
rename, so it does not claim to act on every member at their next message. Only the first name, last name and username
are sent to Anthropic's Claude, never messages.

The Deletion page (`d`, between the Requirements and Access pages of Main) has three levels, all built from two tables (three sections: Service messages, Bot messages and Member messages), and
is not drawn or reachable in a Staff Group (a press there is answered as an invalid request and writes nothing):

| Item | Code | Fields | Notes |
|------|------|--------|-------|
| Hub | `a=op`, `p=d` | none more | Service messages, Bot messages and Member messages; Back is Main |
| Section list | `a=dl` | `t` `s` (Service messages), `b` (Bot messages) or `m` (Member messages) | one cell per row; Back is the hub |
| Row screen | `a=dr` | `t` a row code | `sj` joined, `sl` left, `sp` photo, `sh` title, `sn` pinned, `sv` video chat, `sf` forum topic, `sb` boost, `sa` auto-delete timer, `sk` background, `sg` giveaway, `so` other (service); `bw` welcome, `bg` goodbye, `br` rules, `ba` automatic punishments, `bm` manual punishments (bot); `mc` Commands, `me` Edited messages, `ms` Self-destruct (member); Back is the row's list |
| Choice | `a=st`, `p=d` | `t` a row code, `v` `n` or a preset | `n` is Keep on a service row and Off on a bot row; a service row's presets are `0` (Delete now), `10`, `60`, `600`, `3600`, `21600`, `86400` seconds, a bot row's `60`, `600`, `3600`, `21600`, `86400`; Commands `0` (On) only, Edited messages (the grace) `30`, `60`, `300`, `900`, `3600`, Self-destruct `60`, `300`, `600`, `3600`, `21600`, `86400`; `tg` is refused |
| Self-destruct Yes | `a=dy`, `p=d` | `t` `ms`, `v` one of its presets (never `n`), `e` expiry in unix minutes | the Yes of the Self-destruct card |

Every choice of the Service and Bot sections, and Commands and Edited messages, writes at once: there is no confirmation card and no `e` field. The one card is Self-destruct's: choosing a delay writes nothing and draws a two-tap card (`[Yes | No]`), its Yes `a=dy` writes after the whole press pipeline again (an expired Yes, or one from a presser who lost the change-info right, writes nothing), and Off is one tap with no card and cancels the waiting deletions. A stored delay a member rule does not accept reads as Off. `v` is the canonical decimal seconds of one of the
row's own presets (no sign, space, non-ASCII digit or leading zero); anything else, a missing or extra field, is an invalid
request refused before any lookup. A press of the stored value writes nothing. Joined and left share one flag with
`/cleanservice`: Keep clears it and removes both delay rows, Delete now or After N turns it on. The largest button of the
page is 57 bytes (the card's Yes; 53 in a group menu, every choice at most 50). Every deletion screen carries one line about the bot's right to delete (nothing when it can, a warning
when it cannot, an unknown mark when that could not be read); the rule can still be set while the line shows.

A switch is pressed with the toggle action `tg`: its `t` must be a control of the page (`e`, `d`, `o` or `u` as the table
lists them) and its `v` the absolute target, `0` or `1`; anything else is an invalid request, refused before any lookup.
Media under text (`u`) is drawn only while the stored media is a photo, a video or a GIF; a press on a switch that is no
longer drawn writes nothing and redraws the page with a notice.

### Warns

#### `rmAllChatWarns`

- **Handler**: `warnsButtonHandler`
- **Source**: `warns.go`

#### `rmWarn`

- **Handler**: `rmWarnButton`
- **Source**: `warns.go`

## Code Example

### Encoding and Decoding Callback Data

```go
// Encode callback data
data, err := callbackcodec.Encode("restrict", map[string]string{
    "a":   "ban",
    "uid": "123456789",
})
// -> "restrict|v1|a=ban&uid=123456789"

// Empty payload
data, err := callbackcodec.Encode("helpq", map[string]string{})
// -> "helpq|v1|_"

// Decode callback data
decoded, err := callbackcodec.Decode("restrict|v1|a=ban&uid=123456789")
namespace := decoded.Namespace  // "restrict"
action, _ := decoded.Field("a") // "ban"
uid, _ := decoded.Field("uid")  // "123456789"
```

### EncodeOrFallback

```go
data := callbackcodec.EncodeOrFallback("restrict", map[string]string{
    "a": "ban",
}, "fallback_data")
```

Encodes callback data with graceful fallback. If encoding fails, returns the fallback string instead of an error.

### Registering a Callback Handler

```go
dispatcher.AddHandler(handlers.NewCallback(
    callbackquery.Prefix("restrict"),
    myModule.restrictHandler,
))
```
