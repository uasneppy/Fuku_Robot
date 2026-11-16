<div align="center">

<a href="https://ibb.co/jP8DcHMB"><img src="https://i.ibb.co/VcLT73Sr/Profile-banner-1.jpg" alt="Profile-banner-1" border="0"></a>

<h1>Xenobot-72</h1>

<p><b>Telegram group management in Go. Moderation, raid protection, captcha. No sweat.</b></p>

<p>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go 1.26"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://github.com/PaulSonOfLars/gotgbot"><img src="https://img.shields.io/badge/built%20with-gotgbot%20v2-26A5E4?logo=telegram&logoColor=white" alt="Built with gotgbot v2"></a>
  <img src="https://img.shields.io/badge/storage-PostgreSQL%20%2B%20Redis-336791?logo=postgresql&logoColor=white" alt="PostgreSQL + Redis">
</p>

<p>
  <a href="README.uk.md">Українська</a> •
  <a href="#features">Features</a> •
  <a href="#commands">Commands</a> •
  <a href="#getting-started">Getting started</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#license">License</a>
</p>

</div>

---

I am Xenobot-72, version 72 of 74. Jaya Abadi built me to run this facility. A facility is a Telegram group. Running it means moderation, spam and raid protection, captcha, rules, notes, greetings and logs, and I run as many facilities as you give me.

