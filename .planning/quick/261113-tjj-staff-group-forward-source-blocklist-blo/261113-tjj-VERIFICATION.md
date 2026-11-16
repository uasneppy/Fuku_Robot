---
task: 261113-tjj-staff-group-forward-source-blocklist-blo
verified: 2026-11-14
status: human_needed
branch: claude/ecstatic-goldberg-qsss05
commits:
  - 9ef2cfc feat(staff): forward source blocklist storage and Staff Group commands
  - 95445fd fix(test): remove unused fwdLastText helper
  - ba2f490 feat(staff): forward blocklist watcher at group -9
  - 17e5ce0 fix(staff): mark the group-list placeholder constant as not a credential
  - b907052 feat(settings): private-only Blocked sources page for the Staff Group forward blocklist
gaps: []
human_verification:
  - "Live Telegram labelling of forwards (see section 5)"
---

# Quick task 261113-tjj: Verification Report

**Goal:** Staff Group forward-source blocklist. A forward from a listed channel or group is deleted in every group linked to the Staff Group, and the forwarding member gets the list's one action (none, warn, mute, kick, ban). One list per Staff Group, one action for the whole list, managed by a private-only settings page and matching commands that share apply functions.

**Verdict:** The code delivers every must-have truth. Nothing is FAILED and nothing is missing. The status is `human_needed` only because the watcher's core behaviour rests on how Telegram labels a forward from a channel and from an anonymous administrator of a group. No test can prove that label, and the owner must confirm it live (section 5).

**Method:** Read the code at HEAD (`b907052`). Checked each cited symbol against its definition. Read the gotgbot origin-merge source in the module cache. Confirmed the locale keys in the eight language files. Ran `CGO_ENABLED=0 go build ./...` (passed). Checked the test names and the negative cases in the test bodies. Did not rerun the test suite, per the instructions.

---

## 1. Requested checks (1 to 8)

