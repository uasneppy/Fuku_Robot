# 261115-pvl fact sheet (Task 1 steps 0 to 4)

Built at HEAD `83dd98e` on `claude/ecstatic-goldberg-qsss05`, release `v2.24.0`. Every row names the command or `file:line` that proves it. A row marked **UNVERIFIED** must not be used by a README writer.

How the README writers use this file:

- Prose may state only facts that have a row here. Numbers in prose must be in section G (the checker enforces it).
- Command coverage: section C0 is the partition of all 231 registry names into groups. Every name must appear as `/name` in the Commands section of both READMEs (the checker enforces it against `261115-pvl-commands.txt`).
- Forbidden strings (checker FAILS on any hit, in prose or in code): `divkix`, `divideprojects`, `divanshu`, `alita`, the original repository path, the word `upstream`, the word `fork` (and `forked`, `forking`).
- The old bot name `fuku` (any case) is allowed only inside fenced code, inline code spans, URLs, link targets and HTML attributes (clone URL, image name, module path, directory and package names in the tree, Redis key prefixes, `FUKU_TEST_DATABASE`, the binary name `fuku_robot`). Never as the bot's name in prose, never in an image `alt`.
- The owner's handle `uasneppy` is allowed (clone URL, image path).
- Do not repeat the LICENSE copyright holder's name in either README. Say "MIT, see `LICENSE`".
- Heading choice (discretion call, section 5 of the plan): plain-text headings with no emoji, so GitHub anchors are robust (emoji vanish from slugs and leave a leading hyphen).

## A. Identity

| Claim | Evidence | Observed | Verdict |
|---|---|---|---|
| Release version | `grep -n 'BotVersion:' fuku/config/config.go` (line 402); `main.go:62` | `"2.24.0"`, default print `v2.24.0` | verified |
| Go version | `head -5 go.mod` | `go 1.26.0`, no toolchain directive | verified |
| Module path | `go.mod:1` | `github.com/uasneppy/fuku_robot` | verified |
| Binary name | `.goreleaser.yaml:26`, `docker/alpine:39` | `fuku_robot` (`/app/fuku_robot` in the image) | verified |
| Image name | `docker-compose.yml:9`, `.goreleaser.yaml:100` | `ghcr.io/uasneppy/fuku_robot:latest`, platforms linux/amd64 and linux/arm64 (`.goreleaser.yaml:105-107`) | verified |
| Licence | `head -3 LICENSE` | MIT License; copyright line names the original author, so do not repeat it | verified |
| Clone URL | `git remote -v` | `https://github.com/uasneppy/Fuku_Robot` (the old README used a `.git` suffix; `app.json:4` has the lower-case form `uasneppy/fuku_robot`) | verified |
| Release binaries | `.goreleaser.yaml:26-35` | darwin, linux, windows on amd64 and arm64, `CGO_ENABLED=0`; windows archives are zip | verified |
| Runtime image | `docker/alpine:33`, `docker/goreleaser:1` | `gcr.io/distroless/static-debian12` (no shell, no curl; healthcheck is `fuku_robot --health`) | verified |
| In-world identity | owner brief section 3a and 4 | Xenobot-72 is version 72 of 74, built by Jaya Abadi to run the facility | in-world, flavour only; may appear at most once and must not be read as a software version |

## B. Counts

| Figure | Command | Output | State as |
|---|---|---|---|
| Command names, aliases included | `CGO_ENABLED=1 go test -tags testtools -count=1 -v -run '^TestZZCount$' ./fuku/utils/helpers` (throwaway test, `fuku.LoadModules` then dump of `helpers.botCommands`) | `COUNT 231` | "231 command names, aliases included, counted from the running dispatcher at release v2.24.0" |
| Registered module loaders | `grep -rn 'RegisterLegacyModule("' fuku/modules --include='*.go' --exclude='*_test.go' \| wc -l` | `39`, no duplicate names (`uniq -d` empty); priorities -10 (BotUpdates) to 270 (Backup) | may say "39 modules"; `LoadHelp` runs last (`defer modules.LoadHelp` in `fuku/main.go:37`). Do not state this as a command count. |
| Migrations | `ls migrations/*.sql \| wc -l`; `git ls-files 'migrations/*.sql' \| wc -l` | `64`, first `20250805200527_initial_migration.sql`, last `20261113130000_add_staff_forward_blocks.sql` | verified; stating it is optional |
| Languages | `ls locales` | `en es fr hi id pt ru uk` plus `config.yml`, which is the help alt-name pseudo-locale and not a language | 8 languages; Ukrainian (`uk`) is the default language |
| Lock types | `fuku/modules/locks.go:61-97` | `lockMap` 16 keys (sticker audio voice document video videonote contact photo gif url bots forward game location rtl anonchannel) plus `restrMap` 6 keys (messages comments media other previews all), union 22, no overlap | 22 lock types |
| Member requirement rules | `fuku/db/memberreq/settings.go:39` (`var Rules`) | 8: last name, username, Russian letters, Arabic letters, Chinese characters, profile photo, channel subscription, AI spam-like name | 8 rules |
| Env names read by non-test code | `grep -rhoE 'Getenv\("[A-Z][A-Z0-9_]+"\)' fuku main.go --include='*.go' --exclude='*_test.go' \| sort -u \| wc -l` | `57`; `FUKU_TEST_DATABASE` (`fuku/db/conn.go:25`) is test tooling | 56 bot variables in Configuration plus tooling names (section D) |
| Deletion kinds | `fuku/db/deletion/kinds.go:19-64`, `fuku/modules/deletion_kinds.go` | 12 service kinds, 5 bot kinds, 3 member kinds | optional |
| Log categories | `fuku/db/logchannels/repository.go:30` | settings, admin, user, automated, reports, other (6) | verified |
| Handler groups | `AddHandlerToGroup` calls and group constants (list below) | see below | verified |

Handler groups, re-verified against code (constant or literal, file:line):

| Group | What | Evidence |
|---|---|---|
| -10 | captcha pending-message sweeper | `captcha.go:2378` |
| -9 | forward blocklist watcher | `staff_forwardblock_watch.go:34`, registered `staff.go:409` |
| -8 | member deletion (edit handler, self-destruct watcher) | `member_deletion.go:37,556-557` |
| -7 | lockdown join guard | `lockdown.go:34`, `lockdown_guard.go:603-615` |
| -6 | fed-ban | `federations.go:35` |
| -5 | service message deletion watcher | `deletion_service.go:20,114` |
| -4 | raid flood counter | `raid_flood.go:48`, `lockdown.go:654` |
| -3 | staff watchers | `staff_watchers.go:24,171-185` |
| -2 | admin cache auto-update (`my_chat_member`) and member requirements watcher | `bot_updates.go:273-275`, `memberreq_watch.go:35` |
| -1 | users tracker | `users.go:57` |
| 0 | commands, help, greetings (default group) | `helpers.register` uses `AddHandler` when `Group` is 0 |
| 1 | settings typed-reply door (private messages) | `settings_input_door.go:24` |
| 2 | raid AI candidate watcher | `raid_ai.go:42` |
| 3 | AI spam | `aispam.go:42` |
| 4 | antiflood | `antiflood.go:57` |
| 5, 6 | locks | `locks.go:33-34` |
| 7 | blacklists | `blacklists.go:31` |
| 8 | reports, reactions | `reports.go:29`, `reactions.go:27` |
| 9 | filters | `filters.go:37` |
| 10 | pins | `pins.go:30` |
| 11 | log channel | `logchannels.go:29` |
| 12 | late raid flood counter (slash-prefixed texts only) | `raid_flood.go:54`, `lockdown.go:660` |
| wrapper | command deletion wraps the dispatcher processor: `modules.CommandDeletionProcessor{Inner: tracing.TracingProcessor{}}` | `main.go:431` |

The old README diagram line `-5 antiraid` is wrong: group -5 is service message deletion and no antiraid handler exists.

## C0. Partition of the 231 registry names (the Commands section layout)

`python3` check of this partition against `261115-pvl-commands.txt` (scratchpad `groups.py`): `groups 29 assigned 231 unique 231 registry 231`, no duplicates, nothing missing, nothing extra. Recommended README grouping (group, count, names):

| Group | N | Names |
|---|---|---|
| Bans | 13 | ban sban tban dban unban banme unbanall kick dkick skick kickme restrict unrestrict |
| Mutes | 5 | mute smute tmute dmute unmute |
| Warns | 12 | warn swarn dwarn warns rmwarn unwarn resetwarns resetwarn resetallwarns warnings setwarnlimit setwarnmode |
| Purges | 4 | del purge purgefrom purgeto |
| Admin | 15 | adminlist promote demote title setgtitle setgrouptitle setgpic setgrouppic setgdesc setgroupdesc setdescription invitelink admincache clearadmincache anonadmin |
| Approvals | 5 | approve unapprove approval approved unapproveall |
| Antiflood | 5 | flood setflood setfloodmode delflood clearflood |
| Lockdown and raid | 8 | lockdown unlockdown lockdownstatus autolockdown autoantiraid, retired: antiraid raidtime raidactiontime |
| Captcha | 7 | captcha captchamode captchatime captchaaction captchamaxattempts captchapending captchaclear |
| AI spam filter | 1 | aispam |
| Blacklists | 15 | blacklists blocklists addblacklist blacklist addblocklist blocklist rmblacklist rmblocklist unblacklist blaction blacklistaction remallbl rmallbl rmblocklistall rmallblocklist |
| Locks | 4 | lock unlock locks locktypes |
| Notes | 10 | save addnote clear rmnote notes saved clearall privnote privatenotes get |
| Filters | 7 | filter addfilter stop rmfilter removefilter filters stopall |
| Greetings | 10 | welcome setwelcome resetwelcome goodbye setgoodbye resetgoodbye cleanwelcome cleangoodbye cleanservice autoapprove |
| Rules | 11 | rules setrules resetrules clearrules privaterules rulesbtn rulesbutton resetrulesbtn resetrulesbutton clearrulesbtn clearrulesbutton |
| Pins | 7 | pinned pin permapin unpin unpinall antichannelpin cleanlinked |
| Reactions | 4 | addreaction removereaction reactions resetreactions |
| Formatting | 2 | markdownhelp formatting |
| Federations | 27 | newfed renamefed delfed joinfed leavefed quietfed fedinfo fedadmins chatfed myfeds fedpromote feddemote feddemoteme fedreason fednotif fban unfban funban fedstat fbanstat subfed unsubfed fedsubs setfedlog unsetfedlog fbanlist importfbans |
| Connections | 5 | connect disconnect connection reconnect allowconnect |
| Log channels | 6 | setlog unsetlog logchannel logcategories log nolog |
| Reports | 2 | report reports |
| Disabling | 5 | disable enable disableable disabled disabledel |
| Misc | 13 | info id ping stat tr tell stickerid zombies github wiki ud runs removebotkeyboard |
| Backup, languages, help | 8 | export import reset lang start help about donate |
| Dev | 9 | stats teamusers addsudo remsudo adddev remdev chatinfo chatlist leavechat |
| Staff Group and forward blocklist | 9 | setstaff unsetstaff staff linkstaff unlinkstaff addfwdblock rmfwdblock fwdblocklist fwdblockaction |
| Settings | 2 | settings cancel |

