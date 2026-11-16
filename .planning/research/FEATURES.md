# Feature Landscape

**Domain:** GroupHelp-style Telegram group management: PM settings hub, owner gate, message editor, member requirements, message deletion (milestone v1.1)
**Researched:** 2026-11-02
**Scope:** only the NEW v1.1 features. Existing behaviour (Phase 9 menu, captcha, lockdown, staff, greetings, rules, locks, filters, notes, warns, reports, log channels) is context, not re-researched.

## Evidence and confidence

| Source | What it gave | Confidence |
|--------|--------------|------------|
| Owner's 11 GroupHelp screenshots (PM list, main menu, rules/welcome editors, member requirements, deletion menu, service messages, scheduled deletion) | The exact UX the owner wants: layout, labels, which toggles exist | HIGH for what is visible; the meaning of two glyph buttons is inferred (see Open questions) |
| Local code: `greetings.go`, `rules.go`, `membership.go`, `settings_pages.go`, `models/greetings.go`, `models/rules.go`, AGENTS.md | What exists today and which seams the new features must use | HIGH |
| Local gotgbot v2.0.0-rc.36 source (`gen_types.go`, `gen_methods.go`) | Bot API field list for service messages, `DeleteMessage` limits, `DeleteMessages`, `ShowCaptionAboveMedia`, `ChatFullInfo.LinkedChatId`, `GetUserProfilePhotos`, `Community*` service types | HIGH (it is the Bot API text the code compiles against) |
| Search summaries of Rose docs (`/cleanservice`, `/cleancommand`, `/cleanwelcome`) | Rose's cleaning model: per-type `cleanservice`, `cleancommand user/admin/other/all` | MEDIUM (docs host was unreachable from this session; only search summaries) |
| Shieldy issue tracker via search | Shieldy deletes the entry (join) message of captcha failures, kicks on timeout | LOW (old, unofficial) |
| GroupHelp, Combot, Group Butler behaviour beyond the screenshots | Web search returned nothing usable; those bots' docs are not indexed | LOW. Statements about them below are marked as recall, not verified |

The `research-plan` seam could not run in this session (`gsd-tools.cjs` fails on a missing `./vendor/re2js.cjs`), and `core.telegram.org` and `missrose.org` are outside the session's network allow-list. Bot API facts therefore rest on the vendored gotgbot source, which is current. If the owner wants the missing GroupHelp detail (button grammar, what the glyph buttons do), a live look at the real bot is the only reliable source.

---

## 1. PM settings hub and discovery

### How it works in GroupHelp (screenshot 3, HIGH)

- `/settings` in the bot's private chat sends one message: "Group settings: choose the group whose settings you want to change", then one full-width button per group, in a single column, titled with the group's name. One button carries a corner-arrow glyph, which is a URL button (a link that opens the group), so the list is a mix of callback and URL buttons.
- The same message carries the troubleshooting text that doubles as the discovery contract: "If a group where you are an administrator is not shown: send `/reload` in the group and retry; send `/settings` in the group, then press Open in private."
- So discovery is by the bot's own memory, never by a Telegram query. Two feeders: `/reload` (refresh the admin list of this group) and the in-group `/settings` -> Open in private deep link. The Bot API cannot list a user's groups (confirmed in PROJECT.md context).
- Picking a group opens that group's main menu (screenshot 5): title "PARAMETERS", the group's name, "Choose one of the parameters", two-column grid of page buttons, the last row Lang / Close / Other. The same message is edited in place; every page has Back.

### Behaviour users expect

