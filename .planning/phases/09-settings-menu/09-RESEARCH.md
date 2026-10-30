# Phase 9: Settings Menu - Research

**Researched:** 2026-10-30
**Domain:** Telegram inline-keyboard settings menu on an existing Go bot (refactor-and-wire; no new tables, no new Redis keys, no new packages)
**Confidence:** HIGH for the code seams and line numbers (every file below was opened this session); MEDIUM for Telegram platform behaviour (no network in this session, tagged `[ASSUMED]`)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** `/settings` is a group command through `helpers.WrapCommand` (admin check, group only, `Disableable` not set) that replies with two buttons: "Open here" and "Open in private". The command sender must pass a live `getChatMember` admin check (creator or administrator), never the admin cache. Anonymous admins get the usual proof first and re-enter through `RegisterAnonymousAdminHandler`.
- **D-02:** A menu opened in the group carries the opener's user ID in every button (`u`). It responds only to that user; anyone else gets a short alert and nothing changes. A menu opened in private carries the group's chat ID in every button (`c`) and manages only that group. In private there is no opener field: the presser is the user in the private chat.
- **D-03:** "Open in private" is a URL button, a Telegram deep link `https://t.me/<bot>?start=settings_<chat id>`, so it works even when the admin never started the bot. The `/start` handler gets one payload branch that parses `settings_<id>` strictly (digits with an optional leading minus, the same bounds as other chat IDs), runs the live admin check for that user and group, and sends the main page to the private chat. A refused check answers with a plain "you are not an admin of that group" reply and sends no menu.
- **D-04:** Every button press re-checks live with `getChatMember` that the presser is still the creator or an administrator of the group being configured (the callback's group in private, the chat the message sits in for a group menu). A failed lookup refuses, like the lockdown buttons. A demoted admin's press is refused and edits nothing.
- **D-05:** Changing a setting needs the same right the matching command needs, checked live per setting, so the menu can never do more than the command: lockdown Lift and the lockdown trigger switches and thresholds need the creator or an administrator with `can_restrict_members` (`lockdownButtonAuthority` / `requireAutoLockdownAuthority`'s rule); Staff Group link and unlink need the live owner of both groups (`chat_status.CheckOwner`, the `/linkstaff` and `/unlinkstaff` rules); captcha and antiflood changes need the admin right those commands check. A refusal shows the command's own refusal text (or an alert) and writes nothing.
- **D-06:** One message, edited in place. The main page has buttons Staff Group, Lockdown, Captcha, Antiflood and Close. Every sub-page has Back and Close. Close deletes the menu message; when it cannot (private chat after 48 h, or a missing right) it edits the message to a short "Menu closed" note with no buttons.
- **D-07:** Callback data uses a new `settings|v1` namespace through `alita/utils/callbackcodec` only (64-byte cap, never `strings.Split`). Fields: page, action, and the group ID (private menu) or the opener's ID (group menu), plus a value where a step needs one. Chat IDs are up to 14 characters with the sign, so a field set stays under the cap; an encode overflow is a bug caught by a test, never a dead button.
- **D-08:** No free-text input and no Redis state. Numbers move with stepper buttons, choices with buttons, and confirmation with a second tap. The menu is stateless, so it works without Redis and on any replica.
- **D-09:** Dangerous actions (Lift lockdown, Unlink Staff Group) ask for a second tap in place ("Lift now? Yes / No") before they run. The Yes press re-checks authority live again and carries an expiry minute like the lockdown cards, so a stale confirm is refused.
- **D-10:** Language: a menu opened in the group speaks the group's language; a menu opened in private speaks the admin's language. All strings are new `settings_*` keys in all 7 locale files, and every user-controlled string (group titles, names) is escaped and spliced in after the translator.
- **D-11:** A press on a menu that cannot render (the group no longer exists, the bot was removed, a lookup failed) answers an alert and edits to a short "cannot open" note; it never leaves a dead button. Every callback is answered exactly once.
- **D-12:** Staff Group page. In a group it shows the link state and health (the same information as the `/staff` row for this group: linked or not, the Staff Group, the bot's health, a lockdown marker) from the fresh `staff.*Fresh` reads. Link and Unlink buttons call the same functions as `/linkstaff` and `/unlinkstaff` (live owner of both groups only; Unlink asks a second tap). Opened inside a Staff Group it shows the `/staff` panel information read-only (the linked groups and their status) and offers no link or unlink there.
- **D-13:** Lockdown page. It shows the current state (since when, who, the reason or the trigger sentence, confirmed or lifting) from a fresh `chat_lockdowns` read, a Lift button that takes the same path as `/unlockdown` (`liftLockdownRow`, a second tap first), and for each of join surge, flood and AI spam an on/off switch and a threshold stepper from 3 to 500, written by the `raiddetection.Set*` functions with the `autolockdown` grammar's rules (a threshold also turns its trigger on). It does not start a lockdown. A basic group shows a "needs a supergroup" note.
- **D-14:** Captcha page. It shows and sets: on or off, the mode (math, text, turnstile; turnstile with the same three ordered refusals as `/captchamode`), the time limit (1 to 10 minutes, a stepper), and the failure action (kick, ban, mute). Each change goes through the same apply code as `/captcha`, `/captchamode`, `/captchatime` and `/captchaaction`.
- **D-15:** Antiflood page. It shows and sets: the limit (a stepper; 0 means off as it does for `/setflood`), the action (mute, ban, kick, warn; `tban` and `tmute` are shown but stay command-only because they need a duration), and the delete-flood-messages switch. Each change goes through the same apply code as `/setflood`, `/setfloodmode` and `/delflood`.
- **D-16:** Pages are rendered by pure functions from a settings snapshot (like `renderStaffPanel`), built by one loader per page from fresh or cached reads. A registry table lists the pages (key, title key, render, handle), so adding a page later is one table entry plus its strings.
- **D-17:** Each setting the menu can change has one apply function that both the command handler and the menu call. It holds validation, the setting-specific refusals (for example Turnstile's three), the repository write and the `cache.DeleteCache` of the affected keys, and returns a typed result the command renders as its reply and the menu renders as a page or alert. The existing command handlers are refactored to call it; their observable replies do not change.
- **D-18:** Pages read through the same cached getters the commands read, so the command's answer and the menu's value are always the same. Keys that a replica could serve stale from the in-process layer are already in `skipLocal` where freshness matters; the plan checks each setting the menu edits.
- **D-19:** Tests prove parity per setting: change by command and read the menu's value; change by menu and read what the command shows; compare the stored rows; and a drift test that fails when a command and the menu disagree about validation (for example a threshold of 2, a time limit of 11, an unknown mode). Authority parity is tested too: a press that the command would refuse is refused by the menu.
- **D-20 (refines D-15, A-1):** `/setfloodmode` accepts only `ban`, `kick` and `mute`; nothing in the code writes `warn`, `tban` or `tmute`, and no antiflood command takes a duration. The Antiflood page therefore offers mute, ban and kick. A stored `warn`, `tban` or `tmute` (an old or imported row) is shown read-only with a note and is never claimed to be settable by a command. The limit control is Off, then 3 to 100, because that is what `/setflood` accepts.
- **D-21 (privacy, A-2):** Phase 1's rule is that nothing about a link is shown to the linked group's members. A menu opened in the group is a message every member can read, so there the Staff Group page shows only a masked line (linked or not, and the bot's health, never the Staff Group's name or ID). The private menu and a menu opened inside a Staff Group show the full information. This keeps D-12's information for the admin who asked for it in private.
- **D-22 (Link picker, A-3):** an owner can have several Staff Groups and D-08 allows no free text, so Link opens a stateless picker with one button per Staff Group the presser owns, each verified live at the press, then calls the same function as `/linkstaff`. A presser who owns no Staff Group gets the command's own refusal.
- **D-23 (reused strings, A-4):** where a command already has the refusal or state text, the menu reuses that existing key, in all 7 languages, instead of duplicating it as a `settings_*` key, so "the menu refuses with the command's words" holds. New `settings_*` keys cover only what is new (page titles, labels, menu-specific alerts).
- **D-24 (value button, A-5):** a stepper's value button redraws the page, which also re-runs the live admin check. A double tap is harmless because a stepper button sends the absolute target value.

### Claude's Discretion

- The exact `settings|v1` field names and abbreviations, and how a stepper encodes its direction and size (for example a step of 1, 5 or 10 for thresholds).
- Whether the main page and sub-pages share one render pipeline with a header and footer, the exact layout, emoji and wording (settled in `09-UI-SPEC.md`).
- Where the apply functions live (a new `alita/modules/settings_apply.go` or beside each command) and how the typed results are named.
- How the `/start` payload branch is wired into the existing help module's `start` handler without changing its other behaviour.

### Deferred Ideas (OUT OF SCOPE)

- Menu pages for other modules (welcome, rules, warns, filters, notes, locks, approvals).
- Starting a lockdown, or running a staff action, from the menu.
- `tban` and `tmute` antiflood actions with a duration picker.
- A menu auto-close timer for group menus left open.
- Per-admin "last menu" shortcuts and a connected-group picker in private.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MENU-01 | `/settings` in a group asks open-here or private | Descriptor reuses `requireLockdownGroup()` + `requireLockdownAuthority(false)` (live admin, anon proof); URL button + `RegisterDeepLinkHandler("settings_", ...)` (section Existing Code, items 5 and 6) |
| MENU-02 | Group menu answers only its opener; private menu manages the group it came from | `u` / `c` fields, structural rules, forwarded-message analysis (Pitfalls 4, 5) |
| MENU-03 | Every press re-checks live that the presser is still an admin | Press pipeline with `lockdownLiveMember` (Pattern 1); authority table with the code-verified corrections (Pattern 4) |
| MENU-04 | One message edited in place with Back and Close | `editStaffMessage` seam, `MaybeInaccessibleMessage` interface methods, Close fallback (Patterns 1, 7) |
| MENU-05 | Staff Group page shows `/staff` panel information; link/unlink only for the owner of both | `buildStaffPanelRows`, `staffPanelTextFor` (helpAsHint), `runUnlinkGroup`, split of `runLinkGroup` (Pattern 5, Existing Code items 8-11) |
| MENU-06 | Lockdown page: state, working Lift, per-trigger switch and threshold | `liftLockdownRow`, `lockdown.GetCurrentFresh`, `autolockdownWrite` (Pattern 6, items 12-15) |
| MENU-07 | Captcha page (mode, time) and Antiflood page | Apply functions (Pattern 3), authority corrections, antiflood fresh-row default (Pitfall 2) |
| MENU-08 | A setting changed in the menu is identical to the command | One apply function per setting; parity tests (Validation Architecture) |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

Read this session: `/home/user/Fuku_Robot/.claude/CLAUDE.md` (it embeds `AGENTS.md`). Directives that bind this phase:

- Go 1.26.0, gotgbot v2, GORM, Redis. Stay on the existing stack; no new dependency is needed. `CGO_ENABLED=0 go build ./...` for compile checks; tests need `CGO_ENABLED=1` and `-tags testtools`; the only valid full run is `make test`.
- Commands go through `helpers.WrapCommand`; admin commands that must work for anonymous admins also need `RegisterAnonymousAdminHandler` + `anonPipelineHandler`. Handler methods use value receivers on `moduleStruct`. Modules register in `init()` with `RegisterLegacyModule(name, priority, load)`; duplicates are dropped silently.
- Callbacks: encode/decode only through `alita/utils/callbackcodec`; `encodeCallbackData` returns `""` on overflow (a dead button), so a test must prove no page ever overflows.
- Every write `cache.DeleteCache`s the keys it affects. Never discard a DB error on a state-changing path.
- Every fire-and-forget goroutine starts with `defer error_handling.RecoverFromPanic(...)`. This phase should need none; direct callback handlers still start with `defer error_handling.RecoverFromPanic("settings", "Settings")` (pattern: `verifyAnonymousAdmin`).
- A missing i18n key returns `""`: copy key names from `locales/en.yml`; all 7 locale files; `make check-translations` only sees literal `GetString` keys (use a `switch` with literal keys).
- Register new secrets with `logredact.RegisterSecret`: this phase adds none.
- Tests use real fixtures (`internal/testdb`, miniredis, hand-written `gotgbot.BotClient` fakes), no mock libraries; assert observable behaviour, never literals or source substrings.
- Migrations are append-only: this phase adds none (confirmed below).
- A new rule that invalidates `AGENTS.md` is updated in the same commit (list in "Docs and AGENTS.md updates").
- Conventional Commits; `feat:` for user-visible changes.
- PostgreSQL must never be started in this environment; SQLite tests only.

## Summary

Phase 9 is a wiring phase. Every setting the menu changes already has a command handler, a repository setter that calls `cache.DeleteCache`, and reply texts in all 7 locales. The work is (1) pulling the validate-write-invalidate core of seven command bodies into apply functions that take an explicit chat (the commands read `ctx.EffectiveChat`, which in a private menu is the private chat, so they cannot be called as they are), (2) a callback handler with a fixed press pipeline, (3) four page loaders and pure renderers, (4) a `/settings` command, a `/start` deep-link branch and about 85 new locale keys. No migration, no model, no Redis key, no shutdown drain, no handler group is added. The harness `AutoMigrate` list already contains every model the pages read (`alita/modules/test_harness_test.go:329-376`).

The UI contract is right about almost everything, but reading the code turned up eight points where it is wrong, silent or blocked by the existing code. The most important: (a) `/setflood`, `/setfloodmode` and `/delflood` call `chat_status.IsUserConnected(b, ctx, true, true)`, and the second `true` makes the **bot being an admin** a requirement with its own refusal text, so the "admin only" row of the authority table is wrong; (b) `SetFloodMode` and `SetFloodMsgDel` on a chat with no row create it through `FirstOrCreate`, and the model's `default:5` on `flood_limit` probably switches antiflood on at limit 5, so the "page drawn from the apply result, not a re-read" rule would show Off while the stored value is 5; (c) `antiflood.GetFloodContext` swallows read errors and returns defaults, so a D-11 "cannot open" page needs `GetAntifloodSettingsCachedContext`; (d) `captcha_settings` and `antiflood` are not in `skipLocal`, so another replica can show the old value for up to 10 s; (e) the `/staff` panel renderer cannot be reused as is (it always adds the Add group, Recent and Unlink buttons and the full help). The remaining points are in "Spec Deltas" below.

**Primary recommendation:** Build `settings_apply.go` first (typed results, explicit chat and bot arguments, commands refactored to call it with their replies pinned by the existing tests), then one `settings|v1` handler whose steps 1-5 are a single function shared by every action, then the four pages as independent files behind a page table.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Menu rendering and press handling | API / Backend (bot process, `alita/modules`) | Telegram client (inline keyboard) | The bot renders text and keyboards; Telegram only displays them |
| Authority (admin, restrict, owner of both) | API / Backend, live Telegram calls | none | Never the admin cache, never callback data (AGENTS.md Permissions) |
| Setting storage | Database (`captcha_settings`, `antiflood_settings`, `chat_raid_detection`, `staff_group_links`, `chat_lockdowns`) | Cache (Redis plus the 10 s in-process layer) | Repositories already own reads (cached) and writes (`DeleteCache`) |
| Menu state | none | none | Stateless by D-08: ownership and target travel in callback data, the live check supplies authority |
| Deep link entry | Telegram client (URL button) | API (`/start` handler) | Anyone can tap a URL button; authority is decided only when `/start` arrives |
| Localisation | API / Backend (`alita/i18n`, 7 locales) | none | Group language for group menus, user language for private menus (`lang.GetLanguage` already does this) |

## Standard Stack

### Core (all in `go.mod` already; nothing to install)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | `v2.0.0-rc.36.0.20260919140833-240296efadb4` (module dir resolved by `go list -m`) | callbacks, `EditText`, `MaybeInaccessibleMessage` | the repo's Telegram client |
| `alita/utils/callbackcodec` | in-repo | `settings\|v1\|...` encode/decode with the 64-byte cap | AGENTS.md Callbacks |
| `alita/i18n` | in-repo | translator, 7 locales | AGENTS.md |
| GORM, go-redis, gocache | as in `go.mod` | repositories already written | no new queries are needed |

### Supporting (in-repo seams to call, not to rewrite)

| Seam | File:line | Use |
|------|-----------|-----|
| `lockdownLiveMember` | `alita/modules/lockdown.go:108` | live `getChatMember`, 5 s |
| `staffIssuerSkipReason` | `alita/modules/staff_action_run.go:615` | creator / admin with `can_restrict_members` |
| `liftLockdownRow` | `alita/modules/lockdown.go:485` | the one lift path |
| `autolockdownWrite` / `writeAutoLockdown` / `parseAutoLockdownArgs` | `alita/modules/autolockdown.go:133`, `136`, `78` | the one trigger write path |
| `runUnlinkGroup`, `checkUnlinkAuthority` | `alita/modules/staff_unlink.go:78`, `52` | unlink |
| `resolveNamedStaffGroup`, `checkLinkTarget`, `storeLink` | `alita/modules/staff_link.go:208`, `232`, `254` | link pieces |
| `buildStaffPanelRows`, `markLockedRows`, `composeStaffPanelText`, `staffPanelTextFor` | `alita/modules/staff_panel.go:419`, `464`, `206`, `271` | Staff page |
| `editStaffMessage`, `isMessageNotModified` | `alita/modules/staff_unlink.go:205`, `staff_panel.go:506` | edit in place |
| `callbackQueryFromContext`, `encodeCallbackData`, `decodeCallbackData`, `answerStaffCallback` | `callback_context.go:8`, `callback_codec.go:15`, `35`, `staff.go:301` | plumbing |
| `RegisterDeepLinkHandler` | `alita/modules/deeplink_router.go:21` | the `/start` payload branch |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Editing `help.go`'s `start` handler | `RegisterDeepLinkHandler("settings_", ...)` from `settings.go` `init()` | Chosen: the router (`deeplink_router.go:21-48`) already routes the private two-argument path by longest prefix, so `start` stays byte-for-byte unchanged |
| Reusing `captchaAdminGate` / `IsUserConnected` in the menu | Explicit-chat checks in the press pipeline | Chosen: both read `ctx.EffectiveChat`/`EffectiveMessage` (`captcha.go:445-463`, `chat_status.go:396-511`), which is the private chat in a private menu |

**Installation:** none. **Version verification:** not applicable (no package is added). Go toolchain `go1.26.0`, `gcc` and `make` are present (`go version`, `which gcc make`).

## Package Legitimacy Audit

No external package is installed by this phase, so the gate has nothing to check.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | - | - | - | - | - | - |

**Packages removed due to [SLOP] verdict:** none. **Packages flagged as suspicious [SUS]:** none.

## Architecture Patterns

### System Architecture Diagram

```
 /settings (group)                         URL button (anyone can tap)
      |                                          |
 WrapCommand: requireLockdownGroup,              v
 requireLockdownAuthority(false)          t.me/<bot>?start=settings_<chat>
 (live getChatMember, anon proof)                |
      |                                    /start in private
      v                                          |
 reply: [Open here | Open in private(url)]  HandleDeepLink -> settingsStartDeepLink
      |  a=oh&u=<opener>                         |  parse payload, live admin check, getChat
      v                                          v
 +----------------------- Main page message (one message, edited in place) ---------------+
 | callback  settings|v1|a=..&(c=<group> | u=<opener>)&p=..&t=..&v=..&e=..&s=..           |
 +--------------------------------------------------------------------------------------+
      |
      v   settingsCallback  (RecoverFromPanic)
 1 decode + structural rules (no I/O)            -> alert common_callback_invalid_request
 2 staffServiceUserIDs[query.From.Id]            -> alert lockdown_alert_post_as_yourself
 3 group menu: From.Id == u                      -> alert settings_not_your_menu
 4 live getChatMember(group, presser)            -> alert settings_not_admin / settings_check_failed
        group = c (private: + getChat) | message chat (group)
 5 per-action right (live, same member record)  -> alert (new keys) or notice (reused texts)
 6 answer callback ONCE (empty)  then apply ---> settings_apply.go --> repositories --> DeleteCache
        |                                              ^
        |                          same functions called by the command handlers
        v
 7 edit same message: pure render(snapshot) ---> editStaffMessage (not-modified = success)
```

### Recommended Project Structure

```
alita/modules/
├── settings.go            # settingsModule var, settingsDesc, LoadSettings, /settings command, deep link, init()
├── settings_menu.go       # callback entry, field parsing, press context, steps 1-5, edit/close helpers
├── settings_pages.go      # page table, snapshots, shared header/footer renderer, keyboard helpers
├── settings_staff.go      # Staff page loader/render/handle (link, picker, unlink, read-only panel)
├── settings_lockdown.go   # Lockdown page
├── settings_captcha.go    # Captcha page
├── settings_antiflood.go  # Antiflood page
└── settings_apply.go      # apply functions + typed results shared with the commands
alita/i18n/settings_locale_test.go        # parity, placeholders, ceilings, help length
```

### Pattern 1: One callback entry and one pipeline (MENU-02, MENU-03, MENU-04)

**What:** one handler `callbackquery.Prefix("settings|")` (no `SetAllowChannel`: the menu never sits in a channel). Steps 1-5 are one function that returns a `settingsPress` (the group `*gotgbot.Chat`, the live `gotgbot.MergedChatMember` of the presser, the translator, whether the menu is private) or has already answered and returns nil. Every action handler then starts from a verified press.

**Why one function:** the order is the security property (service identity before any lookup, opener before the live check, live check before any read or write). A per-action copy would drift.

**Facts the planner needs:**
- `ctx.EffectiveChat` in a callback update is the chat of the message the button sits in, and `EffectiveSender.User` is the presser (`gotgbot ext/context.go`, `case update.CallbackQuery != nil`). So `lang.GetLanguage(ctx)` (`alita/db/lang/repository.go:31-54`) already gives the group's language for a group menu and the user's language for a private menu: `ctxTr(ctx)` is the whole of D-10.
- `query.Message` is a `gotgbot.MaybeInaccessibleMessage`. `GetChat`, `GetMessageId`, `EditText` and `Delete` exist on both concrete types (`gen_helpers.go:233`, `312`; `gen_types.go:3430`, `3440`). Treat only `query.Message == nil` as `common_callback_message_unavailable` (the existing code does the same, e.g. `bans.go:1014-1018`); an `InaccessibleMessage` still carries chat and message ID and can be edited.
- `answerInvalidCallback` (`callback_codec.go:51`) answers a toast without `ShowAlert`; the contract wants an alert, so write a small `settingsAnswer(b, query, text, alert)` helper (or call `answerStaffCallback`, `staff.go:301`, which takes the alert flag) and use it everywhere so "answered exactly once" is one choke point. A test counts `answerCallbackQuery` calls per press.
- Answer **after** step 5 and **before** the apply/redraw for presses that make Telegram calls (captcha off restores every pending user, `disableCaptchaForChat` `captcha.go:548`; Lift does `getChat` + `setChatPermissions`; the Staff page makes up to 2N+1 calls). Refusals decided by an apply function are then notices, which the contract already says.

```go
// Source: shape only; names new to this phase are marked (new).
func (m moduleStruct) settingsCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("settings", "Settings")
	query, ok := callbackQueryFromContext(ctx)           // callback_context.go:8
	if !ok { return ext.EndGroups }
	p := settingsVerifyPress(b, ctx, query)              // (new) steps 1-5; answers and returns nil on refusal
	if p == nil { return ext.EndGroups }
	return settingsPages.dispatch(b, p)                  // (new) table lookup by action/page
}
```

### Pattern 2: Field allow-list per action

`callbackcodec.Decode` keeps unknown keys and takes the first of a repeated key (`callbackcodec.go:61-97`). Define one table `action -> allowed fields` and refuse data with any other key as "Invalid request". This is what makes "carries neither or both of `c` and `u`" and a forged `s` on a non-link action testable.

Parsing helpers to reuse (all strict, all in this repo):
- `c` (group chat ID): `parseStaffChatIDArg` (`staff_link.go:74`): optional minus, digits only, at most 20 characters, negative result. Reuse it for both the callback `c` and the `/start` payload (after `strings.CutPrefix(arg, "settings_")`).
- `u`, asker: `lockdownParseAsker` (`lockdown_alert_callbacks.go:120`): positive int64.
- `e`: `lockdownParseCardFields` (`:130`) already parses `u` and `e` together; for a menu card (group menu has `u`, private has `c`) parse `e` alone: positive int64, then `expiryMinute < time.Now().Unix()/60` is expired (`:278`). `lockdownCardLifetime` is `var lockdownCardLifetime = 5 * time.Minute` (`lockdown_alert.go:82`); add `settingsCardLifetime` the same way so a test can shrink it.

### Pattern 3: Apply functions with an explicit chat (MENU-08, D-17)

**Rule:** an apply function takes the chat it acts on and the bot, never a `*ext.Context`, and returns a small typed result carrying (a) a status, (b) the locale key of the refusal when there is one, (c) the stored value after the write. The command maps the result to the exact reply keys it uses today; the menu maps it to a notice or a redraw.

Proposed shapes (names are new):

```go
type settingsStatus int
const (
	settingsOK settingsStatus = iota
	settingsInvalid     // value outside the rule; nothing written
	settingsRefused     // a setting-specific refusal (Turnstile); Key says which; nothing written
	settingsSaveFailed  // repository error; nothing changed
	settingsPartial     // saved, but a follow-up step failed (captcha off cleanup)
)

type captchaApply struct {
	Status   settingsStatus
	Key      string                  // refusal key for settingsRefused
	Settings *models.CaptchaSettings // fresh cached read after a write, nil on failure
	Err      error                   // the repository error, for the command's `return err`
}
func applyCaptchaEnabled(b *gotgbot.Bot, chatID int64, on bool) captchaApply     // wraps SetCaptchaEnabled + disableCaptchaForChat
func applyCaptchaMode(b *gotgbot.Bot, chat *gotgbot.Chat, mode string) captchaApply  // mode check + captchaWebModeRefusal + SetCaptchaMode
func applyCaptchaTimeout(chatID int64, minutes int) captchaApply
func applyCaptchaAction(chatID int64, action string) captchaApply
func applyFloodLimit(ctx context.Context, chatID int64, limit int) floodApply   // 0 or 3..100
func applyFloodMode(ctx context.Context, chatID int64, mode string) floodApply   // ban|kick|mute
func applyFloodDelete(ctx context.Context, chatID int64, on bool) floodApply
func applyAutoLockdown(chat *gotgbot.Chat, change autoLockdownChange) autoApply  // supergroup check + autolockdownWrite + range
func applyLockdownLift(ctx context.Context, b *gotgbot.Bot, chatID int64, actor *gotgbot.User) liftApply
```

**Smallest refactor per command (observable replies unchanged):**

| Setting | Command body today | Apply core to extract | Reply mapping that must not change |
|---------|-------------------|-----------------------|------------------------------------|
| captcha on/off | `captchaCommand` `captcha.go:465-546`: `SetCaptchaEnabled`; on `off` also `disableCaptchaForChat` (`:548`); `/captcha` with no args shows settings | `applyCaptchaEnabled` | on-fail `captcha_enable_failed` / `captcha_disable_failed` (reply then `return err`); cleanup failure replies `captcha_disable_failed` and returns the error **after the flag is already saved**; success `captcha_enabled_success` / `captcha_disabled_success` |
| captcha mode | `captchaModeCommand` `:587-649`: lower-case, `mode != "math" && mode != "text" && mode != "turnstile"` -> `captcha_mode_invalid`; `turnstile` -> `captchaWebModeRefusal(bot, chat)` reply by the returned key; `SetCaptchaMode`; `ErrInvalidCaptchaMode` -> `captcha_invalid_mode_error`, other -> `captcha_mode_failed`; success `captcha_mode_set_formatted` with mode and `captcha_mode_*_desc` | `applyCaptchaMode` (the helper `captchaWebModeRefusal` `:658` takes only `bot` and `chat.Id` effectively, so it is already context-free) | as listed |
| captcha time | `captchaTimeCommand` `:683-726`: `ParseInt`, `timeout64 < 1 \|\| timeout64 > 10` -> `captcha_timeout_invalid`; `ErrInvalidTimeout` -> `captcha_timeout_range_error`; else `captcha_timeout_failed`; success `captcha_timeout_set_success` | `applyCaptchaTimeout` (range check moves in, so the drift test has one rule) | out-of-range stays `captcha_timeout_invalid` |
| captcha action | `captchaActionCommand` `:728-770`: `action != "kick" && action != "ban" && action != "mute"` -> `captcha_action_invalid`; `ErrInvalidFailureAction` -> `captcha_invalid_action_error`; else `captcha_action_failed`; success `captcha_action_set_success` | `applyCaptchaAction` | as listed |
| flood limit | `setFlood` `antiflood.go:421-473`: off words `[]string{"off", "no", "false", "0"}` -> `SetFloodContext(.., 0)`, reply `<module>_setflood_disabled`; `Atoi` fail -> `_errors_invalid_int`; `num < 3 \|\| num > 100` -> `_errors_set_in_limit`; write fail -> `common_settings_save_failed`; success `_setflood_success` | `applyFloodLimit` | reply keys are built as `strings.ToLower(m.moduleName) + "_..."`; keep that code in the command |
| flood mode | `setFloodMode` `:514-555`: `[]string{"ban", "kick", "mute"}`; unknown -> `_setfloodmode_unknown_type`; write fail -> `common_settings_save_failed`; success `_setfloodmode_success` | `applyFloodMode` | as listed |
| flood delete | `setFloodDeleter` `:557-605`: `"on","yes"` / `"off","no"`; invalid -> `_flood_deleter_invalid_option`; write fail -> `common_settings_save_failed` | `applyFloodDelete` | also registered as `clearflood` (`:613`) |
| auto triggers | `autolockdownApply` `autolockdown.go:195-251`: basic group -> `lockdown_basic_group`; parse -> `lockdown_auto_range` / `lockdown_auto_usage`; `autolockdownWrite`; `ErrThresholdOutOfRange` -> `lockdown_auto_range`; else `lockdown_auto_save_failed`; success `lockdown_auto_saved` + status | `applyAutoLockdown(chat, change)` = chat-type check + `autolockdownWrite(chat.Id, change)` + range mapping; keep `autoLockdownChange` and `parseAutoLockdownArgs` as they are | `TestAutoLockdownSaveFails` replaces the `autolockdownWrite` variable (`autolockdown_test.go:722`): keep that variable and call it from the apply function |
| lift | `unlockdown` `lockdown.go:401-443`: `GetCurrentFresh`; nil -> `lockdown_not_active`; `lifting` -> `lockdown_lift_in_progress` with the lifter; `liftLockdownRow`; outcomes `lockdown_restore_failed`, `lockdown_lift_record_failed`, `lockdown_already_lifted`, else `lockdownLiftedText` | `applyLockdownLift` returning the row, the outcome and the `lockdownLiftResult` | the alert-button path (`lockdown_alert_callbacks.go:264-319`) is a second consumer; do not touch it |

**What stays in the command:** argument parsing, the gate (`captchaAdminGate`, `IsUserConnected`, `requireAutoLockdownAuthority`), and the reply. Handlers keep their `(bot, ctx)` signature because the existing tests call them directly (`captchaModule.captchaCommand(bot, ctx)` etc.).

**Post-write value for the page:** return the value read back after the write rather than computing it (Pitfall 2). Reads stay on the same cached getters as the commands; a write already invalidated the key on this replica.

### Pattern 4: Authority per control, as the code really is (D-05; corrects the UI contract)

All "live" checks use one `lockdownLiveMember(b, groupID, presserID)` per press; one record answers both the admin and the restrict question. Anything but creator/administrator is `settings_not_admin`, a lookup error is `settings_check_failed`.

| Control | What the command does today (file:line) | Menu rule |
|---------|----------------------------------------|-----------|
| Captcha on/off | `/captcha`: `fullAdmin` = `RequireGroup, RequireUserAdmin, RequireBotAdmin` (`captcha.go:2261`, `2269`) plus `captchaAdminGate(.., true)` -> `chat_status.RequireBotAdmin` = `IsBotAdmin` (`access.go:170`, `chat_status.go:303`); refusal `chat_status_bot_not_admin` | live admin, then `chat_status.IsBotAdmin(b, nil, &groupChat)` (`extractChatFromContext` returns an explicit chat first, `chat_status.go:97-100`); notice `chat_status_bot_not_admin` |
| Captcha mode, time, action | `userAdmin` = `RequireGroup, RequireUserAdmin` and `captchaAdminGate(.., false)` (`:2262`, `2270-2272`) | live admin only |
| Antiflood limit, action, delete | **`chat_status.IsUserConnected(b, ctx, true, true)`** (`antiflood.go:423`, `516`, `559`): second `true` = `botAdmin`; it replies `connections_is_user_connected_bot_not_admin` when `!IsUserAdmin(b, chat.Id, b.Id)` (`chat_status.go:486-497`) and checks the user after the bot (`:498-509`) | live admin, then `chat_status.IsUserAdmin(b, groupID, b.Id)`; notice `connections_is_user_connected_bot_not_admin` ("I'm not an admin in this chat!", `en.yml:388`). **The UI contract's "admin only" row is wrong** |
| Antiflood view | `/flood` uses `IsUserConnected(.., false, true)` (`:482`): even viewing needs the bot admin | the page view needs only the live admin (D-04); showing the page does not need to refuse |
| Lockdown Lift, trigger switch, threshold | `requireLockdownAuthority(true)` / `requireAutoLockdownAuthority` change branch (`lockdown.go:131-170`, `autolockdown.go:49-58`); `staffIssuerSkipReason` | `staffIssuerSkipReason(member)`: `""` ok, `staffReasonSkipIssuerNoRight` -> `settings_no_restrict`, else `settings_not_admin` |
| Lockdown view | `requireLockdownAuthority(false)` | live admin |
| Staff link / unlink | link: descriptor is only `RequireGroup()` (`staff_link.go:66`) and the checks are `helpers.IsAnonymousSender`, `CheckOwner` of the Staff Group and of the target (`:208-251`); unlink: `checkUnlinkAuthority` (`staff_unlink.go:52-72`) | the pipeline's live admin step, then exactly those functions; refusal texts via `staffLinkRefusalText` / `staffUnlinkRefusalText` as notices |

Notes: the menu's live user check is stricter than the commands' cached `IsUserAdmin` (which also admits `tgAdminList`, `chat_status.go:198`), so a press is never allowed where the command would refuse. The bot-admin check must use the same predicate as the command (cache based) for authority parity; do not "improve" it to a live `FetchBotMember` here.

### Pattern 5: Staff page (MENU-05, D-12, D-21, D-22)

**Variant choice:** `staff.GetStaffGroupFresh(groupID)` (`staff/repository.go:204`): non-nil -> read-only panel variant; nil -> link variant (a) private / (b) group. The group-menu (b) variant must never reach a string that names the Staff Group.

**Link variant data:** `staff.GetLinkOfGroupFresh(groupID)` (`:325`, uncached authority read). For health, one `buildStaffPanelRows(ctx, b, []models.StaffGroupLink{*link})` (`staff_panel.go:419`; it already calls `recheckLink`, which removes a link whose owner changed and posts its notice, then `FetchBotMember`), then `markLockedRows(rows)` (`:464`). Empty `rows` after the call means the link was removed: draw "Not linked". Wrap in `context.WithTimeout(ctx, staffPanelBuildTimeout)` (45 s, `:31`) and answer the callback empty **first**, as `staffPanelRebuild` does (`:571`).

**Render pieces that need a small refactor (existing output must stay identical, `staff_panel_render_test.go` pins it):**
- `renderStaffRow` (`:162-187`) starts with the *linked group's* title and ID line. The page already prints the group line in its header, so split it into `renderStaffRowHead` + `renderStaffRowStatus` (the status, reason and lockdown lines) and have `renderStaffRow` concatenate them.
- Read-only panel: `renderStaffPanel` (`:297`) always adds Add group, Refresh, Recent and Unlink buttons. Do not call it. Call `staffPanelTextFor(tr, staffPanelView{..., helpAsHint: true})` (`:271`): it already sets `helpAsHint = true` on overflow and tolerates it being true on entry. Add a `limit int` field (zero = `staffPanelMaxUTF16`, 3800, `:38`) so the settings header and footer fit inside the same 3800 units.
- Paging: `staffPanelPageSize = 8` (`:34`); the menu uses its own `a=pg&v=<page>` and clamps the page.

**Link press:** split two things out of `staff_link.go` (no behaviour change; `runLinkGroup` becomes a caller):
1. `listVerifiedOwnStaffGroups(b, issuer) (matched []models.StaffGroup, unknown int, err error)`: the loop of `resolveOwnStaffGroup` (`:173-203`). `resolveOwnStaffGroup` keeps its refusals on top of it. The picker needs the list; today only the refusal `staffRefusalNeedStaffID` escapes.
2. `linkToStaffGroup(b, issuer, target *gotgbot.Chat, staffGroup *models.StaffGroup) staffLinkOutcome`: lines 287-306 of `runLinkGroup` (`checkLinkTarget`, `FetchBotMember`, `storeLink`). `runLinkGroup` = anonymous check + resolve + `linkToStaffGroup`.
And one more: `announceStaffLink(b, out)` = lines 396-405 of `deliverLinkOutcome` (the `staff_link_done` post to the Staff Group, in the Staff Group's language, with the health warning), so the menu can post it without `deliverLinkRefusal` (which replies into the issuing chat).

Decision flow in the menu: zero verified -> notice `staff_link_no_staff_group`; unknown > 0 and no match -> notice `staff_check_failed`; exactly one -> `linkToStaffGroup` at once; more than one -> picker. On a pick: re-list `staff.ListStaffGroupsByOwner(presser)` (`:302`), find the row with `ID == s` (`models.StaffGroup.ID` is `uint`, `models/staff.go:21`), then `resolveNamedStaffGroup(b, &query.From, strconv.FormatInt(row.ChatID, 10))` (`:208`, verifies the live creator), then `linkToStaffGroup`. A target `*gotgbot.Chat` is `query.Message.GetChat()` in a group menu and `chatFullInfo.ToChat()` after `b.GetChat(c, nil)` in a private menu (`gotgbot custom_helpers.go:96`; the same conversion is in `chat_status.go:438`).

`helpers.IsAnonymousSender(ctx, user)` (`command_pipeline.go:289`) works with a callback context: gotgbot sets `EffectiveSender = {User: query.From}` for callbacks, so a real presser is not anonymous; the service-identity step 2 already removed `777000`, `1087968824` and `136817688`.

**Unlink:** ask = `staff.GetLinkOfGroupFresh` (nil -> `staffUnlinkNotLinked` text) then `checkUnlinkAuthority(b, &query.From, *link)`; the card text is `staff_unlink_confirm_prompt` with the group title spliced after translation (pattern at `staff_unlink.go:327-328`). Yes = `runUnlinkGroup(b, &query.From, *link)` (`:78`), which re-checks both owners, deletes with `staff.DeleteLink` (conditional, one winner) and posts `staff_notice_unlinked_by` to the Staff Group only. Both functions take plain arguments; nothing to refactor.

**Privacy (D-21) enforcement points:** (1) the group-menu renderer never receives the Staff Group title or ID: build a `settingsStaffView` whose `StaffTitle`/`StaffChatID` fields are zeroed unless `viewer == private || group is a Staff Group`; (2) `s` is the `staff_groups.id` row ID, never `chat_id`; (3) every reused refusal listed in the UI-SPEC privacy table names only THIS group (confirmed against `staffLinkRefusalText` `staff_link.go:328-357` and `staffUnlinkRefusalText` `staff_unlink.go:115-135`: the only title argument is the linked group's own `GroupTitle`/`target.Title`); (4) `staff_link_done` is posted to the Staff Group, never rendered in a menu (UI-SPEC rule, matches `deliverLinkOutcome`); (5) `renderStaffRow`'s lockdown line carries no reason or starter (`:179-185`).

### Pattern 6: Lockdown page (MENU-06, D-13)

- State: `lockdown.GetCurrentFresh(chatID)` (`lockdown/repository.go:137`, active row, else the lifting row with the highest ID). For `lifting` also `lockdown.TallyJoiners(row.ID)` (`:170`) and the existing `lockdownLiftingStatusText` (`lockdown_status.go:135`) for "x of y done". The body lines of `/lockdownstatus` are assembled inline in the handler (`lockdown_status.go:56-101`); extract only the head (`lockdown_status_active` or `_auto` with `lockdownTriggerSentence`, `lockdownReasonLine`, `lockdown_status_unconfirmed`) into a pure `lockdownStatusHead(tr, row)`; the page does not need the removed/declined/permission lines, and the permissions line is a Telegram call.
- Reason line: reuse `lockdownReasonLine` (`lockdown.go:258`, key `lockdown_reason_line` = `"Reason: {reason}"`, `en.yml:2884`) instead of the new `settings_lockdown_reason` the UI contract lists (D-23). Cut the reason at 200 runes before escaping (`lockdownCapRunes`, `:181`).
- Names: the head uses `lockdownMention` (`:231`, an HTML mention link). In a group menu that is fine (an edit does not notify); keep it.
- Triggers: read with `autolockdownReadSettings(chatID)` (`autolockdown.go:256`, bounded by `raidSettingsTimeout` = 2 s, `raid_join.go:16`; the `raidSettingsLookup` variable is the test seam, `raid_join.go:22`). Writes: `applyAutoLockdown` -> `autolockdownWrite`. `raid_detection` is in `skipLocal` (`alita/db/cache/local.go:68-71`), so a redraw on another replica is fresh.
- AI line: `raidAIConfigured()` (`raid_ai.go:84`) decides `lockdown_auto_ai_unavailable`.
- Lift: ask = fresh `GetCurrentFresh`, not active -> notice `lockdown_already_lifted` / `lockdown_not_active` (not a card); live `staffIssuerSkipReason` -> card with `lockdown_alert_lift_ask` (`{group}` only in `en.yml:2926`; pass the escaped title through a token and splice, like `lockdownGroupText`, `lockdown_alert_callbacks.go:204`). Yes = steps 1-5 again, expiry check, fresh row again, `liftLockdownRow(ctx, b, row, actorID, name)` with `name := lockdownCapRunes(staffFullName(&query.From), lockdownNameMaxRunes)` (as `:294`). Outcome mapping is the `unlockdown` switch (`lockdown.go:434-442`).
- **Public notice (gap in the UI contract):** `/unlockdown` replies into the group, and the alert button path posts `lockdownPostLiftNotice(b, ld, lockdownLiftedText(staffChatTranslator(ld.ChatID), ...))` then `wakeLockdownWorker()` (`lockdown_alert_callbacks.go:314-317`). A Lift from a private menu would otherwise leave the group unaware. Recommendation: do what the alert path does, in both menu variants.
- Basic group: `chat.Type != "supergroup"` -> only the `lockdown_basic_group` note (`autolockdown.go:208`, `lockdown.go:313`) and no controls; `applyAutoLockdown` must also refuse a forged press for it.

### Pattern 7: Edit, Close, not-modified

- Redraw: `editStaffMessage(b, query.Message, text, keyboard)` (`staff_unlink.go:205-225`) sets HTML, disables the link preview, treats `message is not modified` as success (`isMessageNotModified`, `staff_panel.go:506`) and sets `ReplyMarkup` only when the keyboard is non-empty. Omitting the markup removes it, which is how "Menu closed" and "cannot open" lose their buttons (`editStaffCallbackMessage` relies on the same, `staff.go:309-317`).
- Close: `query.Message.Delete(b, nil)`; on error `EditText` to `settings_closed`. Answer once, empty.
- A failed edit is logged and never undoes a write (the spec rule); do not retry a 429.

### Pattern 8: Registration

```go
// settings.go (shape; settingsDesc is new)
var settingsModule = moduleStruct{moduleName: "Settings"}   // moduleName is what generate_docs parses

var settingsDesc = helpers.CommandDescriptor{
	Name:           "settings",
	RequiredChecks: []helpers.CheckFunc{requireLockdownGroup(), requireLockdownAuthority(false)},
}

func LoadSettings(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(settingsModule.moduleName, true)               // core.go:42, drives /help
	helpers.WrapCommand(dispatcher, settingsDesc, settingsModule.settings)  // generate_docs needs this exact call shape
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("settings|"), settingsModule.settingsCallback))
}

func init() {
	RegisterLegacyModule("Settings", 239, LoadSettings)             // after Lockdown (238), before Blacklists (240)
	RegisterAnonymousAdminHandler("settings", anonPipelineHandler(settingsDesc, pipelineSettings))
	RegisterDeepLinkHandler("settings_", settingsStartDeepLink)     // deeplink_router.go:21
}
```

- `requireLockdownGroup()` and `requireLockdownAuthority(false)` are exactly D-01: group only, live `getChatMember`, anonymous admin sent to `PromptAnonAdminProof`, refusals as replies to the command message (`lockdown.go:75-100`, `131-170`). Naming them "lockdown" is a smell; alias them (`requireSettingsGroup = requireLockdownGroup`) rather than copy.
- The anonymous re-entry swaps the callback update for the original message update and sets the tapper as `EffectiveUser`/`EffectiveSender` (`bot_updates.go:208-222`), so `c.User` in the re-entered command is the tapper and becomes `u`. The `anonPipelineHandler` signature is `func(b, ctx) error` taking the domain handler (`moderation.go:349`); `pipelineHandler` adapts a `(c *CommandContext)` handler (`:340`).
- The pattern the docs generator needs is `helpers.WrapCommand(dispatcher, xDesc, module.method)` with `(\w+)Desc` and `(\w+)\.(\w+)` (`scripts/generate_docs/parsers.go:268-280`); `/settings` is not Disableable, so no `AddCmdToDisableable`.
- Help: `settings_help_msg` (key is `<module lower>_help_msg`, `help_system.go:58`), `alt_names.Settings: [settings]` in `locales/config.yml`, then `make generate-docs` produces `docs/src/content/docs/commands/settings/index.md`.
- Deep link (private only; a group `/start` payload goes through the separate group registry, `help.go:361-374`): longest-prefix routing means `settings_` does not collide with `help_`, `rules_`, `note_`, `notes_`, `connect_` or the catch-all `note` (`rules.go:320`, `notes.go:968-970`, `connections.go:362`, `help.go:629`).

```go
func settingsStartDeepLink(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	raw, _ := strings.CutPrefix(arg, "settings_")
	chatID, ok := parseStaffChatIDArg(raw)                 // strict, negative, <= 20 chars
	if !ok { /* reply settings_start_invalid, return ext.EndGroups */ }
	// live member of chatID for user; error -> settings_check_failed; not admin -> settings_start_not_admin
	// b.GetChat(chatID): bot missing or group gone -> settings_cannot_open
	// else SendMessage(ctx.EffectiveChat.Id, mainPage) in the user's language (lang.GetLanguage(ctx) is the user's here)
	return ext.EndGroups
}
```

The bot username for the URL button is `getBotUsername(b)` (`help.go:29`); if it is empty, omit the URL button and log (a bot always has one in practice).

### Anti-Patterns to Avoid

- **Calling the command bodies from the menu.** They read `ctx.EffectiveChat`/`EffectiveMessage` and reply with `msg.Reply`. In a private menu `EffectiveChat` is the private chat, so the command would act on the wrong chat or reply into the wrong place.
- **Computing the new value on the page** instead of reading what the write stored (Pitfall 2).
- **A free-form string in a callback.** Titles, names and reasons never go in callback data; `s` is a row ID.
- **Deciding authority from callback data**, from the group menu's `u` alone, or from the admin cache.
- **Building keys by concatenation** the translator or `check-translations` cannot see (`staffRowReason` shows the literal-switch pattern, `staff_panel.go:134`).
- **Rendering Telegram error text** to users (the lockdown module's rule: log it, show a plain sentence).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Callback encoding and the 64-byte cap | `strings.Split`, manual joins | `encodeCallbackData` / `decodeCallbackData` | overflow returns `""`; Decode URL-decodes |
| Live admin / restrict check | admin cache or own `getChatMember` wrapper | `lockdownLiveMember` + `staffIssuerSkipReason` | exact rule of `/lockdown` and the alert buttons |
| Owner of both groups | owner column | `chat_status.CheckOwner` via `checkUnlinkAuthority` / `resolveNamedStaffGroup` / `checkLinkTarget` | `owner_user_id` is never an authority |
| Two-tap cards with expiry | Redis tokens | asker/expiry in data (`lockdown_alert_callbacks.go:227-239`, `278`) | D-08, works without Redis |
| Lift | own restore/record | `liftLockdownRow` | order of restore then conditional record is a safety property |
| Staff panel text, rows, truncation to 3800 | new layout code | `composeStaffPanelText` / `staffPanelTextFor` | already length-capped and tested |
| HTML escaping of titles after translation | `fmt.Sprintf` into the template | token + `strings.Replace`/`lockdownSplice` (`lockdown.go:211`) with `staffDisplayTitle` (`staff_panel.go:83`) | the translator runs a printf-style pass |
| Deep-link routing | edit `start` | `RegisterDeepLinkHandler` | other payloads untouched |
| Edit-in-place, not-modified handling | new wrapper | `editStaffMessage` | one place for the error rule |

**Key insight:** every behaviour the menu needs is already implemented once in this repo; the risk is divergence, so each shared piece must have exactly one implementation and two callers.

## Runtime State Inventory

Not a rename/refactor of identifiers, so the full inventory does not apply. The refactor does touch command bodies: the observable state is unchanged.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None new. Reads/writes only `captcha_settings`, `antiflood_settings`, `chat_raid_detection`, `staff_group_links`/`staff_groups`, `chat_lockdowns` | none |
| Live service config | None | none |
| OS-registered state | None | none |
| Secrets/env vars | None new | none |
| Build artifacts | None (no new binary, no generated Go) | `make generate-docs` regenerates Markdown only |

**Redis keys:** none added (D-08). **Migrations:** none. **Models:** none new; the existing `AutoMigrate` list already contains `CaptchaSettings`, `AntifloodSettings`, `StaffGroup`, `StaffGroupLink`, `ChatLockdown`, `LockdownJoiner`, `ChatRaidDetection` (`alita/modules/test_harness_test.go:344`, `355`, `366-375`). **Shutdown drains:** none added; menu presses are short synchronous handlers and a Lift in flight at shutdown behaves like the existing alert Lift.

## Common Pitfalls

### Pitfall 1: A forwarded menu keeps working in another chat (HIGH)
**What goes wrong:** inline keyboards survive forwarding of a bot message `[ASSUMED]`; the callback arrives with the copy's chat. **How to avoid:** the structural rules in step 1 (`c` only in a private message, `u` only in a non-private one) plus step 4's live check against the chat the message really sits in (group menu) or the verified `c` (private). A group menu forwarded to another group makes the opener an admin-check subject of *that* group. **Warning sign:** a test with `c` on a group message must be refused before any I/O.

### Pitfall 2: Antiflood creates a row with limit 5 as a side effect (HIGH)
**What goes wrong:** `SetFloodModeContext` and `SetFloodMsgDelContext` return early when the cached value equals the target, otherwise `FirstOrCreate(...).Assign(updates)` (`antiflood/repository.go:36-41`, `73-92`). On a chat with no row the insert omits `flood_limit`, and the model declares `default:5` (`models/antiflood.go:8`). GORM then inserts the default `[ASSUMED: GORM zero value plus default tag]`. So `/setfloodmode ban` or `/delflood on` on a never-configured chat probably turns antiflood on at 5. `TestSetFloodMode` "creates record with valid mode" only asserts the action (`repository_test.go:165-184`). **Consequence for the menu:** pressing Ban or the delete switch on an unconfigured chat has the same side effect (parity), and a page drawn from "limit 0" would lie. **How to avoid:** add a characterization test first (fresh chat -> `SetFloodMode` -> read `flood_limit`); make the apply function re-read with the uncached `antiflood.GetAntifloodSettingsContext(ctx, chatID)` (`optimized.go:17`) after the write; do not change the repository (D-17: replies and behaviour unchanged) unless the owner asks (Question 2).

### Pitfall 3: `GetFloodContext` hides read errors
`GetFloodContext` returns `Limit 0, Action "mute"` on any error (`repository.go:20-30`), so an unreadable database looks like "antiflood off". The Antiflood page needs a real error for D-11: call `GetAntifloodSettingsCachedContext` (`optimized.go:48`), the same cache entry the command reads, and treat an error as cannot-open. Likewise `GetCaptchaSettingsContext` returns the error (`captcha/repository.go:31-54`).

### Pitfall 4: Staleness across replicas
`captcha_settings` and `antiflood` keys are not in `skipLocal` (`alita/db/cache/local.go:42-72` lists only `captcha_pending`, `staff_group`, `staff_link_of`, `raid_detection`). `DeleteCache` evicts the local copy on the writing replica only, so a second replica can show the old value for `CACHE_LOCAL_TTL` (default 10 s, AGENTS.md Data). The command has the same skew. Recommendation: accept and document; adding the keys to `skipLocal` would add a Redis round trip to every group message (`checkFlood` reads antiflood first, `antiflood.go:261`) and every join. The pressing replica is always consistent. Cheap mitigation for an immediate redraw: the apply function returns the DB read-back.

### Pitfall 5: Opener check with anonymous admins and channels
Callback `From` is always a real user, so `u` equality is well defined. After the anonymous proof the tapper is the sender and becomes `u` (`bot_updates.go:221-222`). A press whose `From` is a service identity (`staffServiceUserIDs`, `staff_action_decide.go:513`: `var staffServiceUserIDs = map[int64]bool{777000: true, 1087968824: true, 136817688: true}`) is refused first. A callback from a message in a channel is not delivered to a handler without `SetAllowChannel(true)`, which the menu does not set.

### Pitfall 6: Demoted opener and stale buttons
The group menu stays on screen after the opener loses admin; the live check at step 4 refuses it and edits nothing. A stale confirm card carries `e` and is refused after 5 minutes (lazy, no timer). The Yes press re-runs steps 1-5 and reads the lockdown/link fresh, so a card that outlived its subject says "already lifted" / "not linked" instead of acting.

### Pitfall 7: "Message is not modified" and rate limits
A double tap, a stepper at a bound, or a redraw within the same second on the pages that print `Updated HH:MM:SS` can produce the same text and markup; Telegram answers 400 "message is not modified" (`isMessageNotModified`). Success, not error. A 429 on edit is not retried (spec); the write already happened and the next press redraws. Edit flood limits are `[ASSUMED]` platform behaviour.

### Pitfall 8: Close in private after 48 h, and inaccessible messages
`[ASSUMED]` Telegram only deletes messages younger than 48 hours in some cases; the contract's fallback (edit to `settings_closed`, no buttons) covers it. Do not special-case `InaccessibleMessage`; use the interface methods.

### Pitfall 9: Private menu outlives the group
A private menu holds `c`. If the bot is removed, the group is deleted, or a basic group migrates to a supergroup (new chat ID), the step-4 `getChatMember` fails. `chat_status` has an unexported classifier for "bot missing" (`botMissingMarkers = {"bot was kicked", "bot is not a member", "chat not found"}`, `owner.go:51`, `isBotMissingError` `:55`). Recommendation: export a thin `chat_status.IsBotMissingError(err)` (one line) and map that case to the D-11 cannot-open edit, every other lookup error to `settings_check_failed`. Without it, a removed bot gives a repeatable "could not check your rights" alert on a dead menu.

### Pitfall 10: Slow presses and callback validity
Staff page build (up to 2N+1 calls), captcha off cleanup and Lift can outlast a callback query's validity `[ASSUMED]`. Answer empty after step 5, then work (pattern at `staff_panel.go:570-571`). An apply refusal after the answer is a notice on the redrawn page, as the contract says.

### Pitfall 11: Private chat language and connections
`lang.GetLanguage` in a private chat reads the *user's* language (`repository.go:42-52`), so the private menu follows the admin, never the group (D-10). The private-chat "connected chat" feature (`chat_status.go:414-482`) is only consulted by commands that call `IsUserConnected`; the menu never calls it, so a connection to another group cannot redirect a menu press. `/settings` itself is group only. `CheckDisabledCmd` exempts admins (`chat_status.go:165-167`), and every presser here is a live admin, so `Disableable` and `/disable` have no interaction; `/settings` stays not Disableable (D-01).

### Pitfall 12: Group menu text is public
Every member can read a group menu and the callback data of its buttons. Titles of *this* group and the opener's name are fine; the Staff Group title/ID, Staff Group row IDs other than the presser's own picker, and any user ID other than the opener's must not appear. User text is escaped after translation, capped (64 runes for names and titles, 200 for a reason).

### Pitfall 13: Stepper arithmetic at bounds
Raid thresholds 3..500 (`raiddetection.MinThreshold`/`MaxThreshold`, `raiddetection/repository.go:30-33`), antiflood Off then 3..100 (Off sits below 3: from 3 `-1` gives Off, from Off `+1` gives 3, `+10` gives 12), captcha 1..10. Steps clamp to the bound; a button at a bound keeps valid data and answers `settings_min_value`/`settings_max_value`. A test renders every page at both bounds.

### Pitfall 14: `make check-translations` and generated docs are blind spots
`check-translations` sees literal `GetString` keys only (`scripts/check_translations/main.go`); the locale test must cover the whole `settings_` set. Generated docs read only `locales/en.yml`; `make check-docs` fails on drift.

### Pitfall 15: Stale worktree copy
`/home/user/Fuku_Robot/.claude/worktrees/agent-aae3302fd6b0eafae/` holds an older copy of the module. Greps over `.` hit it; scope searches to `alita/`.

## Code Examples

### Strict group-ID parse shared by callback and deep link
```go
// Source: alita/modules/staff_link.go:74-92 (existing). Reuse, do not re-implement.
func parseStaffChatIDArg(arg string) (int64, bool) {
	if arg == "" || len(arg) > maxStaffChatIDArgLen { return 0, false }
	// optional leading minus, digits only, and id >= 0 is rejected
	...
}
```

### Expiry-minute card (stateless)
```go
// Source: alita/modules/lockdown_alert_callbacks.go:227-239 and 273-282 (existing pattern).
expiry := strconv.FormatInt(time.Now().Add(settingsCardLifetime).Unix()/60, 10) // (new const, mirrors lockdownCardLifetime)
confirm := encodeCallbackData("settings", map[string]string{"a": "ly", "c": groupStr, "e": expiry})
// at Yes:  if expiryMinute < time.Now().Unix()/60 { notice settings_card_expired; no write }
```

### Worst-case callback size test (shape)
```go
for _, tc := range worstCasePages() {            // c = -1001234567890 (14 chars), u = 1234567890, e = 29800000
	for _, btn := range allCallbackButtons(tc.markup) {
		if len(btn.CallbackData) > callbackcodec.MaxCallbackDataLen { t.Errorf(...) }   // const exists, callbackcodec.go:13
	}
	if omitted := tc.omittedButtons; omitted != 0 { t.Errorf(...) }                     // renderer counts encode failures
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Per-module command-only configuration | one callback namespace with apply functions shared by commands | this phase | commands keep their texts, menu cannot drift |
| Admin cache for command gates | live `getChatMember` for any new surface | Phases 4-6 | the menu follows the lockdown rule, not the antiflood/captcha rule |
| `/antiraid` | `/lockdown` + `/autolockdown` | Phase 6 | Lockdown page edits `chat_raid_detection` |

**Deprecated/outdated:** `antiflood` actions `warn`, `tban`, `tmute` are allowed by the DB check (`models/antiflood.go:9`, quoted: `action IN ('mute','ban','kick','warn','tban','tmute')`) but no command writes them (D-20); the page shows them read-only.

## Spec Deltas (UI-SPEC and CONTEXT vs the code)

| # | UI-SPEC / CONTEXT says | Code says | Action |
|---|------------------------|-----------|--------|
| 1 | Antiflood changes need "admin only" | needs live admin **and** bot admin (`IsUserConnected(.., true, true)`), notice `connections_is_user_connected_bot_not_admin` | follow the code; update the authority table and the "Planner flag" outcome; add a parity test |
| 2 | Redraw after a write from the apply result, no re-read | fresh-row antiflood writes may store limit 5 | apply returns a DB read-back (Pitfall 2) |
| 3 | Antiflood/Captcha read failure -> D-11 | `GetFloodContext` never errors | use `GetAntifloodSettingsCachedContext` |
| 4 | Plan checks `skipLocal` per key (D-18) | captcha and antiflood not in `skipLocal` | accept ≤10 s skew, document |
| 5 | `settings_lockdown_reason` new key | `lockdown_reason_line` exists | reuse (D-23); drop the new key and its ceiling row |
| 6 | Staff panel "reuses `composeStaffPanelText` with `helpAsHint`" | `renderStaffRow` includes the linked group's title/ID; `staffPanelTextFor` has a fixed 3800 cap | split `renderStaffRow`; add `limit` to `staffPanelView` |
| 7 | Lift "shows the lift result as a notice" | the group also needs the public notice (`lockdownPostLiftNotice`) as the alert path posts | post it (Question 1) |
| 8 | "message inaccessible answers `common_callback_message_unavailable`" | the interface methods work for both types | nil-only check |
| 9 | Invalid request = alert `common_callback_invalid_request` | `answerInvalidCallback` is a toast | own alert helper |
| 10 | Link flow: "exactly one verified Staff Group links at once, several open the picker" | `resolveOwnStaffGroup` hides the matched list | split into `listVerifiedOwnStaffGroups` + `linkToStaffGroup` |
| 11 | Main page loader "four reads and no Telegram call" | a private menu needs `getChatMember` (step 4) and `getChat` (title/type) per press | state it: 2 calls per private press, 1 per group press; Staff/Lift add more |
| 12 | `settings_btn_yes/no` new keys | `lockdown_alert_btn_confirm/cancel`, `staff_btn_confirm/cancel` exist | UI contract wants icons and one label for both cards: keep the new keys, but do not reuse them for the old surfaces |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | GORM inserts `flood_limit` 5 on `FirstOrCreate` for a zero `Limit` with `default:5` | Pitfall 2 | If wrong, the page-after-write issue disappears; the characterization test settles it either way |
| A2 | Telegram keeps callback keyboards when a bot message is forwarded | Pitfall 1 | Low: the structural and live checks are needed anyway |
| A3 | Bots cannot always delete old messages (48 h) and edits of very old messages may fail | Pitfall 8 | Low: fallback edit exists |
| A4 | A callback query answer expires after a short time (tens of seconds) | Pitfall 10 | If longer, answering first is harmless |
| A5 | Editing the same message rapidly yields 429 | Pitfall 7 | Low: not retried |
| A6 | Forwarded/old group menus still deliver callbacks with an `InaccessibleMessage` that EditText can address | Pattern 1 | If edits fail, the menu shows no change; user must run `/settings` again |

## Open Questions

1. **Which chat type does `callback.Message.GetChat()` report in forum topics, and does `EditText` need `message_thread_id`?**
   - What we know: staff panel edits and lockdown alert edits already work in supergroups.
   - Unclear: topic groups were not exercised by this repo's tests.
   - Recommendation: no code change; put a topic-group case in the `09-SPIKE.md` live checklist.
2. **Does `getChat` on a private menu need a per-press call?**
   - Needed for title, type (basic group refusal) and "bot removed". Recommendation: one `getChat` per private press with a 5 s timeout; cache nothing.

## Questions for the Owner

1. **Public notice on a menu Lift.** `/unlockdown` replies in the group; the alert button posts the lifted notice into the group. A Lift from the menu has no group-visible trace unless it posts the same notice. **Recommendation: post `lockdown_lifted` into the locked group (as the alert button does), in both menu variants.** Cost: one extra post; benefit: members see the group reopened.
2. **Antiflood side effect on an unconfigured chat.** `/setfloodmode` and `/delflood` on a chat that never ran `/setflood` probably also set the limit to 5, i.e. turn antiflood on (Pitfall 2, to be confirmed by a test). The menu would inherit that. **Recommendation: keep it identical to the commands for this phase (D-17: replies and behaviour unchanged), show the stored value on the page, and file a separate fix** (it needs a repository change that also affects the commands).
3. **Cross-replica staleness of captcha and antiflood (≤10 s).** **Recommendation: accept** (single deploy today, same as the commands); do not add the keys to `skipLocal`.

No question blocks planning.

## Existing Code to Extend (file, function, line)

| # | What | Where | Change |
|---|------|-------|--------|
| 1 | Captcha command bodies | `alita/modules/captcha.go`: `captchaAdminGate` 445, `captchaCommand` 465, `disableCaptchaForChat` 548, `captchaModeCommand` 587, `captchaWebModeRefusal` 658, `captchaTimeCommand` 683, `captchaActionCommand` 728, `LoadCaptcha` 2256 | call `applyCaptcha*`; gate and replies stay |
| 2 | Captcha repository | `alita/db/captcha/repository.go`: `GetCaptchaSettingsContext` 31, `SetCaptchaEnabled` 56, `SetCaptchaMode` 76, `SetCaptchaTimeout` 97, `SetCaptchaFailureAction` 136; errors 19-24 | none; each setter already calls `cache.DeleteCache(cache.CacheKey("captcha_settings", chatID))` |
| 3 | Antiflood commands | `alita/modules/antiflood.go`: `setFlood` 421, `flood` 475, `setFloodMode` 514, `setFloodDeleter` 557, `LoadAntiflood` 607 | call `applyFlood*`; `IsUserConnected` calls stay |
| 4 | Antiflood repository | `alita/db/antiflood/repository.go` 15-98, `optimized.go` 17-52 | none (see Pitfall 2) |
| 5 | Autolockdown | `alita/modules/autolockdown.go`: `requireAutoLockdownAuthority` 49, `autoLockdownChange` 62, `parseAutoLockdownArgs` 78, `autolockdownWrite` 133, `writeAutoLockdown` 136, `autolockdownApply` 195, `autolockdownReadSettings` 256 | extract `applyAutoLockdown`; keep the variable seam |
| 6 | Raid settings repository | `alita/db/raiddetection/repository.go`: `GetSettings` 72, `ErrThresholdOutOfRange` 104, `Set*` 129+ | none |
| 7 | Lock / lift | `alita/modules/lockdown.go`: `requireLockdownGroup` 93, `lockdownLiveMember` 108, `requireLockdownAuthority` 131, `unlockdown` 401, `liftLockdownRow` 485, `lockdownLiftedText` 536; `lockdown_alert_callbacks.go`: `lockdownButtonAuthority` 165, `lockdownLiftConfirm` 264, `lockdownPostLiftNotice` 704; `lockdown_status.go` 31-101 | extract `applyLockdownLift` and `lockdownStatusHead` |
| 8 | Staff link | `alita/modules/staff_link.go`: `parseStaffChatIDArg` 74, `resolveOwnStaffGroup` 173, `resolveNamedStaffGroup` 208, `checkLinkTarget` 232, `runLinkGroup` 273, `staffLinkRefusalText` 328, `deliverLinkOutcome` 381 | split as in Pattern 5 |
| 9 | Staff unlink | `alita/modules/staff_unlink.go`: `checkUnlinkAuthority` 52, `runUnlinkGroup` 78, `staffUnlinkRefusalText` 115, `editStaffMessage` 205, `staffUnlinkConfirmPrompt` 317 | none |
| 10 | Staff panel | `alita/modules/staff_panel.go`: `renderStaffRow` 162, `staffPanelView` 190, `composeStaffPanelText` 206, `staffPanelTextFor` 271, `renderStaffPanel` 297, `buildStaffPanelRows` 419, `markLockedRows` 464, `isMessageNotModified` 506 | split `renderStaffRow`; add `limit` |
| 11 | Staff reads | `alita/db/staff/repository.go`: `GetStaffGroupFresh` 204, `ListStaffGroupsByOwner` 302, `GetLinkOfGroupFresh` 325, `DeleteLink` 360 | none |
| 12 | `/start` | `alita/modules/help.go`: `start` 335 (private 2-arg path -> `HandleDeepLink`, 361-374 group path), `getBotUsername` 29; `deeplink_router.go` 21-48 | none; register from `settings.go` |
| 13 | Anonymous re-entry | `alita/modules/anonymous_admin_router.go` 14-30; `moderation.go` `pipelineHandler` 340, `anonPipelineHandler` 349; `bot_updates.go` `verifyAnonymousAdmin` 118 | register `settings` |
| 14 | Registration and help | `alita/modules/registry.go` 21; `core.go` `SetModuleEnabled` 42; `help_system.go` 58; `locales/config.yml` `alt_names` | add `Settings` |
| 15 | Locales | `locales/{en,es,fr,hi,id,pt,ru}.yml` (+ `config.yml`) | ~85 `settings_*` keys + `settings_help_msg` |
| 16 | Callback plumbing | `callback_codec.go` 15-56, `callback_context.go` 8, `staff.go` `answerStaffCallback` 301 | none |
| 17 | Cache | `alita/db/cache/local.go` `skipLocal` 56-72 | none (decision: no change) |
| 18 | Test harness | `alita/modules/test_harness_test.go` (`newModuleBotClient` 61, `newModuleCallbackContext` 184, AutoMigrate 329), `lockdown_fake_test.go` (`lockdownFake` 33, `newLockdownEnv` 339), `staff_action_fake_test.go` (`setMember` 139, `edits` 648, `lastAnswer` 677), `staff_helpers_test.go` (`withStaffLocale` 209) | add a `settingsEnv` that wraps `lockdownEnv` and loads `LoadSettings` |

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | yes | go1.26.0 | none |
| gcc / cgo | SQLite tests | yes | `/usr/bin/gcc` | none |
| make | `make test`, `make generate-docs` | yes | `/usr/bin/make` | direct `go test -tags testtools` |
| SQLite (via go-sqlite3) | tests | yes (cgo) | n/a | none |
| miniredis | tests (lockdown fixtures use it) | in `go.mod` | v2.39.0 | none |
| PostgreSQL | not used; **must never be started** | n/a | n/a | SQLite harness covers everything here |
| Network / live Telegram | not available | no | n/a | live checks go to `09-SPIKE.md` |

**Missing dependencies with no fallback:** none.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing`, `-tags testtools`, `-race` |
| Config file | none; harness in `alita/modules/test_harness_test.go`, `internal/testdb` |
| Quick run command | `go test -tags testtools -race -count=1 -run '^TestSettings' ./alita/modules ./alita/i18n` |
| Full suite command | `make test` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MENU-01 | `/settings` replies with Open here + URL button; non-admin and basic group refused; anon admin gets proof then choice; `/start settings_<id>` branch (valid, invalid, non-admin, check failed, group gone) | integration (dispatcher, `lockdownEnv`) | `go test -tags testtools -race -count=1 -run '^TestSettingsCommand\|^TestSettingsStart' ./alita/modules` | no, Wave 0 |
| MENU-02 | `u` / `c` structural rules; another user refused with `settings_not_your_menu`; forwarded-menu cases; private menu manages only `c` | integration | `-run '^TestSettingsOwnership'` | no |
| MENU-03 | step order (service id before lookup, opener before live check, demoted admin refused and nothing edited, lookup failure refuses); exactly one answer per press | integration | `-run '^TestSettingsPressPipeline'` | no |
| MENU-04 | one message edited in place, Back/Close, Close falls back to edit, not-modified is success, no dead button for worst-case IDs | integration + pure render | `-run '^TestSettingsNavigation\|^TestSettingsCallbackBytes'` | no |
| MENU-05 | Staff page variants (a)/(b)/read-only; link (none, one, several, picker, forged `s`); unlink two-tap; privacy test over 7 locales | integration | `-run '^TestSettingsStaff'` | no |
| MENU-06 | Lockdown page states; Lift two-tap with expiry and double Yes; trigger toggles and steppers incl. bounds; basic group | integration | `-run '^TestSettingsLockdown'` | no |
| MENU-07 | Captcha and Antiflood pages; Turnstile three ordered refusals; antiflood fresh-row limit; stored `warn/tban/tmute` read-only | integration | `-run '^TestSettingsCaptcha\|^TestSettingsAntiflood'` | no |
| MENU-08 | per setting: command-then-menu, menu-then-command, rows equal; drift table; authority parity table (incl. antiflood bot-admin) | integration | `-run '^TestSettingsParity\|^TestSettingsDrift\|^TestSettingsAuthorityParity'` | no |

Parity design: run the real command through `ProcessUpdate` (lockdown env dispatcher already loads commands) and the menu press through the same dispatcher, then compare `captcha.GetCaptchaSettings`, `antiflood.GetFlood`, `raiddetection.GetSettings` and the raw rows. Use `withStaffLocale(t)` (marker locale, `staff_helpers_test.go:209`) to assert that a menu refusal used the **same key** as the command's reply (`staffMarker(key)`), which is how "the menu refuses with the command's words" is proven.

Drift table (one table feeds both entries): raid threshold 2, 3, 500, 501; captcha time 0, 1, 10, 11; captcha mode `image`, `math`, `turnstile`; captcha action `warn`; flood limit 1, 2, 3, 100, 101, off; flood mode `warn`, `ban`; same verdict from the command and the menu.

### Sampling Rate
- **Per task commit:** the quick run for the files touched.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./alita/modules ./alita/i18n ./alita/db/antiflood ./alita/db/captcha`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make check-docs` green; `node --test scripts/captcha-page/*.test.mjs` is untouched by this phase.

### Wave 0 Gaps
- [ ] Characterization tests that pin today's reply keys for `/captcha*`, `/setflood*`, `/delflood`, `/autolockdown`, `/unlockdown`, `/linkstaff`, `/unlinkstaff` before the refactor (several already exist, listed below).
- [ ] `TestSetFloodModeOnFreshChatStoredLimit` in `alita/db/antiflood` (settles A1).
- [ ] `settingsEnv` fixture in `alita/modules/settings_env_test.go`: lockdown env plus a Staff Group fixture (`linkLockdownStaffGroup`, `lockdown_alert_test.go:22`), per-user member records (`staffActionFake.setMember`), a press helper building `gotgbot.CallbackQuery{Message: gotgbot.Message{...}}` like `staffActionEnv.tapData` (`staff_action_fake_test.go:582`), and `lastAnswer`/`edits` accessors.
- [ ] `alita/i18n/settings_locale_test.go` (pattern: `lockdown_locale_test.go`, `captcha_web_locale_test.go`): key parity over `settings_*`, non-empty, identical placeholder sets (`staffPlaceholders`), per-key ceilings from the UI-SPEC table, alert keys ≤ 200 characters, `settings_help_msg` fits one message with the header (`lockdownHelpLength` approach, 4096 units), reused Staff texts keep the placeholders in the privacy table, no `<i>/<u>/<pre>/<a>` tags.
- [ ] No new DB guarantee (no migration, no partial index, no CHECK, no conditional update): nothing to add to `make test-postgres-integrity`.

**Existing tests that pin the refactored replies and must stay green:**
- Captcha: `TestCaptchaCommandTogglesAndDisplaysSettings`, `TestCaptchaCommandRejectsUnknownOption`, `TestCaptchaDisableFinalizesAttemptsAndRestoresUsers`, `TestCaptchaSubcommandsCreateSettingsWithDefaults`, `TestCaptchaSubcommandsRequireArgumentsOrShowCurrentSettings`, `TestCaptchaSubcommandsRejectInvalidValues`, `TestCaptchaCommandsPropagateGotgbotRequestErrors` (errors must still be returned), `TestCaptchaModeTurnstile`, `TestCaptchaModeTurnstileRefusals`, `TestCaptchaModeListsTurnstileInEveryLanguage`, `TestLoadCaptchaRegistersHelpAndHandlers` (`captcha_command_test.go`, `captcha_mode_turnstile_test.go`).
- Antiflood: `TestAntifloodCommandsUpdateSettingsAndDisplay`, `TestAntifloodCommandsHandleDisabledAndValidationBranches` (`antiflood_command_test.go:25`, `65`) and `alita/db/antiflood/repository_test.go`.
- Autolockdown: `TestAutoLockdown`, `TestAutolockdownAI`, `TestAutoLockdownArgs`, `TestAutoLockdownAuthority`, `TestAutoLockdownBadInput`, `TestAutoLockdownBasicGroup`, `TestAutoLockdownSaveFails` (replaces `autolockdownWrite`), `TestAutoLockdownReadsAreBounded` (replaces `raidSettingsLookup`).
- Lift: `TestLockdownLiftManualChange`, `TestLockdownLiftRestoreFailure`, `TestUnlockdownLiftsOnce`, `TestUnlockdownWithoutLockdown`, `TestLockdownLiftRecordFailure` (replaces `lockdownBeginLift`), `lockdown_alert_lift_test.go`, `TestLockdownAnonymousAdminProof`.
- Staff: `staff_panel_render_test.go` (pins `renderStaffRow`/`renderStaffPanel`/`composeStaffPanelText` output), `staff_unlink_button_test.go`, `staff_history_test.go`, and the link tests that call `runLinkGroup`/`resolveOwnStaffGroup`/`deliverLinkOutcome`.

### What only a live check can prove (to `09-SPIKE.md`, owner step)
URL button into `/start` for a user who never started the bot; Close on a private message older than 48 h; `show_alert` rendering on iOS/Android/Desktop; button truncation in hi and ru; a forwarded menu; a forum topic group; rapid stepper tapping and 429 behaviour; callback validity after a slow Staff build.

## Security Domain

### Applicable ASVS Categories (level 1, `security_enforcement` enabled, block on `high`)

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (Telegram identity is the session) | service identities refused (step 2) |
| V3 Session Management | no (stateless menu) | none stored |
| V4 Access Control | **yes** | live `getChatMember` per press; per-control right; owner of both groups; opener check; `c`/`u`/`s` never authorities |
| V5 Input Validation | **yes** | field allow-list per action, strict integer parsers, bounded ranges re-validated by the apply function, HTML escaping after translation |
| V6 Cryptography | no | none |
| V7 Error handling and logging | yes | Telegram error text goes to logs only; no token or ID of the Staff Group in group-visible text |
| V8 Data protection | yes | Staff Group title/ID masked in group menus (D-21) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged or replayed callback data (wrong `c`, `u`, `s`, `e`) | Spoofing / Tampering | structural rules, live check on the real chat, `s` matched against the presser's own list, expiry minute |
| Forwarded menu pressed in another chat | Spoofing | `c` only in private, `u` only outside private, authority re-read for the message's chat |
| Demoted or removed admin pressing a stale menu | Elevation of privilege | step 4 live check; apply functions are not reachable without it |
| Opener impersonation in a group menu | Spoofing | `From.Id == u` before any lookup |
| Information disclosure of the Staff Group to group members | Information disclosure | masked view, row-ID `s`, positional picker labels, privacy test over 7 locales |
| HTML/markup injection through titles, names, reasons | Tampering | escape after translation, caps (64 / 200 runes) |
| Double submit / race (two Lift taps, two admins) | Tampering | absolute target values, `BeginLift` and `DeleteLink` conditional writes |
| Lookup flood (one press = up to 2N+1 Telegram calls) | Denial of service | answer first, 45 s build bound, at most 8 groups per page; existing pacing for staff fan-out is not used because no fan-out happens |
| Deep-link payload injection | Tampering | strict `parseStaffChatIDArg`, payload never authorises, live admin check at `/start` |
| Privilege expansion between command and menu | Elevation of privilege | authority parity test per control (Pattern 4 table) |

## API Coverage

This phase integrates no external API or service. The Telegram Bot API methods used are all already used by the bot: `editMessageText`, `answerCallbackQuery`, `deleteMessage`, `getChatMember`, `getChat`, `sendMessage`, plus the `/start` deep-link payload and the `getChatAdministrators` calls inside `chat_status.CheckOwner`. Coverage gate: nothing to declare beyond this list.

## Docs and AGENTS.md updates (same change set)

- `AGENTS.md`:
  - **Callbacks:** the `settings|v1` namespace and its fields (`a`, `c`, `u`, `p`, `t`, `v`, `e`, `s`), the structural rules, that `s` is the `staff_groups.id` row ID never a chat ID, and the press pipeline order.
  - **Permissions:** authority per control (including the antiflood bot-admin rule), the menu's live check versus the commands' cached check, and that `/settings` reuses `requireLockdownAuthority(false)` with anonymous-admin re-entry.
  - **Handlers:** `Settings` module (priority 239), no new handler group, `RegisterDeepLinkHandler("settings_", ...)` and the strict payload rule.
  - **Subsystem traps:** the apply-function rule ("a command and the menu never diverge; a new setting adds one apply function"), no Redis state, D-21 masking, the public notice on a menu Lift, and the antiflood fresh-row side effect if the owner decides to keep it.
  - **Testing:** the harness list is unchanged; name the parity and drift tests.
- `docs/src/content/docs/api-reference/commands.md` and `permissions.md` (frozen pages, `MANUALLY MAINTAINED` marker): add the `/settings` row and the menu's per-control rights.
- `locales/config.yml`: `Settings: [settings]`; `locales/*.yml`: all new keys; `settings_help_msg` in all 7.
- `make generate-docs` then commit `docs/src/content/docs/commands/settings/index.md` and the regenerated `commands/index.mdx` counts.

## Sources

### Primary (HIGH confidence; files read this session)
- `/home/user/Fuku_Robot/.planning/phases/09-settings-menu/09-CONTEXT.md`, `09-UI-SPEC.md`; `.planning/REQUIREMENTS.md` (MENU-01..08); `.planning/ROADMAP.md`, `.planning/STATE.md`; `.planning/config.json`
- `/home/user/Fuku_Robot/.claude/CLAUDE.md`, `/home/user/Fuku_Robot/AGENTS.md`
- Go sources under `/home/user/Fuku_Robot/alita/`: `modules/{captcha,antiflood,autolockdown,lockdown,lockdown_alert_callbacks,lockdown_status,staff,staff_link,staff_unlink,staff_panel,help,deeplink_router,anonymous_admin_router,callback_codec,callback_context,i18n_helpers,registry}.go`, `modules/bot_updates.go` (118-228), `modules/moderation.go` (337-360), `db/captcha/repository.go`, `db/antiflood/{repository,optimized}.go`, `db/raiddetection/repository.go`, `db/staff/repository.go`, `db/lockdown/repository.go`, `db/cache/local.go`, `db/lang/repository.go`, `db/models/{antiflood,captcha,raid_detection,staff,lockdown}.go`, `utils/chat_status/{chat_status,access,owner}.go`, `utils/helpers/command_pipeline.go`, `utils/callbackcodec/callbackcodec.go`
- Test infrastructure: `alita/modules/{test_harness_test,lockdown_fake_test,staff_action_fake_test,staff_helpers_test,antiflood_command_test,captcha_command_test}.go`, `alita/i18n/{lockdown_locale_test,captcha_web_locale_test}.go`, `scripts/check_translations/main.go`, `scripts/generate_docs/parsers.go`
- gotgbot module source (`ext/context.go`, `gen_helpers.go`, `gen_types.go`, `custom_helpers.go`) in the Go module cache at `github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.36.0.20260919140833-240296efadb4`

### Secondary (MEDIUM confidence)
- Phase 8 research precedent: `.planning/phases/08-turnstile-web-captcha/08-RESEARCH.md` (section structure, docs section)

### Tertiary (LOW confidence; `[ASSUMED]`, no network this session)
- Telegram platform behaviour: forwarded inline keyboards, 48 h deletion limit, callback answer validity window, edit flood limits; GORM default-tag insert behaviour.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, nothing new is installed; every seam was opened and cited with line numbers.
- Architecture: HIGH for the apply/press/pages split; MEDIUM for the exact helper signatures (planner's discretion).
- Pitfalls: HIGH for 2-4 and 11 (code-derived); MEDIUM for platform behaviours (assumed).

**Research date:** 2026-10-30
**Valid until:** 2026-11-30 (the code under `alita/modules` changes with every phase; re-check line numbers if another phase lands first)