Names that two modules register: `ban mute kick unban unmute tban tmute sban dban skick dkick smute dmute` are registered by Bans or Mutes (per-group commands) and again by the StaffActions interceptors (`fuku/modules/staff_action.go:44-60`, priority 65, group 0, ahead of Bans 70 and Mutes 80). Outside a Staff Group an interceptor returns `ext.ContinueGroups` and does nothing; inside a Staff Group `ban tban mute tmute kick unban unmute` fan out to every linked group and `sban dban skick dkick smute dmute` are refused with a hint. Registry is a set, so each name counts once.

Non-slash triggers in the Commands section: `#notename` (any message starting with `#` is read as `/get`, `notes.go:1151`), `@admin` and `@admins` in reports (regexp `(?i)(^|\s)@admins?(\s|$)`, `reports.go:90`).

## C1. Command inventory: aliases, arguments, who may run, source

Legend for the "who" column: **D** = proven by the descriptor's `RequiredChecks` (file:line), **H** = proven by a guard call inside the handler (function named), **T** = stated by the shipped help text (`locales/en.yml` key) and not separately proven in code; a writer may use a **T** statement only in the help text's own words. `restrict checks` = `CheckDisabled, RequireGroup, RequireUserAdmin, RequireBotAdmin, CanUserRestrict, CanBotRestrict` (`bans.go:1071-1080`); `delete variant` adds `CanBotDelete, CanUserDelete` (`bans.go:1082`). `<user>` in help text means a reply, `@username`, a mention or a numeric ID. Durations are `m`, `h`, `d` (`bans_help_msg`); the staff commands also take `w`.

### Bans (`bans.go:1122-1137`, anonymous-admin re-entry `bans.go:1145-1152`)

| Command | Arguments and effect | Who | Evidence |
|---|---|---|---|
| `/ban <user> [reason]` | ban; `/sban` silently and deletes the command; `/dban` bans the author of the replied-to message and deletes that message; `/tban <user> x(m/h/d)` temporary | admin with restrict right, bot admin with restrict right, group only; sban and dban also need delete rights | D `bans.go:1103-1109`; `bans_help_msg` |
| `/unban <user>` | lifts a ban | same as ban | D `bans.go:1107` |
| `/kick`, `/skick`, `/dkick` | remove (can rejoin), silently, or kick and delete the replied-to message (reply required, `moderationDkick` deletes the reply then kicks) | restrict checks (`/skick`, `/dkick` delete variant) | D `bans.go:1110-1112`; `bans.go:80-97` |
| `/kickme`, `/banme` | remove yourself | any member in a group; bot must be able to restrict; Disableable | D `bans.go:1108,1113` |
| `/unbanall` | owner-only confirmation to unban known users | group creator, bot can restrict | D `bans.go:1109` (`RequireUserOwner`) |
| `/restrict`, `/unrestrict` | inline keyboard: kick, ban, mute / unmute, unban | user can restrict, bot can restrict, group only | D `bans.go:1114-1115` |

### Mutes (`mute.go:305-309`, re-entry `mute.go:315-319`)

`/mute`, `/smute`, `/dmute`, `/tmute <user> x(m/h/d)`, `/unmute`: same meaning as the ban variants. Who: restrict checks (`mute`, `tmute`, `unmute`) and delete variant (`smute`, `dmute`). D `mute.go:294-298`; `mutes_help_msg`.

### Warns (`warns.go:980-1001`)

| Command | Effect | Who | Evidence |
|---|---|---|---|
| `/warn <user> [reason]`, `/swarn`, `/dwarn` | warn; silently and delete the command; warn and delete the replied-to message | restrict checks | D `warns.go:968-970` |
| `/warns` | show a user's warnings | any member (Disableable) | D `warns.go:955,973` |
| `/rmwarn` (`/unwarn`), `/resetwarns` (`/resetwarn`) | remove the latest warning; reset to 0 | group, bot admin, user admin | D `warns.go:958-965,971-972`, aliases `warns.go:953-954` |
| `/resetallwarns` | delete every warning in the chat | group creator (confirmation button) | H `resetAllWarns` calls `RequireUserOwner` (`warns.go:775,787`); old README did not say so |
| `/warnings` | the chat's warn settings | group, bot admin, user admin | D `warns.go:1001-1009` |
| `/setwarnlimit <n>`, `/setwarnmode <ban/kick/mute>` | limit (1 to 100 on the menu page; default 3) and action at the limit (default mute) | admin, bot admin | H `IsUserConnected(true,true)` `warns.go:641`; defaults `fuku/db/warns/repository.go:25` |

### Purges (`purges.go:506-509`, re-entry `purges.go:516-517`)

`/purge` (delete from the replied-to message to now), `/del` (delete the replied-to message), `/purgefrom` and `/purgeto` (mark a start, then purge up to an end). Who: group, bot admin, bot can delete, user admin, user can delete. D `purges.go:477-485`; `purges_help_msg`.

### Admin (`admin.go:853-862`, `admin.go:864`, re-entry `admin.go:945-947`)

| Command | Who | Evidence |
|---|---|---|
| `/adminlist` | anyone in a group, but the bot must be an admin; Disableable | D `admin.go:753` |
| `/promote`, `/demote` `<user>` | group, user admin, bot admin, user can promote, bot can promote | D `admin.go:762-772` |
| `/title <user> <custom title>` | same promote checks | D `admin.go:782` |
| `/setgtitle` (`/setgrouptitle`), `/setgpic` (`/setgrouppic`, reply to a photo), `/setgdesc` (`/setgroupdesc`, `/setdescription`) | user admin and bot admin, both with change-info right | D `admin.go:815-837` |
| `/invitelink` | group, bot admin, user can invite | D `admin.go:792` |
| `/clearadmincache` | group, bot admin, user admin | D `admin.go:800` |
| `/admincache` | refreshes the admin list; the sender must be a creator or admin, the bot must be admin, group only | H `adminCache` `admin.go:704-745` |
| `/anonadmin <on/off>` | with no argument shows the mode; changing it is for the group creator only; bot must be admin | D `admin.go:808`, H `anonAdmin` `RequireUserOwner` `admin.go:675` |

### Approvals (`approvals.go:513-517`)

`/approve <user> [reason]`, `/unapprove`, `/approval`, `/approved`: admin (H `RequireUserAdmin`). `/unapproveall`: group creator (H `approvals.go:385-389`). Approved users skip antiflood, blacklists, locks and captcha (`approvals_help_msg`; AGENTS "Subsystem traps").

### Antiflood (`antiflood.go:694-698`)

`/flood` (settings), `/setflood <number/off/no>` (3 to 100 messages; 0, off or no disables, `applyFloodLimit` `antiflood.go:478-490`), `/setfloodmode <ban/kick/mute>` (`floodModes`, `antiflood.go:432,493-498`), `/delflood` (`/clearflood`) `<yes/no/on/off>` (also delete flood messages). Who: admin commands (`antiflood_help_msg`), handlers call `IsUserConnected`. Counters are per replica in memory (AGENTS "Subsystem traps").

### Lockdown and raid (`lockdown.go:661-664`, `antiraid.go:167-168`, re-entry `lockdown.go:674-677`, `antiraid.go:178`)

| Command | Meaning | Who | Evidence |
|---|---|---|---|
| `/lockdown [reason]` | lock now, no confirm step; only admins can talk (approved users included) and new members are removed until the lift | group (supergroup needed), live creator or admin with can_restrict_members, checked with a live `getChatMember`; bot must be admin with can_restrict_members | D `lockdown.go:60`; `lockdown_help_msg` |
| `/unlockdown` | lift; permissions restored exactly; the people the bot removed are unbanned, except accounts banned for good with Ban recent joiners | same as `/lockdown` | D `lockdown.go:64` |
| `/lockdownstatus` | locked or not, since when, by whom, why | any live creator or admin | D `lockdown_status.go:21` |
| `/autolockdown [joins\|flood\|ai] [N\|on\|off]` | show or change the automatic triggers; a number from 3 to 500 sets the threshold and turns it on | any admin can look; only the creator or an admin with can_restrict_members can change | D `autolockdown.go:37`; `fuku/db/raiddetection/repository.go:32-35` |
| `/autoantiraid N\|off` | alias of the join-surge part of `/autolockdown`, same checks | same | D `antiraid.go:52`, `antiraid_help_msg` |
| `/antiraid`, `/raidtime`, `/raidactiontime` | retired: one pointer command (`antiraidRetiredDesc`, Aliases `raidtime`, `raidactiontime`, no checks) that replies `lockdown_antiraid_retired` and changes nothing; an old `antiraid` button is answered once with the same text; nobody is temp-banned by raid mode any more | anyone | D `antiraid.go:43-47`, `antiraid.go:57-66`; `antiraid_help_msg` |

### Captcha (`captcha.go:2383-2397`)

| Command | Meaning | Who |
|---|---|---|
| `/captcha <on/off>` | turn verification on or off; new members are muted until they pass | group, user admin, bot admin |
| `/captchamode <math/text/turnstile>` | challenge type; default math | group, user admin; `turnstile` is refused unless the web captcha is configured (see E7) |
| `/captchatime <1-10>` | timeout in minutes, default 2 | group, user admin |
| `/captchaaction <kick/ban/mute>` | action on failure or timeout, default kick | group, user admin |
| `/captchamaxattempts <1-10>` | maximum attempts, default 3 | group, user admin, bot admin |
| `/captchapending`, `/captchaclear` | view or clear messages held from users with a pending captcha | user admin |

All D `captcha.go:2383-2395`; defaults `fuku/db/models/captcha.go:8-12`. The shipped help text writes `/captchaattempts`, which is NOT a registered name: the README must use `/captchamaxattempts`.

### AI spam filter (`aispam.go:961`, desc `aispam.go:100`)