| # | Check | Result | Evidence (file and symbol) |
|---|-------|--------|----------------------------|
| 1 | A listed-channel forward in a linked group is deleted, and the member gets the action. Other cases are left alone. | VERIFIED | `fuku/modules/staff_forwardblock_watch.go`: `forwardBlockProcess` runs the pipeline. `forwardBlockSourceOf` takes the source from `ForwardOrigin.MergeMessageOrigin()`: `Chat` for a channel origin, `SenderChat` for a `chat` origin, nothing for `user` or `hidden_user`. `forwardBlockLinkLookup` (`staff.GetLinkOfGroup`) returns nil for an unlinked group and for the Staff Group itself, so both stop there. `rules.IsBlocked(source)` stops an unlisted source. `forwardBlockEnforce` deletes, then applies the action. gotgbot `gen_types.go` confirms the mapping: channel to `Chat`, chat to `SenderChat`, user and hidden user to no chat. Tests: `TestForwardBlockWatcherDeletes`, `TestForwardBlockWatcherSpares` (unlinked, Staff Group, user, hidden user, unlisted), `TestForwardBlockSourceOf`. |
| 2 | Exemptions. Unknown never acts. | VERIFIED | Sender shape: `forwardBlockSender` calls `raidFloodSenderCounts` (`raid_flood.go`), which rejects `sender_chat`, `IsAutomaticForward`, `staffServiceUserIDs` and the bot itself, and then `IsBot()`. Admins and approved users: `memberDeletionExempt` (`member_deletion.go`). An approvals error is exempt. An unknown admin status is exempt (`!= memberDeletionNotAdmin`). Unknown bot admin: `chat_status.IsBotAdmin` false, so no action (logged once through a gate). Unknown list: `forwardBlockRules` returns false. Unknown link: `forwardBlockFreshLink` error returns false. Unknown owner check: `staffRecheckUnknown` returns false, not memoised. Tests: `TestForwardBlockWatcherSpares` (admin, approved, anonymous admin, channel identity, automatic forward, bot, service account, bot itself), `TestForwardBlockWatcherUnknownNeverActs` (unknown admin status, recheck unknown, and the other unreadable states). |
| 3 | Only the live Staff Group creator writes. Other admins only view. A refusal writes nothing. | VERIFIED | Commands: `forwardBlockChecks(true)` appends `requireStaffGroupCreator()`, which uses `chat_status.CheckOwner` live (`OwnerMatch` only; unknown gives `staff_check_failed`; mismatch gives `staff_fwdblock_owner_only`). `fwdBlockAction` with an argument calls `requireStaffGroupCreator()` before any write. Reads (`fwdblocklist`, `fwdblockaction` with no argument) use `forwardBlockChecks(false)`. Page: `settingsFwdBlockDef` sets `Right: settingsRightStaffOwner`. `settingsRightRefusalFor` returns the creator-only refusal `staff_fwdblock_owner_only` for every other status. The page's Ask, Yes, Add and `vs` actions use `def.Right` (`settings_list.go` `settingsListActions`, `settings_list_input.go` `settingsListAddActions`). The pager, `ve` and Cancel use `settingsRightAdmin`. Writes happen only after the checks pass. |
| 4 | `@username` is resolved live and stored as its numeric ID. Non-channel or group targets are refused. Unresolved numeric IDs are stored with no title. | VERIFIED | `parseForwardBlockRef` is strict: a negative numeric ID via `parseStaffChatIDArg` (which returns false for a non-negative value), or `staffUsernameRe` `^@[A-Za-z0-9_]{4,32}$`. `forwardBlockLookup` makes one live `getChat` through `forwardBlockResolveChat` (raw request, 8 s bound). For a username: a Telegram client error (not 429) gives `forwardBlockNotFound`. A non-negative ID, a type other than channel, supergroup or group (for example a user's private chat), or a mismatched ID gives `forwardBlockNotSource`. A transport error or 429 gives `forwardBlockCheckFailed` and stores nothing. For a numeric ID: a client error stores the ID as typed with no title (`forwardBlockAdded`, title empty). The stored value is the numeric `chat.ID`, and the name is never re-resolved (`AddForwardBlock` stores `source_chat_id`). Tests: `TestParseForwardBlockRef`, `TestApplyForwardBlockAdd`, `TestForwardBlockResolveChatRequest`. |
| 5 | Page and commands share the apply functions. Every write drops `staff_forward_block:<staff chat>`, which is in `skipLocal`. | VERIFIED | Page calls `applyForwardBlockAdd` (`settingsFwdBlockTyped`), `applyForwardBlockRemoveByID` (`settingsFwdBlockRemove`) and `applyForwardBlockAction` (`settingsFwdBlockRunSet`). The commands call `applyForwardBlockAdd`, `applyForwardBlockRemove` and `applyForwardBlockAction`. The repository is reached only through the `forwardBlockWrites` seam. `staff/forwardblock.go`: `AddForwardBlock`, `RemoveForwardBlockByID`, `RemoveForwardBlockBySource` (only when a row went) and `SetForwardBlockAction` each call `invalidateForwardBlockKeys` after commit. `fuku/db/cache/local.go`: `staffForwardBlockPrefix` is in the `skipLocal` return expression. `local_test.go` covers the prefix. |
| 6 | Chat migration moves the list and its action. Unset deletes them. An unlinked group stops being enforced at the next forward. | VERIFIED | `staff/rekey.go`: `RekeyChat` calls `rekeyForwardBlocksTx`, which moves `staff_forward_blocks` and `staff_forward_block_settings` inside the same transaction, and then calls `invalidateForwardBlockKeys(old, new)`. `staff/repository.go`: `DeleteStaffGroupWithLinks` calls `deleteForwardBlocksTx` in the same transaction as the Staff Group and link deletes, and invalidates the key after commit. The only production caller of unset (`fuku/modules/staff.go:348`) uses it. Unlinking: `GetLinkOfGroup` is cached under `staff_link_of`, which is in `skipLocal`, so the watcher's link gate returns nil at the next forward. Tests: `TestRekeyChatMovesStaffGroupAndLinks`, `TestStaffForwardBlockRekeyAndUnset`. |
| 7 | Watcher is the only handler of group -9, never ends an update, starts no goroutine. Locale keys exist in all 8 files. | VERIFIED | `staff.go:409`: `AddHandlerToGroup(handlers.NewMessage(forwardBlockFilter, ...), staffForwardBlockGroup)`. `staffForwardBlockGroup = -9`. No other constant or `AddHandlerToGroup` uses -9. `forwardBlockWatcher` presets `result = ext.ContinueGroups` before its `defer error_handling.RecoverFromPanic`, so no path returns nil or `EndGroups`. Grep finds no `go ` statement and no Redis use in the watcher or apply files. The 30 keys read by the new code exist in `locales/{en,es,fr,hi,id,pt,ru,uk}.yml` (checked with a loop over the eight real languages; `config.yml` is the alt-name pseudo-locale and is not a language). Notices: `forwardblock_notice.go` queues through `scheduleBotMessage(sent, deletion.KindBotAuto)`. Tests: `TestForwardBlockWatcherNeverEndsUpdate`, `TestForwardBlockWatcherGroup`, `TestForwardBlockWatcherPunishments`. |
| 8 | Migration agrees with the model (UNIQUE and CHECK). | VERIFIED | `migrations/20261113130000_add_staff_forward_blocks.sql` is the newest file, is new in `9ef2cfc`, and no existing migration changed. It has no top-level BEGIN, COMMIT or CONCURRENTLY. `staff_forward_blocks` has `uk_staff_forward_blocks_source UNIQUE (staff_chat_id, source_chat_id)` and `chk_staff_forward_blocks_source CHECK (source_chat_id < 0)`. `staff_forward_block_settings` has `chk_staff_forward_block_action CHECK (action IN ('none','warn','mute','kick','ban'))`. `models/staff_forward_block.go` carries the same unique index name, the same column names and the same check names and expressions. `staff_forward_block_settings.staff_chat_id` is the primary key in both. The action vocabulary `staff.ForwardBlockActions` equals `blacklistActionValues` (`TestForwardBlockVocabulary`). Tests: `TestStaffForwardBlockConstraints`, `TestStaffForwardBlockMigrationStatements`. |

---

## 2. Must-have truths (plan `must_haves.truths`)

| # | Truth (abridged) | Status | Evidence |
|---|------------------|--------|----------|
| T1 | A forward from a listed source in a linked group is deleted and the member gets the one action. The same forward in an unlinked group, in the Staff Group, from an unlisted source, or from a user or hidden user is left alone. | VERIFIED (live origin label to confirm, section 5) | Check 1 above. |
| T2 | Creators, admins, approved users, anonymous admins, `sender_chat` senders, automatic forwards, bots, service accounts and the bot are never acted on. Unreadable state never acts. | VERIFIED | Check 2 above. |
| T3 | Only the live Staff Group creator can add, remove or set the action, by command and by page. Other admins only view. A refusal writes nothing. | VERIFIED | Check 3 above. Note: the creator rule is checked against the Staff Group only. The link invariant (`recheckLink`, which the watcher runs before every enforcement) is what makes that creator the creator of every linked group. |
| T4 | `@username` is resolved live and stored as its numeric ID. Unresolvable names and non-channel or non-group targets are refused. Negative numeric IDs are stored as typed, with no title when unresolved. | VERIFIED | Check 4 above. |
| T5 | Page and commands call the same apply functions. Every write drops `staff_forward_block:<staff chat>` (in `skipLocal`). | VERIFIED | Check 5 above. |
| T6 | Chat migration moves the list and action. Unset deletes them. An unlinked group stops being enforced at the next forward. | VERIFIED | Check 6 above. |
| T7 | A bot without the delete or restrict right logs one line per kind per hour per chat and is never retried. An album punishes the member once. | VERIFIED | `forwardBlockWarnf` with five `memberDeleteWarnGate` values (`forwardBlockWarnGates`), one per kind: cannot act, list unreadable, authority unknown, delete failed, punish failed. Delete and restrict are attempted once per message, with no retry loop. `forwardBlockEnforce` claims the per-(chat, member) latch `forwardBlockPunished` (60 s) before punishing, so an album's later items delete but do not punish again. Tests: `TestForwardBlockWatcherBotRights`, `TestForwardBlockWatcherPunishments` (the album subtest at line 156). |

---

## 3. Artifacts and key links (plan frontmatter)

| Artifact | Status | Notes |
|----------|--------|-------|
| `migrations/20261113130000_add_staff_forward_blocks.sql` | VERIFIED | Check 8. |
| `fuku/db/staff/forwardblock.go` | VERIFIED | Cached read (`GetForwardBlockRules`), fresh reads, add (soft cap 200 inside the transaction, `ON CONFLICT DO NOTHING`), remove by ID (conditional, chat-scoped), remove by source, set action (column-targeted upsert), invalidation after commit. |
| `fuku/modules/staff_forwardblock.go` | VERIFIED | Apply functions, live resolver, shared texts, four commands registered in `staff.go:403-406`. |
| `fuku/modules/staff_forwardblock_watch.go` | VERIFIED | Pipeline, exemptions, authority memo (success only, 60 s, keyed by chat and link row), punish latch, failure gates, no goroutine. |
| `fuku/modules/settings_fwdblock.go` | VERIFIED | Private-only page `v`, `Hidden` via the cached `staff.GetStaffGroup` gate, typed add through `applyForwardBlockAdd`, action screen, `settingsRightStaffOwner` on writes. `settingsListGuardHidden` (`settings_list.go`) guards every action of the table. |
| key link: watcher to `staff.GetLinkOfGroup`, then `GetForwardBlockRules` under a `skipLocal` key | VERIFIED | `forwardBlockLinkLookup`, `forwardBlockRulesLookup`; key prefix in `skipLocal`. |
| key link: watcher to `recheckLink` (`staff_recheck.go`), memoised on success only | VERIFIED | `forwardBlockAuthorized`: `forwardBlockFreshLink`, memo `live` and `put` only on `staffRecheckOK`. |
| key link: `RekeyChat` and `DeleteStaffGroupWithLinks` to both tables | VERIFIED | `rekeyForwardBlocksTx`, `deleteForwardBlocksTx`. |
| key link: page writes to the apply functions | VERIFIED | Check 5. |

---

## 4. Summary claims checked

- "Five commits, nothing under `.planning/`": `git log` shows the five commits in the order listed. `git status` shows only the untracked task directory, which is correct.
- "Migration is the only new schema": `git diff e713ed5 HEAD --name-status -- migrations` shows one added file.
- "Locale keys in all 8 files": verified (Check 7).
- "`blacklistWatcher` not touched": not re-read here. The plan's file list does not include it.
- "Test families exist and pass": the names exist (for example `TestForwardBlockWatcher*`, `TestApplyForwardBlock*`, `TestSettingsFwdBlock*`, `TestStaffForwardBlock*`). I did not rerun them.

---

## 5. Human verification required (the owner's live Telegram check)

The code path is complete. These points depend on Telegram's live behaviour, which no fixture proves. Each needs a live run in a staging Staff Group with a linked group, and the owner should check them before relying on the feature.

1. **A channel post forwarded from a listed channel is deleted in a linked group.** The watcher reads the source from a `channel` forward origin (`forwardBlockSourceOf`, `Chat` field). Expected: the message is deleted, and the member gets the list's action (for example a warn notice or a mute notice).
2. **An anonymous administrator's post forwarded from a listed group is recognised.** The watcher assumes Telegram labels such a forward as origin type `chat` with `sender_chat` equal to the source group, and reads it through `SenderChat`. This is the main assumption to check. Expected: deleted, with the action applied. If Telegram labels it differently, this case is silently not enforced (the code acts on nothing, which is the safe direction). Note the blocked message's own `sender_chat` is not the source here.
3. **`@username` resolution.** Add a public channel by `@name` with `/addfwdblock`. Expected: the reply names the title, and the list shows the numeric negative ID. Add a private channel by its numeric ID that the bot is not in. Expected: stored unverified, with the ID only.
4. **Rights in a linked group.** Confirm the bot's delete right and restrict right behave as the watcher expects (a missing delete right is logged once per hour and nothing is deleted; a missing restrict right still deletes and posts no punishment).
5. **Notice wording.** Confirm the mute, kick and ban notices read correctly in the group's language (they reuse the blacklist templates with the reason `staff_fwdblock_reason`).

---

## 6. Gaps, warnings and accepted limits

No BLOCKER and no WARNING from the code reading. The following are the limits the plan and summary accept. They are stated here and not treated as defects:

- The punish latch, the authority memo and the failure gates are per replica. Two replicas can each punish an album's member once, and each can run the owner check once a minute.
- The 200-source cap is a soft cap read inside the insert transaction, so two concurrent adds can pass it by one.
- A stored source title can go stale after a rename.
- A forward that the blacklist watcher also matches can be acted on by both, because the new watcher never ends an update.
- A `sender_chat` channel identity and an anonymous admin posting the forward are exempt (stated limit).
- A numeric source that Telegram cannot resolve is stored with no title, and nothing checks that the ID is real. This is what the plan asks for.
- Entry numbering on the page follows the text sort, and `/fwdblocklist` follows row-ID order.

---

## 7. Verdict

**status: human_needed**

All seven must-have truths, the eight requested checks and the five artifacts are verified in code at HEAD. No gap was found. The status is `human_needed` because section 5, item 2 (the Telegram origin label for an anonymous administrator's post) is a live-Telegram assumption that no automated check can confirm. The other items there are confirmations of behaviour already implemented.
