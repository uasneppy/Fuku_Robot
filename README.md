<div align="center">

<h1>Fuku Robot</h1>

<p><b>A Telegram group manager written in Go: moderation, anti-spam, captcha, federations and more.</b></p>

<p>A fork of <a href="https://github.com/Divkix/Alita_Robot">Alita Robot</a> by <a href="https://github.com/Divkix">Divkix</a></p>

<p>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go 1.26"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://github.com/Divkix/Alita_Robot"><img src="https://img.shields.io/badge/fork%20of-Divkix%2FAlita__Robot-6f42c1?logo=github" alt="Fork of Divkix/Alita_Robot"></a>
  <a href="https://github.com/PaulSonOfLars/gotgbot"><img src="https://img.shields.io/badge/built%20with-gotgbot%20v2-26A5E4?logo=telegram&logoColor=white" alt="Built with gotgbot v2"></a>
  <img src="https://img.shields.io/badge/storage-PostgreSQL%20%2B%20Redis-336791?logo=postgresql&logoColor=white" alt="PostgreSQL + Redis">
</p>

<p>
  <a href="#-whats-different-from-alita">What's different</a> •
  <a href="#-features">Features</a> •
  <a href="#-commands">Commands</a> •
  <a href="#-getting-started">Getting started</a> •
  <a href="#-credits--license">Credits</a>
</p>

</div>

---