`/aispam` shows whether the filter is on and exactly what is sent to the model; `/aispam <on/off>` turns it on or off for the chat. Group, user admin, Disableable (D `aispam.go:100`). Opt-in per chat; deletes quietly (no warn, no mute, no reply) when the model is confident: 80 percent for English text, 90 percent for other languages (`aispamThresholdEnglish = 0.8`, `aispamThresholdNonEnglish = 0.9`, `aispam.go:50-51`; `aispam_help_msg`). Needs `TYPESAFE_API_KEY`; deletions are mirrored to `MESSAGE_DUMP` (old README claim, see E5).

### Blacklists (`blacklists.go:808-821`)

| Command | Meaning | Who |
|---|---|---|
| `/blacklists` (`/blocklists`) | list the words | anyone (`CheckDisabledCmd`) |
| `/addblacklist` (`/blacklist`, `/addblocklist`, `/blocklist`) `<words>` | add words | admin with restrict right, bot admin with restrict right (H `addBlacklist`) |
| `/rmblacklist` (`/rmblocklist`, `/unblacklist`) `<words>` | remove | same |
| `/blaction` (`/blacklistaction`) `<mute/kick/ban/warn/none>` | action on a match; default none = delete only (`blacklists_help_msg`) | same rights |
| `/remallbl` (`/rmallbl`, `/rmblocklistall`, `/rmallblocklist`) | remove every word | group creator (H `rmAllBlacklists` `blacklists.go:553-564`) |

A one-character word outside Unicode category So (and a blank one) is refused (AGENTS "Subsystem traps", A19-2).

### Locks (`locks.go:583-587`)

`/lock <type>`, `/unlock <type>`, `/locks`, `/locktypes`. Lock and unlock: admin plus bot admin (H, `IsUserConnected(true,true)`, `RequireUserAdmin`, `RequireBotAdmin` in `lockPerm`); `/locks` and `/locktypes` anyone. 22 types, listed in section B. Admins and approved members are never affected.

### Notes (`notes.go:1138-1157`)

`/get <name>` and `#name` (anyone); `/save` (`/addnote`) `<name> <text>` (reply to a message to save it, media included) and `/clear` (`/rmnote`) `<name>`: can_change_info (H `CanUserChangeInfo`); `/notes` (`/saved`) list; `/privnote` (`/privatenotes`) toggles sending notes in PM through a button; `/clearall`: group creator with a confirmation button (H `rmAllNotes` `notes.go:641`, status creator). The old README did not say `/clearall` is creator-only.

### Filters (`filters.go:899-906`)

`/filter` (`/addfilter`) `<trigger> <reply>` (quote multi-word triggers; reply to a file to store it; `{admin}` in the reply makes it fire only for admins, `filters_help_msg`), `/filters` (anyone), `/stop` (`/rmfilter`, `/removefilter`) `<trigger>`: can_change_info (H `CanUserChangeInfo`); `/stopall`: group creator with a confirmation (H `rmAllFilters` `filters.go:557`). A chat holds at most 150 filters (`filtersCapMax`, `filters.go:57`).

### Greetings (`greetings.go:1504-1524`)

`/welcome`, `/goodbye` `<yes/no/on/off>`; `/setwelcome`, `/setgoodbye` `<reply/text>`; `/resetwelcome`, `/resetgoodbye`; `/cleanwelcome` (and `/cleangoodbye`) `<yes/no/on/off>` delete the previous message; `/cleanservice <yes/no/on/off>` delete "x joined the group" service messages; `/autoapprove <yes/no/on/off>` approve join requests automatically (`greetings_help_msg`). Who: T "Admin Commands". Descriptors carry only `CheckDisabled` (D `greetings.go:1519`); the handler gates differ per command (the menu mirrors them: welcome and goodbye switch need an admin, clean* need can_change_info, autoapprove needs the bot as admin and can_change_info, AGENTS "Permissions").

### Rules (`rules.go:522-532`)

`/rules` (anyone, group), `/setrules <text>`, `/resetrules` (`/clearrules`), `/privaterules <yes/no/on/off>`, `/rulesbtn` (`/rulesbutton`) `<text>`, `/resetrulesbtn` (`/resetrulesbutton`, `/clearrulesbtn`, `/clearrulesbutton`). Admin commands (T `rules_help_msg`; H `IsUserConnected(true,true)` in `setRules`). Rules can carry one media item and URL buttons (E9).

### Pins (`pins.go:745-752`, re-entry `pins.go:768-771`)

`/pinned` (group, bot admin, anyone), `/pin [loud]` (`loud` or `notify` notifies members), `/permapin <text>`, `/unpin`, `/unpinall`: group, user admin, bot admin, both can pin (D `pins.go:688-721`); `/antichannelpin <yes/no/on/off>`, `/cleanlinked <yes/no/on/off>`: admin (H `IsUserConnected` `pins.go:354,430`).

### Reactions (`reactions.go:41-44`)

`/addreaction <keyword> <emoji>`, `/removereaction <keyword>`, `/resetreactions`: can_change_info (H); `/reactions` list. `reactions_help_msg`.

### Formatting (`formatting.go:190`)

`/markdownhelp` (`/formatting`): interactive guide to markdown, buttons and fillings such as `{first}`.

### Federations (`federations.go:1243-1272`)

By help text (`federations_help_msg`, T): owner in PM: `/newfed <name>`, `/renamefed <name>`, `/delfed` (confirm), `/fedpromote`, `/feddemote`, `/fedreason on/off`, `/fednotif on/off`, `/subfed`, `/unsubfed`, `/fedsubs` (a federation may subscribe to at most 5 others, `maxFedSubs = 5` `fuku/db/federations/repository.go:23`), `/fbanlist [csv|json|minicsv]`, `/importfbans` (replied CSV or JSON), `/setfedlog`, `/unsetfedlog`; group owner: `/joinfed <fed_id>`, `/leavefed`, `/quietfed on/off`; federation admins: `/fban`, `/unfban` (`/funban`), `/fedstat` (`/fbanstat`), `/feddemoteme`; anyone: `/fedinfo`, `/fedadmins`, `/chatfed`, `/myfeds`. Fed-ban check runs at handler group -6.

### Connections (`connections.go:353-357`)

`/connect <chatid>`, `/disconnect`, `/reconnect`, `/connection` (private chat), `/allowconnect <yes/no>` (admin). Connected admins edit the chat from PM; any member can view; banned or kicked users cannot connect (`connections_help_msg`).

### Log channels (`logchannels.go:323-328`)

`/setlog` (send it in the channel, then forward that message to the group; the admin check of `/setlog` itself is T only), `/unsetlog` (admin, H), `/logchannel`, `/logcategories`, `/log <category>`, `/nolog <category>` (admin, group only, needs a linked channel; H `setLogCategories` `logchannels.go:206-224`). Categories: settings, admin, user, automated, reports, other.

### Reports (`reports.go:620-622`)

`/report [reason]` and `@admin` (reply to a message; not triggered by admins); `/reports <on/off/yes/no>`: admin in a group, toggles the sender's own status in PM; `/reports block|unblock` (reply) and `/reports showblocklist`. T `reports_help_msg`; H `reports` `RequireUserAdmin`, `RequireBotAdmin`.

### Disabling (`disabling.go:264-269`)

`/disable <command>`, `/enable <command>`, `/disableable`, `/disabled`, `/disabledel <yes/no/on/off>`. A disabled command stops working for non-admins only; `/disabledel` deletes disabled commands used by non-admins. T `disabling_help_msg`.

### Misc (`misc.go:487-498`, `misc_extras.go:303-312`)