| Behaviour | Expectation | Confidence |
|-----------|-------------|------------|
| List shows only groups the bot is in and the user administers | A group the bot was removed from disappears; a user demoted disappears | HIGH (Phase 9's live check already enforces it per press) |
| Self-service repair for a missing group | One obvious action (`/reload` or `/settings` in the group) makes it appear | HIGH |
| Pagination | A user with 15+ groups still gets a working list; Telegram caps a keyboard at 100 buttons and the message at 4096 chars | HIGH |
| Group title only, no IDs | Titles are escaped; two groups with the same title are distinguishable (suffix with a short ID tail or member count) | MEDIUM |
| A fresh pick shows current state | The Main page header shows the group's name; every button shows a state suffix (Fuku already does this) | HIGH |

### Table stakes

| Feature | Why expected | Complexity | Depends on |
|---------|--------------|------------|------------|
| `/settings` in PM lists manageable groups, one button each, paged at about 8 | The headline feature; without it the owner still types commands in groups | Med | New persisted "who administers what" index (below); `settings_pages.go` renderer; `callbackcodec` |
| Persisted user-to-group admin index, fed by `/admincache` (add a `/reload` alias), every admin-cache refresh, `chat_member` promote/demote updates and the in-group Open in private path; pruned by `my_chat_member` when the bot leaves | The only discovery mechanism Telegram allows | High (new table, migration, repo with `cache.DeleteCache`, test fixtures, harness `AutoMigrate` lists) | `fuku/utils/cache/adminCache.go` (`loadAdminCacheFromTelegram` is the natural write hook), `chat_member` already in `AllowedUpdates` |
| Every pick re-checks live (`getChatMember`) and prunes the stale row on a definite "not an admin / bot not in chat" | The index is a hint, never an authority (existing Phase 9 rule D-04) | Low (the press pipeline step 4 exists; add the prune) | `settingsPress.verify`, `chat_status.IsBotMissingError` |
| Anonymous admins appear in their PM list | `getChatAdministrators` returns the real user ID even for an anonymous admin; in PM they are a normal user | Low | admin cache stores user IDs |
| Same Main page and Back/Close behaviour as the in-group menu | Pages are reused; only the entry differs | Low | `settingsMainEntries`, `registerSettingsPage` |
| Empty-state text when the list is empty ("Send /reload in your group, or /settings there") | Prevents the dead-end the owner will otherwise hit on day one | Low | locale keys x7 |

### Differentiators

| Feature | Value | Complexity | Notes |
|---------|-------|------------|-------|
| State badge on each list row (lockdown lock icon, Staff Group linked, bot-not-admin warning) | Owner sees at a glance which of many groups need attention | Med | `lockdown.ListActiveByChatsFresh` exists (one query for many chats); bot-admin state needs a cheap cached check. Never name the Staff Group (D-21) |
| Recently-opened first ordering | Owner mostly toggles the same three groups | Low | needs a `last_opened_at` column on the index |
| Bounded live refresh of visible rows (max 8 `getChatMember` calls per page, 60 s Redis memo) | Removes stale rows before the user taps | Med | Optional; the live check on pick already makes stale rows harmless |

### Anti-features

| Anti-feature | Why avoid | Instead |
|--------------|-----------|---------|
| Live-checking every group on every list open | N groups x Telegram calls per `/settings`; rate limits and slow list | Index plus live check on pick |
| Listing groups from the `connections` table | Connections are a different relationship (a user chose to connect), not "administers" | Keep connections as is; build the index separately. A pick in the hub does not need to write a connection |
| Showing a group to a non-owner admin as a read-only entry | Owner decided hiding beats read-only | Hide it (section 2) |

---

## 2. Owner gate

### How it works (owner decision plus common practice)

GroupHelp and Rose let any admin (Rose: any admin with the relevant right) change settings. An owner-only default with an opt-in "let admins in" switch is the owner's own policy, not a copied GroupHelp behaviour, so there is no reference behaviour to match: this is a Fuku design. The closest precedent in the codebase is Phase 9's `settingsView.Creator` and `requireLockdownAuthority` (live creator check) and the staff link's "live owner of both groups" rule.

### Behaviour that must hold

| Surface | Owner | Admin, toggle off (default) | Admin, toggle on | Non-admin |
|---------|-------|-----------------------------|------------------|-----------|
| PM `/settings` list | listed | not listed | listed | not listed |
| In-group `/settings` | menu choice (group / private) | refused with one plain line, no menu | as today | as today (refused) |
| Deep link `/start settings_<id>` | opens | `settings_start_not_admin` (the same words a plain member gets) | opens | `settings_start_not_admin` |
| Every callback press | allowed | refused (alert), including a menu opened before the owner flipped the switch | allowed | refused |
| The toggle itself | changeable by owner only, always | cannot see the control | cannot change it even when on | n/a |
| In-group setting commands (`/captchamode`, `/setflood`, ...) | unchanged | unchanged | unchanged | unchanged |

Rules that follow from it:

- The gate is decided from the press's own live `getChatMember` record (creator vs administrator), never the admin cache, exactly like Phase 9 step 4. The toggle read must be fresh or `skipLocal`-listed so another replica never serves a stale "admins allowed" from the in-process layer (AGENTS.md data rule).
- A refusal in the PM must not reveal whether the bot is in a group (existing `settings_start_not_admin` reasoning). Refusing an admin of a group they can already see in-group may say "only the owner" because they already know the group exists and that they are an admin there.
- The toggle row lives on its own small page (Access) visible only to the creator; other admins never see the button.

### Table stakes

| Feature | Why expected | Complexity | Depends on |
|---------|--------------|------------|------------|
| Per-group `admins_may_configure` flag, default false, one apply function, `cache.DeleteCache` on write | Owner's explicit requirement | Low-Med (migration, model, repo, cache key in `skipLocal`) | Phase 9 "one setting one code path" rule |
| Gate applied in all three entries (PM list filter, in-group `/settings`, deep link) and in the press pipeline | A gate enforced in only some places is no gate | Med | `settingsPress.verify`, `settingsStartDeepLink`, `requireLockdownAuthority(false)` path, the `settings|` callback |
| `settingsRight` gains an owner dimension, so per-control rights stack on top of the gate | Controls keep their own right (restrict, bot admin) | Low | `settingsRight*` constants |
| Parity/drift/authority tests for the gate | Repo convention for every setting | Med | `settings_env_test.go` toolkit |

### Edge cases the requirements must decide (flagged, not resolved)

- **No live creator.** Telegram can have a group with no owner (the owner deleted their account or left; gotgbot has `ChatOwnerLeft` and `ChatOwnerChanged` service types). Default-deny then locks everyone out. Suggested rule: when `getChatAdministrators` shows no creator, admins are treated as allowed, still checked live. MEDIUM confidence that this case occurs; the fallback is a design call for the owner.
- **Ownership transfer.** The new owner appears only after an admin-cache refresh or an in-group `/settings`. A live check on pick covers it; the index must also accept a creator row appearing for a user who was an admin before.
- **Staff Group.** Its own page is read-only for its members; the gate does not hide a Staff Group from its creator, and nobody but the creator sees the linked groups.

### Differentiators

| Feature | Value | Complexity | Notes |
|---------|-------|------------|-------|
| Audit line in the group's log channel when the owner flips the switch | Admins learn why access changed | Low | `actionlog.Destination`, admin category |

### Anti-features

| Anti-feature | Why avoid | Instead |
|--------------|-----------|---------|
| Per-page or per-setting owner/admin granularity | Owner asked for one switch; a matrix is a support burden | One group-level switch |
| Making in-group commands owner-only too | Out of scope by the owner's statement; would break existing moderators | Leave commands alone |

---

## 3. Message editor (Rules, Welcome, Goodbye)

### How it works in GroupHelp (screenshots 2, 6, 7, 8, HIGH)

One editor screen per message (Rules, Welcome, Goodbye share the shape):

```
Rules
Text            [done]
Media           [done]
URL buttons     [done]
Use the buttons below to choose what to set
[ Text ]          [ Preview ]
[ Media ]         [ Preview ]
[ URL buttons ]   [ Preview ]
[ Media under text: off ]
[ Full version (preview) ]
[ Back ]
```

- The three checkmark lines show which parts are set. Each part has a set button and its own preview button, and "Full version" previews the whole message the way members will see it.
- "Media under text" is a toggle that places the media below the text. In Telegram terms this is `show_caption_above_media` (gotgbot `ShowCaptionAboveMedia` exists on `SendPhoto/Video/Animation` options) for captioned media, and the link-preview "above text" placement for a media link. It does not apply to documents, audio, voice or stickers.
- The Rules menu sits one level above: "Group rules, shown with /rules", [Set message], [Allowed commands], [Back]. The "Allowed commands" button belongs to GroupHelp's per-command permission feature and is out of scope here.
- The Welcome menu (screenshot 6): Status Active/Off, Mode "send welcome at every join", [Turn off][Turn on], [Set message], [Always send][Greet once], [Delete last message: on/off], [Back]. A "Choose topic" button exists in newer builds (screenshot 8); the owner already ruled it out of v1.1.
- Input flow (recall of how all bots in this class behave, MEDIUM; the screenshots show only the menu): tapping Text, Media or URL buttons replaces the menu with a prompt ("send the new text, or press Cancel"); the user's next message in the PM is consumed as the value; the menu is shown again with the checkmark updated. Preview buttons send a real rendered copy into the PM.

### What users expect from each part

| Part | Expectation | Telegram constraint |
|------|-------------|---------------------|
| Text | Formatting typed in Telegram (bold, links, spoilers, custom emoji) survives; placeholders such as the member's name and the group title work; "remove" is possible | Capture entities from the message, not raw text. Fuku stores HTML today (`tgmd2html.MD2HTMLV2(OriginalMDV2)`). Text length limit is 4096 characters, but 1024 once media is attached because it becomes the caption: the editor must refuse or warn at save, not fail at send time |
| Media | Send a photo, video, GIF, sticker, document, audio or voice; the bot keeps its `file_id`; one media only; replacing and removing both work | `file_id` is per bot, fine here. Stickers and video notes cannot carry a caption: text must then go in a separate message or be refused (decision needed) |
| URL buttons | A simple grammar, multiple per row, several rows; invalid URLs are rejected with a reason | Only `http(s)://`, `tg://` and `t.me` links are valid; no callback buttons from a user; Telegram answers 400 for a bad URL when the message is sent, so the preview send is also the validation |
| Preview per part | Shows that part alone (text only, media only, buttons only) | Buttons-only preview needs a placeholder text because a message cannot be only a keyboard |
| Full preview | The real rendered message with the opener's name filled in, sent as a new PM message | Same send path as production (`media.SendGreeting`, `FormattingReplacer`) so the preview cannot differ from the real send |
| Media under text | A toggle | Only for photo, video, animation; hide or disable the toggle for other media types |
| Cancel | Cancel button on the prompt, plus `/cancel`; any other menu tap also clears the waiting state | State must expire (about 10 minutes) so a user is not trapped |

### The input state

A "waiting for your message" state is required and is new infrastructure (PROJECT.md says free-text editing was deferred in v1.0 as MENU2-02).

- Key by `(user, private chat)`, value holds group ID, field (`text`, `media`, `buttons`, `filter_trigger`, ...), the menu message ID to edit, and an expiry. Redis, short TTL, `SET` overwrite so a new prompt replaces the old one; outside the `fuku:cache:` prefix like the other operational keys.
- It must work across replicas, so it cannot be in memory. Without Redis the editor should say plainly that editing needs Redis; the stateless pages keep working.
- A watcher at a new private-chat-only handler position consumes the next message when a state exists and returns `ext.EndGroups`; with no state it returns `ext.ContinueGroups` and does nothing. Slash-prefixed text (other than `/cancel`) cancels and passes through so commands still work. The group number must be added to the AGENTS.md handler list; it must run before any PM handler that could swallow a free message (greetings/notes/filters do not act in PM, but connections might).
- The press pipeline's live admin check runs at the prompt and again when the message is saved: an admin demoted between the two must be refused.
- Albums arrive as several updates; accept the first item and ignore the rest (Fuku already handles media groups elsewhere, `g:<media group id>`).
- The user's reply must never be echoed back with HTML unescaped; the saved text is rendered by the same sanitising path as today.

### Table stakes

| Feature | Why expected | Complexity | Depends on |
|---------|--------------|------------|------------|
| Rules/Welcome/Goodbye pages with the three-part editor and checkmarks | The headline of feature 3 | High | Input-state infrastructure; new pages via `registerSettingsPage` |
| Text edit with entity capture, length checks (4096 / 1024 with media) | Core | Med | `formatting.FormattingReplacer`, `tgmd2html` |
| One media edit and remove | Core | Med | `media.SendGreeting` already sends the types Welcome supports; Rules needs it added |
| URL buttons edit and remove with validation | Core | Med | Existing welcome button storage (`ButtonArray`) and `keyboard.BuildKeyboard`; Rules needs the same |
| Per-part preview and Full preview | The owner named it; it is also the only safe way to catch a bad URL or a too-long caption | Med | Same send path as production |
| "Media under text" toggle | In the screenshots and the owner's list | Low-Med | `ShowCaptionAboveMedia`; storage column per message |
| Cancel and expiry | A prompt with no exit is a trap | Low | input state |
| Welcome: "Greet once" vs "Always send" | In the owner's list. Meaning: a member who leaves and re-joins is not greeted again | Med-High | Needs a persistent "already greeted" record. Recommend a table with `INSERT ... ON CONFLICT DO NOTHING` and `RowsAffected == 1` deciding, so two replicas never both greet; a Redis-only set would re-greet after a flush. Growth is one row per member per group; acceptable, document it |
| Welcome: Delete last welcome | In the owner's list | Low | Already exists as `clean_old` / `CleanWelcome` (`SetCleanWelcomeSetting`); the page only exposes it through the same apply function |
| Goodbye gets the same editor | In the owner's list | Low once Welcome exists | `GoodbyeSettings` has the same columns |
| Rules gain media and buttons, with a migration, and `/rules` plus the `start rules_<id>` deep link render them | Owner's list; today Rules are text only (`RulesSettings`: rules, rules_btn, private) | Med | New columns (file ID, type, buttons JSON, media-under-text), `sendRules` and `rulesDeepLinkHandler` both change; the `rules` row is part of backup/export so the legacy backup module needs the new fields |
| Enable/disable switch and status text on Welcome and Goodbye (exists as `/welcome on|off`) | Visible in screenshot 6 | Low | `greetingToggle` apply function |
| Every save goes through one apply function that the command also calls (`/setwelcome`, `/setrules`) | Phase 9 rule: one setting, one code path | Med | The existing `set*` commands must be refactored to call it |

### Differentiators

| Feature | Value | Complexity | Notes |
|---------|-------|------------|-------|
| Reuse the same editor for filter replies and notes ("add by typing") | One component serves feature 3 and feature 6; avoids a second input flow | Med | Design the editor as "a message slot" (text, media, buttons, flag) from the start; filters and notes already store type, file ID and buttons |
| Auto-delete the previous preview message when a new one is sent or the menu closes | A PM with 12 stale previews is noisy | Low-Med | Track the preview message ID in the same Redis state |
| "Send test to me" using the real placeholders (the opener's name, the group's member count) | Confidence before publishing | Low | Same as Full preview |
| Remove-this-part buttons with a Yes tap | Avoids an accidental wipe of a long text | Low | `settings_card.go` two-tap card |

### Anti-features

| Anti-feature | Why avoid | Instead |
|--------------|-----------|---------|
| A draft / publish step | GroupHelp saves immediately; a draft doubles state and the "which version is live" question | Save on receipt; the preview shows what is saved |
| Multiple media or albums in one welcome | Owner asked for one media; albums complicate captions and deletion | One media |
| Storing media bytes or re-uploading | `file_id` is enough and cheaper | Keep `file_id` and type |
| A new templating language | Existing placeholders (`FormattingReplacer`) already work and members' texts are migrated | Keep them; list the supported ones in the prompt text |
| Topic (forum thread) picker for the welcome | Owner ruled it out for v1.1 | Welcome goes where the join happened (already `threadID`) |
| Letting arbitrary members open the editor from the in-group `/rules` | Editing belongs behind the gate | Menu only |

---

## 4. Member requirements and name blocks

### How it works in GroupHelp (screenshots 4 and 9, HIGH)

Two screens:

1. Summary screen: "REQUIREMENTS FOR..." with five lines, each "name: current action" (Last name: Off; Username: Mute; Profile photo: Off; Channel subscription: Off; Must add members: Off). "BLOCKS..." with four lines (Arabic name, Chinese name, Russian name, Spam name: each Off or an action). Then two shared toggles with their own explanation and status:
   - "Check on join. If on, the bot checks requirements and blocks even when users join, and when they send a message." Status Active.
   - "Delete messages. If on, the bot deletes messages of users who do not meet the Required conditions or are subject to a block." Status Active.
   Top buttons switch between [REQUIREMENTS] and [NAME BLOCKS].
2. Per-requirement screen (screenshot 4): header shows the current state, a blue selected tab, then the rule being edited ("Required last name") and a punishment row: [Off][Warn][Kick] / [Kick (?)][Mute][Ban] (the selected one is green, the others grey), then the other rules as buttons (Username, Profile photo, Must add, Channel subscription), then Back. The punishment set is exactly the owner's: off, warn, kick, ban, mute (plus the "delete" the second toggle covers).

"Must add members" is out of scope.

### When checks run (recall from how GroupHelp-class bots behave, MEDIUM, plus constraints from this codebase)

- **On join** (only if "check on join" is on): from the `chat_member` update and from the `new_chat_members` service message. They are two deliveries of one join; use the existing `claimRecentJoinProcessing` dedupe (the lockdown guard dedupes by its own row instead).
- **On every message** from a member, always, regardless of the join toggle? The screenshot text says the toggle adds join-time checking "even when joining, and when sending a message", so message-time checking is the base behaviour and the join check is the extra. Fuku should do the same: message-time is always on when a rule has an action; "check on join" adds the join-time pass.
- **On name change**: caught at the next message (the message carries the current name) or for the AI spam-name rule through the users tracker noticing a changed name.

### Per-rule behaviour users expect

| Rule | What is checked | Data source | Cost per check | Known false results |
|------|-----------------|-------------|----------------|--------------------|
| Last name | `last_name` empty | `message.From` / `chat_member.new_chat_member.user` | free | none |
| Username | `username` empty | same | free | none; a user with only collectible usernames is fine |
| Profile photo | member has at least one photo | `getUserProfilePhotos(user, limit 1)` | one API call; must be cached | Privacy: a user who hides their photo from "everyone" looks photo-less to the bot (PROJECT.md). The notice text should say so; this is an accepted false positive |
| Channel subscription | member of the group's linked channel | `getChat(group).LinkedChatId` (verified in gotgbot `ChatFullInfo`) then `getChatMember(channel, user)` | one or two API calls; cache | The bot must be an administrator of the channel to read members; if not, the check cannot run and must fail open with a visible status line on the page ("bot is not an admin of the linked channel"). A group with no linked channel: the rule cannot be turned on, show why |
| Arabic / Chinese / Russian name | script of first and last name | pure Go, `unicode.Arabic`, `unicode.Han`, `unicode.Cyrillic` range tables | free | See "Name blocks" |
| AI spam name | a spam-looking nickname | Claude Haiku, name only | one provider call; cache by name hash | See "Name blocks" |

Message-time checks run on every message of a large group, so the design must keep a compliant member's message to roughly one cache hit: a short-lived "passed" memo per `(chat, user)` (a couple of minutes) in Redis or the local layer, invalidated by a per-chat settings generation so a rule change takes effect at once. Photo and channel lookups get their own result cache (longer positive TTL, short negative TTL so "I fixed it" is not fooled by a stale "no photo").

### Exemptions (who is never checked)

Mirror `aispamCarvedOut` and the raid sender filter, the existing patterns:

- Administrators and the creator (cached admin list on the message hot path; a live check only on the punishing path), approved users (`IsApprovedForUpdate`, the repo rule that approved users skip captcha, antiflood, locks, blacklists), the bot, other bots, Telegram service IDs (`staffServiceUserIDs`), senders with `sender_chat` (anonymous admins, a channel's automatic forward in a comment group: these are the most important exemption for the channel-subscription rule, otherwise the channel's own posts are "unsubscribed").
- Members with a pending captcha (the captcha already mutes and holds their messages; do not stack a second punishment) and members in a lockdown (the join guard at group `-7` ends the update first, so a join-time check is never reached; message-time checks of a muted lockdown member are moot).
- Edited messages and service messages are not message-time checks.

### Punishments and the notice

| Action | Behaviour | Notes |
|--------|-----------|-------|
| Off | No check | Default for every rule |
| Warn | Adds one warning through the existing warn path, which itself applies the group's warn-limit action | **Pitfall:** checking every message would warn on every message. Apply a punishment once per violation episode (Redis latch per `(chat, user, rule)`, cleared when the member passes or the latch expires) and let "delete messages" handle the rest |
| Kick | Remove the member so they can rejoin | One `unbanChatMember` with `only_if_banned=false` on a current member; the member can rejoin and is checked again at join. Reuse the staff `decideStaffAction` idea: the call depends on the live status, never lift a ban by accident |
| Ban | Permanent until an admin acts | Waits for an admin; no "I fixed it" button (a banned member cannot see the group) |
| Mute | Restrict until the requirement is met; shows the "I fixed it" button | Must not replace an existing admin mute or captcha mute (see below) |

**The notice** (recall of the common shape, MEDIUM): one message in the group that mentions the member, names what is missing ("you need a username and a profile photo"), says what happens, and for a mute carries one callback button "I fixed it". It should be:

- sent once per member per window (the same latch as the punishment), not once per message;
- auto-deleted after the group's "punishments" delay from section 5;
- localised per group language, and escaped (names are attacker-controlled);
- optional per group ("notice in the group" on/off), because some owners prefer silent enforcement.

**"I fixed it" verification:**

1. Callback data carries only numbers or a token (`callbackcodec`, 64 bytes): chat ID and user ID fit; no names.
2. Only the muted member may press. Anyone else gets an alert "not for you" and nothing changes (the same pattern as the Phase 9 "not your menu" and the captcha button). A restricted member can still tap inline buttons, so the mute does not block the press.
3. The handler re-checks every requirement that applies to that user now, live, without trusting the cache: re-reads the member's current name/username from a fresh `getChatMember`, then a fresh photo and channel lookup. If anything is still missing the answer is an alert naming what is missing (not an edit), the member stays muted, and a short cooldown (a few seconds, Redis) stops tap spam against the API.
4. If everything passes it unmutes only when all of these hold: the bot's own requirement-mute record exists for that user, the live status is still restricted, no captcha attempt is pending, no lockdown is active for the group (an active lockdown mutes everyone with default permissions; an unmute during it must use the stored pre-lockdown permissions through `resolveUnmutePermissions`, the one choke point), and the user was not muted by an admin or staff action afterwards. Then edit or delete the notice.
5. The press is answered exactly once; failure to unmute (missing rights) is reported honestly and keeps the record so a later press retries.

A bot-owned record of "muted by requirements" (one row per member and group, with the rule names) is therefore needed; without it the button cannot distinguish its own mute from an admin's, and it would be an unmute-anyone button. This is the main complexity driver of the whole feature.

### Interaction with captcha and lockdown

| Situation | Recommended behaviour | Why |
|-----------|----------------------|-----|
| Captcha enabled and the member just joined | Run the join-time requirement check after the captcha pass, from the pass path (`finishCaptchaPass`), not in parallel with the challenge | Both mute the joiner; whichever unmutes first would lift the other. `ProcessSingleJoin` in `membership.go` is the seam: Challenge returns before Welcome |
| Captcha off | Run the check in `ProcessSingleJoin` before the welcome | A member who is kicked or banned should not be greeted |
| Failed requirement on join, action mute | Welcome is held back until "I fixed it" passes, then sent | A greeting for a muted, non-compliant member is noise; this is a decision for the owner |
| Message-time check on a member who passed captcha but fails a rule | Apply the rule's punishment | Safety net for the join path |
| Lockdown active | No join-time check (the guard ends the update); "I fixed it" unmute goes through `resolveUnmutePermissions` | AGENTS.md: every unmute path goes through it |
| A captcha pass unmutes a member who is under a requirement mute | Must not happen silently: on a pass, if a requirement-mute record exists, keep them muted | Otherwise captcha pass bypasses the requirement until their next message |
| Admin's `/mute` or staff `/mute` on someone | The requirement system never overwrites a mute that is not its own | Use the live-status decision table idea from `decideStaffAction` |
| Rejoin after kick | Checked again at join | Owner's decision |
| Existing members when a rule is switched on | Not mass-processed; checked on their next message | Mass processing would be a bulk moderation job; the owner excluded mass tools |

### Name blocks: language checks

| Block | Detection | False positives to disclose | Confidence |
|-------|-----------|-----------------------------|-----------|
| Arabic name | Any letter in `unicode.Arabic` in first/last name | Also hits Persian and Urdu names, which use the same script | HIGH (stdlib) |
| Chinese name | Any `unicode.Han` letter | Also hits Japanese names written in kanji and Korean hanja. Treat "Han and no kana/hangul" as the narrower variant if Japanese members matter | HIGH (stdlib) |
| Russian name | Cyrillic text containing one of the letters ы ъ э ё (either case) and none of the Ukrainian-only letters і ї є ґ | Belarusian, Kazakh, Kyrgyz and other Cyrillic languages also use ы, э, ё; ъ appears in Bulgarian. A Ukrainian name with no distinctive letter is invisible to the check, and so is a Russian name without those letters. The setting is a heuristic and the page must say so | HIGH that this is the only reliable script-level signal (matches PROJECT.md); MEDIUM on recall of false-positive languages |
| Spam name | Claude Haiku (the existing `fuku/utils/claude` client and `ANTHROPIC_API_KEY`), sending only the name | See below | MEDIUM |

Scope of the text checked: first name plus last name; the owner may decide whether the username counts. Normalise with NFKC and strip zero-width and bidi control characters before checking, otherwise a trivial insertion defeats script detection (and many spam names use lookalike letters, which is also what the AI check is for).

**AI spam-name check** (new, but with strong precedent in this codebase from the raid image path):

- Trigger: at join and when a name changes, never per message. Key the verdict by a hash of the normalised name; cache it in Redis (the verdict depends on the name, not the group, so one cache serves all groups). A repeat of a known spam name costs nothing.
- Guards to copy from `raid_ai_image.go`: kill switch `ENABLE_AISPAM`, no key means off with a visible "not set up" line on the page (like `lockdown_auto_ai_unavailable`), an hourly budget per group and global, a provider breaker, a shared or separate bounded pool, a request with a forced strict tool returning one number, no sampling parameters, derived from the cancellable AI context, drained at shutdown. Fail open: no verdict means no punishment.
- It must not run inline on the join handler (a provider call blocks the update); enqueue it and apply the rule's punishment from the worker, which needs the same live-status decision as every other punishment. A member who is already gone by then is a no-op.
- A named false-positive risk: legitimate names in other scripts or with emoji can score high. Use a high bar (a code constant, like `raidAIImageBar` 0.85) and recommend warn or mute rather than ban as the default choice in the page text, though all five actions stay selectable because the owner asked for them.
- Privacy statement for the page: only the name leaves the server.
- No test may call the real provider (AGENTS.md); `httptest` servers only.

### Table stakes

| Feature | Why expected | Complexity | Depends on |
|---------|--------------|------------|------------|
| Summary page with the current action of each rule, plus the two shared toggles (check on join, delete messages) | Screenshots 4 and 9 | Med | `registerSettingsPage`; new settings table with one row per chat |
| Per-rule punishment picker: off, warn, kick, mute, ban | Owner's list | Med | One apply function per rule |
| Last name, username, profile photo checks | Core of the feature | Med (photo needs caching) | `getUserProfilePhotos`, Redis memo |
| Linked-channel subscription | Owner's list | Med-High | `getChat.LinkedChatId`, `getChatMember(channel)`, a visible "bot not admin of the channel" status |
| Arabic, Chinese, Russian name blocks | Owner's list | Low | stdlib `unicode` tables; unit tests over a name table |
| Notice in the group, once per episode, auto-deleted | Users must know why they were muted | Med | Latch key, scheduled deletion (section 5) |
| "I fixed it" for mutes with a live re-check and ownership of the mute | Owner's list | High | Requirement-mute record, `resolveUnmutePermissions`, captcha pending check, lockdown check |
| Delete messages toggle | Owner's list | Low-Med | Bot needs `can_delete_messages`; report in the page if not |
| Exemptions (admins, approved, bots, service IDs, `sender_chat`, pending captcha) | Without them the bot mutes its own admins and the channel | Med | `IsApprovedForUpdate`, admin cache, captcha pending flag |
| Kick lets the member rejoin and be checked again; ban waits for an admin; warn only warns | Owner's list | Low | existing kick/ban/warn paths |
| Everything inert and honest when the bot lacks the needed right | A silent no-op is worse than a refusal | Low | `chat_status` live bot checks |

### Differentiators

| Feature | Value | Complexity | Notes |
|---------|-------|------------|-------|
| AI spam-name check (Haiku) | Catches lookalike and promo nicknames that script checks never will | High | Reuses `fuku/utils/claude`; new name-only instruction; budgets and breaker as above |
| Custom notice text per group | Groups have their own tone | Med | Reuse the message editor slot |
| Per-rule status line when a rule cannot work (no linked channel, bot not channel admin, no key) | Prevents a rule that looks on but does nothing | Low | |
| Count of members currently muted by requirements shown on the page | Lets the owner see leftover mutes | Low | The requirement-mute record gives it |

### Anti-features

| Anti-feature | Why avoid | Instead |
|--------------|-----------|---------|
| "Must add members" | Out of scope (owner) | Omit |
| Lifting a ban or kick through "I fixed it" | A banned member cannot see the group's button; a kicked one rejoins | Only mutes get the button |
| Mass re-check of all existing members when a rule is enabled | A bulk moderation job; Bot API cannot list members | Check on next message |
| Sending message text to an AI | Out of scope in PROJECT.md | Name only |
| Treating a hidden profile photo as a certain violation | Telegram hides it from bots; users cannot fix it from their side if privacy blocks it | Tell the user in the notice how to allow the bot to see it (set photo visible) or accept the false positive |
| Banning on an AI verdict by default | A wrong ban is not undoable by "I fixed it" | Default off; recommend warn or mute |
| Per-rule custom thresholds or scoring | Over-engineering for one owner | One action per rule |

---

## 5. Message deletion

### How it works in GroupHelp (screenshots 1, 10, 11, HIGH for the menus; semantics of two glyph buttons are inferred)

The deletion menu (screenshot 1): Commands, Global silence, Edited messages, Service messages, Deletion time, Mass deletion, Delete all messages, Self-destruct of messages, Back. Out of scope for v1.1: Global silence, Mass deletion, Delete all messages.

**Service messages** (screenshot 11): per type, a status line ("Joined: delete immediately", "Left: delete immediately", "Photo changed", "Title changed" ... off for the last two) and a three-row control block per type: label | off | on, then a stepper row ( ➥ , -1 , (0) , +1 ). The `(0)` is the current delay in seconds (0 means immediately); -1 and +1 move it. The first glyph button is not explained by the screenshot (see Open questions). Types shown: Joined, Left, Photos, Title (named "description" in the status text: a mismatch in GroupHelp's own labels), Pinned message, Topics, Boosts, Voice chat invitations, Checklist, Community.

**Scheduled deletion of the bot's own messages** (screenshot 10): header "Scheduled deletion: choose which messages you want deleted a while after they were sent, and the period." A "Current state" list: Welcome: in 10 minutes, Goodbye: off, Rules: in 10 minutes, Own commands: off, Punishments: in 2 minutes, Manual punishments: in 5 minutes. Per kind: label | on/off, then a stepper row ( ➥ , -5 , -1 , +1 , +5 ) in minutes.

Mapping to this milestone's owner list: Welcome, Goodbye, Rules (welcome/goodbye/rules), Punishments (automatic punishments: requirement and AI actions, captcha failures, antiflood), Manual punishments (replies to `/ban`, `/mute`, `/warn` by admins). "Own commands" appears in the screenshot but not in the owner's text; treat it as "the bot's replies to commands" and ask the owner (Open questions).

### Telegram facts that shape every item (HIGH, from gotgbot source)

- `DeleteMessage`: only messages sent less than 48 hours ago; service messages are deletable except supergroup/channel/forum-topic creation; in a group the bot needs `can_delete_messages` for others' messages; it can always delete its own. `DeleteMessages` deletes up to 100 messages of one chat in one call.
- Service message fields the Bot API exposes today (these are the deletable types): `new_chat_members`, `left_chat_member`, `new_chat_title`, `new_chat_photo`, `delete_chat_photo`, `pinned_message`, `forum_topic_created/edited/closed/reopened`, `general_forum_topic_hidden/unhidden`, `video_chat_scheduled/started/ended/participants_invited`, `boost_added`, `checklist_tasks_done/added`, `message_auto_delete_timer_changed`, `chat_background_set`, `community_chat_added/joined/removed`. The Community types exist in the vendored Bot API, which resolves what GroupHelp's "Community" row means (the Community service messages, not a rename of join).
- The `allowed_updates` list already contains `message`, `edited_message` and `chat_member`; service messages arrive as ordinary `message` updates.
- Because deletion cannot reach past 48 hours, every delay must be capped below that (suggest at most 24 hours for service messages, 47 hours absolute for bot messages and self-destruct, shown in the UI as a bound; GroupHelp's own steppers are minutes).

### Per feature: expected behaviour

| Feature | Expectation | Notes and pitfalls |
|---------|-------------|--------------------|
| Service message deletion per type, immediate or after N seconds | Immediate is the default when on. After N seconds the message stays visible briefly (useful for the "X pinned a message" notice) | Immediate: delete inline in the handler. Delayed: must be durable, see below. `pinned_message` service: deleting the notice does not unpin |
| Overlap with existing `cleanservice` | Today `/cleanservice` deletes join and leave notices at once (`ShouldCleanService`, `cleanService` in `greetings.go`) | The new page must read and write the same field for join and leave, or `/cleanservice` and the page drift apart. Join and leave keep their current column; new types get new columns. `cleanService` also drives join processing (the welcome and captcha path reads `NewChatMembers` there), so the deletion refactor must not skip that path |
| Bot messages deleted after N minutes (welcome, goodbye, rules, automatic punishments, manual punishments) | The notice vanishes on its own; a rule reply after 10 minutes; the punishment line after 2 | Every site that sends one of those kinds must schedule its own message; this touches `SendWelcomeMessage`, `leftMember`, `sendRules`, the new requirement notice, captcha failure notices, warn/ban/mute replies, antiflood replies. Add one helper, `scheduleBotMessageDelete(kind, chat, msg)`, so each site is one line |
| Interplay with "Delete last welcome" (`clean_old`) | Both can be on: the newest welcome stays until its timer or the next welcome | The delayed delete finding a message already gone is success (`DeleteMessageWithErrorHandling` already treats "not found" as non-error) |
| Members' commands | A member's `/rules`, `/id`, `/notes` message is deleted after the bot answers | Rose has per-class choices: `cleancommand user`, `admin`, `other`, `all` (MEDIUM). Recommend exactly two choices at first: members' commands and admins' commands, because a deleted admin command hides the audit trail from other admins. Do not delete before the handler ran: delete after the command handler returns, not at group `-10`. Needs `can_delete_messages`; exempt nothing else |
| Edited messages | The bot deletes a message when its author edits it (stops post-then-edit-to-link). Admins and approved users are exempt | Needs a handler for the `edited_message` update, which no module handles today (`edited_message` is in `AllowedUpdates` but the raid flood watchers note gotgbot drops edited messages for ordinary handlers: a handler must be registered with the edited-message filter). Telegram can also deliver an `edited_message` when only a link preview loads or a media caption is touched, so not every edit is an author's rewrite. Options to avoid false deletes: ignore edits within a short grace window after sending (typo fixes) as a differentiator, and ignore edits whose text and entities are unchanged. The bot cannot see the old text, so "unchanged" needs a stored hash per message, which is expensive; recommend the grace window plus an age cutoff instead |
| Self-destruct of every member message after N minutes | A "this group is ephemeral" mode | One scheduled-deletion row per message from every non-exempt member: the highest-volume write in the whole milestone. Needs batching and `DeleteMessages` (100 per call), pacing across replicas (a fleet-wide pacer like `staffPaced` and `lockdownPaced`), and the 48-hour cap. Admins and approved users should be exempt, and the bot's own messages follow the other rules |

### Durable scheduling (the structural dependency)

Existing code uses `time.AfterFunc` for small things (`misc.go`); that loses everything on a restart and is wrong for 10-minute and multi-hour delays on several replicas. Scheduled deletions need:

- A PostgreSQL table `scheduled_deletions (chat_id, message_id, delete_at, kind, claimed_at, attempts)`, unique on `(chat_id, message_id)`, indexed on `delete_at`. Same approach as the lockdown worker: a DB-driven loop on every replica that claims rows with conditional updates, so two replicas never double-delete, a restart resumes, and Redis loss changes nothing (a Redis sorted set would lose pending deletions on a flush; for the self-destruct feature that means messages that never disappear).
- Insert on send for bot messages (cheap, low volume). For self-destruct, insert per member message; consider batching inserts and a retention rule (delete the row once handled).
- A worker that groups due rows by chat, calls `DeleteMessages` for up to 100 ids, treats "message not found" and "can't be deleted" as done, backs off on 429 through a fleet-wide pacer, gives up after N attempts (bot lost the right) and logs once per chat.
- A shutdown drain registered after DB-close (AGENTS.md rule); the delete is idempotent, so a cut-off cycle loses nothing.
- Rows older than 48 hours cannot be deleted: expire them rather than retrying forever.
- Backup/export/import/reset must leave the table alone (like the staff and lockdown tables).
- Per-group enable flags are read on the hot message path; keep them in one cached settings row.

### Table stakes

| Feature | Why expected | Complexity | Depends on |
|---------|--------------|------------|------------|
| Deletion hub page with Service messages, Scheduled deletion, Commands, Edited messages, Self-destruct and Back | Screenshot 1 | Low | `registerSettingsPage` |
| Service message page: types join, leave, photo, title, pinned, topics, boosts, voice chat invites, checklist, community; per type off, immediate or after N seconds | Owner's list and screenshot 11 | Med | New settings columns; a single dispatcher handler for service messages; join and leave tie into the existing `cleanservice` |
| Scheduled deletion page: welcome, goodbye, rules, automatic punishments, manual punishments; per kind off or N minutes | Screenshot 10 | Med | Durable scheduler; send-site hooks |
| Durable scheduler (table, worker, drain, pacer) | Everything delayed depends on it | High | Lockdown worker pattern, `staffPaced`-style pacer, shutdown registration |
| Members' commands deletion | Owner's list | Low-Med | Run after the command; needs delete right |
| Edited messages deletion with admin/approved exemption | Owner's list | Med | New edited-message handler at a new group number (document it) |
| Self-destruct after N minutes with exemptions | Owner's list | High (volume) | Scheduler batching and pacing |
| Stepper buttons send the absolute target value, never a delta | Existing settings rule (`v`) | Low | Existing stepper helper |
| Visible warning when the bot lacks `can_delete_messages` | Otherwise every toggle is a silent no-op | Low | live bot check |

### Differentiators

| Feature | Value | Complexity | Notes |
|---------|-------|------------|-------|
| Presets (immediately, 5 s, 30 s, 1 min, 10 min, 1 h) plus the +/- stepper | A stepper at +1 second is slow to reach a minute | Low | Absolute target values keep callbacks safe |
| Grace window before an edit is deleted | Typo fixes are not punished | Low | A per-group number; default a few minutes |
| Exempt approved users and admins from self-destruct and command deletion explicitly | Matches the repo's "approved users skip" rule | Low | Already needed |

### Anti-features

| Anti-feature | Why avoid | Instead |
|--------------|-----------|---------|
| Global silence, mass deletion, delete all messages | Owner excluded them | Omit |
| Delay above 48 hours | The Bot API will refuse the delete | Cap and say so on the page |
| `time.AfterFunc` for scheduled deletes | Lost on restart, per-process, no pacing | The durable scheduler |
| Redis-only scheduling for self-destruct | A flush or restart silently leaves messages | PostgreSQL rows |
| Deleting a member's command before the handler ran | Breaks the command | Delete after the handler returns |
| Deleting the Staff Group's own messages by group settings | A Staff Group is also a group; its commands and summaries must not vanish | Exclude Staff Groups from self-destruct and command deletion, or at least from bot summaries; flag for the roadmap |
| Deleting log-channel copies or alert copies | Lockdown alert copies are edited, never deleted | Keep them out of every deletion kind |

---

## 6. Remaining pages

Each page is a thin menu over the existing command's apply function (Phase 9 rule: one setting, one code path, plus parity, drift and authority-parity tests). The gate from section 2 applies to all of them.

| Page | Expected controls (recall of GroupHelp, MEDIUM, adjusted to Fuku's existing commands) | Complexity | Notes |
|------|-------------------------------------------------------------------------------------|------------|-------|
| Warnings | Warn limit stepper, action choice (ban/kick/mute/tmute...), warn time if the module has it, reset | Low-Med | `warns.go` apply functions must be extracted from the commands |
| Reports | `/report` on or off, maybe who is notified | Low | `reports.go` |
| Language | Choice of the 7 UI languages for the group, a Lang button on Main | Low | `language.go`; the Main page has a Lang button in the screenshot |
| Log channel and categories | Show the linked channel, toggle each category, an Unset card. Setting the channel stays a command flow because it needs a message in the channel (`/setlog` in the channel, then forward) | Med | `logchannels.go`; the page explains the two-step flow rather than inventing a new one |
| Anonymous-admin mode | One toggle (the existing `AnonAdmin` setting) | Low | Security-relevant: on means anonymous admin commands skip the proof tap. Show a plain explanation; the gate already limits it to the owner by default |
| Locks | A paged list of lock types with a toggle per type, plus allow-list entries | Med | ~30 lock types; callback data per type must fit 64 bytes; page size about 8 |
| Blacklists | Browse triggers (paged), delete one with a Yes tap, add by typing | Med | Uses the input state from section 3 |
| Filters | Browse, delete, add by typing (trigger, then reply message) | Med-High | Filters carry media and buttons; reuse the editor slot |
| Notes | Browse, delete, add by typing (name, then content) | Med-High | Same; private-note flag exists |

Table stakes: browse plus delete plus add-by-typing for blacklists, filters, notes; every control calls the command's apply function. Differentiators: search within a list, edit a filter's reply with the full editor. Anti-features: bulk import by pasting hundreds of lines in a menu flow (use the backup import), per-trigger permissions.

Main page: grows from 4 pages (Staff, Lockdown, Captcha, Antiflood) to about 16. Use the two-column layout of screenshot 5 and a bottom row of Lang / Close / Other. The Main renderer asks every page for a state suffix on each open, so reads must be cached (Phase 9 pages already read through cached getters) and a failed read shows the "unknown" mark, not an error.

---

## Feature dependencies

```
Persisted user->group admin index (new table)
   -> PM group list
        -> Owner gate in list filter
Owner gate flag + live creator check
   -> in-group /settings gate, deep link gate, press pipeline gate
   -> every new page (all pages assume the gate)

Redis input state ("waiting for your message") + private-chat watcher group
   -> Message editor (text, media, buttons)
        -> Rules media+buttons (migration, sendRules, deep link rules_<id>)
        -> Welcome/Goodbye editor
        -> Notices with custom text (differentiator)
   -> Add-by-typing for blacklists, filters, notes

Greeted-members table -> Welcome "greet once"
Existing clean_old -> Welcome "delete last welcome" (page only)

Durable scheduled-deletion table + worker + pacer + shutdown drain
   -> Bot messages deleted after N minutes (welcome, goodbye, rules, punishments)
   -> Service messages "after N seconds"
   -> Self-destruct
   -> Requirement notices auto-delete

Requirement settings table (one row per chat, per-rule action + 2 toggles)
   -> join-time check (hook in ProcessSingleJoin / finishCaptchaPass)
   -> message-time check (new watcher, own group number, ContinueGroups)
   -> Requirement-mute record (new table)
        -> "I fixed it" callback (live re-check, resolveUnmutePermissions, captcha pending, lockdown)
   -> Punishment episode latch (Redis) -> warn/notice once per episode
   -> Photo/channel lookup cache (Redis)
   -> AI spam-name: fuku/utils/claude + budgets + breaker + name-hash verdict cache
   -> Name-block detectors (stdlib unicode, no dependency)

Existing cleanservice (ShouldCleanService) <-> new service-message page (must share storage for join and leave)
Existing handler group list in AGENTS.md -> new groups for: private input watcher, requirement watcher, edited-message handler, service-message deleter (all documented there)
```

Cross-module collisions to design for from the start:

1. Captcha and requirements both mute a joiner.
2. A captcha pass, `/unmute`, and "I fixed it" are three unmute paths; all go through `resolveUnmutePermissions`.
3. Lockdown overrides everything at group `-7`.
4. `/cleanservice`, the new service page and the `cleanService` join-processing path are one code site.
5. Every bot message that gets a delay needs its send site changed.

---

## MVP recommendation (ordering by dependency, in the owner's priority)

1. **Persisted admin index, PM group list, owner gate.** One migration (index table, access flag). Everything else is a page reached through it; the gate must exist before the new pages so no page ever ships ungated. Reuse `/admincache` as `/reload`.
2. **Input-state infrastructure and the message editor** for Welcome, Goodbye and Rules (text, media, URL buttons, previews, media under text), including the Rules migration. Build the editor around a generic "message slot" so filters and notes can use it later. Add "greet once" and expose `clean_old` as "delete last welcome".
3. **Requirement settings, detectors and punishments**: last name, username, photo, channel subscription, Arabic/Chinese/Russian blocks, notice, the requirement-mute record and "I fixed it". Add the AI spam-name rule last inside this phase, because it needs the most guards; the five script and field rules stand alone.
4. **Durable scheduler, then deletion pages**: service messages (immediate first, delays second), scheduled deletion of bot messages, members' commands, edited messages, then self-destruct last (highest volume). Wire the requirement notice to the scheduler.
5. **Remaining pages**: warnings, reports, language, log channel, anonymous-admin mode, locks, then blacklists/filters/notes browse-delete-add.

Defer or cut if the milestone runs long: self-destruct (volume and pacing risk), per-part custom notice text, AI spam names (needs the owner's accuracy spike like the raid image check), a grace window for edits, list search.

## Open questions for the owner (not blockers for research)

- GroupHelp's glyph button in each stepper row (the arrow before -1/+1): does it mean "type a custom value", "reset" or "go to default"? The screenshots do not show it. Suggest "type a number" via the input state.
- "Own commands" in the scheduled-deletion list: the bot's replies to commands? Not in the owner's list; confirm before building.
- Welcome for a member who fails a requirement and is muted: hold it until the member passes? Suggested yes.
- A group with no live owner: allow admins? Suggested yes, still live-checked.
- Should the Staff Group be exempt from deletion kinds (summaries and cards)? Suggested yes.

## Sources

- Owner screenshots, images 1 to 11 (UX reference, HIGH for the visible UI)
- `/home/user/Fuku_Robot/.planning/PROJECT.md`, `/home/user/Fuku_Robot/AGENTS.md`
- `/home/user/Fuku_Robot/fuku/modules/greetings.go`, `rules.go`, `membership.go`, `settings_pages.go`; `fuku/db/models/greetings.go`, `rules.go`; `fuku/config/config.go` (`AllowedUpdates`); `fuku/utils/helpers/telegram_helpers.go` (HIGH)
- gotgbot v2.0.0-rc.36 source in the module cache: `gen_types.go` (Message service fields, `Community*`, `ChatFullInfo.LinkedChatId`), `gen_methods.go` (`DeleteMessage` limits, `DeleteMessages`, `ShowCaptionAboveMedia`, `GetUserProfilePhotos`) (HIGH)
- Search summaries of Rose docs: [clean command](https://missrose.org/docs/cleaning/clean-command/), [advanced greetings](https://missrose.org/docs/greetings/advanced/), [clean message](https://missrose.org/docs/cleaning/clean-message/) (MEDIUM; pages could not be fetched)
- Search summary of the Shieldy issue [1inch/shieldy#52](https://github.com/1inch/shieldy/issues/52) (LOW)
- GroupHelp Combot, Group Butler: no usable sources found; no claims here depend on them (LOW)