**Fuku Robot** is [uasneppy](https://github.com/uasneppy)'s fork of **Alita Robot**, a Telegram group-management bot
written in Go on top of [gotgbot](https://github.com/PaulSonOfLars/gotgbot). It moderates groups (bans, mutes, warns,
purges), keeps spam out (antiflood, anti-raid, join captcha, blacklists, locks and an optional AI spam filter), stores
notes, filters, rules and greetings, and ties several groups together through federations, connections and log
channels. Data lives in PostgreSQL, and Redis serves as a shared cache and holds the anti-raid state.

The long-term goal of the fork is a GroupHelp-style bot for the owner's own communities: one staff room that
protects every linked group at once, plus stronger raid protection.

> [!NOTE]
> Right now the `main` branch runs **upstream Alita Robot's code unchanged** (upstream commit `34ba3f6`, version
> string `2.23.14`). The Fuku-specific features are being built on a separate branch and are not merged yet.
> See [What's different from Alita](#-whats-different-from-alita) for the details.

## 📑 Table of contents

- [What's different from Alita](#-whats-different-from-alita)
- [Features](#-features)
- [Commands](#-commands)
- [Architecture](#-architecture)
- [Tech stack](#-tech-stack)
- [Getting started](#-getting-started)
- [Configuration](#-configuration)
- [Endpoints and observability](#-endpoints-and-observability)
- [Project structure](#-project-structure)
- [Development](#-development)
- [Keeping the fork in sync](#-keeping-the-fork-in-sync)
- [Documentation](#-documentation)
- [Credits & License](#-credits--license)

## 🔀 What's different from Alita

### On `main` today

| | Fuku Robot `main` | Upstream Alita Robot |
| --- | --- | --- |
| Bot code | Identical to upstream commit `34ba3f6` (3 Oct 2026). Apart from this README, `main` has no commits of its own. | Still moving. In October 2026 upstream `main` was 74 commits ahead, with tags up to `v2.26.1`. |
| Names inside the code | Unchanged: Go module `github.com/divkix/Alita_Robot`, binary `alita_robot`, Redis keys under `alita:`, Docker labels and service names. | Same |
| What users see | The bot's name and username come from BotFather, so you can call it Fuku. Some built-in texts still say "Alita", and the `/start` and `/about` menus link to upstream's support group and update channel. | Same |
| Deploy manifests | Still point at upstream: `docker-compose.yml` pulls `ghcr.io/divkix/alita_robot:latest`, and `app.json` and `.goreleaser.yaml` name the upstream repository and image. | Same |

Upstream `main` now has features this fork's `main` lacks, for example expiring warns, optional CAS and lols.bot checks
on new members, and stored reports with a report card in the log channel. See
[Keeping the fork in sync](#-keeping-the-fork-in-sync) to pull them in.

### In development: the `claude/cool-hamilton-hvs9nl` branch

The fork's own work lives on
[`claude/cool-hamilton-hvs9nl`](https://github.com/uasneppy/Fuku_Robot/tree/claude/cool-hamilton-hvs9nl).
It branches from the same upstream commit, is 658 commits ahead of it and is **not merged into `main`**. Its last
commit (18 Oct 2026) pauses Phase 6 after code review. The branch follows a nine-phase roadmap
([`.planning/ROADMAP.md`](https://github.com/uasneppy/Fuku_Robot/blob/claude/cool-hamilton-hvs9nl/.planning/ROADMAP.md)
on that branch):

- [x] **Phase 1: Staff Group links.** A group owner makes one group the Staff Group with `/setstaff` (`/unsetstaff`
  removes it). A person who owns both groups links another group to it with `/linkstaff` and removes it with
  `/unlinkstaff`. `/staff` shows the live status of every linked group. A link breaks by itself when the owners stop
  matching.
- [x] **Phase 2: Staff actions across groups.** In the Staff Group, `/ban`, `/mute`, `/kick`, `/unban`, `/unmute`,
  `/tban` and `/tmute` act in **every** linked group after a Confirm tap. Each group gets a live check of the issuer's
  restrict right, and one live summary reports done, skipped and failed groups.
- [x] **Phase 3: Audit and undo.** Each staff action is posted to the linked groups' log channels, recorded with every
  group's prior state, listed under "Recent actions" in `/staff`, and can be reversed with "Undo everywhere".
- [x] **Phase 4: Manual lockdown.** `/lockdown` mutes everyone except admins and bans or declines new joiners until it
  is lifted. `/unlockdown` restores the exact previous permissions, and `/lockdownstatus` shows its state. Lockdowns are
  stored in PostgreSQL and survive restarts.
- [x] **Phase 5: Lockdown alerts.** Each lockdown alerts the Staff Group and the log channel with Lift lockdown,
  "Ban N recent joiners" and "Revoke link" buttons.
- [ ] **Phase 6: Automatic raid detection** (in progress). Join surges and new-member floods trigger a lockdown on
  their own (`/autolockdown`), replacing the old `/antiraid` mode.
- [ ] **Phase 7:** AI-assisted raid detection: spam verdicts feed a burst trigger, but rules still decide.
- [ ] **Phase 8:** a Cloudflare Turnstile web captcha served as a Telegram Mini App.
- [ ] **Phase 9:** an inline-button `/settings` menu.

> [!IMPORTANT]
> Everything in this subsection is on the development branch only. Everything else in this README describes `main`.

## ✨ Features

**Moderation**
- Ban and mute with silent (`s`), delete-the-message (`d`) and timed (`t`) variants, kick with silent and delete
  variants, plus inline restrict and unrestrict keyboards.
- Warnings with a configurable limit and a ban, kick or mute action when the limit is reached.
- Purges, message deletion, promote and demote, custom admin titles, and changing the group title, photo and
  description.
- Approvals: trusted members skip antiflood, locks, blacklists and captcha without becoming admins.

**Anti-spam and raid protection**
- **Antiflood:** ban, kick or mute anyone who sends more than N messages in a row (N from 3 to 100).
- **AntiRaid:** a raid mode that temp-bans new joiners. It can switch on by itself when N or more people join within a
  minute. By default a raid lasts 6 h and each ban lasts 1 h.
- **Join captcha:** a math or text-in-image challenge with a 1–10 minute timeout and a kick, ban or mute on failure.
  The user's messages are held while the captcha is pending.
- **Blacklists** that delete, warn, mute, kick or ban, and **22 lock types** (URLs, media, stickers, forwards, bots,
  RTL text and more).
- **AI spam filter** (optional): checks messages with the TypeSafe API and quietly deletes confident spam (from 80 %
  for English messages, 90 % for other languages). It is off in every chat until an admin turns it on.

**Content**
- Notes (`/get name` or `#name`, media included, optionally sent in PM), keyword filters with auto-replies, and
  welcome and goodbye messages that can clean up service messages.
- Rules with a custom button, pins with anti-channel-pin, auto-reactions, and a formatting guide for markdown, buttons
  and fillings.

**Several groups**
- **Federations:** shared ban lists across groups, subscriptions to other federations, federation admins, a federation
  log chat, and fban import and export as CSV or JSON.
- **Connections** (manage a group's settings from PM), per-group **log channels** with categories, `/report` and
  `@admin` reports, and per-chat **command disabling**.

**Operations**
- Back up a group's settings to JSON and import or reset them module by module.
- 7 interface languages: English, Spanish, French, Hindi, Indonesian, Portuguese and Russian.
- Webhook or long-polling mode, `/health`, Prometheus `/metrics` and optional OpenTelemetry tracing.
- Versioned SQL migrations whose checksums are verified at startup, a Redis read-through cache with an in-process LRU
  layer, and graceful shutdown.
- Distroless Docker image, plus GoReleaser builds for Linux, macOS and Windows on amd64 and arm64.

## 💬 Commands

The code registers **30 modules plus the help menu** and **more than 200 command triggers** (aliases included).
`<user>` means a reply, `@username`, a mention or a numeric user ID. Times use `m`, `h` or `d`, for example `30m`,
`6h`, `2d`. Use `/help <module>` in the bot for the full text.

### Moderation

<details>
<summary><b>Bans</b>: <code>/ban</code> <code>/tban</code> <code>/kick</code> <code>/restrict</code> …</summary>

Admins only, except `/kickme` and `/banme`.

| Command | What it does |
| --- | --- |
| `/ban <user> [reason]` | Ban a user |
| `/sban <user>` | Ban silently and delete your command |
| `/dban` | Ban the author of the replied-to message and delete that message |
| `/tban <user> <time>` | Ban for a set time |
| `/unban <user>` | Lift a ban |
| `/unbanall` | Owner only: unban every user the bot knows in the chat, after confirmation |
| `/kick <user>` · `/skick <user>` · `/dkick` | Kick, kick silently, or kick and delete the replied-to message |
| `/kickme` · `/banme` | Remove yourself from the chat |
| `/restrict` · `/unrestrict` | Choose ban, kick or mute (or unban, unmute) from an inline keyboard |

</details>

<details>
<summary><b>Mutes</b>: <code>/mute</code> <code>/tmute</code> <code>/unmute</code> …</summary>

Admins only.

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

| Command | What it does |
| --- | --- |
| `/warn <user> [reason]` · `/swarn` · `/dwarn` | Warn; silently; or warn and delete the replied-to message |
| `/warns <user>` | Show a user's warnings |
| `/rmwarn` (`/unwarn`) | Remove a user's latest warning |
| `/resetwarns` (`/resetwarn`) | Reset a user's warnings to 0 |
| `/resetallwarns` | Reset every warning in the chat |
| `/warnings` | Show the chat's warn settings |
| `/setwarnlimit <n>` | Number of warnings before the action |
| `/setwarnmode <ban/kick/mute>` | Action taken at the limit |

</details>

<details>
<summary><b>Purges</b>: <code>/purge</code> <code>/del</code> …</summary>

Admins only.

| Command | What it does |
| --- | --- |
| `/purge` | Delete every message from the replied-to one up to now |
| `/del` | Delete the replied-to message |
| `/purgefrom` · `/purgeto` | Mark a start message, then purge up to an end message |

</details>

<details>
<summary><b>Admin</b>: <code>/promote</code> <code>/demote</code> <code>/title</code> …</summary>

`/adminlist` is for everyone; the rest are admin commands.

| Command | What it does |
| --- | --- |
| `/adminlist` | List the chat's admins |
| `/promote <user>` · `/demote <user>` | Promote or demote a user |
| `/title <user> <title>` | Set a custom admin title |
| `/setgtitle` (`/setgrouptitle`) | Change the group title |
| `/setgpic` (`/setgrouppic`) | Reply to a photo to make it the group picture |
| `/setgdesc` (`/setgroupdesc`, `/setdescription`) | Change the group description |
| `/invitelink` | Get the chat's invite link |
| `/admincache` · `/clearadmincache` | Refresh or clear the cached admin list |
| `/anonadmin <on/off>` | AnonAdmin mode: when on, anonymous admins skip the confirm-button check |

</details>

<details>
<summary><b>Approvals</b>: <code>/approve</code> <code>/approved</code> …</summary>

Admins only. Approved users are exempt from antiflood, blacklists, locks and captcha.

| Command | What it does |
| --- | --- |
| `/approve <user> [reason]` · `/unapprove <user>` | Approve or unapprove a user |
| `/approval <user>` | Check a user's approval status |
| `/approved` | List approved users |
| `/unapproveall` | Remove every approval (group creator only) |

</details>

### Protection

<details>
<summary><b>Antiflood</b>: <code>/setflood</code> <code>/setfloodmode</code> …</summary>

| Command | What it does |
| --- | --- |
| `/flood` | Show the current antiflood settings |
| `/setflood <n/off>` | Act after `n` messages in a row (3–100); `0`, `off` or `no` disables it |
| `/setfloodmode <ban/kick/mute>` | Action for flooders |
| `/delflood` (`/clearflood`) `<yes/no>` | Also delete the flooded messages |

</details>

<details>
<summary><b>AntiRaid</b>: <code>/antiraid</code> <code>/autoantiraid</code> …</summary>

Admins only. Raid state is kept in Redis, so this module does nothing without Redis.

| Command | What it does |
| --- | --- |
| `/antiraid` | Show the state with enable and disable buttons |
| `/antiraid on` · `/antiraid <duration>` · `/antiraid off` | Start raid mode for the configured or a custom time, or stop it |
| `/raidtime <time>` | How long raid mode lasts (default 6h) |
| `/raidactiontime <time>` | How long new joiners are temp-banned (default 1h) |
| `/autoantiraid <N/off>` | Switch raid mode on by itself at N or more joins per minute |

</details>

<details>
<summary><b>Captcha</b>: <code>/captcha</code> <code>/captchamode</code> …</summary>

Admins only. New members stay muted until they solve the challenge.

| Command | What it does |
| --- | --- |
| `/captcha <on/off>` | Turn captcha verification on or off |
| `/captchamode <math/text>` | Arithmetic problem, or text shown in an image |
| `/captchatime <1-10>` | Timeout in minutes (default 2) |
| `/captchaaction <kick/ban/mute>` | Action on failure or timeout (default kick) |
| `/captchamaxattempts <1-10>` | Maximum attempts (default 3) |
| `/captchapending` · `/captchaclear` | View or clear messages held from users with a pending captcha |

</details>

<details>
<summary><b>AI spam filter</b>: <code>/aispam</code></summary>

| Command | What it does |
| --- | --- |
| `/aispam` | Show whether the filter is on and exactly what is sent to the model |
| `/aispam <on/off>` | Turn the filter on or off for this chat (admins) |

Needs `TYPESAFE_API_KEY` set by the bot operator. Deletions are mirrored to the `MESSAGE_DUMP` chat.

</details>

<details>
<summary><b>Blacklists</b>: <code>/addblacklist</code> <code>/blaction</code> …</summary>

| Command | What it does |
| --- | --- |
| `/blacklists` (`/blocklists`) | List the blacklisted words (anyone) |
| `/addblacklist` (`/blacklist`, `/addblocklist`, `/blocklist`) `<words>` | Add words |
| `/rmblacklist` (`/rmblocklist`, `/unblacklist`) `<words>` | Remove words |
| `/blaction` (`/blacklistaction`) `<mute/kick/ban/warn/none>` | Action on a match (`none` only deletes the message) |
| `/remallbl` (`/rmallbl`, `/rmblocklistall`, `/rmallblocklist`) | Remove every blacklisted word (owner) |

</details>

<details>
<summary><b>Locks</b>: <code>/lock</code> <code>/unlock</code> …</summary>

Admins only.

| Command | What it does |
| --- | --- |
| `/lock <types…>` · `/unlock <types…>` | Lock or unlock content types |
| `/locks` | Show the active locks |
| `/locktypes` | List every lock type |

Lock types: `all`, `messages`, `media`, `other`, `previews`, `comments`, `sticker`, `audio`, `voice`, `document`,
`video`, `videonote`, `contact`, `photo`, `gif`, `url`, `bots`, `forward`, `game`, `location`, `rtl`, `anonchannel`.
See [lock types](docs/src/content/docs/api-reference/lock-types.md).

</details>

### Content

<details>
<summary><b>Notes</b>: <code>/save</code> <code>/get</code> <code>#note</code> …</summary>

| Command | What it does |
| --- | --- |
| `/get <name>` · `#name` | Get a note (anyone) |
| `/save` (`/addnote`) `<name> <text>` | Save a note; reply to a message to save it, media included |
| `/clear` (`/rmnote`) `<name>` | Delete a note |
| `/notes` (`/saved`) | List the notes |
| `/clearall` | Delete every note in the chat |
| `/privnote` (`/privatenotes`) | Send notes in PM through a button |

</details>

<details>
<summary><b>Filters</b>: <code>/filter</code> <code>/stop</code> …</summary>

| Command | What it does |
| --- | --- |
| `/filter` (`/addfilter`) `<trigger> <reply>` | Reply automatically when someone says the trigger (quote multi-word triggers; reply to a file to save it) |
| `/filters` | List the filters |
| `/stop` (`/rmfilter`, `/removefilter`) `<trigger>` | Remove a filter |
| `/stopall` | Remove every filter |

Add `{admin}` to a reply to make the filter fire only for admins.

</details>

<details>
<summary><b>Greetings</b>: <code>/setwelcome</code> <code>/cleanservice</code> …</summary>

Admins only.

| Command | What it does |
| --- | --- |
| `/welcome` · `/goodbye` `<on/off>` | Turn the welcome or goodbye message on or off |
| `/setwelcome` · `/setgoodbye` | Set the text (or reply to a message) |
| `/resetwelcome` · `/resetgoodbye` | Back to the default |
| `/cleanwelcome` · `/cleangoodbye` `<on/off>` | Delete the previous welcome or goodbye message |
| `/cleanservice <on/off>` | Delete "X joined the group" service messages |
| `/autoapprove <on/off>` | Approve join requests automatically |

</details>

<details>
<summary><b>Rules</b>: <code>/rules</code> <code>/setrules</code> …</summary>

| Command | What it does |
| --- | --- |
| `/rules` | Show the chat rules (anyone) |
| `/setrules <text>` | Set the rules |
| `/resetrules` (`/clearrules`) | Clear the rules |
| `/privaterules <on/off>` | Send the rules in PM |
| `/rulesbtn` (`/rulesbutton`) `<text>` | Set the rules button text |
| `/resetrulesbtn` (`/resetrulesbutton`, `/clearrulesbtn`, `/clearrulesbutton`) | Reset the button text |

</details>

<details>
<summary><b>Pins</b>: <code>/pin</code> <code>/permapin</code> …</summary>

| Command | What it does |
| --- | --- |
| `/pinned` | Show the current pinned message |
| `/pin [loud]` | Pin the replied-to message; `loud` or `notify` notifies members |
| `/permapin <text>` | Pin a custom message with markdown and buttons |
| `/unpin` · `/unpinall` | Unpin one or all |
| `/antichannelpin <on/off>` | Stop Telegram from auto-pinning linked-channel posts |
| `/cleanlinked <on/off>` | Delete messages sent by the linked channel |

</details>

<details>
<summary><b>Reactions</b>: <code>/addreaction</code> …</summary>

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

| Who | Commands |
| --- | --- |
| Federation owner (PM) | `/newfed <name>`, `/renamefed`, `/delfed`, `/fedpromote`, `/feddemote`, `/fedreason on/off`, `/fednotif on/off`, `/subfed`, `/unsubfed`, `/fedsubs`, `/fbanlist [csv/json/minicsv]`, `/importfbans`, `/setfedlog`, `/unsetfedlog` |
| Group owner | `/joinfed <fed_id>`, `/leavefed`, `/quietfed on/off` |
| Federation admins | `/fban`, `/unfban` (`/funban`), `/fedstat` (`/fbanstat`), `/feddemoteme` |
| Anyone | `/fedinfo`, `/fedadmins`, `/chatfed`, `/myfeds` |

</details>

<details>
<summary><b>Connections</b>: <code>/connect</code> …</summary>

| Command | What it does |
| --- | --- |
| `/connect <chat id>` | Manage a group from PM |
| `/disconnect` · `/reconnect` | Leave the connected chat, or reconnect to the last one |
| `/connection` | Show the current connection |
| `/allowconnect <yes/no>` | Allow members to connect (admins) |

</details>

<details>
<summary><b>Log channels</b>: <code>/setlog</code> <code>/log</code> …</summary>

Add the bot as an admin of a channel, send `/setlog` there, then forward that message to the group.

| Command | What it does |
| --- | --- |
| `/setlog` · `/unsetlog` | Set or remove the log channel |
| `/logchannel` | Show the current log channel |
| `/logcategories` | List the categories: settings, admin, user, automated, reports, other |
| `/log <category>` · `/nolog <category>` | Turn a category on or off |

</details>

<details>
<summary><b>Reports</b>: <code>/report</code> <code>@admin</code></summary>

| Command | What it does |
| --- | --- |
| `/report [reason]` · `@admin` | Reply to a message to report it to the admins |
| `/reports <on/off>` | Turn reporting on or off (in PM, for yourself) |
| `/reports block` · `unblock` · `showblocklist` | Block users from reporting and list them |

</details>

<details>
<summary><b>Disabling</b>: <code>/disable</code> <code>/enable</code> …</summary>

Admins only. A disabled command only stops working for non-admins.

| Command | What it does |
| --- | --- |
| `/disable <command>` · `/enable <command>` | Disable or re-enable a command |
| `/disableable` | List the commands that can be disabled |
| `/disabled` | List the disabled commands |
| `/disabledel <on/off>` | Delete disabled commands when non-admins use them |

</details>

### Utilities

<details>
<summary><b>Misc</b>: <code>/info</code> <code>/id</code> <code>/tr</code> …</summary>

| Command | What it does |
| --- | --- |
| `/info [user]` · `/id` | User info; chat ID, or a user's ID when used as a reply |
| `/ping` · `/stat` | Ping Telegram; message count of the chat |
| `/tr <lang> <text/reply>` | Translate a message |
| `/tell <text>` | Admins: the bot replies to the replied-to message with your text |
| `/stickerid` | File ID of the replied-to sticker |
| `/zombies` | Kick the deleted accounts the bot knows about (admins) |
| `/github <user[/repo]>` · `/wiki <query>` · `/ud <term>` | GitHub, Wikipedia and Urban Dictionary lookups |
| `/removebotkeyboard` · `/runs` | Remove a stuck bot keyboard; a random quip |

</details>

<details>
<summary><b>Backup, Languages, Help</b>: <code>/export</code> <code>/lang</code> <code>/help</code> …</summary>

| Command | What it does |
| --- | --- |
| `/export [modules…]` | Export the group's settings to JSON (admins) |
| `/import [modules…]` | Restore from a replied-to backup file (group creator) |
| `/reset [modules…]` | Reset settings to their defaults (group creator) |
| `/lang` | Choose a language for yourself or the group |
| `/start` · `/help [module]` · `/about` · `/donate` | Navigation, help, bot info, donation info |

</details>

<details>
<summary><b>Dev</b>: bot owner and team only</summary>

| Command | What it does |
| --- | --- |
| `/stats` · `/teamusers` | Bot statistics; list the team |
| `/addsudo` · `/remsudo` · `/adddev` · `/remdev` | Manage the team (owner) |
| `/chatinfo` · `/chatlist` · `/leavechat` | Inspect chats, list active chats, leave a chat |

</details>

`Users` (records users, chats and channels) and `BotUpdates` (the bot joining a group, admin-cache updates,
anonymous-admin checks) are internal modules with no commands.

## 🧭 Architecture

```mermaid
flowchart TD
    TG["Telegram Bot API<br/>api.telegram.org or API_SERVER"]

    subgraph Delivery["Update delivery"]
        WH["HTTP server on HTTP_PORT<br/>POST /webhook, secret-checked"]
        LP["gotgbot Updater<br/>long polling"]
    end

    TG -->|"USE_WEBHOOKS=true"| WH
    TG -->|"USE_WEBHOOKS=false"| LP
    WH --> D
    LP --> D

    D["gotgbot Dispatcher<br/>DISPATCHER_MAX_ROUTINES workers, tracing processor"]

    subgraph Groups["Handler groups, run in order"]
        direction TB
        G1["-10 captcha · -6 fed-ban · -5 antiraid<br/>-2 admin cache · -1 user tracker"]
        G2["0 commands, help, greetings"]
        G3["3 AI spam · 4 antiflood · 5/6 locks · 7 blacklists<br/>8 reports, reactions · 9 filters · 10 pins · 11 log channel"]
        G1 --> G2 --> G3
    end

    D --> Groups
    Groups --> M["alita/modules<br/>30 modules + help"]
    M --> R["alita/db repositories"]
    R -->|"reads"| L["In-process LRU<br/>CACHE_LOCAL_TTL"]
    L -->|"miss"| RD[("Redis<br/>cache + antiraid state")]
    RD -->|"miss"| PG[("PostgreSQL<br/>schema from migrations/*.sql")]
    R -->|"writes, then cache invalidation"| PG
    M -.->|"optional"| TS["TypeSafe API<br/>AI spam verdicts"]
    M -->|"replies and actions"| TG
```

- **Startup.** `alita/config` loads `.env` and validates it. `alita/db` connects to PostgreSQL (5 attempts) and runs
  the SQL migrations when `AUTO_MIGRATE=true`. `main.go` then sets up Redis, the embedded locales, tracing, the bot
  client, the dispatcher, the monitors and the HTTP server. It loads the modules, registers `/start` and `/help` for
  private chats, and posts a "Started Bot!" message to `MESSAGE_DUMP`.
- **Handler groups.** Watchers (captcha, antiflood, locks and so on) pass the update on to the next group. Commands
  end it. A module registers itself in `init()`. Newer commands go through `helpers.WrapCommand`, a pipeline that
  runs the permission and disable checks; older ones register `handlers.NewCommand` directly. See
  [handler groups](docs/src/content/docs/architecture/handler-groups.md) and [AGENTS.md](AGENTS.md).
- **Data.** Each domain has a repository package in `alita/db/<domain>/`. Reads go through a cache loader (process
  memory, then Redis, then PostgreSQL), and every write invalidates the keys it affects.
- **Shutdown.** Handlers stop in reverse order (HTTP server or updater, background workers, DB connections), so
  in-flight work is drained before the database closes.

## 🧰 Tech stack

| Area | Technology |
| --- | --- |
| Language | Go 1.26 (`go.mod`) |
| Telegram | [gotgbot v2](https://github.com/PaulSonOfLars/gotgbot), [gotg_md2html](https://github.com/PaulSonOfLars/gotg_md2html) |
| Database | PostgreSQL via [GORM](https://gorm.io); plain SQL migrations in [`migrations/`](migrations) |
| Cache | Redis ([go-redis v9](https://github.com/redis/go-redis), [gocache](https://github.com/eko/gocache)) plus an in-process LRU ([golang-lru](https://github.com/hashicorp/golang-lru)) with msgpack encoding |
| Features | [base64Captcha](https://github.com/mojocn/base64Captcha) for captchas, [Aho-Corasick](https://github.com/cloudflare/ahocorasick) keyword matching, TypeSafe API for AI spam |
| Observability | Prometheus client, OpenTelemetry (OTLP gRPC or console exporter), logrus JSON logs with secret redaction |
| Tests | testify, SQLite (GORM driver), miniredis; optional real PostgreSQL |
| Tooling | golangci-lint v2, pre-commit, GoReleaser v2, Docker (distroless runtime) |
| Docs site | [Blume](https://useblume.dev/) on Bun, deployed to Cloudflare Workers |

## 🚀 Getting started

### Prerequisites

- A bot token from [@BotFather](https://t.me/BotFather).
- Your numeric Telegram user ID (`OWNER_ID`).
- A chat or channel where the bot can post, usually a private channel with an ID like `-100…` (`MESSAGE_DUMP`).
- **Either** Docker with Compose v2, **or** Go 1.26+, PostgreSQL and Redis. The bundled Compose stack uses
  PostgreSQL 17 and Redis 7, and CI tests against PostgreSQL 16.

### 1. Create the bot with BotFather

1. Send `/newbot` to [@BotFather](https://t.me/BotFather), choose a display name (for example *Fuku Robot*) and a
   username, and copy the token.
2. Add the bot to your group and **make it an admin**, with the rights the features need: delete messages,
   ban/restrict members, pin messages, invite users. Admin bots receive every group message, so privacy mode does not
   need to change.
3. Add the bot as an admin of your `MESSAGE_DUMP` channel, and of any log channel you plan to use with `/setlog`.

### 2. Configure

```bash
git clone https://github.com/uasneppy/Fuku_Robot.git
cd Fuku_Robot
cp sample.env .env
# Fill in at least BOT_TOKEN, OWNER_ID, MESSAGE_DUMP and DATABASE_URL.
```

`.env` is git-ignored and loaded on startup; real environment variables take precedence over it. Every option is
listed under [Configuration](#-configuration).

### 3a. Run with Docker Compose (production stack, webhook mode)

[`docker-compose.yml`](docker-compose.yml) starts the bot together with PostgreSQL 17 and Redis 7. It sets
`AUTO_MIGRATE=true`, `USE_WEBHOOKS=true` and `DROP_PENDING_UPDATES=true`, publishes port 8080 and health-checks the
container with `alita_robot --health`. You need a public HTTPS domain that proxies to port 8080.

> [!WARNING]
> The stock compose file pulls **upstream's** published image (`ghcr.io/divkix/alita_robot:latest`), so it runs
> upstream's latest release and not this repository's code. To run this fork, build it locally with an override file.
> `local.docker-compose.yml` is already git-ignored:
>
> ```yaml
> # local.docker-compose.yml
> services:
>   alita:
>     image: fuku-robot:local
>     build:
>       context: .
>       dockerfile: docker/alpine
> ```

```bash
# .env must define BOT_TOKEN, OWNER_ID, MESSAGE_DUMP, WEBHOOK_DOMAIN and WEBHOOK_SECRET
docker compose -f docker-compose.yml -f local.docker-compose.yml up -d --build
docker compose logs -f alita
```

Compose passes only those five variables from `.env` into the bot container. Put any other option (for example
`TYPESAFE_API_KEY` or `METRICS_AUTH_TOKEN`) under `services.alita.environment` in your override file. The stack also
ships fixed database and Redis passwords (`alita` / `redis`). Neither service publishes a port, but change the
passwords if you reuse the stack elsewhere.

### 3b. Run a local debug stack (polling mode)

[`debug.docker-compose.yml`](debug.docker-compose.yml) builds [`docker/alpine.debug`](docker/alpine.debug) (Alpine
with a shell), passes your whole `.env` to the bot, and publishes PostgreSQL on `5432` and Redis on `6379`. Add these
to `.env` first:

```ini
POSTGRES_USER=postgres
POSTGRES_PASSWORD=<choose-a-password>
POSTGRES_DB=alita_robot
REDIS_PASSWORD=<choose-a-password>
AUTO_MIGRATE=true
```

```bash
docker compose -f debug.docker-compose.yml up --build
```

The stack also defines an optional local Bot API server (`aiogram/telegram-bot-api`). To use it, set
`TELEGRAM_API_ID`, `TELEGRAM_API_HASH` and `API_SERVER=http://telegram-bot-api:8081`.

### 3c. Run from source

```bash
# PostgreSQL and Redis must be reachable with the values in .env.
# For a fresh database, set AUTO_MIGRATE=true in .env (or run `make psql-migrate`).
make run                       # = go run main.go; long polling unless USE_WEBHOOKS=true

# or build a static binary
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o alita_robot .
./alita_robot                  # keep migrations/ next to it when AUTO_MIGRATE=true
./alita_robot --version        # prints the version and exits
```

Locales are embedded in the binary. Migrations are read from `MIGRATIONS_PATH` (default `migrations`).

### Other deployment targets

The repository still has upstream's PaaS manifests: [`render.yaml`](render.yaml) (Render blueprint: Docker image,
managed PostgreSQL and Redis, `HTTP_PORT=10000`), [`railway.toml`](railway.toml) (health check on `/health`),
[`app.json`](app.json) plus [`Procfile`](Procfile) (Heroku with PostgreSQL and Redis add-ons) and
[`nixpacks.toml`](nixpacks.toml). Before using `app.json`, check its `repository` field, which still points at upstream.

### Webhook vs. polling

| | Polling (default) | Webhook |
| --- | --- | --- |
| Enable | `USE_WEBHOOKS=false` | `USE_WEBHOOKS=true` + `WEBHOOK_DOMAIN` + `WEBHOOK_SECRET` |
| What happens | The bot deletes any webhook, then long-polls | The bot calls `setWebhook` for `<WEBHOOK_DOMAIN>/webhook` with your secret token |
| Needs | Outbound internet only | A public HTTPS URL routed to `HTTP_PORT` |

In both modes the HTTP server runs, so `/health` and `/metrics` are always available. More in
[webhooks](docs/src/content/docs/self-hosting/webhooks.md).

## 🔧 Configuration

The bot reads every variable below in [`alita/config/config.go`](alita/config/config.go), except the `OTEL_*`
variables, which [`alita/utils/tracing/tracing.go`](alita/utils/tracing/tracing.go) reads.
[`sample.env`](sample.env) is an annotated template. Booleans accept `true`, `yes` or `1`. An integer that fails to
parse falls back to its default, with only a warning in the log.

<details open>
<summary><b>Required</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `BOT_TOKEN` | — | Bot token from BotFather |
| `OWNER_ID` | — | Your numeric Telegram user ID. The owner can use the Dev commands. |
| `MESSAGE_DUMP` | — | Chat or channel ID for the startup message and AI-spam deletion reports |
| `DATABASE_URL` | — | PostgreSQL DSN, e.g. `postgres://<user>:<password>@<host>:5432/<db>?sslmode=disable` |

</details>

<details>
<summary><b>Redis and cache</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `REDIS_ADDRESS` | `localhost:6379` | Redis `host:port`. When set, it takes precedence over `REDIS_URL`. |
| `REDIS_URL` | — | Alternative `redis://user:password@host:port` URL (Heroku and Railway style), used only when `REDIS_ADDRESS` is empty |
| `REDIS_PASSWORD` | empty | Redis password; overrides the one in `REDIS_URL` |
| `REDIS_DB` | `1` | Database number, 0–15; overrides the one in `REDIS_URL` |
| `DISABLE_CACHE` | `false` | Skip the read-through cache so every read hits PostgreSQL. The bot keeps running, degraded, without Redis. |
| `CACHE_LOCAL_TTL` | `10` | Seconds a value stays in process memory (0–300; `0` turns the local layer off) |
| `CACHE_LOCAL_MAX_ENTRIES` | `50000` | Size of the in-process LRU (100–1,000,000) |
| `CLEAR_CACHE_ON_STARTUP` | `false` | Delete `alita:cache:*` keys at startup; operational keys are kept |

</details>

<details>
<summary><b>Telegram, updates and HTTP</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `API_SERVER` | `https://api.telegram.org` | Custom Bot API server URL, for example a local `telegram-bot-api` |
| `USE_WEBHOOKS` | `false` | `true` for webhook mode, `false` for long polling |
| `WEBHOOK_DOMAIN` | — | Required with webhooks. Public base URL including `https://`; the bot appends `/webhook`. |
| `WEBHOOK_SECRET` | — | Required with webhooks. Secret token Telegram sends with every update. |
| `DROP_PENDING_UPDATES` | `false` | Throw away the updates that queued up while the bot was offline |
| `HTTP_PORT` | `8080` | Port for `/health`, `/metrics`, `/db_metrics` and `/webhook` (1–65535) |
| `PORT` | — | Used when `HTTP_PORT` is unset (PaaS convention) |
| `DISPATCHER_MAX_ROUTINES` | `200` | Updates processed at the same time (1–1000) |
| `HTTP_MAX_IDLE_CONNS` | `100` | Idle connections kept for the Bot API client |
| `HTTP_MAX_IDLE_CONNS_PER_HOST` | `50` | Idle connections per host for the Bot API client |
| `ENABLED_LOCALES` | `en` | Comma-separated languages offered by `/lang`: `en,es,fr,hi,id,pt,ru` |
| `DEBUG` | `false` | Debug logging with caller info; also changes the monitoring defaults below |

</details>

<details>
<summary><b>Database pool and migrations</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `DB_MAX_IDLE_CONNS` | `50` | Idle connections (1–100) |
| `DB_MAX_OPEN_CONNS` | `200` | Open connections (1–1000) |
| `DB_CONN_MAX_LIFETIME_MIN` | `240` | Connection lifetime in minutes (1–1440) |
| `DB_CONN_MAX_IDLE_TIME_MIN` | `60` | Idle time in minutes (1–60) |
| `AUTO_MIGRATE` | `false` | Apply `migrations/*.sql` at startup. The deploy manifests set it to `true`. |
| `AUTO_MIGRATE_SILENT_FAIL` | `false` | Keep running when a migration fails (development only) |
| `MIGRATIONS_PATH` | `migrations` | Folder that holds the SQL migrations |

</details>

<details>
<summary><b>AI spam filter</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `TYPESAFE_API_KEY` | — | TypeSafe API key ([typesafe.ai](https://typesafe.ai)). Without it the filter does nothing. |
| `ENABLE_AISPAM` | `true` | Global kill switch. Chats still have to opt in with `/aispam on`. |

</details>

<details>
<summary><b>Monitoring, maintenance and profiling</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `METRICS_AUTH_TOKEN` | — | When set, `/metrics` and `/db_metrics` require `Authorization: Bearer <token>` |
| `ENABLE_PERFORMANCE_MONITORING` | `true` if `DEBUG=false`, else `false` | Auto-remediation based on goroutine and memory limits |
| `ENABLE_BACKGROUND_STATS` | `true` if `DEBUG=false`, else `false` | Background statistics collection |
| `ENABLE_DB_MONITORING` | `false` | Check the DB connection pool every minute |
| `RESOURCE_MAX_GOROUTINES` | `1000` | Goroutine limit used by auto-remediation |
| `RESOURCE_MAX_MEMORY_MB` | `500` | Memory limit used by auto-remediation |
| `RESOURCE_GC_THRESHOLD_MB` | `400` | Memory above which a GC is triggered |
| `ENABLE_AUTO_CLEANUP` | `true` | Mark chats inactive after `INACTIVITY_THRESHOLD_DAYS` without activity |
| `INACTIVITY_THRESHOLD_DAYS` | `30` | Days without activity before a chat counts as inactive |
| `ACTIVITY_CHECK_INTERVAL` | `1` | Hours between activity checks |
| `ENABLE_PPROF` | `false` | Expose `/debug/pprof/*`. Never enable in production. |

</details>

<details>
<summary><b>Tracing (OpenTelemetry)</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP gRPC endpoint. When empty and the console exporter is off, tracing is disabled. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `false` | Connect without TLS (`true`) |
| `OTEL_EXPORTER_CONSOLE` | `false` | Pretty-print spans to stderr (`true`) |
| `OTEL_SERVICE_NAME` | `alita_robot` | Service name on the spans |
| `OTEL_TRACES_SAMPLE_RATE` | `1.0` | Sampling ratio, clamped to 0.0–1.0 |

</details>

<details>
<summary><b>Read by tooling, not by the bot</b></summary>

| Variable | Used by |
| --- | --- |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | `debug.docker-compose.yml` (PostgreSQL container and the bot's `DATABASE_URL`) |
| `TELEGRAM_API_ID`, `TELEGRAM_API_HASH` | `debug.docker-compose.yml` (local Bot API server) |
| `PSQL_DB_HOST`, `PSQL_DB_PORT`, `PSQL_DB_NAME`, `PSQL_DB_USER`, `PSQL_DB_PASSWORD`, `PSQL_DB_SSLMODE` | `make psql-migrate`, `make psql-status`, `make psql-reset` |
| `ALITA_TEST_DATABASE`, `ALITA_TEST_MIGRATION_CHAIN` | Tests: opt in to the PostgreSQL-backed tests and the migration-chain test (both use `DATABASE_URL`) |

</details>

> [!CAUTION]
> Never commit `.env` or paste real tokens, owner IDs, database URLs or webhook secrets into issues. The bot redacts
> its own secrets from logs, but only the ones it knows about.

## 📡 Endpoints and observability

| Endpoint | Description |
| --- | --- |
| `GET /health` | JSON with `status`, `database` and `redis` checks, `version` and `uptime`; returns `503` when unhealthy |
| `GET /metrics` | Prometheus metrics (bearer auth when `METRICS_AUTH_TOKEN` is set) |
| `GET /db_metrics` | Database pool metrics as JSON (same auth) |
| `POST /webhook` | Telegram updates; only registered in webhook mode |
| `/debug/pprof/` | Go profiling; only when `ENABLE_PPROF=true` |

`alita_robot --health` calls `/health` on the configured port and exits `0` or `1`, which suits distroless images
without curl. Logs are JSON (logrus), and known secrets are redacted. More in
[monitoring](docs/src/content/docs/self-hosting/monitoring.md) and
[profiling](docs/src/content/docs/self-hosting/profiling.md).

## 📁 Project structure

```text
Fuku_Robot/
├── main.go                    # entry point: cache, locales, tracing, bot, HTTP server, webhook or polling
├── alita/
│   ├── main.go                # LoadModules, ListModules, InitialChecks
│   ├── config/                # every env var: parsing, defaults, validation
│   ├── db/                    # GORM connection, migration runner, one repository package per domain
│   │   ├── models/            # GORM models (TableName() matches the SQL tables)
│   │   ├── cache/             # read-through loader: process memory → Redis → PostgreSQL
│   │   └── migrations/        # SQL migration runner with checksum verification
│   ├── i18n/                  # locale manager and translator
│   ├── modules/               # one file per bot module: bans.go, notes.go, captcha.go, federations.go, …
│   └── utils/                 # command pipeline, permission checks, callback codec, cache, HTTP server, tracing, monitoring, …
├── locales/                   # en, es, fr, hi, id, pt, ru + config.yml (help aliases)
├── migrations/                # 42 timestamped .sql files: the schema's source of truth
├── docker/                    # alpine (production), alpine.debug, goreleaser, pr-build Dockerfiles
├── docs/                      # Blume docs site: generated command pages, guides, architecture notes
├── scripts/                   # docs generator, translation checker, psql migrate and backup, test-result gate
├── internal/testdb/           # SQLite-backed test database helper
├── formal/antiflood/          # TLA+ and Lean models of the antiflood counter reset
├── .github/workflows/         # CI, release, docs deploy, Dependabot auto-merge, Pullfrog
├── docker-compose.yml         # production stack (bot + PostgreSQL 17 + Redis 7, webhook mode)
├── debug.docker-compose.yml   # local build + PostgreSQL + Redis + optional local Bot API server
├── sample.env                 # annotated configuration template
├── AGENTS.md                  # contributor rulebook: handlers, cache, migrations, tests
└── Makefile
```

## 🧪 Development

| Target | What it does |
| --- | --- |
| `make run` | `go run main.go` |
| `make test` | The full test suite: `-tags testtools -race`, coverage, result gate |
| `make test-postgres-integrity` | PostgreSQL-only integrity tests (needs `DATABASE_URL` and `ALITA_TEST_DATABASE=true`) |
| `make lint` · `make check-duplicates` | golangci-lint v2; duplicate-code check |
| `make check-translations` | Report locale keys missing from any language |
| `make generate-docs` · `make check-docs` | Regenerate the docs pages, or fail on drift |
| `make docs-dev` | Run the Blume docs dev server (needs Bun) |
| `make inventory` | Write the command inventory to `.planning/INVENTORY.{json,md}` |
| `make psql-migrate` · `make psql-status` · `make psql-reset` | Apply migrations with `psql`, show their status, or **drop the whole schema** |
| `make validate-db` · `make backup-db` | Look for orphaned rows; `pg_dump` backup of `DATABASE_URL` |
| `make build` | GoReleaser snapshot build (needs GoReleaser v2 and Docker) |
| `make bump-version TAG=vX.Y.Z` | The only supported way to change version strings |
| `make tidy` · `make vendor` | `go mod tidy` / `go mod vendor` |

```bash
make test                                                            # always use the Makefile target
go test -tags testtools -race -count=1 -run '^TestName$' ./alita/db/warns   # a single test
CGO_ENABLED=0 go build ./...                                         # quick compile check
```

- Tests need `CGO_ENABLED=1` (SQLite); production builds use `CGO_ENABLED=0`. Without `-tags testtools`, test files
  are silently left out.
- Tests run on SQLite and miniredis by default. Set `DATABASE_URL` and `ALITA_TEST_DATABASE=true` to use real
  PostgreSQL.
- Before adding a module, migration or command, read [AGENTS.md](AGENTS.md). It covers migration naming and checksums,
  cache invalidation, handler groups, the locale rule (every new key goes into all 7 files) and conventional commits.
- [`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs gosec and govulncheck, lint, build, tests with a 78 %
  coverage floor, docs and translation drift checks, and a Docker build. On pushes to `main` it publishes
  `ghcr.io/<owner>/<repo>:dev`. GitHub usually keeps workflows off on a fork until you enable them in the **Actions** tab. The
  docs deploy needs Cloudflare secrets, and the release pipeline still publishes to upstream's image name.

## 🔄 Keeping the fork in sync

The fork's `main` does not follow upstream on its own. To pull in upstream's fixes and features:

```bash
git remote add upstream https://github.com/Divkix/Alita_Robot.git   # once
git fetch upstream
git switch main
git merge upstream/main          # or: git rebase upstream/main
git push origin main
```

You can also use GitHub's **Sync fork** button. After syncing `main`, merge it into the development branch too
(`git switch claude/cool-hamilton-hvs9nl && git merge main`), resolving conflicts in the touched modules such as
`antiraid.go`, `bans.go`, `captcha.go` and `greetings.go`.

## 📚 Documentation

- **Upstream docs site:** [alita-docs.divkix.me](https://alita-docs.divkix.me) covers setup, configuration and every
  module of Alita Robot. Most of it applies to this fork's `main` too.
- **In this repository:** the same site's sources live in [`docs/`](docs/README.md). Good starting points are
  [Docker](docs/src/content/docs/self-hosting/docker.mdx),
  [from source](docs/src/content/docs/self-hosting/source.mdx),
  [database](docs/src/content/docs/self-hosting/database.md),
  [troubleshooting](docs/src/content/docs/self-hosting/troubleshooting.md),
  [environment reference](docs/src/content/docs/api-reference/environment.md),
  [caching](docs/src/content/docs/architecture/caching.md) and
  [request flow](docs/src/content/docs/architecture/request-flow.mdx).
- **Contributor rules:** [AGENTS.md](AGENTS.md).

## 🙏 Credits & License

Fuku Robot is built entirely on **[Alita Robot](https://github.com/Divkix/Alita_Robot)** by
**[Divanshu Chauhan (Divkix)](https://github.com/Divkix)** and
[its contributors](https://github.com/Divkix/Alita_Robot/graphs/contributors). Almost every line of code, test,
translation and document in this repository comes from that project. Thank you!

It also relies on [gotgbot](https://github.com/PaulSonOfLars/gotgbot) and
[gotg_md2html](https://github.com/PaulSonOfLars/gotg_md2html) by PaulSonOfLars, plus the other open-source libraries
listed in [`go.mod`](go.mod).

**License:** [MIT](LICENSE), Copyright (c) 2020-2025 Divanshu Chauhan. This fork is distributed under the same MIT
License. Any copy or substantial portion of the software must include the original copyright notice and the MIT
permission notice from [`LICENSE`](LICENSE).

<div align="center">
<sub>Fuku Robot is an independent fork and is not affiliated with or endorsed by the Alita Robot project.</sub>
</div>