`/info [user]`, `/id` (chat ID, or the replied-to user's ID), `/ping`, `/stat` (message count; group only, H `RequireGroup`), `/tr <lang code> <text or reply>`, `/tell <text>` (admin, group: H `IsUserAdmin`), `/stickerid`, `/zombies` (kick deleted accounts the bot knows about: user admin, bot can restrict; H), `/github <user|user/repo>`, `/wiki <search>` (English Wikipedia), `/ud <term>`, `/removebotkeyboard`, `/runs` (a random quip). T `misc_help_msg`.

### Backup, languages, help (`backup.go:759-761`, `language.go:235`, `help.go:619-624`)

`/export [modules…]` (group, user admin, bot admin; H `exportHandler`), `/import [modules…]` (reply to a backup file; group, bot admin, group creator; H `validateImportRequest` `backup.go:292-304`), `/reset [modules…]` (group creator, bot admin; H `resetHandler`). Rate limits per chat: export once per 5 minutes, import once per 10 minutes, reset once per hour (`ratelimit.DefaultExportCooldown`, `DefaultImportCooldown`, `DefaultResetCooldown`, `fuku/utils/ratelimit/backup_ratelimit.go:39-41`). `/lang` (private: your own language; group: admin sets the group's language, H `changeLanguage`). `/start`, `/help [module]`, `/about`, `/donate`.

### Dev (`devs.go:417-425`)

`/stats`, `/chatinfo`, `/chatlist`, `/leavechat`: bot owner or a dev (H `IsDev` and owner id `devs.go:388`); `/teamusers`: owner or team (H `devs.go:285-300`); `/addsudo`, `/remsudo`, `/adddev`, `/remdev`: bot owner only (H `manageTeamRole` `devs.go:191-193`). The `/stats` text also carries the in-memory AI and web captcha counters (AGENTS "Data").

### Staff Group and forward blocklist (`staff.go:398-406`, `staff_forwardblock.go:41-75`)

| Command | Meaning | Who | Evidence |
|---|---|---|---|
| `/setstaff` | make this group your Staff Group | group creator, sent as yourself (not anonymously) | D `staff.go:27` (`RejectAnonymousSender`, `RequireGroup`); T `staff_help_msg` |
| `/unsetstaff` | remove Staff status and unlink every group (also deletes its forward blocklist) | group creator | D `staff.go:31`; T |
| `/linkstaff [ID]` | link this group to your Staff Group (add the Staff Group chat ID when you own more than one) | creator of both groups | D `staff_link.go:66`; T |
| `/unlinkstaff` | unlink this group | creator of both groups | D `staff_unlink.go:42`; T |
| `/staff` | in the Staff Group: its chat ID, linked groups with Unlink buttons, Recent actions (10 a page, `staffHistoryPageSize` `staff_history.go:23`), a lock line for a group in lockdown | answered only inside a Staff Group (a chat that is not one gets silence, `staff.go:172-175`); the command has no member check of its own, the buttons (Unlink, Recent actions, Undo) check the presser live; writers: say "in the Staff Group" and do not promise an admin check on the command | D `staff.go:35` (no checks), H `staffPanel` `staff.go:171-190`; T |
| `/ban /tban /mute /tmute /kick /unban /unmute` in the Staff Group | act in every linked group; `/ban` and `/mute` take an optional duration, `/tban` and `/tmute` require it; a card first, only the sender confirms, expires after 5 minutes; result lists every group as done, skipped or failed; Undo button | per group: only where the issuer is a live admin who can restrict members, never against a group's admins or owner | `staff_action.go:56-74`; `staffActionCardLifetime = 5 * time.Minute` `staff_action_card.go:36`; T |
| `/addfwdblock <ID or @username>`, `/rmfwdblock <ID>` | add or remove a blocked forward source (channel or group) | the Staff Group's live creator | D `staff_forwardblock.go:30-76` (`forwardBlockChecks(true)`: `RejectAnonymousSender`, `RequireGroup`, `requireStaffGroup`, `RequireUserAdmin`, `requireStaffGroupCreator`) |
| `/fwdblocklist`, `/fwdblockaction [none\|warn\|mute\|kick\|ban]` | show the list; show or set the one action (default none = delete only); setting it is a write | any admin of the Staff Group can read; only the creator writes | D `forwardBlockChecks(false)`; creator check inside the handler |

### Settings (`settings.go:233`, `settings_input.go:681-716`)

`/settings`: opens the button menu. In a group it opens there or in private; in a private chat with the bot it lists the groups you manage. Only the group's owner can open it unless the owner lets admins in (E10). `/cancel`: private chat only; ends the wait for a typed reply of the editor (`settings_edit_cancelled`); in any other chat it is not the bot's command and passes through (`settingsCancelPassOn`). D `settings.go:40`, `settings_input.go:681`.

## D. Environment

Every name below is read by non-test code (`grep -rhoE 'Getenv\("[A-Z][A-Z0-9_]+"\)' fuku main.go --include='*.go' --exclude='*_test.go' | sort -u` printed 57; 56 are bot variables, `FUKU_TEST_DATABASE` is tooling). `config.go` calls `setDefaults()` (line 473) and then `ValidateConfig` (line 475), so a default below is applied before the required check. Booleans accept `true`, `yes` or `1` only (`fuku/config/types.go:46-49`); an integer that does not parse falls back to the default with a warning (CACHE_LOCAL_TTL logs it, `config.go:122-136`; the others go through `typeConvertor.Int`). The webhook route is `/webhook` (`fuku/utils/httpserver/server.go:225`).

| Name | Default | Required | Read at | Notes |
|---|---|---|---|---|
| `BOT_TOKEN` | none | yes (`config.go:332`) | `config.go:401` | |
| `OWNER_ID` | none | yes, greater than 0 (`config.go:335`) | `config.go:407` | the owner can use the Dev commands |
| `MESSAGE_DUMP` | none | yes, greater than 0 (`config.go:338`) | `config.go:408` | receives the "Started Bot!" message (`main.go:538-540`) and the AI spam deletion mirror (`aispam.go:373`) |
| `DATABASE_URL` | none | yes (`config.go:341`) | `config.go:411` | PostgreSQL DSN |
| `REDIS_ADDRESS` | `localhost:6379` (`config.go:516-518`) | yes unless `DISABLE_CACHE` (`config.go:344`) | `config.go:39` | takes precedence over `REDIS_URL` |
| `REDIS_URL` | none | alternative to the address | `config.go:43` | used only when `REDIS_ADDRESS` is empty; its host becomes the address and its password the password |
| `REDIS_PASSWORD` | empty | no | `config.go:65` | overrides the password in `REDIS_URL` |
| `REDIS_DB` | 1 (`config.go:90-101`) | no | `config.go:91` | integer 0 to 15 |
| `DISABLE_CACHE` | false | no | `config.go:436` | skips the read-through cache; the bot can continue without Redis with degraded caching and state (`fuku/utils/cache/cache.go:82-115`) |
| `CACHE_LOCAL_TTL` | 10 seconds | no | `config.go:123` | 0 to 300; 0 turns the in-process layer off |
| `CACHE_LOCAL_MAX_ENTRIES` | 50000 | no | `config.go:439` | 100 to 1000000 |
| `CLEAR_CACHE_ON_STARTUP` | false | no | `config.go:435` | clears the cache keys at startup (`fuku/utils/cache/cache.go:122`); operational Redis keys outside the `fuku:cache:` prefix are kept (AGENTS "Data") |
| `API_SERVER` | `https://api.telegram.org` | no | `config.go:403` | custom Bot API server |
| `USE_WEBHOOKS` | false | no | `config.go:427` | false means long polling; true needs the two names below |
| `WEBHOOK_DOMAIN` | none | with webhooks | `config.go:428` | public base URL; the bot registers `<domain>/webhook` (`server.go:228`) |
| `WEBHOOK_SECRET` | none | with webhooks | `config.go:429` | checked from the header `X-Telegram-Bot-Api-Secret-Token` (`server.go:382`) |
| `DROP_PENDING_UPDATES` | false | no | `config.go:409` | |
| `HTTP_PORT` | 8080 | no | `config.go:106` | 1 to 65535; falls back to `PORT` (`config.go:108`) |
| `PORT` | none | no | `config.go:108` | used when `HTTP_PORT` is unset; `--health` reads the same two names (`main.go:364`) |
| `DISPATCHER_MAX_ROUTINES` | 200 | no | `config.go:433` | 1 to 1000 |
| `HTTP_MAX_IDLE_CONNS` | 100 | no | `config.go:445` | |
| `HTTP_MAX_IDLE_CONNS_PER_HOST` | 50 | no | `config.go:446` | |
| `ENABLED_LOCALES` | `uk,en,es,fr,hi,id,pt,ru` | no | `config.go:496` | `uk` is always offered (`config.go:498-503`) |
| `DEBUG` | false | no | `config.go:405` | debug logging with caller info; turns the two monitoring defaults off |
| `DB_MAX_IDLE_CONNS` | 50 | no | `config.go:413` | 1 to 100 |
| `DB_MAX_OPEN_CONNS` | 200 | no | `config.go:414` | 1 to 1000 |
| `DB_CONN_MAX_LIFETIME_MIN` | 240 | no | `config.go:415` | 1 to 1440 minutes |
| `DB_CONN_MAX_IDLE_TIME_MIN` | 60 | no | `config.go:416` | 1 to 60 minutes |
| `AUTO_MIGRATE` | false in code; the deploy manifests set true | no | `config.go:448` | applies `migrations/*.sql` at startup |
| `AUTO_MIGRATE_SILENT_FAIL` | false | no | `config.go:449` | keep running after a checksum mismatch (`migrations/runner.go:196-226`); development only |
| `MIGRATIONS_PATH` | `migrations` | no | `config.go:450` | |
| `TYPESAFE_API_KEY` | none | no | `config.go:461` | the AI spam filter and the raid AI text checks are inert without it |
| `ENABLE_AISPAM` | true (`config.go:591-593`) | no | `config.go:460` | global kill switch for every AI check, the spam-name rule included |
| `ANTHROPIC_API_KEY` | none | no | `config.go:463` | raid image checks and the AI spam-name rule; without it neither runs and no image or name leaves the server |
| `ANTHROPIC_MODEL` | `claude-haiku-5-5` (`fuku/utils/claude/client.go:28`) | no | `config.go:464` | must accept a forced tool call; an invalid name is replaced by the default with a warning |
| `METRICS_AUTH_TOKEN` | none | no | `config.go:458` | when set, `/metrics` and `/db_metrics` need `Authorization: Bearer <token>` (`server.go:159-166`) |
| `ENABLE_PERFORMANCE_MONITORING` | true when `DEBUG` is false and the name is unset | no | `config.go:431` | |
| `ENABLE_BACKGROUND_STATS` | true when `DEBUG` is false and the name is unset | no | `config.go:432` | |
| `ENABLE_DB_MONITORING` | false | no | `config.go:418` | checks the DB pool every minute (`main.go:137-141`) |
| `RESOURCE_MAX_GOROUTINES` | 1000 | no | `config.go:452` | |
| `RESOURCE_MAX_MEMORY_MB` | 500 | no | `config.go:453` | |
| `RESOURCE_GC_THRESHOLD_MB` | 400 | no | `config.go:454` | |
| `ENABLE_AUTO_CLEANUP` | true when unset (`config.go:533`) | no | `config.go:443` | marks chats inactive |
| `INACTIVITY_THRESHOLD_DAYS` | 30 | no | `config.go:441` | |
| `ACTIVITY_CHECK_INTERVAL` | 1 hour | no | `config.go:442` | |
| `ENABLE_PPROF` | false | no | `config.go:456` | registers `/debug/pprof/` (`main.go:275`, `server.go:202`); never in production |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | none | no | `fuku/utils/tracing/tracing.go:64` | OTLP gRPC; with this empty and the console exporter off, tracing is disabled (`tracing.go:91`) |
| `OTEL_EXPORTER_OTLP_INSECURE` | false | no | `tracing.go:66` | plain gRPC when `true` |
| `OTEL_EXPORTER_CONSOLE` | false | no | `tracing.go:65` | spans to stderr when `true` |
| `OTEL_SERVICE_NAME` | `fuku_robot` (`tracing.go:33-36`) | no | `tracing.go:33` | |
| `OTEL_TRACES_SAMPLE_RATE` | 1.0 | no | `tracing.go:39` | clamped to 0.0 to 1.0 |
| `CAPTCHA_WEB_BASE_URL` | none | with the other three | `config.go:466` | https URL, host only, path empty or `/` |
| `TELEGRAM_MINI_APP_SHORT_NAME` | none | with the other three | `config.go:467` | 1 to 64 letters, digits, underscores |
| `TURNSTILE_SITE_KEY` | none | with the other three | `config.go:468` | |
| `TURNSTILE_SECRET_KEY` | none | with the other three | `config.go:469` | at least 6 characters; registered with `logredact` (`config.go:610-619`) |
| `CAPTCHA_WEB_TRUSTED_PROXIES` | none | optional | `config.go:470` | comma-separated IPs or CIDRs; setting it without the four above stops startup (`config.go:266`) |
| `FUKU_TEST_DATABASE` | none | tests only | `fuku/db/conn.go:25` | with `DATABASE_URL` opts in to the PostgreSQL-backed tests |

Turnstile rule (E7): all four of `CAPTCHA_WEB_BASE_URL`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET_KEY` or none (`config.go:237-244,246-289`); some but not all stops startup and names each missing variable. Without Redis the web captcha runtime is not installed and Turnstile groups get the math captcha (AGENTS "Startup and shutdown", E7).

Names read by tooling and not by the bot (verified): `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `TELEGRAM_API_ID`, `TELEGRAM_API_HASH` (`debug.docker-compose.yml:13,29,50-51`); `PSQL_DB_HOST`, `PSQL_DB_PORT`, `PSQL_DB_NAME`, `PSQL_DB_USER`, `PSQL_DB_PASSWORD`, `PSQL_DB_SSLMODE` (`Makefile:62-81`, `scripts/migrate_psql.sh:28`); `FUKU_TEST_MIGRATION_CHAIN` (`fuku/db/migrations/repository_chain_test.go:15`).

Compose passes only five variables into the bot container: `BOT_TOKEN`, `OWNER_ID`, `MESSAGE_DUMP`, `WEBHOOK_DOMAIN`, `WEBHOOK_SECRET` (`docker-compose.yml:22-26` from `.env`), and fixes `DATABASE_URL`, `REDIS_ADDRESS`, `REDIS_PASSWORD`, `AUTO_MIGRATE`, `USE_WEBHOOKS`, `HTTP_PORT`, `DROP_PENDING_UPDATES` itself (`docker-compose.yml:15-21`). Any other variable (Turnstile, `METRICS_AUTH_TOKEN`, `TYPESAFE_API_KEY`) must be added under `services.fuku.environment` in an override file. The stack ships fixed credentials (`fuku` / `fuku` for PostgreSQL, `redis` for Redis, `docker-compose.yml:15,17,53-55,74`); neither service publishes a port; change them before reuse. `debug.docker-compose.yml` passes the whole `.env` (`env_file`), publishes PostgreSQL on 5432 and Redis on 6379, uses `postgres:15-alpine` and `redis:latest`, and defines an optional `aiogram/telegram-bot-api` service (no published port).

Endpoints (`fuku/utils/httpserver/server.go`): `GET /health` (JSON: `status`, `checks` for database and redis, `version`, `uptime`; HTTP 503 when unhealthy, lines 118-143); `/metrics` (Prometheus, bearer auth when a token is set, line 178); `/db_metrics` (JSON, same auth, line 186); `POST /webhook` (webhook mode only, 405 for other methods; secret-header checked, lines 225,260-264,382); `/debug/pprof/` (only with `ENABLE_PPROF`, line 202); `/captcha`, `/captcha/verify`, `/captcha/health` only when the web captcha is configured (`fuku/utils/captchaweb/verify.go:47-49`, mounted by `registerCaptchaWeb` `main.go:469-475`). `fuku_robot --health` calls `http://localhost:<port>/health` and exits 0 only on HTTP 200 (`main.go:46-57`); `--version` prints the version and exits (`main.go:59-64`). With `METRICS_AUTH_TOKEN` empty `/metrics` and `/db_metrics` are unauthenticated and the server logs a warning (`server.go:175-190`).

Shutdown (AGENTS "Startup and shutdown", code `fuku/utils/shutdown/graceful.go:29-32`): handlers run in reverse registration order inside a 60 second budget (`shutdownTimeout = 60 * time.Second`), 10 seconds each unless registered with their own timeout; the compose file sets `stop_grace_period: 75s` (`docker-compose.yml:13`) so Docker does not kill the process first.

## E. Behaviour claims register

Each row is one sentence-level claim a README may make. Evidence column names the source; "AGENTS" means the bullet found with `grep -n` in `AGENTS.md` (the section name in quotes). Verdict `verified` means a README may state it as worded; `help-text` means the shipped help text says it (use its wording).

| ID | Claim | Evidence | Verdict |
|---|---|---|---|
| E1 | A Staff Group is one group of trusted admins that manages other groups; the owner makes a group the Staff Group with `/setstaff`, links groups with `/linkstaff`, and a group can be unlinked with `/unlinkstaff`. | `staff_help_msg` (`locales/en.yml:2661`); `staff.go:27-31`, `staff_link.go:66`, `staff_unlink.go:42` | verified |
| E2 | A link is kept only while its maker is the live creator of both groups; the bot rechecks and unlinks when the owner changes (`OwnerMismatch` only; a failed check is unknown and never unlinks). | AGENTS "Subsystem traps" (Staff links); sweeper `StartStaffSweeper` hourly (`main.go:513`) | verified in AGENTS; do not give detail |
| E3 | In the Staff Group `/ban`, `/tban`, `/mute`, `/tmute`, `/kick`, `/unban`, `/unmute` act in every linked group, after a confirmation card that only the sender can confirm and that expires after 5 minutes; the result lists each group as done, skipped or failed; the bot acts in a linked group only where the issuer is a live admin who can restrict members, never against that group's admins or owner. | `staff_help_msg`; `staff_action.go:56-74`; `staff_action_card.go:36` | verified |
| E4 | Every finished staff action has an Undo button usable by any Staff Group member; a kick cannot be undone; each group is put back only where the person pressing it can restrict members and only if nothing changed since. | `staff_help_msg` | help-text |
| E5 | Each applied staff action and each undo is posted to the log channel (admin category) of the group it applied to, naming who and whom but not the Staff Group. | `staff_help_msg`; AGENTS "Subsystem traps" (StaffActions) | verified |
| E6 | `/lockdown` locks a supergroup at once: only admins can talk, new members are removed until the lift; `/unlockdown` restores every permission exactly and unbans the people the bot removed, except accounts banned for good with Ban recent joiners; a lockdown never ends on its own. The bans on joiners last 330 days from each join. Join requests are declined while locked. | `lockdown_help_msg`; `lockdown_marker.go:35` (`lockdownBanSpan`) | verified |
| E7 | Automatic detection starts the same lockdown when 15 different accounts join within 60 seconds (join surge) or when 30 items (messages, photos, GIFs, stickers) arrive within 30 seconds with at least 60 percent from at least 3 members who joined in the last 24 hours (message flood); both are on by default, thresholds 3 to 500 per group, set with `/autolockdown`; admins, approved users and handled commands never count; after a lift the bot does not auto-lock again for 10 minutes; when it sees a raid but cannot lock it warns the Staff Group and the log channel at most once a day. Counters live in Redis and are inert without it; `/lockdown` still works. These settings are not in backups. | `repository.go:23-35` (defaults 15, 30, 5; 3 and 500); `raid_counters.go:31,34`; `raid_flood.go:58-63`; `raid_trigger.go:31,53`; `lockdown_help_msg`; AGENTS "Data" | verified |
| E8 | AI burst trigger: spam verdicts from the AI checks (TypeSafe for suspicious text of members who joined in the last 24 hours, Anthropic Claude for their photos, GIFs and stickers, when each is configured) count; 5 verdicts from at least 3 accounts within 120 seconds lock the group; one verdict never does, and AI never deletes or bans on this path. | `lockdown_help_msg`; `raid_ai_count.go:35-38`; `repository.go:29-31` | verified |
| E9 | Alert and buttons: when a group is locked the bot posts an alert to its linked Staff Group and, when the admin log category is on, to its log channel, then an hourly reminder; the alert has Lift, Ban recent joiners (bans for good, cannot be undone) and Revoke link buttons; every press is checked live against the locked group, being in the Staff Group is not enough. | `lockdown_help_msg`; `lockdown_alert.go:77` (`lockdownReminderEvery = time.Hour`) | verified |
| E10 | `/antiraid`, `/raidtime` and `/raidactiontime` are retired pointers; nobody is temp-banned by a raid mode any more; `/autoantiraid N` still sets the join-surge threshold. | `antiraid.go:43-66`; `antiraid_help_msg` | verified |
| E11 | Captcha modes: math, text and Turnstile (a Cloudflare check on a web page opened as a Telegram Mini App). New members are muted until they pass; default time limit 2 minutes, default action kick, default 3 attempts; one attempt per member and group. | `captcha_help_msg`; `fuku/db/models/captcha.go:8-12` | verified |
| E12 | Turnstile needs all four of `CAPTCHA_WEB_BASE_URL`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET_KEY`; math and text need none of it; without Redis the web captcha is unavailable and Turnstile groups get the math captcha; `/captchamode turnstile` is refused unless the runtime is up, Redis answers and the bot is an admin with the restrict right. The server decides the result (signed Mini App data plus Cloudflare siteverify); the page only shows the answer. The secret key is scrubbed from logs. | `config.go:237-289`; `captcha.go:712`; `captcha-web.md` (read in full); AGENTS "Startup and shutdown" | verified |
| E13 | The captcha routes share the HTTP port with `/metrics`, `/db_metrics`, `/health`, pprof and the webhook; the public proxy must forward only `/captcha` and `/captcha/*`; the bot warns at startup when `METRICS_AUTH_TOKEN` or `CAPTCHA_WEB_TRUSTED_PROXIES` is empty; the secret key must never be committed. | `captcha-web.md` ("One port, many endpoints"); `captcha_web.go:184-195` | verified |
| E14 | AI spam filter: opt-in per chat with `/aispam on`, deletes quietly (no warn, no mute, no reply) when the model is confident: 80 percent for English, 90 percent for other languages; inert without `TYPESAFE_API_KEY`; `ENABLE_AISPAM=false` turns every AI check off. | `aispam_help_msg`; `aispam.go:50-51`; `config.go:591-593` | verified |
| E15 | Antiflood: `/setflood` takes 3 to 100 messages in a row, 0, off or no disables; action ban, kick or mute; the first write on a chat with no row stores a limit of 5. | `antiflood.go:478-498`; AGENTS "Subsystem traps" (D-27) | verified |
| E16 | Blacklists: action mute, kick, ban, warn or none (none only deletes); a one-character word outside symbols is refused. Locks: 22 types; admins and approved members are exempt. Filters: at most 150 per chat. | `blacklists_help_msg`; `locks.go:61-97`; `filters.go:57`; AGENTS | verified |
| E17 | Warns: default limit 3 and default action mute; `/setwarnmode` takes ban, kick or mute. | `fuku/db/warns/repository.go:25`; `warns_help_msg` | verified |
| E18 | Member requirements: eight rules (no last name, no username, Russian letters, Arabic letters, Chinese characters, no visible profile photo, not subscribed to the group's linked channel, a spam-like name judged by AI), each with its own punishment off, warn, kick, ban or mute; every rule starts off; set only in `/settings` (no command); supergroups only; members are checked at their next message and, with Check on join, right after the captcha; a muted member gets an "I fixed it" button that frees only them and only from a mute the requirements placed; a check that cannot be answered punishes nobody. The AI rule sends only first name, last name and username to Anthropic's Claude, never messages, and is off when `ENABLE_AISPAM` is false or `ANTHROPIC_API_KEY` is empty. | `memberreq/settings.go:28-39`; `memberreq.go:48-52`; `settings_mreq_*` locale keys (`locales/en.yml:3311-3354`) | verified |
| E19 | The Russian-letters rule means the letters ы, ъ, э and ё; Arabic and Chinese mean those scripts; the rules go by the first and last name, never the username. | `settings_mreq_hint_letters` (`locales/en.yml`) | verified; README.uk.md must keep those four letters inside a code span or omit them (the checker rejects them in Ukrainian prose) |
| E20 | Scheduled deletion (Deletion page of `/settings`): 12 service message kinds (delete at once or after 10 seconds, 1 minute, 10 minutes, 1 hour, 6 hours, 24 hours, or keep), 5 bot message kinds (welcome, goodbye, rules, automatic punishment notices, manual punishment replies; off by default; 1 minute to 24 hours) and 3 member rules (Commands, Edited messages with a grace of 30 seconds, 1, 5, 15 or 60 minutes, Self-destruct 1, 5, 10 minutes or 1, 6, 24 hours). Everything starts at Keep or Off; the queue survives a restart; a message older than 47 hours is dropped quietly; the page does not exist in a Staff Group and Staff Group messages, lockdown alerts, captcha challenges and log posts are never deleted. | `deletion/kinds.go:19-98`; `deletion_kinds.go:54-59,66-227`; `deletion_worker.go:46`; AGENTS "Handlers" (ServiceDeletion, MemberDeletion), "Data" | verified |
| E21 | Command deletion: with Member messages > Commands on, every command the bot handles is deleted after it answered, an admin's included; another bot's command or an unknown word is never touched. | `command_deletion.go` via `main.go:431`; AGENTS "Handlers" (Command deletion) | verified |
| E22 | Settings menu: `/settings` opens a button menu; only the group's owner can open it unless the owner lets admins in on the Access page; a menu opened in the group answers only to the person who opened it; every press re-checks that the person is still allowed; every change is the same as the matching command; no state is kept by the menu itself. | `settings_help_msg`; `settings.go:40`; `settings_gate.go:89`; AGENTS "Callbacks", "Permissions" | verified |
| E23 | Pages in the menu (names from `locales/en.yml:3036-3481`): Staff Group, Lockdown, Captcha, Antiflood, Warnings, Locks, Welcome, Goodbye, Rules, Notes, Filters, Blacklist, Requirements, Deletion, Switches, Language, Log channel, Anonymous admins, Access (owner only), and Blocked sources (Staff Group only). Welcome, Goodbye and Rules take their text, media and buttons from a message the owner sends to the bot in private; `/cancel` ends the wait. | `registerSettingsPage` calls (grep output above); `settings_edit.go:727` | verified |
| E24 | The Blacklist, Filters and Notes pages work only in the bot's private menu (a group menu shows a note); words, keywords and notes are added by pressing Add and typing in private; delete asks Yes or No; a filter keyword or note name that exists is refused, not overwritten. | `settings_list.go:23,628`; AGENTS "Callbacks" (list engine) | verified |
| E25 | Forward blocklist: a Staff Group keeps one list of blocked sources (channels and groups, at most 200) and one action (none, warn, mute, kick, ban; default none = delete only); in every linked group the bot deletes a message forwarded from a listed source and applies the action to the member; only the Staff Group's live creator changes the list, other admins of the Staff Group can view; the list is edited by `/addfwdblock`, `/rmfwdblock`, `/fwdblockaction` or the Blocked sources page. | `staff_forwardblock.go:30-76`; `fuku/db/staff/forwardblock.go:23`; `staff_help_msg` | verified |
| E26 | Forward blocklist limit: a forward from a normal member's own message of another group is labelled by Telegram with that member (a user), so it can never match; only forwards of channel posts and of a group post made by an anonymous admin match. Members who are admins or approved, channel identities posting as the channel, and anonymous admins posting a blocked forward are exempt. | `forwardBlockSourceOf` `staff_forwardblock_watch.go:173-190` (a `user` or `hidden_user` origin has no chat, so nothing matches); `staff_help_msg` last line; AGENTS "Handlers" (StaffForwardBlock) | verified |
| E27 | Approved users skip antiflood, blacklists, locks and captcha. | `approvals_help_msg` | verified |
| E28 | Federations share a ban list across groups; one federation per user; a federation may subscribe to at most 5 others. | `federations_help_msg`; `fuku/db/federations/repository.go:23` | verified |
| E29 | Backup: `/export` writes the group's settings to a JSON file, `/import` restores from a replied backup file (creator), `/reset` resets to defaults (creator); limits per chat: export 1 per 5 minutes, import 1 per 10 minutes, reset 1 per hour. Lockdown, staff, deletion, member requirement and forward blocklist tables are never part of backups. | `backup_help_msg`; `ratelimit/backup_ratelimit.go:39-41`; AGENTS "Data" (each table's last sentence) | verified |
| E30 | Eight languages (en, es, fr, hi, id, pt, ru, uk); Ukrainian is the default; `ENABLED_LOCALES` limits `/lang`. | `ls locales`; `config.go:496-504` | verified |
| E31 | Anonymous admins: an anonymous admin gets a proof-tap prompt before admin commands; after the tap the command runs as that person; the AnonAdmin mode can be switched by the group creator. | `RegisterAnonymousAdminHandler` sites; `admin.go:675`; AGENTS "Handlers" | verified |
| E32 | Storage: PostgreSQL through GORM; Redis as shared cache and for operational state; an in-process LRU in front (10 seconds by default); every write drops the cache keys it affects; schema comes from `migrations/*.sql` with a SHA-256 checksum per file (`AUTO_MIGRATE=true`). | `config.go:122-136`; AGENTS "Data", "Migrations" | verified |
| E33 | Operations: health, metrics, optional pprof and OpenTelemetry tracing; logs are JSON; configured secrets are scrubbed from logs (`logredact`); the dispatcher runs up to 200 updates at once by default; shutdown drains in-flight work inside 60 seconds. | section D; `config.go:610-619` | verified |
| E34 | Polling is the default; with webhooks the bot registers `<WEBHOOK_DOMAIN>/webhook` with the secret token; in polling mode the bot deletes any webhook first. The HTTP server runs in both modes. | `main.go:280-297,336-339` | verified |
| E35 | `#notename` works like `/get`; `@admin` (or `@admins`) in a reply reports it to the admins. | `notes.go:1151`; `reports.go:90` | verified |

## F. Stale claims register (old README seed claims)

| Old claim | Verdict | Live value / evidence |
|---|---|---|
| `main` is the original code unchanged (commit `34ba3f6`, version 2.23.14) | false | the working branch is at release 2.24.0 (`config.go:402`); `34ba3f6` is an ancestor (`git merge-base --is-ancestor`); `origin/main` still reads 2.23.14 until the draft PR merges |
| 30 modules and more than 200 command triggers | false | 39 `RegisterLegacyModule` calls and 231 command names (section B); the generated docs index says "30 modules, 159 commands" and `make inventory` another number: both undercount, cite neither |
| AntiRaid as a live module with `/raidtime` and `/raidactiontime` timings, "default 6h" and "1h" | false | those commands are retired pointers (E10); automatic detection replaces the raid mode (E7) |
| 42 migrations | false | 64 |
| Handler group `-5 antiraid`; "Redis holds the anti-raid state" | false | group -5 is the service message deletion watcher; the old `fuku:antiraid:*` keys are removed by a startup sweep; the raid counters live under `fuku:raid:*` in Redis (AGENTS "Data") |
| Compose pulls the other project's image | false | `ghcr.io/uasneppy/fuku_robot:latest` (`docker-compose.yml:9`); commit `54dac65` moved the release image to this repository's registry; `.github/workflows/release.yml:254-263` names the same image |
| `app.json` points at the other project | false | `"repository": "https://github.com/uasneppy/fuku_robot"` (`app.json:4`) |
| Captcha is only math or text | false | Turnstile is a third mode (E11, E12) |
| AI filter acts from 80 percent for English and 90 percent for other languages | true | `aispam.go:50-51` |
| Dev-branch roadmap, "What's different from upstream", "Keeping the fork in sync", all credit text | dropped | D-02; not carried over |
| CI uses PostgreSQL 16, Compose PostgreSQL 17 and Redis 7 | true | `ci.yml:251,264`; `docker-compose.yml:49,71` |
| CI floor 78 percent coverage; gosec and govulncheck; docs and translation drift checks; Docker build test | true | `ci.yml:376,52-68,405-424,425` |
| The release pipeline still publishes to the other project's image name | false (stale) | `release.yml:254` names `ghcr.io/uasneppy/fuku_robot`; `.goreleaser.yaml:100` too |
| Beyond the plan: the old Getting started told readers to build a local override because the stock image was another project's | drop | the stock image is this repository's now |
| Beyond the plan: old Moderation table said `/adminlist` is for everyone | refine | anyone, but the bot must be an admin in the group (`admin.go:753`) |
| Beyond the plan: old tables omitted that `/clearall`, `/stopall` and `/resetallwarns` are group-creator-only | refine | section C1 |
| Beyond the plan: help text and old README disagree on `/captchamaxattempts` vs `/captchaattempts` | README uses the registered name | `captcha.go:2392` |
| Beyond the plan: the old README says the debug stack uses "PostgreSQL" without a version | refine | `debug.docker-compose.yml` uses `postgres:15-alpine` and `redis:latest`; say nothing about versions there |

## G. Stated figures

The only numbers allowed in README prose (outside code blocks, code spans, URLs, HTML tags and pipe-table cells). One line each: `- <number> - <what, and the command or file:line that produced it>`. A writer may use a number only in the sense given here. Write each number exactly as shown (for example `2.24.0`, not `v2.24.0` inside a longer token; `v2.24.0` still yields the token `2.24.0`).

- 231 - command names, aliases included, counted from the running dispatcher at release v2.24.0 (`go test ... -run '^TestZZCount$' ./fuku/utils/helpers`, `COUNT 231`)
- 2.24.0 - release version (`fuku/config/config.go:402`, `main.go:62`)
- 1.26 - Go minor version (`go 1.26.0` in `go.mod`)
- 2 - gotgbot major version (`github.com/PaulSonOfLars/gotgbot/v2` in `go.mod`); also the default captcha time limit in minutes (`fuku/db/models/captcha.go:10`)
- 8 - languages (`ls locales` minus `config.yml`); also the number of member requirement rules (`memberreq/settings.go:39`)
- 22 - lock types (`lockMap` 16 plus `restrMap` 6, `locks.go:61-97`); also the Node major version CI uses for the captcha page script tests (`ci.yml:340`)
- 39 - module registrations (`grep -rn 'RegisterLegacyModule("' fuku/modules ... | wc -l`)
- 64 - migrations (`ls migrations/*.sql | wc -l`)
- 6 - log categories (`logchannels/repository.go:30`); also the number of `restrMap` lock keys
- 3 - default warn limit (`warns/repository.go:25`); default captcha attempts (`captcha.go:12` model); minimum trigger threshold (`raiddetection/repository.go:33`); minimum new accounts for the flood and AI triggers (`raid_flood.go:63`, `raid_ai_count.go:38`); lowest antiflood limit (`antiflood.go:484`)
- 5 - default AI burst threshold (`raiddetection/repository.go:31`); most federations a federation can subscribe to (`federations/repository.go:23`); minutes before a staff card expires (`staff_action_card.go:36`); minutes between exports (`backup_ratelimit.go:39`)
- 10 - minutes with no automatic re-lock after a lift (`raid_trigger.go:31`); minutes between imports (`backup_ratelimit.go:40`); Staff Group history entries per page (`staff_history.go:23`); highest captcha time limit and attempts (`captcha.go` model check ranges 1 to 10)
- 15 - default join surge threshold (`raiddetection/repository.go:25`)
- 30 - default message flood threshold (`raiddetection/repository.go:28`); flood window in seconds (`raid_flood.go:58`)
- 60 - join surge window in seconds (`raid_counters.go:31`); percent of flood items that must come from new members (`raid_flood.go:61`); seconds in the shutdown budget (`graceful.go:29`)
- 120 - AI burst window in seconds (`raid_ai_count.go:35`)
- 24 - hours a member counts as new (`raid_counters.go:34`); hours between warnings about a group that cannot be locked (`raid_trigger.go:53`); longest delete delay in hours
- 330 - days a lockdown ban on a joiner lasts (`lockdown_marker.go:35`)
- 500 - highest automatic trigger threshold (`raiddetection/repository.go:35`)
- 100 - highest warn limit (`warns.go:96`); highest antiflood limit (`antiflood.go:484`)
- 150 - most filters in one chat (`filters.go:57`)
- 200 - most blocked sources in a Staff Group list (`staff/forwardblock.go:23`); default dispatcher routines (`config.go:551`)
- 366 - days: a staff ban or mute longer than this is permanent (`staff_help_msg`)
- 47 - hours after which a queued message is too old for Telegram to delete and is dropped (`deletion_worker.go:46`)
- 80 - percent confidence for English AI spam deletions (`aispam.go:50`)
- 90 - percent confidence for other languages (`aispam.go:51`)
- 75 - seconds of `stop_grace_period` (`docker-compose.yml:13`)
- 78 - percent coverage floor in CI (`ci.yml:376`)
- 17 - PostgreSQL major version in the compose stack (`docker-compose.yml:49`)
- 16 - PostgreSQL major version CI tests against (`ci.yml:251`)
- 7 - Redis major version in the compose stack and CI (`docker-compose.yml:71`, `ci.yml:264`)
- 8080 - default HTTP port (`config.go:524`, section D)
- 1 - lower bound of ranges (captcha time 1 to 10, warn limit 1 to 100); once per period in backup limits (`backup_ratelimit.go:39-41`); one hour cooldown for `/reset`
- 0 - a value that switches a setting off (`/setflood 0`, `CACHE_LOCAL_TTL=0`)
- 20 - deletion message kinds: 12 service, 5 bot, 3 member (`deletion/kinds.go:19-98`)
- 12 - service message kinds (`deletion/kinds.go:19-42`)
- 29 - command groups in the Commands section (C0 partition)
- 72 - in-world only: the bot is version 72 of 74
- 74 - in-world only: the bot is version 72 of 74
- 4 - web captcha settings that must all be set (`config.go:237-244`)
- 57 - env names read by non-test code including the test tooling name (section D)
- 56 - bot environment variables, `FUKU_TEST_DATABASE` excluded (section D)

## H. Links inventory, allow-lists and review parts

### H1. Internal link targets (`test -e` from the repository root, all verified present at HEAD `83dd98e`)

`LICENSE`, `go.mod`, `sample.env`, `AGENTS.md`, `docker-compose.yml`, `debug.docker-compose.yml`, `docker/alpine`, `docker/alpine.debug`, `Makefile`, `migrations`, `locales`, `.github/workflows/ci.yml`, `.github/SECURITY.md`, `fuku/config/config.go`, `fuku/utils/tracing/tracing.go`, `docs/README.md`, `docs/src/content/docs/self-hosting/{docker.mdx,source.mdx,binary.mdx,database.md,webhooks.md,monitoring.md,profiling.md,troubleshooting.md,captcha-web.md}`, `docs/src/content/docs/api-reference/{environment.md,lock-types.md,permissions.md,callbacks.md,database-schema.md}`, `docs/src/content/docs/architecture/{handler-groups.md,caching.md,request-flow.mdx,module-pattern.md,anonymous-admin.md,project-structure.mdx}`, `docs/src/content/docs/commands/index.mdx`, `docs/src/content/docs/commands/<module>/index.md` for `staff lockdown settings captcha antiraid bans mutes warns purges admin approvals antiflood aispam blacklists locks notes filters greetings rules pins reactions formatting federations connections logchannels reports disabling misc backup languages help devs`, `docs/src/content/docs/getting-started/quick-start.mdx`, `docs/src/content/docs/contributing/index.mdx`. The checker proves every relative link again on the finished files.

The docs pages and the generated docs index still carry the old name in their own text (`grep -c` of the old name: `captcha-web.md` 1, `commands/index.mdx` 3, `docs/README.md` 4). Link to them by relative path and never quote them. Renaming the docs is a separate job (follow-up list in Task 3).

### H2. External hosts (candidates; every one is https; none is the original project)

`github.com/PaulSonOfLars/gotgbot`, `github.com/PaulSonOfLars/gotg_md2html`, `gorm.io`, `github.com/redis/go-redis`, `github.com/eko/gocache`, `github.com/hashicorp/golang-lru`, `github.com/mojocn/base64Captcha`, `github.com/cloudflare/ahocorasick` (all in `go.mod`), `t.me/BotFather`, `useblume.dev` (docs site generator named in `docs/README.md`), `img.shields.io` (badges; URL shape only, not fetched). Do not link `typesafe.ai`: not verified.

### H3. Allow-list of non-registry slash words in the Commands section

The checker requires every `/word` in a code span of the Commands section to be a registry name or listed here. These belong to BotFather (`/newbot`, `/newapp`, `/myapps`) or to HTTP endpoints, and should normally not appear in that section at all.

- allow-slash: newbot newapp myapps health metrics db_metrics webhook

### H4. Name-search results (Task 3, fix-up commit `83327ab`, README line numbers are identical in both files)

- `grep -niE 'alita|divideprojects|divkix|divanshu|upstream|fork' README.md README.uk.md`: 0 hits in either file (also Python search for the Ukrainian words «форк», «апстрім», «оригінальний проєкт»: 0). No link to the original repository, its support group or its update channel; external hosts are github.com, gorm.io, img.shields.io, t.me and useblume.dev only. Nothing says or hints at another name for the bot.
- `fuku` (any case): 31 hits in each file, 23 lines, all inside code blocks, code spans, URLs or link targets; a Python scan that strips fenced blocks, code spans, link targets, attributes and URLs finds 0 hits in prose. Hit lines (same in both files): 665, 666, 675, 677 (module and package paths `fuku/modules`, `fuku/db`, `fuku/config` in the mermaid block and in code spans), 738 (clone URL with the repository path), 739 and 990 (checkout directory), 748 (image `ghcr.io/uasneppy/fuku_robot:latest` and binary `fuku_robot`), 753, 763 and 770 (Compose service name `fuku`, demo database credentials), 779 and 953 (`POSTGRES_DB=fuku_robot`, `OTEL_SERVICE_NAME` default), 798 to 800 and 983 (binary name), 834 (source paths in link text, code spans), 966, 1023 and 1041 (env names `FUKU_TEST_DATABASE`, `FUKU_TEST_MIGRATION_CHAIN`), 992 (structure tree), 1036 (`go test ... ./fuku/db/warns`). Every one is an identifier the reader must type or see (clone URL, image, module path, structure tree, binary, service name, env name), which is what the brief allows.
- `uasneppy` appears in the clone URL and the image path only (allowed).
- LICENSE is untouched and still not named by holder in either README (License section points to the file).

### H5. Voice review (filled in by Tasks 1 to 3)

Task 1 (README.md, commit `6733c61`, checker `--skip-link README.uk.md README.md` exit 0, `SUMMARY fails=0 reviews=1` after two fixes):

- REVIEW [figures] spelled-out number words on README.md lines 68, 102, 104, 108, 150, 251, 628, 756, 758, 812, 834, 880, 914, 922. Judged one by one, all are verified counts or plain editorial words: "Three modes" (math, text, Turnstile: E11), "All three" triggers (join surge, flood, AI burst: E7, E8), "three buttons" of the lockdown alert (Lift, Ban recent joiners, Revoke link: E9), "Eight rules" (section B, 8), "six categories" (section B), "These three need the right to change group info" (`/setgtitle`, `/setgpic`, `/setgdesc` families: C1 Admin), "These three commands" (retired raid commands: C0), "Two facts about this stack" (editorial list header), "five variables" (section D, Compose passes five), "four settings" (Turnstile, section D), "the two monitoring defaults" (`ENABLE_PERFORMANCE_MONITORING` and `ENABLE_BACKGROUND_STATS`, section D). No unverified count.
- REVIEW [voice] never-joke stem 'kill' (line 679, "so Docker does not kill the process"): fixed by rewording to "does not stop the process before the shutdown ends". Not a joke, but removed so the stem is gone.
- Earlier FAIL [figures] 256 (`SHA-256` in prose): fixed by putting `SHA-256` in a code span.
- Series references (INFO counts per H2): Features 2 (intro-level "tempe" and "ration" in the Content blurb are one reference; Jaya Abadi, facility and "version 72 of 74" are in the intro before the first H2 and count towards no H2), Commands 3 (tempe and ration in Greetings, facility in Rules), all other H2 sections 0. Each H3 or details block carries at most one reference. The in-world "version 72 of 74" appears once.
- Never-joke list re-read against the finished file: no starvation, war, death, gore or cannibalism text; no gloating in the ban, mute, warn, purge or report descriptions (their opening lines are neutral announcer lines: "Notice. Both the admin and I must hold the right to restrict members."); food humour limited to "Today's ration is tempe." in Content and Greetings.

Task 3 (cross-file pass, fix-up commit `83327ab`):

- Decision 1 applied per H2. Features had one reference (tempe, in Content; the words ration and tempe are one reference). Commands had two, tempe (Greetings) and the facility word (Rules), plus the glossary word "supervisor" in the Admin line, which is not a series reference a reader must know (the Admin block says admins). Kept the Rules line ("Regulations. Every facility has them.", facility is defined in the intro) and dropped the repeated tempe joke from Greetings in both files. New Greetings opener, English: "Greetings. I welcome new members and say goodbye to the ones who leave."; Ukrainian: "Привітання. Вітаю нових учасників і прощаюся з тими, хто йде." (present tense, gender-free). Checker INFO after the fix: Features 2 words (one reference), Commands 1, every other H2 0, in both files.
- The intro (before the first H2) holds Jaya Abadi, facility and "version 72 of 74", once; the software version is only stated as release v2.24.0 in Commands, so the two numbers are not confusable.
- Ukrainian read-through: formal ви throughout, no informal pronouns, feminine "я написана" and "привітна" only where gendered, other first-person forms are present tense; two checker REVIEW lines (`я перевіряю в`, `я знаю в`) are false positives of the masculine-past regex. One wording defect fixed: the `/reports block` row said «Заблокувати користувачам скарги», now «Заборонити користувачам скаржитися й показати їх список». Nothing declined after `Xenobot-72`.
- Code block comments stay English in both files (byte-identical blocks, accepted choice).
- REVIEW [figures] spelled-out numbers: unchanged judgement above (all verified counts or editorial); README.md lines 150 and 922 have no Ukrainian counterpart word, accepted.
- No never-joke subject, no gloating, no joke in the near-zero sections (re-read again).

## I. Baseline: the checker on the OLD README (red run)

Command (from the checkout root, before any edit of `README.md`): `python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py README.md`, exit status 1.

Result: exit status 1, `SUMMARY fails=99 reviews=2`. FAIL lines by check: commands 1, figures 6, links 9, names 83 (names 83 = hits of the original project, its developer, its name, `upstream`, `fork` and the old bot name in prose; the old name hits inside code and URLs are INFO lines, 38 of them).

```text
FAIL [commands] README.md: 13 registry command(s) missing from the Commands section: /addfwdblock /autolockdown /fwdblockaction /fwdblocklist /linkstaff /lockdown /lockdownstatus /rmfwdblock /setstaff /staff /unlinkstaff
FAIL [links] README.md:976: external link to a forbidden target https://alita-docs.divkix.me
FAIL [links] README.md:990: external link to a forbidden target https://github.com/Divkix/Alita_Robot
FAIL [links] README.md:991: external link to a forbidden target https://github.com/Divkix
FAIL [links] README.md:992: external link to a forbidden target https://github.com/Divkix/Alita_Robot/graphs/contributors
FAIL [links] README.md:7: external link to a forbidden target https://github.com/Divkix/Alita_Robot
FAIL [links] README.md:7: external link to a forbidden target https://github.com/Divkix
FAIL [links] README.md:12: external link to a forbidden target https://github.com/Divkix/Alita_Robot
FAIL [links] README.md:12: external link to a forbidden target https://img.shields.io/badge/fork%20of-Divkix%2FAlita__Robot-6f42c1?logo=github
FAIL [links] README.md: no link to README.uk.md within the first 30 lines
FAIL [figures] README.md:79: number 18 is not in the fact sheet's Stated figures list
FAIL [figures] README.md:999: number 2020 is not in the fact sheet's Stated figures list
FAIL [figures] README.md:999: number 2025 is not in the fact sheet's Stated figures list
FAIL [figures] README.md:79: number 2026 is not in the fact sheet's Stated figures list
FAIL [figures] README.md:78: number 658 is not in the fact sheet's Stated figures list
FAIL [figures] README.md:101: number 9 is not in the fact sheet's Stated figures list
FAIL [names] README.md:7: forbidden string 'fork'
FAIL [names] README.md:7: forbidden string 'Divkix'
FAIL [names] README.md:7: forbidden string 'Alita'
FAIL [names] README.md:7: forbidden string 'upstream'
FAIL [names] README.md:7: forbidden string 'Divkix'
FAIL [names] README.md:7: forbidden string 'Divkix'
FAIL [names] README.md:12: forbidden string 'Divkix'
FAIL [names] README.md:12: forbidden string 'Alita'
... (75 more names lines)
REVIEW [figures] README.md: spelled-out number words on lines [79, 392, 396, 398, 400, 402, 404, 406, 689]
REVIEW [voice] README.md:824: never-joke stem 'kill'
```

Reading: the old file fails on the original project and developer names (for example lines 7, 12, 976 to 992), on the 13 registry commands missing from its Commands section (the four blocklist commands, the staff and lockdown commands), on the missing README.uk.md link, on six numbers not backed by the fact sheet (18, 658, 9, 2020, 2025, 2026) and on external links to the original project. The checker is red on the old file, as the plan requires.


## J. Follow-up list (Task 3, step 6): places outside the two READMEs that still carry the old name or the original developers

Observed at HEAD `83327ab` with `git grep -nIiE 'divkix|divideprojects|alita|divanshu' -- . ':!.planning' ':!LICENSE'` (19 files) and `git grep -cIi fuku`. Nothing here was edited. U = visible to a bot user or a developer-bot owner at run time.

Original developers (handle, name, support group, site), hit counts per file:

- `locales/{en,es,fr,id,pt,ru,uk}.yml` 3 each, `locales/hi.yml` 2 (U): `@DivideProjects` in the about text (en line 627), the donate text (`@DivideProjectsBot`, en 694) and `bot_updates_creators_plug` (en 915), in all 8 languages.
- `.goreleaser.yaml` 2 (lines 115 and 116): OCI `vendor` "Divkix" and `authors` with the developer's name and address. `docker/alpine` 3, `docker/alpine.debug` 3, `docker/pr-build` 3 (OCI authors, url `https://divkix.me`, vendor). Visible with `docker inspect` on a built image.
- `.github/CODEOWNERS` 1, `.github/FUNDING.yml` 2 (ko_fi and github sponsor), `.github/dependabot.yml` 3 (assignees): GitHub shows these (the funding file puts a Sponsor button on the repository page).
- `docs/pages/index.astro` 2 (`t.me/DivideProjects` link, U on the docs site), `docs/blume.config.ts` 1 (`github: { owner: "divkix", repo: "Fuku_Robot" }`), `docs/README.md` 1 (`https://fuku.divkix.me` live documentation link).
- `migrations/20261004130000_add_staff_role_exclusivity_trigger.sql` 4: applied migration, must not be edited (checksum); it is a comment or identifier hit and not user-visible.

Old bot name, counts of `fuku` (any case):

- `locales/*.yml`: en 17, es 17, fr 18, hi 16, id 17, pt 16, ru 14, uk 17 (132 in all). User-visible and the largest item: `help_about`, `help_bot_intro`, `formatting_help_msg`, `help_configuration_step-1`, `help_info_about_header`, the button `help_button_add_fuku` ("Add Fuku to chat!"), the `languages_help_msg` and `help_help_msg` texts, `https://t.me/FukuRobotUpdates` news channel, and a Crowdin link `crowdin.com/project/fuku_robot`. en.yml line 630 also claims "has been online since 2020 and has served thousands of groups", a history claim the README deliberately does not make.
- `fuku/modules/help.go` 9 hits, of which 6 are import paths; real ones: line 97 `Url: "https://t.me/FukuRobotUpdates"` (the News button, U) and lines 466 and 471 (`help_button_add_fuku`, U).
- `fuku/db/devs/repository.go` line 203: `"<u>Fuku's Stats:</u>"` is the developer `/stats` heading (U for the bot owner and devs). `fuku/db/backup/types.go` line 33 writes `BotName: "FukuRobot"` into every `/export` file (U: visible in the JSON).
- `docs/`: 38 files, 364 hits; largest: `self-hosting/binary.mdx` 79, `architecture/caching.md` 50, `self-hosting/source.mdx` 26, `self-hosting/docker.mdx` 25, `self-hosting/monitoring.md` 19, `architecture/module-pattern.md` 18. The docs pages linked from the READMEs (captcha-web.md 1, docker.mdx, source.mdx, database.md, monitoring.md, caching.md, troubleshooting.md and others) still carry the old name. The command pages under `docs/src/content/docs/commands` have 11. The generated command index counts (`make inventory` and the docs index) undercount, as noted in section B.
- Deploy files: `docker-compose.yml` 12 (comment line 1, service `fuku`, container names, demo credentials `fuku`), `debug.docker-compose.yml` 5, `docker/alpine` 6, `docker/alpine.debug` 10, `docker/goreleaser` 2, `docker/pr-build` 6, `render.yaml` 6 (service and database names `fuku-robot`, `fuku-db`, `fuku-redis`), `Procfile` 1 (`bin/Fuku_Robot`), `app.json` 2 (`"name": "fuku-robot"`, repository URL), `nixpacks.toml` 2, `sample.env` 2, `.goreleaser.yaml` 4 (`project_name`, binary, OCI description "Official Fuku Go Robot Docker Image"). Most are identifiers (module path, binary, container names) that a rename would change together with the image tag, so treat as a separate decision.
- `AGENTS.md` 173 hits and `.claude/CLAUDE.md` 39 hits. `.claude/CLAUDE.md` line 9 (intro: "Fuku Robot is a fork of Fuku Robot ...") still says fork, and the project name appears in the stack text. Not user-visible, but a contributor reads it.

BotFather steps for the owner (nothing in the repository can do these):

1. `/setname` for the bot's display name: `Xenobot-72`.
2. `/setusername` for a new username (must end in `bot`; changing it changes every `t.me` link and the Mini App URL, and the `/newapp` short name is separate). Decide first whether to keep the current username.
3. `/setdescription` (shown on the empty chat screen) and `/setabouttext` (profile page) in the new voice, and per-language versions with the language option if wanted.
4. `/setuserpic` for the profile picture.
5. The news channel `FukuRobotUpdates` (linked from `help.go` and the help text) also carries the old name; renaming it or the link is the owner's call.

Shipped help text bug found while checking (not fixed here): the captcha help text (`captcha_help_msg`) tells users to send `/captchaattempts <1-10>`, but the registered command is `/captchamaxattempts`. Present in en (line 318), fr (480), hi (263), id (266), pt (256), ru (192) and uk (125); es.yml does not mention either spelling. The READMEs use `/captchamaxattempts`.