I am written in Go on top of [gotgbot](https://github.com/PaulSonOfLars/gotgbot). My data lives in PostgreSQL. Redis holds the shared cache and the raid counters. Nobody has to stay up at night to watch a group, because I do not sleep.

If you run several groups, one Staff Group of trusted admins can act on all of them with one command. I check each admin's rights in each group before I do anything. No sweat. I will handle it.

## Table of contents

- [Features](#features)
- [Commands](#commands)
- [Architecture](#architecture)
- [Tech stack](#tech-stack)
- [Getting started](#getting-started)
- [Configuration](#configuration)
- [Endpoints and observability](#endpoints-and-observability)
- [Project structure](#project-structure)
- [Development](#development)
- [Documentation](#documentation)
- [Attributions](#attributions)
- [License](#license)

## Features

### Moderation

Procedure first. Rights are checked every time, mine included.

- Ban and mute, each with silent (`s`), delete-the-message (`d`) and timed (`t`) variants. Kick with silent and delete variants. Inline keyboards for restrict and unrestrict.
- Warnings with a limit (default 3, from 1 to 100) and an action at the limit: mute (the default), kick or ban.
- Purges, message deletion, promote and demote, custom admin titles, and changing the group title, photo and description.
- Approvals: trusted members skip antiflood, blacklists, locks and captcha without becoming admins.
- Anonymous admins prove who they are with one tap on a button before an admin command runs, and the command then runs as the person who tapped. `/anonadmin` shows or changes the group's AnonAdmin mode, and only the group creator can change it.

### Protection

Spam gets the same treatment as everything else: a procedure, and no opinions.

- **Antiflood.** Ban, kick or mute anyone who sends more than N messages in a row, with N from 3 to 100. `/setflood` with `0`, `off` or `no` switches it off.
- **Locks.** 22 lock types: links, media, stickers, forwards, bots, right-to-left text and more. Admins and approved members are never affected.
- **Blacklists.** An action on a match: mute, kick, ban, warn or none. With none, I only delete the message.
- **Filters.** Keyword auto-replies, at most 150 filters in one chat.
- **AI spam filter.** Off in every chat until an admin turns it on with `/aispam on`. When the model is confident, I delete the message quietly: no warn, no mute, no reply. Confident means 80 percent for English text and 90 percent for other languages. The filter does nothing without `TYPESAFE_API_KEY`, and `ENABLE_AISPAM=false` switches off every AI check I have. Deletions are mirrored to the `MESSAGE_DUMP` chat.
- **Join captcha.** Three modes: math, text in an image, and Turnstile. New members stay muted until they pass. The defaults are a time limit of 2 minutes, kick on failure and 3 attempts, and each member gets one attempt per group. Turnstile is a Cloudflare check on a web page that opens as a Telegram Mini App. The bot's server decides the result, and the page only shows it. Math and text need none of the Turnstile setup. Without Redis the web captcha is unavailable, and a group set to Turnstile gets the math captcha instead. Setup is in [Getting started](#getting-started).

### Staff Group

A Staff Group is one group of trusted admins that manages your other groups.

The owner of a group makes it the Staff Group with `/setstaff`, sent as yourself and not anonymously. A person who owns both groups links another group to it with `/linkstaff`, and `/unlinkstaff` removes the link. A link lasts only while the person who made it is the live creator of both groups. I recheck, and I remove the link when the owner changes.

In the Staff Group, `/ban`, `/tban`, `/mute`, `/tmute`, `/kick`, `/unban` and `/unmute` act in every linked group.

- I post a confirmation card first. Only the sender can confirm it, and it expires after 5 minutes.
- In each linked group I act only where the issuer is a live admin who can restrict members, and never against that group's admins or owner.
- The result lists every group as done, skipped or failed. Nothing is dropped from the list.
- `/ban` and `/mute` take an optional duration, `/tban` and `/tmute` require one, with `m`, `h`, `d` or `w`. A duration longer than 366 days means permanent.
- `/sban`, `/dban`, `/skick`, `/dkick`, `/smute` and `/dmute` are refused in the Staff Group with a hint.
- Every finished action gets an Undo button that any Staff Group member can press. A kick cannot be undone. Each group is put back only where the person pressing it can restrict members, and only if nothing changed since.
- Each applied action and each undo is posted to the log channel of the group it applied to, under the admin category. The post names who and whom. It never names the Staff Group.
- `/staff` shows the Staff Group's chat ID, its linked groups with Unlink buttons, Recent actions (10 a page) and a lock line for any group in lockdown.

**Forward blocklist.** A Staff Group keeps one list of blocked forward sources, channels and groups, at most 200, and one action for the whole list: none, warn, mute, kick or ban. None is the default and only deletes the message. In every linked group I delete a message forwarded from a listed source and apply the action to the member who forwarded it. Only the Staff Group's live creator changes the list. Other admins of the Staff Group can view it. The list is edited with `/addfwdblock`, `/rmfwdblock` and `/fwdblockaction`, or on the Blocked sources page of the settings menu.

Stated limit: when a normal member forwards a message that another member posted in a group, Telegram labels the forward with that original sender, not with the group, so it can never match the list. Only forwards of channel posts, and of a group post made by an anonymous admin, can match. Admins, approved members, channels posting as themselves and anonymous admins posting a blocked forward are exempt.

### Lockdown and automatic raid detection

`/lockdown` locks a supergroup at once, with no confirm step. Only admins can talk, new members are removed, and join requests are declined until the lift. `/unlockdown` restores every permission exactly and unbans the people I removed, except accounts banned for good with Ban recent joiners. A lockdown never ends on its own. The bans I place on joiners last 330 days from each join.

Authority is checked live, never from the admin cache. `/lockdown` and `/unlockdown` need the live creator or an admin with the right to restrict members, and the bot must be an admin with that right.

Automatic detection starts the same lockdown. Counters live in Redis, so detection is inert without it, and `/lockdown` still works. These settings are not part of backups.

- **Join surge.** 15 different accounts joining within 60 seconds.
- **Message flood.** 30 items (messages, photos, GIFs, stickers) within 30 seconds, with at least 60 percent of them from at least 3 members who joined in the last 24 hours.
- **AI burst.** 5 spam verdicts from at least 3 accounts within 120 seconds. One verdict never locks a group, and AI never deletes or bans on this path. Suspicious text from members who joined in the last 24 hours is checked by TypeSafe. Their photos, GIFs and stickers are checked by Anthropic Claude. Each check runs only when it is configured, and without the Anthropic key no image leaves the server.
- All three are on by default, and `/autolockdown` sets each threshold from 3 to 500. Admins, approved users and commands I handle never count. After a lift I do not lock the group again automatically for 10 minutes. When I see a raid but cannot lock the group, I warn the Staff Group and the log channel, at most once every 24 hours.

When a group is locked, I post an alert to its linked Staff Group and, when the admin log category is on, to its log channel, then an hourly reminder. The alert has three buttons: Lift, Ban recent joiners (bans for good, and cannot be undone) and Revoke link. Every press is checked live against the locked group. Being in the Staff Group is not enough.

### Member requirements

Eight rules, each with its own punishment: off, warn, kick, ban or mute. Every rule starts off, and they are set only in `/settings`, because there is no command for them. Supergroups only.

- No last name. No username.
- Russian letters (the letters `ы ъ э ё`), Arabic letters or Chinese characters in the first or last name. The name rules go by the first and last name, never the username.
- No profile photo that I can see.
- Not subscribed to the group's linked channel. This works only in a group linked to a channel, and only while I am an admin of that channel.
- A spam-like name, judged by AI. I send only the first name, last name and username to Anthropic's Claude, never messages. The rule is off when `ENABLE_AISPAM` is false or `ANTHROPIC_API_KEY` is empty.

Members are checked at their next message and, with Check on join, right after the captcha. Delete messages removes the messages of a held member. A muted member gets an "I fixed it" button that frees only that member, and only from a mute the requirements placed. A check that cannot be answered punishes nobody.

### Message deletion

Scheduled deletion lives on the Deletion page of `/settings` and keeps a chat readable. Everything starts at Keep or Off, and the queue survives a restart.

- **Service messages.** 12 kinds, among them member joined, member left, pinned message and title change. Each is kept, deleted at once, or deleted after 10 seconds, 1 minute, 10 minutes, 1 hour, 6 hours or 24 hours.
- **Bot messages.** 5 kinds: the welcome, the goodbye, the rules, automatic punishment notices, and replies to manual punishment commands. They are off by default and can be set from 1 minute to 24 hours.
- **Member messages.** 3 rules. Commands deletes every command I handle after I have answered it, an admin's included. A command meant for another bot, or an unknown word, is never touched. Edited messages deletes a member's message that is edited later than a grace of 30 seconds, 1, 5, 15 or 60 minutes after it was sent. Self-destruct deletes every message a member sends after 1, 5, 10 minutes or 1, 6, 24 hours.

A message older than 47 hours is dropped quietly, because Telegram no longer allows deleting it. Staff Group messages, lockdown alerts, captcha challenges and log posts are never deleted, and the Deletion page does not exist in a Staff Group.

### Settings menu

`/settings` opens a button menu. Only the owner of the group can open it, unless the owner lets admins in on the Access page. A menu opened in the group answers only to the person who opened it. Every press checks again that the person is still allowed, and every change is the same change the matching command makes. The menu itself keeps no state; the one exception is the ten-minute wait for a typed reply in the message editor.

Its pages: Staff Group, Lockdown, Captcha, Antiflood, Warnings, Locks, Welcome, Goodbye, Rules, Notes, Filters, Blacklist, Requirements, Deletion, Switches, Language, Log channel, Anonymous admins, Access (owner only) and Blocked sources (in a Staff Group only).

- Welcome, Goodbye and Rules take their text, media and buttons from a message you send to me in private. `/cancel` ends the wait.
- Blacklist, Filters and Notes work only in the private menu, and a menu opened in the group shows a note instead. You add an entry with Add and type it in private. Delete asks Yes or No. A filter keyword or a note name that already exists is refused, never overwritten.
- In a private chat with me, `/settings` lists the groups you manage.

### Content

Notes, filters and greetings are where I get to be friendly. Today's ration is tempe.

- Notes with `/get name` or `#name`, media included, optionally sent in private.
- Welcome and goodbye messages, with media, buttons and a switch that cleans up the previous one. Greet once welcomes each member only the first time they join.
- Rules with media, URL buttons and a custom button, pins with anti-channel-pin, automatic reactions, and a formatting guide for markdown, buttons and fillings.

### Several groups

- **Federations.** Shared ban lists across groups, with subscriptions to other federations (at most 5), federation admins, a federation log chat, and import and export of federation bans as CSV or JSON.
- **Connections.** Manage a group's settings from a private chat.
- **Log channels.** Per-group log channels with six categories: settings, admin, user, automated, reports and other.
- **Reports and disabling.** `/report` and `@admin` reports, and per-chat command disabling.

### Languages and backup

- 8 interface languages: Ukrainian (the default), English, Spanish, French, Hindi, Indonesian, Portuguese and Russian.
- `/export` writes a group's settings to a JSON file. `/import` and `/reset` work module by module. Lockdown, staff, deletion, member requirement and forward blocklist data are never part of backups.

### Operations

- Webhook or long-polling mode, `/health`, Prometheus `/metrics` and optional OpenTelemetry tracing.
- Versioned SQL migrations with a checksum (`SHA-256`) per file, verified at startup.
- A Redis read-through cache with an in-process LRU layer in front, and logs as JSON with configured secrets redacted.
- Graceful shutdown inside a 60 second budget.
- A distroless Docker image, and release builds for Linux, macOS and Windows on amd64 and arm64.

## Commands

I answer to 231 command names, aliases included, counted from the running dispatcher at release v2.24.0. `<user>` means a reply, `@username`, a mention or a numeric user ID. Times use `m`, `h` or `d`, for example `30m`, `6h`, `2d`. Send `/help <module>` in a chat for the full text of a module.

Rights are checked on every use. Each table says who may run a command.

### Moderation

<details>
<summary><b>Bans</b>: <code>/ban</code> <code>/tban</code> <code>/kick</code> <code>/restrict</code> …</summary>

Notice. Both the admin and I must hold the right to restrict members. I check each time. The silent and delete variants also need the right to delete messages, for the admin and for me. In a Staff Group the same commands act in every linked group, as described under Staff Group below.

| Command | What it does |
| --- | --- |
| `/ban <user> [reason]` | Ban a user |
| `/sban <user>` | Ban silently and delete your command |
| `/dban` | Ban the author of the replied-to message and delete that message |
| `/tban <user> <time>` | Ban for a set time |
| `/unban <user>` | Lift a ban |
| `/unbanall` | Group creator only: unban every user I know in the chat, after confirmation |
| `/kick <user>` · `/skick <user>` · `/dkick` | Kick (the user can rejoin), kick silently, or kick and delete the replied-to message |
| `/kickme` · `/banme` | Remove yourself from the chat. Any member can use them, and I must be able to restrict |
| `/restrict` · `/unrestrict` | Choose ban, kick or mute (or unban, unmute) from an inline keyboard |

</details>

<details>
<summary><b>Mutes</b>: <code>/mute</code> <code>/tmute</code> <code>/unmute</code> …</summary>

Notice. Same rights as for bans.

| Command | What it does |
| --- | --- |
| `/mute <user>` | Mute a user |
| `/smute <user>` | Mute silently and delete your command |
| `/dmute` | Mute the author of the replied-to message and delete that message |
| `/tmute <user> <time>` | Mute for a set time |
| `/unmute <user>` | Unmute |

</details>

<details>
<summary><b>Warns</b>: <code>/warn</code> <code>/warns</code> <code>/setwarnlimit</code> …</summary>

Notice. I keep the count. Issuing a warning needs the same rights as a ban.

| Command | What it does |
| --- | --- |
| `/warn <user> [reason]` · `/swarn` · `/dwarn` | Warn; warn silently and delete your command; warn and delete the replied-to message |
| `/warns <user>` | Show a user's warnings. Any member can ask |
| `/rmwarn` (`/unwarn`) | Remove a user's latest warning |
| `/resetwarns` (`/resetwarn`) | Reset a user's warnings to 0 |
| `/resetallwarns` | Delete every warning in the chat. Group creator only, with a confirmation button |
| `/warnings` | Show the chat's warn settings |
| `/setwarnlimit <n>` | Number of warnings before the action (1 to 100, default 3) |
| `/setwarnmode <ban/kick/mute>` | Action taken at the limit (default mute) |

</details>

<details>
<summary><b>Purges</b>: <code>/purge</code> <code>/del</code> …</summary>

Cleaning procedure. The admin and I both need the right to delete messages.

| Command | What it does |
| --- | --- |
| `/purge` | Delete every message from the replied-to one up to now |
| `/del` | Delete the replied-to message |
| `/purgefrom` · `/purgeto` | Mark a start message, then purge up to an end message |

</details>

<details>
<summary><b>Admin</b>: <code>/promote</code> <code>/demote</code> <code>/title</code> …</summary>

Supervisor access is granted by procedure.

| Command | What it does |
| --- | --- |
| `/adminlist` | List the chat's admins. Anyone can ask, but I must be an admin of the chat |
| `/promote <user>` · `/demote <user>` | Promote or demote a user. The admin and I both need the right to promote |
| `/title <user> <custom title>` | Set a custom admin title (same rights as promote) |
| `/setgtitle` (`/setgrouptitle`) | Change the group title |
| `/setgpic` (`/setgrouppic`) | Reply to a photo to make it the group picture |
| `/setgdesc` (`/setgroupdesc`, `/setdescription`) | Change the group description. These three need the right to change group info, for the admin and for me |
| `/invitelink` | Get the chat's invite link (the admin needs the right to invite users) |
| `/admincache` | Refresh the cached admin list |
| `/clearadmincache` | Clear the cached admin list |
| `/anonadmin <on/off>` | With no argument, show the AnonAdmin mode. Only the group creator changes it, and I must be an admin |

</details>

<details>
<summary><b>Approvals</b>: <code>/approve</code> <code>/approved</code> …</summary>

Approved members skip antiflood, blacklists, locks and captcha. I keep the list.

| Command | What it does |
| --- | --- |
| `/approve <user> [reason]` · `/unapprove <user>` | Approve or unapprove a user (admins) |
| `/approval <user>` | Check a user's approval status |
| `/approved` | List approved users |
| `/unapproveall` | Remove every approval. Group creator only |

</details>

### Protection

<details>
<summary><b>Antiflood</b>: <code>/setflood</code> <code>/setfloodmode</code> …</summary>

Flood control. I count the messages so that you do not have to. These are admin commands.

| Command | What it does |
| --- | --- |
| `/flood` | Show the current antiflood settings |
| `/setflood <number/off/no>` | Act after this many messages in a row (3 to 100). `0`, `off` or `no` disables it |
| `/setfloodmode <ban/kick/mute>` | Action for flooders |
| `/delflood` (`/clearflood`) `<yes/no/on/off>` | Also delete the flooded messages |

</details>

<details>
<summary><b>Lockdown</b>: <code>/lockdown</code> <code>/unlockdown</code> <code>/autolockdown</code> …</summary>

Notice. Authority is checked live with Telegram for the person who sent the command, never from the admin cache. Lockdowns need a supergroup. For what a lockdown does and how automatic detection works, see Lockdown and automatic raid detection under Features.

| Command | What it does |
| --- | --- |
| `/lockdown [reason]` | Lock the group at once. Needs the live creator or an admin who can restrict members, and I must be an admin who can restrict members |
| `/unlockdown` | Lift the lockdown and restore the permissions exactly. Same checks as `/lockdown` |
| `/lockdownstatus` | Show whether the group is locked, since when, by whom and why. Any live creator or admin |
| `/autolockdown [joins\|flood\|ai] [N\|on\|off]` | Show or change the automatic triggers. A number from 3 to 500 sets that threshold and turns the trigger on. Any admin can look. Only the creator or an admin who can restrict members can change |
| `/autoantiraid N\|off` | Alias for the join surge part of `/autolockdown`, with the same checks |

</details>

<details>
<summary><b>Captcha</b>: <code>/captcha</code> <code>/captchamode</code> …</summary>

New members stay muted until they solve the challenge. Math and text need no setup. Turnstile is described in Getting started.

| Command | What it does |
| --- | --- |
| `/captcha <on/off>` | Turn captcha verification on or off. The admin and I must both be admins |
| `/captchamode <math/text/turnstile>` | Challenge type (default math). `turnstile` is refused unless the web captcha is configured, Redis answers and I am an admin with the right to restrict members |
| `/captchatime <1-10>` | Timeout in minutes (default 2) |
| `/captchaaction <kick/ban/mute>` | Action on failure or timeout (default kick) |
| `/captchamaxattempts <1-10>` | Maximum attempts (default 3). I must be an admin |
| `/captchapending` · `/captchaclear` | View or clear messages held from users with a pending captcha |

</details>

<details>
<summary><b>AI spam filter</b>: <code>/aispam</code></summary>

The filter is opt-in per chat. These are the commands. The rest is under Features.

| Command | What it does |
| --- | --- |
| `/aispam` | Show whether the filter is on and exactly what is sent to the model |
| `/aispam <on/off>` | Turn the filter on or off for this chat (admins) |

</details>

<details>
<summary><b>Blacklists</b>: <code>/addblacklist</code> <code>/blaction</code> …</summary>

Words I remove on sight. You decide which. A word of one character is refused unless it is a symbol, because it would match almost every message.

| Command | What it does |
| --- | --- |
| `/blacklists` (`/blocklists`) | List the blacklisted words (anyone) |
| `/addblacklist` (`/blacklist`, `/addblocklist`, `/blocklist`) `<words>` | Add words. The admin and I both need the right to restrict members |
| `/rmblacklist` (`/rmblocklist`, `/unblacklist`) `<words>` | Remove words (same rights) |
| `/blaction` (`/blacklistaction`) `<mute/kick/ban/warn/none>` | Action on a match. `none` only deletes the message (same rights) |
| `/remallbl` (`/rmallbl`, `/rmblocklistall`, `/rmallblocklist`) | Remove every blacklisted word. Group creator only |

</details>

<details>
<summary><b>Locks</b>: <code>/lock</code> <code>/unlock</code> …</summary>

Locks delete a kind of message. Admins and approved members are never affected. `/lock` and `/unlock` need an admin, and I must be an admin too.

| Command | What it does |
| --- | --- |
| `/lock <types…>` · `/unlock <types…>` | Lock or unlock content types |
| `/locks` | Show the active locks (anyone) |
| `/locktypes` | List every lock type (anyone) |

Lock types: `all`, `messages`, `media`, `other`, `previews`, `comments`, `sticker`, `audio`, `voice`, `document`, `video`, `videonote`, `contact`, `photo`, `gif`, `url`, `bots`, `forward`, `game`, `location`, `rtl`, `anonchannel`.
See [lock types](docs/src/content/docs/api-reference/lock-types.md).

</details>

### Staff Group

<details>
<summary><b>Staff Group and forward blocklist</b>: <code>/setstaff</code> <code>/linkstaff</code> <code>/staff</code> <code>/addfwdblock</code> …</summary>

Notice. Cross-group commands are checked per group. The behaviour is described under Staff Group in Features.

| Command | What it does |
| --- | --- |
| `/setstaff` | Make this group your Staff Group. Group creator only, sent as yourself and not anonymously |
| `/unsetstaff` | Remove Staff status, unlink every group and delete the forward blocklist. Group creator only |
| `/linkstaff [ID]` | Link this group to your Staff Group. Add the Staff Group's chat ID when you own more than one. You must be the creator of both groups |
| `/unlinkstaff` | Unlink this group. You must be the creator of both groups |
| `/staff` | In the Staff Group: its chat ID, linked groups with Unlink buttons, Recent actions, and a lock line for a group in lockdown. In any other chat I stay silent |
| `/ban` · `/tban` · `/mute` · `/tmute` · `/kick` · `/unban` · `/unmute` | In the Staff Group these act in every linked group, after a confirmation card |
| `/addfwdblock <ID or @username>` · `/rmfwdblock <ID>` | Add or remove a blocked forward source, a channel or a group. The Staff Group's live creator only |
| `/fwdblocklist` | Show the list. Any admin of the Staff Group |
| `/fwdblockaction [none\|warn\|mute\|kick\|ban]` | Show or set the list's one action (default none, which only deletes). Any admin of the Staff Group can show it. Only the creator can set it |

</details>

### Content

<details>
<summary><b>Notes</b>: <code>/save</code> <code>/get</code> <code>#note</code> …</summary>

Notes. I remember so that the group does not have to. A message that starts with `#` is read as `/get`.

| Command | What it does |
| --- | --- |
| `/get <name>` · `#name` | Get a note (anyone) |
| `/save` (`/addnote`) `<name> <text>` | Save a note. Reply to a message to save it, media included. Needs the right to change group info |
| `/clear` (`/rmnote`) `<name>` | Delete a note (same right) |
| `/notes` (`/saved`) | List the notes |
| `/clearall` | Delete every note in the chat. Group creator only, with a confirmation button |
| `/privnote` (`/privatenotes`) | Send notes in a private chat through a button |

</details>

<details>
<summary><b>Filters</b>: <code>/filter</code> <code>/stop</code> …</summary>

Filters. When a trigger appears, I answer. A chat holds at most 150 filters.

| Command | What it does |
| --- | --- |
| `/filter` (`/addfilter`) `<trigger> <reply>` | Reply automatically when someone says the trigger. Quote multi-word triggers. Reply to a file to save it. Needs the right to change group info |
| `/filters` | List the filters (anyone) |
| `/stop` (`/rmfilter`, `/removefilter`) `<trigger>` | Remove a filter (same right) |
| `/stopall` | Remove every filter. Group creator only, with a confirmation |

Add `{admin}` to a reply to make the filter fire only for admins.

</details>

<details>
<summary><b>Greetings</b>: <code>/welcome</code> <code>/setwelcome</code> <code>/cleanservice</code> …</summary>

Greetings. I welcome new members and say goodbye to the ones who leave. These are admin commands, and the settings menu offers the same changes.

| Command | What it does |
| --- | --- |
| `/welcome` · `/goodbye` `<yes/no/on/off>` | Turn the welcome or goodbye message on or off |
| `/setwelcome` · `/setgoodbye` | Set the text, or reply to a message |
| `/resetwelcome` · `/resetgoodbye` | Back to the default |
| `/cleanwelcome` · `/cleangoodbye` `<yes/no/on/off>` | Delete the previous welcome or goodbye message |
| `/cleanservice <yes/no/on/off>` | Delete "x joined the group" service messages |
| `/autoapprove <yes/no/on/off>` | Approve join requests automatically |

</details>

<details>
<summary><b>Rules</b>: <code>/rules</code> <code>/setrules</code> …</summary>

Regulations. Every facility has them. The rules can carry one media item and URL buttons, set from the settings menu in private. These are admin commands, except `/rules`.

| Command | What it does |
| --- | --- |
| `/rules` | Show the chat rules (anyone) |
| `/setrules <text>` | Set the rules |
| `/resetrules` (`/clearrules`) | Clear the rules |
| `/privaterules <yes/no/on/off>` | Send the rules in a private chat |
| `/rulesbtn` (`/rulesbutton`) `<text>` | Set the rules button text |
| `/resetrulesbtn` (`/resetrulesbutton`, `/clearrulesbtn`, `/clearrulesbutton`) | Reset the button text |

</details>

<details>
<summary><b>Pins</b>: <code>/pin</code> <code>/permapin</code> …</summary>

Pinned notices stay at the top.

| Command | What it does |
| --- | --- |
| `/pinned` | Show the current pinned message (anyone, and I must be an admin) |
| `/pin [loud]` | Pin the replied-to message. `loud` or `notify` notifies members |
| `/permapin <text>` | Pin a custom message with markdown and buttons |
| `/unpin` · `/unpinall` | Unpin one or all |
| `/antichannelpin <yes/no/on/off>` | Stop Telegram from auto-pinning linked-channel posts (admins) |
| `/cleanlinked <yes/no/on/off>` | Delete messages sent by the linked channel (admins) |

Pinning needs an admin who can pin, and I must be able to pin too.

</details>

<details>
<summary><b>Reactions</b>: <code>/addreaction</code> …</summary>

I acknowledge a keyword with an emoji. Changing reactions needs the right to change group info.

| Command | What it does |
| --- | --- |
| `/addreaction <keyword> <emoji>` | React with the emoji when the keyword appears |
| `/removereaction <keyword>` | Remove a reaction |
| `/reactions` | List the reactions |
| `/resetreactions` | Clear every reaction |

</details>

<details>
<summary><b>Formatting</b>: <code>/markdownhelp</code></summary>

`/markdownhelp` (`/formatting`) opens an interactive guide to markdown, buttons and fillings such as `{first}`.

</details>

### Several groups

<details>
<summary><b>Federations</b>: <code>/newfed</code> <code>/fban</code> …</summary>

Federations share a ban list across groups. The roles below come from the shipped help text. A federation can subscribe to at most 5 others.

| Who | Commands |
| --- | --- |
| Federation owner (private chat) | `/newfed <name>`, `/renamefed <name>`, `/delfed`, `/fedpromote`, `/feddemote`, `/fedreason on/off`, `/fednotif on/off`, `/subfed`, `/unsubfed`, `/fedsubs`, `/fbanlist [csv/json/minicsv]`, `/importfbans`, `/setfedlog`, `/unsetfedlog` |
| Group owner | `/joinfed <fed_id>`, `/leavefed`, `/quietfed on/off` |
| Federation admins | `/fban`, `/unfban` (`/funban`), `/fedstat` (`/fbanstat`), `/feddemoteme` |
| Anyone | `/fedinfo`, `/fedadmins`, `/chatfed`, `/myfeds` |

</details>

<details>
<summary><b>Connections</b>: <code>/connect</code> …</summary>

Manage a group from a private chat. Connected admins edit the chat from there. Any member can view. Users who are banned or kicked cannot connect.

| Command | What it does |
| --- | --- |
| `/connect <chat id>` | Manage a group from a private chat |
| `/disconnect` · `/reconnect` | Leave the connected chat, or reconnect to the last one |
| `/connection` | Show the current connection |
| `/allowconnect <yes/no>` | Allow members to connect (admins) |

</details>

<details>
<summary><b>Log channels</b>: <code>/setlog</code> <code>/log</code> …</summary>

Everything I do gets filed. Add me as an admin of a channel, send `/setlog` there, then forward that message to the group.

| Command | What it does |
| --- | --- |
| `/setlog` · `/unsetlog` | Set or remove the log channel |
| `/logchannel` | Show the current log channel |
| `/logcategories` | List the categories: settings, admin, user, automated, reports, other |
| `/log <category>` · `/nolog <category>` | Turn a category on or off. Admins, in a group with a log channel set |

</details>

<details>
<summary><b>Reports</b>: <code>/report</code> <code>@admin</code></summary>

A report reaches the admins. Admins cannot trigger one.

| Command | What it does |
| --- | --- |
| `/report [reason]` · `@admin` | Reply to a message to report it to the admins |
| `/reports <on/off/yes/no>` | In a group, an admin turns reporting on or off. In a private chat it changes your own setting |
| `/reports block` · `unblock` · `showblocklist` | Block users from reporting and list them |

</details>

<details>
<summary><b>Disabling</b>: <code>/disable</code> <code>/enable</code> …</summary>

A disabled command stops working for non-admins only. These are admin commands.

| Command | What it does |
| --- | --- |
| `/disable <command>` · `/enable <command>` | Disable or re-enable a command |
| `/disableable` | List the commands that can be disabled |
| `/disabled` | List the disabled commands |
| `/disabledel <yes/no/on/off>` | Delete disabled commands when non-admins use them |

</details>

### Utilities

<details>
<summary><b>Misc</b>: <code>/info</code> <code>/id</code> <code>/tr</code> …</summary>

Small requests are processed too.

| Command | What it does |
| --- | --- |
| `/info [user]` | User info |
| `/id` | The chat ID, or the replied-to user's ID |
| `/ping` | Ping Telegram |
| `/stat` | Message count of the chat (groups only) |
| `/tr <lang code> <text or reply>` | Translate a message |
| `/tell <text>` | Admins: I reply to the replied-to message with your text |
| `/stickerid` | File ID of the replied-to sticker |
| `/zombies` | Kick the deleted accounts I know about. The admin needs the right to restrict members, and so do I |
| `/github <user[/repo]>` · `/wiki <query>` · `/ud <term>` | GitHub, English Wikipedia and Urban Dictionary lookups |
| `/removebotkeyboard` · `/runs` | Remove a stuck bot keyboard; a random quip |

</details>

<details>
<summary><b>Backup, languages, help</b>: <code>/export</code> <code>/lang</code> <code>/help</code> …</summary>

Backups are rate limited per chat: `/export` once every 5 minutes, `/import` once every 10 minutes and `/reset` once an hour.

| Command | What it does |
| --- | --- |
| `/export [modules…]` | Export the group's settings to JSON. An admin, and I must be an admin |
| `/import [modules…]` | Restore from a replied-to backup file. Group creator only, and I must be an admin |
| `/reset [modules…]` | Reset settings to their defaults. Group creator only, and I must be an admin |
| `/lang` | In a private chat, choose your own language. In a group, an admin sets the group's language |
| `/start` · `/help [module]` · `/about` · `/donate` | Navigation, help, bot info, donation info |

</details>

<details>
<summary><b>Dev</b>: bot owner and team only</summary>

Request denied for everyone else.

| Command | What it does |
| --- | --- |
| `/stats` | Bot statistics. Bot owner or a dev |
| `/chatinfo` · `/chatlist` · `/leavechat` | Inspect chats, list active chats, leave a chat. Bot owner or a dev |
| `/teamusers` | List the team. Bot owner or the team |
| `/addsudo` · `/remsudo` · `/adddev` · `/remdev` | Manage the team. Bot owner only |

</details>

<details>
<summary><b>Settings</b>: <code>/settings</code> <code>/cancel</code></summary>

The button menu is described under Settings menu in Features.

| Command | What it does |
| --- | --- |
| `/settings` | Open the settings menu. In a private chat with me it lists the groups you manage. In a group only the owner can open it, unless the owner lets admins in |
| `/cancel` | In a private chat with me, end the wait for a typed reply of the settings menu. In any other chat it is not my command and passes through |

</details>

### Retired

<details>
<summary><b>Raid mode</b>: <code>/antiraid</code> <code>/raidtime</code> <code>/raidactiontime</code></summary>

Retired. These three commands are one pointer to Lockdown. They change nothing, and nobody is temporarily banned by a raid mode any more. Automatic detection replaces it. Tune detection with `/autolockdown`, or with `/autoantiraid` for the join surge threshold.

| Command | What it does |
| --- | --- |
| `/antiraid` · `/raidtime` · `/raidactiontime` | Reply with a pointer to Lockdown and change nothing. A press on an old raid mode button gets the same reply |

</details>

`Users` (records users, chats and channels) and `BotUpdates` (the bot joining a group, admin-cache updates, anonymous-admin checks) are internal modules with no commands.

## Architecture

```mermaid
flowchart TD
    TG["Telegram Bot API<br/>api.telegram.org or API_SERVER"]

    subgraph Delivery["Update delivery"]
        WH["HTTP server on HTTP_PORT<br/>POST /webhook, secret-checked<br/>/captcha routes only when configured"]
        LP["gotgbot Updater<br/>long polling"]
    end

    TG -->|"USE_WEBHOOKS=true"| WH
    TG -->|"USE_WEBHOOKS=false"| LP
    WH --> D
    LP --> D

    D["gotgbot Dispatcher<br/>DISPATCHER_MAX_ROUTINES workers<br/>tracing and command deletion processors"]

    subgraph Groups["Handler groups, run in order"]
        direction TB
        G1["-10 captcha sweeper · -9 forward blocklist · -8 member deletion<br/>-7 lockdown join guard · -6 fed-ban · -5 service message deletion<br/>-4 raid flood counter · -3 staff watchers<br/>-2 admin cache, member requirements · -1 user tracker"]
        G2["0 commands, help, greetings · 1 settings typed reply · 2 raid AI watcher"]
        G3["3 AI spam · 4 antiflood · 5/6 locks · 7 blacklists · 8 reports, reactions<br/>9 filters · 10 pins · 11 log channel · 12 late raid flood counter"]
        G1 --> G2 --> G3
    end

    D --> Groups
    Groups --> M["fuku/modules<br/>modules register in init(), help loads last"]
    M --> R["fuku/db repositories"]
    R -->|"reads"| L["In-process LRU<br/>CACHE_LOCAL_TTL"]
    L -->|"miss"| RD[("Redis<br/>cache, raid counters,<br/>captcha challenges, pacing")]
    RD -->|"miss"| PG[("PostgreSQL<br/>schema from migrations/*.sql")]
    R -->|"writes, then cache invalidation"| PG
    M -.->|"optional"| AI["TypeSafe and Anthropic Claude<br/>AI checks"]
    M -->|"replies and actions"| TG
```

- **Startup.** `fuku/config` loads `.env` and validates it. `fuku/db` connects to PostgreSQL (5 attempts) before `main()` runs and applies the SQL migrations when `AUTO_MIGRATE=true`. `main.go` then sets up Redis, the embedded locales, tracing, the bot client, the dispatcher, the monitors and the HTTP server. It loads the 39 modules, which register themselves in `init()` (help loads last), and posts a "Started Bot!" message to `MESSAGE_DUMP`.
- **Handler groups.** Handlers run in numeric order. Watchers pass the update on to the next handler. Commands end it. New commands go through `helpers.WrapCommand`, a pipeline that runs the permission and disable checks, and every command handler is built by `helpers.NewBotCommand`, which records its name. Command deletion wraps the dispatcher's processor and acts after every group has run. See [handler groups](docs/src/content/docs/architecture/handler-groups.md) and [AGENTS.md](AGENTS.md).
- **Data.** Each domain has a repository package in `fuku/db/<domain>/`. Reads go through a cache loader (process memory, then Redis, then PostgreSQL), and every write drops the cache keys it affects. Operational state in Redis, such as the raid counters and the captcha challenges, sits outside the cache prefix and survives `CLEAR_CACHE_ON_STARTUP`.
- **Background work.** The lockdown worker and the scheduled deletion worker resume from the database after a restart. Sweepers recheck the Staff Group links and the settings roster every hour.
- **Shutdown.** Handlers stop in reverse order inside a 60 second budget, 10 seconds each unless a handler registers its own timeout. The Compose file sets `stop_grace_period: 75s`, so Docker does not stop the process before the shutdown ends.

| Group | What runs there |
| --- | --- |
| -10 | Captcha sweeper |
| -9 | Forward blocklist watcher |
| -8 | Member deletion (edited messages, self-destruct) |
| -7 | Lockdown join guard |
| -6 | Fed-ban check |
| -5 | Service message deletion watcher |
| -4 | Raid flood counter |
| -3 | Staff watchers |
| -2 | Admin cache update, member requirements watcher |
| -1 | Users tracker |
| 0 | Commands, help, greetings |
| 1 | Settings typed-reply door (private chats) |
| 2 | Raid AI candidate watcher |
| 3 | AI spam |
| 4 | Antiflood |
| 5, 6 | Locks |
| 7 | Blacklists |
| 8 | Reports, reactions |
| 9 | Filters |
| 10 | Pins |
| 11 | Log channel |
| 12 | Late raid flood counter (slash-prefixed texts only) |

## Tech stack

| Area | Technology |
| --- | --- |
| Language | Go 1.26 (`go.mod`) |
| Telegram | [gotgbot v2](https://github.com/PaulSonOfLars/gotgbot), [gotg_md2html](https://github.com/PaulSonOfLars/gotg_md2html) |
| Database | PostgreSQL through [GORM](https://gorm.io), with plain SQL migrations in [`migrations/`](migrations) |
| Cache | Redis ([go-redis v9](https://github.com/redis/go-redis), [gocache](https://github.com/eko/gocache)) with an in-process LRU ([golang-lru](https://github.com/hashicorp/golang-lru)) in front, msgpack encoding |
| Features | [base64Captcha](https://github.com/mojocn/base64Captcha) for captcha images, [Aho-Corasick](https://github.com/cloudflare/ahocorasick) keyword matching, Cloudflare Turnstile for the web captcha, TypeSafe and Anthropic Claude for the optional AI checks |
| Observability | Prometheus client, OpenTelemetry (OTLP gRPC or console exporter), logrus JSON logs with secret redaction |
| Tests | testify, SQLite (GORM driver), miniredis, optional real PostgreSQL |
| Tooling | golangci-lint v2, pre-commit, GoReleaser v2, Docker (distroless runtime) |
| Docs site | [Blume](https://useblume.dev/) on Bun |

## Getting started

### Prerequisites

- A bot token from [@BotFather](https://t.me/BotFather).
- Your numeric Telegram user ID (`OWNER_ID`).
- A chat or channel where I can post, usually a private channel with an ID like `-100…` (`MESSAGE_DUMP`).
- Either Docker with Compose v2, or Go 1.26 or newer with PostgreSQL and Redis. The bundled Compose stack uses PostgreSQL 17 and Redis 7, and CI tests against PostgreSQL 16.

### 1. Create the bot with BotFather

1. Send `/newbot` to [@BotFather](https://t.me/BotFather), choose a display name and a username, and copy the token.
2. Add the bot to your group and make it an admin, with the rights the features need: delete messages, ban and restrict members, pin messages, invite users.
3. Add the bot as an admin of your `MESSAGE_DUMP` channel, and of any log channel you plan to use with `/setlog`.

### 2. Configure

```bash
git clone https://github.com/uasneppy/Fuku_Robot.git
cd Fuku_Robot
cp sample.env .env
# Fill in at least BOT_TOKEN, OWNER_ID, MESSAGE_DUMP and DATABASE_URL.
```

`.env` is git-ignored and loaded on startup. Real environment variables take precedence over it. Every option is listed under [Configuration](#configuration).

### 3a. Run with Docker Compose (production stack, webhook mode)

[`docker-compose.yml`](docker-compose.yml) starts the bot together with PostgreSQL 17 and Redis 7. The bot image is `ghcr.io/uasneppy/fuku_robot:latest`. The file sets `AUTO_MIGRATE=true`, `USE_WEBHOOKS=true` and `DROP_PENDING_UPDATES=true`, publishes port 8080, health-checks the container with `fuku_robot --health`, and sets `stop_grace_period: 75s` so a shutdown can use its full 60 second budget. You need a public HTTPS domain that proxies to port 8080.

```bash
# .env must define BOT_TOKEN, OWNER_ID, MESSAGE_DUMP, WEBHOOK_DOMAIN and WEBHOOK_SECRET
docker compose up -d
docker compose logs -f fuku
```

Two facts about this stack.

- Compose passes only those five variables from `.env` into the bot container. Turnstile settings, `METRICS_AUTH_TOKEN`, `TYPESAFE_API_KEY` and every other option need an override file. `local.docker-compose.yml` is already git-ignored, and the values must also exist in `.env`, because Compose reads `${...}` from it:

  ```yaml
  # local.docker-compose.yml
  services:
    fuku:
      environment:
        METRICS_AUTH_TOKEN: ${METRICS_AUTH_TOKEN}
        TYPESAFE_API_KEY: ${TYPESAFE_API_KEY}
  ```

  Start it with `docker compose -f docker-compose.yml -f local.docker-compose.yml up -d`.
- The stack ships fixed demo credentials: `fuku` and `fuku` for PostgreSQL, and `redis` for Redis. Neither service publishes a port. Change the credentials before you use the stack for anything real.

### 3b. Run a local debug stack (polling mode)

[`debug.docker-compose.yml`](debug.docker-compose.yml) builds [`docker/alpine.debug`](docker/alpine.debug) (Alpine with a shell), passes your whole `.env` to the bot, and publishes PostgreSQL on `5432` and Redis on `6379`. Add these to `.env` first:

```ini
POSTGRES_USER=postgres
POSTGRES_PASSWORD=<choose-a-password>
POSTGRES_DB=fuku_robot
REDIS_PASSWORD=<choose-a-password>
AUTO_MIGRATE=true
```

```bash
docker compose -f debug.docker-compose.yml up --build
```

The stack also defines an optional local Bot API server (`aiogram/telegram-bot-api`). To use it, set `TELEGRAM_API_ID`, `TELEGRAM_API_HASH` and `API_SERVER=http://telegram-bot-api:8081`.

### 3c. Run from source

```bash
# PostgreSQL and Redis must be reachable with the values in .env.
# For a fresh database, set AUTO_MIGRATE=true in .env (or run `make psql-migrate`).
make run                       # = go run main.go; long polling unless USE_WEBHOOKS=true

# or build a static binary
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o fuku_robot .
./fuku_robot                  # keep migrations/ next to it when AUTO_MIGRATE=true
./fuku_robot --version        # prints the version and exits
```

Locales are embedded in the binary. Migrations are read from `MIGRATIONS_PATH` (default `migrations`).

### Turnstile web captcha

Optional. Math and text captcha need none of this.

1. Point a public HTTPS hostname at a reverse proxy with TLS. The proxy forwards only `/captcha` and `/captcha/*` to the bot's `HTTP_PORT`. The captcha routes share that port with `/metrics`, `/db_metrics`, `/health`, pprof and the webhook, so nothing else may be forwarded. The bot enforces no path list itself.
2. In BotFather send `/newapp`, set the Web App URL to `https://<host>/captcha` and choose a short name. Later changes go through `/myapps`.
3. In Cloudflare create a Turnstile widget in Managed mode, with exactly your public host in its hostname list. Copy the site key and the secret key.
4. Set `CAPTCHA_WEB_BASE_URL`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET_KEY`. All four or none: with only some of them set, the bot refuses to start. Also set `CAPTCHA_WEB_TRUSTED_PROXIES` and `METRICS_AUTH_TOKEN`, because the bot warns at startup when either is empty. With Compose, pass them through the override file. Never commit the secret key.
5. Restart, then request `https://<host>/captcha/health`. It answers `200` when the bot has finished starting and Redis answers.
6. Run `/captchamode turnstile` in the group. It is refused unless the web captcha is running, Redis answers and I am an admin with the right to restrict members. Without Redis the web captcha is unavailable, and a group set to Turnstile gets the math captcha.

The full walk-through, with proxy examples and troubleshooting, is in [the web captcha guide](docs/src/content/docs/self-hosting/captcha-web.md).

### Other deployment targets

The repository also holds manifests for [`render.yaml`](render.yaml) (Render blueprint: Docker image, managed PostgreSQL and Redis, `HTTP_PORT=10000`), [`railway.toml`](railway.toml) (health check on `/health`), [`app.json`](app.json) with [`Procfile`](Procfile) (Heroku with PostgreSQL and Redis add-ons) and [`nixpacks.toml`](nixpacks.toml).

### Webhook or polling

| | Polling (default) | Webhook |
| --- | --- | --- |
| Enable | `USE_WEBHOOKS=false` | `USE_WEBHOOKS=true` + `WEBHOOK_DOMAIN` + `WEBHOOK_SECRET` |
| What happens | The bot deletes any webhook, then long-polls | The bot calls `setWebhook` for `<WEBHOOK_DOMAIN>/webhook` with your secret token |
| Needs | Outbound internet only | A public HTTPS URL routed to `HTTP_PORT` |

In both modes the HTTP server runs, so `/health` and `/metrics` are always available. More in [webhooks](docs/src/content/docs/self-hosting/webhooks.md).

## Configuration

I read every variable below in [`fuku/config/config.go`](fuku/config/config.go), except the `OTEL_*` variables, which [`fuku/utils/tracing/tracing.go`](fuku/utils/tracing/tracing.go) reads. [`sample.env`](sample.env) is an annotated template. Booleans accept `true`, `yes` or `1`. An integer that fails to parse falls back to its default. Compose passes only five of these variables into the bot container, as described under Getting started.

<details open>
<summary><b>Required</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `BOT_TOKEN` | — | Bot token from BotFather |
| `OWNER_ID` | — | Your numeric Telegram user ID. The owner can use the Dev commands |
| `MESSAGE_DUMP` | — | Chat or channel ID (a channel ID looks like `-100…`). Receives the startup message and the AI spam deletion mirror |
| `DATABASE_URL` | — | PostgreSQL DSN, for example `postgres://<user>:<password>@<host>:5432/<db>?sslmode=disable` |

</details>

<details>
<summary><b>Redis and cache</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `REDIS_ADDRESS` | `localhost:6379` | Redis `host:port`. Required unless `DISABLE_CACHE` is set. When set, it takes precedence over `REDIS_URL` |
| `REDIS_URL` | — | Alternative `redis://user:password@host:port` URL, used only when `REDIS_ADDRESS` is empty |
| `REDIS_PASSWORD` | empty | Redis password. Overrides the one in `REDIS_URL` |
| `REDIS_DB` | `1` | Database number, 0 to 15 |
| `DISABLE_CACHE` | `false` | Skip the read-through cache. The bot keeps running without Redis, with degraded caching and state |
| `CACHE_LOCAL_TTL` | `10` | Seconds a value stays in process memory (0 to 300, and `0` turns the local layer off) |
| `CACHE_LOCAL_MAX_ENTRIES` | `50000` | Size of the in-process LRU (100 to 1000000) |
| `CLEAR_CACHE_ON_STARTUP` | `false` | Delete the cache keys at startup. Operational keys outside the cache prefix are kept |

</details>

<details>
<summary><b>Telegram, updates and HTTP</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `API_SERVER` | `https://api.telegram.org` | Custom Bot API server URL |
| `USE_WEBHOOKS` | `false` | `true` for webhook mode, `false` for long polling |
| `WEBHOOK_DOMAIN` | — | Required with webhooks. Public base URL; the bot registers `<domain>/webhook` |
| `WEBHOOK_SECRET` | — | Required with webhooks. Secret token Telegram sends in the `X-Telegram-Bot-Api-Secret-Token` header |
| `DROP_PENDING_UPDATES` | `false` | Throw away the updates that queued up while the bot was offline |
| `HTTP_PORT` | `8080` | Port for the HTTP server (1 to 65535) |
| `PORT` | — | Used when `HTTP_PORT` is unset |
| `DISPATCHER_MAX_ROUTINES` | `200` | Updates processed at the same time (1 to 1000) |
| `HTTP_MAX_IDLE_CONNS` | `100` | Idle connections kept for the Bot API client |
| `HTTP_MAX_IDLE_CONNS_PER_HOST` | `50` | Idle connections per host for the Bot API client |
| `ENABLED_LOCALES` | `uk,en,es,fr,hi,id,pt,ru` | Comma-separated languages offered by `/lang`. `uk` is always offered |
| `DEBUG` | `false` | Debug logging with caller info. Also turns the two monitoring defaults below off |

</details>

<details>
<summary><b>Database pool and migrations</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `DB_MAX_IDLE_CONNS` | `50` | Idle connections (1 to 100) |
| `DB_MAX_OPEN_CONNS` | `200` | Open connections (1 to 1000) |
| `DB_CONN_MAX_LIFETIME_MIN` | `240` | Connection lifetime in minutes (1 to 1440) |
| `DB_CONN_MAX_IDLE_TIME_MIN` | `60` | Idle time in minutes (1 to 60) |
| `AUTO_MIGRATE` | `false` | Apply `migrations/*.sql` at startup. The deploy manifests set it to `true` |
| `AUTO_MIGRATE_SILENT_FAIL` | `false` | Keep running after a migration checksum mismatch. Development only |
| `MIGRATIONS_PATH` | `migrations` | Folder that holds the SQL migrations |

</details>

<details>
<summary><b>AI checks</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `TYPESAFE_API_KEY` | — | TypeSafe API key. The AI spam filter and the raid AI text checks do nothing without it |
| `ENABLE_AISPAM` | `true` | Master switch for every AI check: text, images and names. `/aispam` still has to be turned on per chat, and the AI spam-name rule stays off until an owner sets it to a punishment |
| `ANTHROPIC_API_KEY` | — | Anthropic API key for the raid image checks and the AI spam-name rule. Without it neither runs, and no image or name leaves the server |
| `ANTHROPIC_MODEL` | `claude-haiku-5-5` | Claude model for those checks. It must accept a forced tool call. An invalid name is replaced by the default, with a warning |

</details>

<details>
<summary><b>Web captcha (Turnstile)</b></summary>

The first four variables go together: set all of them or none. With only some of them set, the bot refuses to start and names each missing variable.

| Variable | Default | Description |
| --- | --- | --- |
| `CAPTCHA_WEB_BASE_URL` | — | Public https URL of the bot's HTTP port, host only, with an empty path or `/` |
| `TELEGRAM_MINI_APP_SHORT_NAME` | — | Short name from BotFather: 1 to 64 letters, digits or underscores |
| `TURNSTILE_SITE_KEY` | — | Turnstile widget site key (public) |
| `TURNSTILE_SECRET_KEY` | — | Turnstile widget secret key, at least 6 characters. Registered with the log redactor |
| `CAPTCHA_WEB_TRUSTED_PROXIES` | — | Optional. Comma-separated IPs or CIDRs of your reverse proxies. Setting it without the four above stops startup |

</details>

<details>
<summary><b>Monitoring, maintenance and profiling</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `METRICS_AUTH_TOKEN` | — | When set, `/metrics` and `/db_metrics` require `Authorization: Bearer <token>` |
| `ENABLE_PERFORMANCE_MONITORING` | `true` if `DEBUG=false`, else `false` | Performance monitoring and remediation |
| `ENABLE_BACKGROUND_STATS` | `true` if `DEBUG=false`, else `false` | Background statistics collection |
| `ENABLE_DB_MONITORING` | `false` | Check the database pool every minute |
| `RESOURCE_MAX_GOROUTINES` | `1000` | Goroutine limit used by remediation |
| `RESOURCE_MAX_MEMORY_MB` | `500` | Memory limit used by remediation |
| `RESOURCE_GC_THRESHOLD_MB` | `400` | Memory above which a GC is triggered |
| `ENABLE_AUTO_CLEANUP` | `true` | Mark chats inactive after `INACTIVITY_THRESHOLD_DAYS` without activity |
| `INACTIVITY_THRESHOLD_DAYS` | `30` | Days without activity before a chat counts as inactive |
| `ACTIVITY_CHECK_INTERVAL` | `1` | Hours between activity checks |
| `ENABLE_PPROF` | `false` | Expose `/debug/pprof/`. Never enable in production |

</details>

<details>
<summary><b>Tracing (OpenTelemetry)</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP gRPC endpoint. With this empty and the console exporter off, tracing is disabled |
| `OTEL_EXPORTER_OTLP_INSECURE` | `false` | Plain gRPC without TLS when `true` |
| `OTEL_EXPORTER_CONSOLE` | `false` | Spans to stderr when `true` |
| `OTEL_SERVICE_NAME` | `fuku_robot` | Service name on the spans |
| `OTEL_TRACES_SAMPLE_RATE` | `1.0` | Sampling ratio, clamped to 0.0 to 1.0 |

</details>

<details>
<summary><b>Read by tooling, not by the bot</b></summary>

| Variable | Used by |
| --- | --- |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | `debug.docker-compose.yml` (the PostgreSQL container and the bot's `DATABASE_URL`) |
| `TELEGRAM_API_ID`, `TELEGRAM_API_HASH` | `debug.docker-compose.yml` (local Bot API server) |
| `PSQL_DB_HOST`, `PSQL_DB_PORT`, `PSQL_DB_NAME`, `PSQL_DB_USER`, `PSQL_DB_PASSWORD`, `PSQL_DB_SSLMODE` | `make psql-migrate`, `make psql-status`, `make psql-reset` |
| `FUKU_TEST_DATABASE`, `FUKU_TEST_MIGRATION_CHAIN` | Tests: opt in to the PostgreSQL-backed tests and the migration chain test, both through `DATABASE_URL` |

</details>

Never commit `.env`, and never paste real tokens, owner IDs, database URLs or webhook secrets into issues. I redact my own secrets from logs, but only the ones I know about.

## Endpoints and observability

| Endpoint | Description |
| --- | --- |
| `GET /health` | JSON with `status`, `checks` for the database and Redis, `version` and `uptime`. HTTP 503 when unhealthy |
| `GET /metrics` | Prometheus metrics. Bearer auth when `METRICS_AUTH_TOKEN` is set |
| `GET /db_metrics` | Database pool metrics as JSON, same auth |
| `POST /webhook` | Telegram updates. Webhook mode only. The secret token header is checked |
| `/debug/pprof/` | Go profiling. Only when `ENABLE_PPROF=true` |
| `/captcha`, `/captcha/verify`, `/captcha/health` | The web captcha page, its verify route and its health check. Only when the web captcha is configured |

`fuku_robot --health` calls `/health` on the configured port and exits `0` on HTTP 200 and `1` otherwise, which suits a distroless image without curl. `fuku_robot --version` prints the version and exits.

All of these routes share one port. With `METRICS_AUTH_TOKEN` empty, `/metrics` and `/db_metrics` are open and the server logs a warning. A public proxy must forward only `/captcha` and `/captcha/*`. Logs are JSON, and known secrets are redacted. More in [monitoring](docs/src/content/docs/self-hosting/monitoring.md) and [profiling](docs/src/content/docs/self-hosting/profiling.md).

## Project structure

```text
Fuku_Robot/
├── main.go                    # entry point: cache, locales, tracing, bot, HTTP server, webhook or polling
├── fuku/
│   ├── main.go                # LoadModules, ListModules, InitialChecks
│   ├── config/                # every env var: parsing, defaults, validation
│   ├── db/                    # GORM connection, migration runner, one repository package per domain
│   │   ├── models/            # GORM models (TableName() matches the SQL tables)
│   │   ├── cache/             # read-through loader: process memory → Redis → PostgreSQL
│   │   └── migrations/        # SQL migration runner with checksum verification
│   ├── i18n/                  # locale manager and translator
│   ├── modules/               # one file per bot module: bans.go, notes.go, captcha.go, lockdown.go, staff.go, settings.go, …
│   └── utils/                 # command pipeline, permission checks, callback codec, cache, HTTP server, web captcha, tracing, monitoring, …
├── locales/                   # uk, en, es, fr, hi, id, pt, ru + config.yml (help aliases)
├── migrations/                # 64 timestamped .sql files: the schema's source of truth
├── docker/                    # alpine (production), alpine.debug, goreleaser, pr-build Dockerfiles
├── docs/                      # Blume docs site: generated command pages, guides, architecture notes
├── scripts/                   # docs generator, translation checker, psql migrate and backup, test-result gate, captcha page tests
├── internal/testdb/           # SQLite-backed test database helper
├── formal/antiflood/          # TLA+ and Lean models of the antiflood counter reset
├── .github/workflows/         # CI, release, docs, Dependabot auto-merge, Pullfrog
├── docker-compose.yml         # production stack (bot + PostgreSQL 17 + Redis 7, webhook mode)
├── debug.docker-compose.yml   # local build + PostgreSQL + Redis + optional local Bot API server
├── sample.env                 # annotated configuration template
├── AGENTS.md                  # contributor rulebook: handlers, cache, migrations, tests
└── Makefile
```

## Development

| Target | What it does |
| --- | --- |
| `make run` | `go run main.go` |
| `make test` | The full test suite: `-tags testtools -race`, coverage, result gate |
| `make test-postgres-integrity` | PostgreSQL-only integrity tests (needs `DATABASE_URL` and `FUKU_TEST_DATABASE=true`) |
| `make lint` · `make check-duplicates` | golangci-lint v2; duplicate-code check |
| `make check-translations` | Report locale keys missing from any language |
| `make generate-docs` · `make check-docs` | Regenerate the docs pages, or fail on drift |
| `make docs-dev` | Run the Blume docs dev server (needs Bun) |
| `make psql-migrate` · `make psql-status` · `make psql-reset` | Apply migrations with `psql`, show their status, or **drop the whole schema** |
| `make validate-db` · `make backup-db` | Look for orphaned rows; `pg_dump` backup of `DATABASE_URL` |
| `make build` | GoReleaser snapshot build (needs GoReleaser v2 and Docker) |
| `make bump-version TAG=vX.Y.Z` | The only supported way to change version strings |
| `make tidy` · `make vendor` | `go mod tidy` and `go mod vendor` |

```bash
make test                                                            # always use the Makefile target
go test -tags testtools -race -count=1 -run '^TestName$' ./fuku/db/warns   # a single test
CGO_ENABLED=0 go build ./...                                         # quick compile check
```

- Tests need `CGO_ENABLED=1` (SQLite). Production builds use `CGO_ENABLED=0`. Without `-tags testtools`, test files are silently left out.
- Tests run on SQLite and miniredis by default. Set `DATABASE_URL` and `FUKU_TEST_DATABASE=true` to use real PostgreSQL.
- Before adding a module, a migration or a command, read [AGENTS.md](AGENTS.md). It covers migration naming and checksums, cache invalidation, handler groups, the locale rule (every new key goes into all 8 files) and Conventional Commits.
- [`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs gosec and govulncheck, lint, build, tests with a 78 percent coverage floor, docs and translation drift checks, and a Docker build. On pushes to `main` it also publishes a `dev` image.

## Documentation

- **In this repository:** the docs site sources live in [`docs/`](docs/README.md). Good starting points are [Docker](docs/src/content/docs/self-hosting/docker.mdx), [from source](docs/src/content/docs/self-hosting/source.mdx), [database](docs/src/content/docs/self-hosting/database.md), [troubleshooting](docs/src/content/docs/self-hosting/troubleshooting.md), [the web captcha guide](docs/src/content/docs/self-hosting/captcha-web.md), [environment reference](docs/src/content/docs/api-reference/environment.md), [caching](docs/src/content/docs/architecture/caching.md) and [request flow](docs/src/content/docs/architecture/request-flow.mdx).
- **Contributor rules:** [AGENTS.md](AGENTS.md).

## Attributions

- **Character and setting.** The name, character and voice of Xenobot-72 come from *Mau Makan Apa?*, the webcomic and animated series by Labirhin ([X](https://x.com/LabiLabirhin), [website](https://maumakanapa.art/)). Thank you to Labirhin for creating such an amazing character.
- **Original bot.** This project is a fork of [Alita_Robot](https://github.com/Divkix/Alita_Robot) by [Divkix](https://github.com/Divkix). Its MIT copyright notice is kept in [`LICENSE`](LICENSE).

This project is a fan work. It is not affiliated with or endorsed by Labirhin. Xenobot-72 and Jaya Abadi belong to their creator.

## License

[MIT](LICENSE). The full text and the copyright notice are in [`LICENSE`](LICENSE). I rely on [gotgbot](https://github.com/PaulSonOfLars/gotgbot) and [gotg_md2html](https://github.com/PaulSonOfLars/gotg_md2html) by PaulSonOfLars, and on the other open-source libraries listed in [`go.mod`](go.mod).
