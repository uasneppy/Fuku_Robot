# Phase 19: the Blacklist, Filters and Notes pages, live checks and confirmations

This file collects the checks that need a live Telegram client, a real supergroup or your decision. It is for you, the
owner. No tool exists for them: you do them by hand on a test supergroup (and a second small group), with the bot as an
administrator and again as a plain member, and record each answer here. It was completed, ordered and numbered once by plan
19-12 against the finished code and the 19-01 to 19-11 summaries.

**None of these checks blocks the phase.** Discuss was skipped for Phase 19 ("skip-discuss"), so every decision in
19-CONTEXT.md is a provisional owner default, and you chose to defer the live checks, as in Phases 7 to 18. The phase is
complete when its automatic gate passes; this list is how you confirm or change the starting values afterwards. Nothing here
changes whether the code is safe: for every outcome (an unreadable list, a refused or failed press, a refused or failed typed
step, a bot that is not an administrator, a lost Redis, an unreachable Telegram, a stale or forged button) a page shows the
cannot-open edit or a plain notice, a refused or failed press or typed step writes nothing, a menu opened in the group shows
no word, keyword, name, count or action, a forged row ID of another chat deletes nothing, and the one bounded user-text field
of the wait (`w`, the first step's keyword or name) is never logged, never in a button and never in callback data. The checks
change wording, defaults and your confidence, not safety.

**This file records no result.** The code is written against Telegram's documented behaviour and the fakes of the tests; what
the real server does is yours to measure.

## Before you rely on it in a real group

- **The pages change the same lists as the commands.** A word, keyword or note added or deleted on a page is what `/addblacklist`,
  `/rmblacklist`, `/filter`, `/stop`, `/save` and `/clear` read and write, and the reverse; there is no second copy and no new
  stored state, only the wait for your typed reply (Redis, ten minutes). Another replica can show an old list for up to
  `CACHE_LOCAL_TTL` (10 s by default) after a write to `blacklist`, `filter_list`, `filters_optimized` or `notes_list`, exactly
  as after the command; the page itself always reads fresh.
- **A blacklisted word acts on every member's message at once.** From the next message of any non-exempt member the message is
  deleted and the word's action is applied (warn, mute, kick, ban, or only the delete). Admins and approved members are exempt.
  A single letter, digit or punctuation mark is refused (A19-2), but a short common word still matches inside longer words
  (`hell` matches `shell`), because the matcher compares lower-cased literal substrings.
- **The action applies to every word.** Setting it on the Blacklist page rewrites the action of all the group's words; words
  added later take the action all words share (A19-3).
- **A filter answers every message that contains its keyword**, with the stored reply and its buttons, for every member.
- **A note is readable by every member** with `#name` or `/get name`, unless it is stored as admin-only (the page marks those
  with "admins only").
- **A delete cannot be undone from the menu.** It asks first (a Yes/No card naming the entry), but once confirmed the entry is
  gone; adding it again gives it a new row ID.
- **The lists are private-menu only.** A menu opened in the group shows a short note and the Open in private button, never an
  entry or a count, so members of the group cannot read a list from it. Main shows `In private` for the three cells there.
- **The bot must be an administrator for most changes**, as for the commands (blacklist writes also need the bot's restrict
  right; notes need the bot as an administrator); the page refuses with the command's own notice and writes nothing.

## Command behaviour this phase changed

Plans 19-01, 19-05 and 19-08 moved ten commands onto apply functions behind goldens that were committed first and never
re-recorded: every other request and reply byte of `/addblacklist`, `/rmblacklist`, `/blacklistaction`, `/blacklists`,
`/filter`, `/stop`, `/filters`, `/save`, `/clear` and `/notes` is unchanged, quirks included. These six changes are on purpose,
each in its own commit with its own test and rulebook lines. Note on reverting: each change is one commit, but later plans of
the phase added to the same test files and to AGENTS.md, so a plain `git revert` of a commit conflicts in those files (checked
with `git apply -R --check`); the production change to undo is the small hunk named for each, and the conflicts are in the tests
and the rulebook. The migration commit reverts cleanly.

1. **A19-2 for `/addblacklist`: a word of one code point is refused.** A word that is exactly one code point outside Unicode
   category So (or blank) is refused with the reply `blacklists_blacklist_word_too_broad` ("These words are too broad (a single
   letter, digit or punctuation mark would match almost every message): ..."), listed beside the too-long words, and is never
   stored; the page refuses it too. Before, `/addblacklist a` stored `a`. Commit `12ee6a4`,
   `feat(19-01): /addblacklist refuses a word that would match almost every message (A19-2)` (production hunk: `blacklists.go`,
   and `keywordTooBroad` in `list_validation.go`, which the filter keyword shares).
2. **A19-2 for `/filter`: a keyword of one code point is refused.** Same rule, reply `filters_keyword_too_broad`, nothing
   stored; the Filters page uses the same check. Before, `/filter a` stored `a`. Commit `f0dd7f7`,
   `feat(19-05): /filter refuses a keyword that would match almost every message (A19-2)` (production hunk: `filters.go`).
3. **A19-3: a new blacklist word takes the action every word shares.** A word added (by `/addblacklist` or the page) takes the
   one action all of the chat's words already have (`mute`, `kick`, `warn`, `ban` or `none`), else `warn`. Before, every new word
   was stored with `warn`, so a word added after `/blacklistaction ban` acted with Warn and the list was mixed. Commit
   `3025b0f`, `feat(19-01): a new blacklist word takes the action every word shares (A19-3)` (production hunk:
   `blacklistSharedAction` in `blacklists.go`). If declined, the action screen's hint text in English becomes "The action is
   stored with each word, and a word added later starts with Warn." (UI-SPEC section 5) in the 8 locale files.
4. **A19-4: `/save` refuses a name that could never be fetched.** A name that contains white space, starts with `#` or is over
   100 code points is refused with the reply `notes_name_invalid` and nothing is stored, because `#name` and `/get` take the first
   space-separated token. Before, `/save "two words"` or `/save #x` stored a note nobody could fetch. `/clear` is not touched.
   The tests drive the quoted form `/save "two words"` and `/save #x` in the quoted and the reply form (plan 19-08 notes that a
   bare `/save two words` as a reply was not usable as the test input). Commit
   `2eeac8f`, `feat(19-08): /save refuses a note name that could never be fetched (A19-4)` (production hunk: `noteNameInvalid`
   in `list_validation.go` and `checkNoteName` in `notes.go`; the locale key goes with it).
5. **Fail-closed `/filter` lookup.** When the existing-keyword or filter-count lookup of `/filter` fails, the command now answers
   `common_settings_save_failed` and stores nothing, where it used to read the failure as "not found" and "0" and add the
   filter anyway (AGENTS: never discard a database error on a state-changing path). The command's own cap pre-check
   (`filters_limit_exceeded`) keeps its old hiding read. Commit `4f794f1`,
   `fix(19-05): /filter refuses to add when its existence or count lookup fails` (production hunk: five lines of `filters.go`).
6. **Fail-closed `/save` lookup.** The same for the existing-name lookup of `/save`: `notes_save_failed` and nothing stored
   where it used to add the note. `/clear` keeps its cached list read. Commit `ed7988a`,
   `fix(19-08): /save refuses to add when its existence lookup fails` (production hunk: `notes.go`, six lines).

One schema change also belongs here. **The legacy `chk_blacklists_action` made `/blacklistaction none` fail on a chain-built
PostgreSQL database.** Plan 19-01 proved it on a throwaway PostgreSQL 16 cluster built by the migration chain: the table kept
both `chk_blacklists_action` (validated, without `none`) and `chk_blacklist_action` (NOT VALID, with `none`), and
`UPDATE blacklists SET action = 'none'` failed with SQLSTATE 23514 on the first. Migration
`migrations/20261113120000_drop_legacy_blacklist_action_check.sql` (one `DROP CONSTRAINT IF EXISTS`) drops it, and
`TestBlacklistActionValuesOnPostgres` joined `make test-postgres-integrity`. It runs with `AUTO_MIGRATE=true` at the next deploy.
Commit `76408de`, `fix(19-01): drop the legacy blacklist action check that refuses none`; reverting the file would restore the
failure, so it is not an item to decline (check 1 confirms it on production).

## The checks

### 1. The production CHECK constraints of `blacklists` and `/blacklistaction none`

**Why:** the measurement above was on a database built from the repository's migration chain. Your production database may
have been built or altered another way, and the sandbox cannot see it. The migration is idempotent, so it is safe either way.

**Steps:** before the deploy, on the production database: `SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint WHERE
conrelid = 'blacklists'::regclass;`. After the deploy run it again. In a test group with at least one blacklisted word run
`/blacklistaction none`.

**Record:** before the deploy the list had both `chk_blacklists_action` and `chk_blacklist_action`: yes / no. After: only
`chk_blacklist_action`: yes / no. `/blacklistaction none` answered success and the Blacklist page then showed `Action: Delete
only`: yes / no.

### 2. Main at 11 rows on a phone

**Why:** Main now has 18 grid cells (nine full rows, no lone cell), Access for the owner and the navigation row: 11 rows for the
owner and 10 for an administrator, in a private and in a group menu. That is over the content-page budget (the same reasoning as
Phase 18's check 18: Main is navigation, not a content page), and only a real screen says whether it is too tall.

**Steps:** as the group's creator open `/settings` in the bot's private chat, pick a group and look at Main in uk, ru, hi and id;
then open it in the group as the creator and as an admin the owner let in. Check the cells are readable (the longest page names
are Blacklist, Member requirements, Anonymous admins, Language) and no row is cut.

**Record:** readable at 11 rows in every language: yes / no. Too tall, wants the `📚 Lists` hub (below): yes / no.

### 3. A list page of eight long entries on a phone

**Why:** each entry line is cut at 32 code points and stripped of bidirectional controls, the numbered `🗑 N` buttons are bound
to the numbers on the lines, and the page must stay readable with mixed scripts and a right-to-left word.

**Steps:** in a test group add eight long blacklisted words (mixed Latin, Cyrillic, CJK, one Arabic or Hebrew word, one with
`&` and `<`) and open the Blacklist page in private. Check the numbers, the lines, the cut with an ellipsis, that the
right-to-left word does not reorder its number or the next line, and that each `🗑 N` button is the entry it says. Page through
with more than eight words. Repeat for Filters (keywords) and Notes (names, with one stored admin-only).

**Record:** readable, numbers match the buttons, no reordering: yes / no. Page line `Page N/M` and Prev/Next correct: yes / no.

### 4. The two-step add in a real client, compared with the commands

**Why:** the page extracts the reply with the same function the commands use (`content.ExtractTypedNoteOrFilter`), and the tests
prove the stored row equal for text with entities, a captioned photo, a sticker and markdown buttons, but a real client sends
albums, formatted captions and split long messages in ways a fake cannot.

**Steps:** on the Filters page press Add, type a keyword, then send a photo with a caption; compare the row's behaviour in the
group with `/filter K2` sent as a reply to the same photo. Repeat with a formatted text with buttons (`[label](buttonurl:https://example.com)`),
a sticker, and a text longer than 4096 characters (the client splits it). Do the same on Notes with a name, then compare with
`/save N2` as a reply. On Blacklist type several words in one message and a caption of a photo.

**Record:** the page's filter answers exactly as the command's: yes / no. Note fetched by `#name` and `/get`: yes / no. A split
long text stored only its first part and the rest were ignored without a reply (a stated limit): yes / no.

### 5. The effect in the group

**Why:** the end-to-end test proves each list changes what the bot does through the real dispatcher with Telegram faked; only a
live group shows the real deletes, replies and warns.

**Steps:** add a word from the Blacklist page, then as a plain member send it (deleted, warned once with the group's warn
settings), as an admin and as an approved member (kept). Set Delete only on the action screen and repeat (deleted, no new warn).
Add a filter and a note from the pages and trigger them as a member (`#name`, `/get name`). Delete each from its page behind the
card and confirm each stops.

**Record:** blacklist acts, filter answers, note fetched, each stops after its delete: yes / no.

### 6. A group menu shows nothing, and Open in private

**Why:** D-01: members of the group can read a menu opened in the group, so it must never show an entry or a count.

**Steps:** in the group open `/settings`, then Blacklist, Filters and Notes; also press Main. Check Main shows `In private` for
the three cells and each page is the note with Open in private. Press Open in private and confirm it opens the private Main
(not the list page; the deep link carries only the group, A19-A9), then tap the cell.

**Record:** no word, keyword, name or count anywhere in the group: yes / no. Open in private worked: yes / no.

### 7. The six changed commands in a real group

**Why:** the six changes above are covered by tests; their texts are not read by a native speaker and the fail-closed paths need
a real database fault.

**Steps:** `/addblacklist a`, `/addblacklist a hello` (one word refused, one stored), `/filter a` as a reply, `/save "two words"` and
`/save #x` as a reply, and `/blacklistaction ban` followed by `/addblacklist newword` (the new word shows Ban on the action
screen, not Mixed). If you can break the database briefly on a test deployment, run `/filter k` and `/save n` as replies and
check the failed-save text and that nothing is stored.

**Record:** each refused with the key named above: yes / no. New word took Ban: yes / no. Fail-closed answers: yes / no / not tried.

### 8. The Blacklist action screen: mixed, Delete only and an inert stored value

**Why:** a group can hold words on different actions (words added before A19-3, or a database edit). The screen draws `Action: X`
only when every word agrees, else `Mixed` with a count line per stored value, and draws a stored `tban`, `tmute` or `delete` as
stored with an inert line (the watcher only deletes for those).

**Steps:** in a test group set one word to Warn and another to Ban through the database, open the action screen (Mixed, two count
lines). Press Mute: every word becomes Mute and the screen shows `Action: Mute`. Set a word to `tban` by hand and check the inert
line and that no choice is marked. Open Set action on an empty list (no button) and press a stale `bs` on an emptied list (the
notice).

**Record:** Mixed with counts: yes / no. Press set all words: yes / no. Inert line: yes / no.

### 9. A native-speaker read of the 67 new strings and the three command keys

**Why:** the executors translated the 67 new `settings_` keys (2 + 17 + 16 + 4 + 14 + 14, the locale files now hold 452) and
`blacklists_blacklist_word_too_broad`, `filters_keyword_too_broad` and `notes_name_invalid` in es, fr, hi, id, pt, ru and uk under
the ceilings; none was read by a native speaker.

**Steps:** have a speaker of each language read, in this order: the three page names (`settings_page_blacklist`,
`settings_page_filters`, `settings_page_notes`; "Blacklist" should be the term the bot's own help uses), `settings_bl_opt_none`
("Delete only" must say the message is deleted and nothing else happens, not "Off" or "None"), the too-broad sentences
(`settings_bl_wait_rules`, `settings_flt_wait_keyword_rules`, `blacklists_blacklist_word_too_broad`, `filters_keyword_too_broad`:
a reason, not an error), `settings_entries_group_note` (it says members never see the lists; it must not sound like the list is
secret from admins), `settings_entries_part_*` (the grammar of "the wait for the {part} ended" in ru, uk, hi), the Add and Set
action button labels (at most 20 code points with the icon), and `settings_bl_action_count` and `settings_flt_count` (colon
and spacing, fr puts a space before `:`).

**Record:** per language, per group of keys, ok / reword: _____

### 10. The owner confirmations A19-1 to A19-11 (research), as built

Each was recommended at planning and is in the code. Confirm or change; what changes if declined:

- **A19-1** The pages read each list fresh, chat-scoped and with the error returned, not through the commands' cached readers
  (which hide errors and, for filters and notes, carry no row ID). If declined: the commands' cached getters would have to carry IDs
  (a cached value type change during a rolling deploy). _____
- **A19-2** One-code-point words and keywords are refused (`/addblacklist` and `/filter`, commands 1 and 2 above). If declined:
  revert the two commits; the pages then accept what the commands accept and the prompts drop the refused-character sentence. _____
- **A19-3** A new blacklist word takes the shared action (command 3 above). If declined: revert; the hint's English changes (above). _____
- **A19-4** The note name rule for `/save` (command 4). If declined: revert; the page accepts a name with spaces, which can never be
  fetched, and the `w` bound becomes Telegram's own 4096 limit. _____
- **A19-5** Main shows the three counts in a private menu only, through the `PrivateOnly` hook, and `In private` in a group menu
  without calling the page. If declined: the fixed word in both menus. _____
- **A19-6** The step-1 keyword or name (`w`, at most 100 code points) and its message ID (`q`) live in the existing wait hash for
  at most the wait's ten minutes, never logged and never in callback data. If declined: a token and a second key are needed. _____
- **A19-7** An existing keyword or name is refused from the page, never overwritten (at step 1, and again at step 2); the commands
  keep their overwrite card; editing content from the menu is out of scope. _____
- **A19-8** Main keeps the flat two-column grid at 18 cells: 9 rows + Access + navigation, 11 (owner) / 10 (admin) (checks 2). _____
- **A19-9** Every delete asks first behind the two-tap card naming the entry, and a cancelled card returns to the page it came
  from. If declined for blacklist words (cheap to re-add): a table change. _____
- **A19-10** The three lists are private-menu only (D-01). _____
- **A19-11** `none` is labelled `Delete only`, because the message is deleted and nothing else happens (the watcher deletes first). _____

### 11. The flagged UI-SPEC points

The UI-SPEC chose a default for each; they were flagged for you. Confirm or change:

- **A19-A1** Main keeps the flat grid at 18 cells (11 / 10 rows). Neither fallback hub is built (D-23 for Phase 18's "More" hub and
  this phase's `📚 Lists` hub that would put the three pages under one cell, Main 16 cells, 8 rows + Access + navigation = 10 / 9);
  both are table changes. _____
- **A19-A3** Private-only lists through the `PrivateOnly` hook (the fallback needed no framework edit but ran page code in a
  group menu). _____
- **A19-A4** The delete card names the entry (escaped, cut at 100 code points) because a numbered button alone does not say what
  it deletes. _____
- **A19-A5** The Blacklist action has its own screen behind `⚡ Set action`, with `Mixed` counts, `Delete only` and no Action button
  on an empty list. Alternative: the five choices on the list page itself. _____
- **A19-A7** An existing keyword or name is refused on the page (A19-7). _____
- **A19-A9** The group note's Open in private opens the private Main, not the list page itself; a page-specific deep link would need a
  new payload. Without a bot username the row is omitted. _____
- **A19-A11** The command tightening (A19-2, A19-4, commands 1, 2 and 4 above). _____
- **A19-A17** `settings_help_msg` is unchanged: it does not name the pages added since Phase 16, the three list pages included
  (check 20 of Phase 18 stays open). One line in 8 locale files plus `make generate-docs` adds it. _____

### 12. The planning-level choices and deviations worth knowing

These amend the plans or narrow the contract; they were accepted at planning or execution and are in the code:

- **The framework edits were their own plan (19-02)**: `PrivateOnly` (M39), `settingsCardRowsTo` (M40), the ninth right
  `settingsRightRestrictBotRestrict` and the shared bot halves (M42).
- **The pager is `bp` and Add is `bn`** (19-03): the UI-SPEC's `bg` and `ba` collide with the Deletion page's bot row codes; there
  are 17 list action codes in all (the plans said sixteen), `bp bx by bo bs bn bc`, `ig ix iy ia ic`, `eg ex ey ea ec`.
- **Ten apply functions, not seven**: the three Blacklist writes plus `applyBlacklistAction`, three for Filters and three for Notes.
- **The Notes page's right is `settingsRightChangeInfoBotAdmin`, Filters' `settingsRightChangeInfo`, Blacklist's the ninth
  right**; viewing, paging and Cancel need only the live admin check.
- **A refused typed extraction is `DataType == -1`** (19-06), not an `ErrorMsg`, as `/filter` reads it; the two-step engine's
  `Apply` takes the sender's translator (19-07).
- **The two-step engine is proved on a test-only page** installed and removed by `withTestSettingsList` (19-06), and the M41 store
  layer (the advance and set-prompt scripts) arrived in 19-04's last task with its production caller in 19-06.
- **`/filter` and `/save` fail closed on a failed lookup**, each in its own revertable commit (19-05, 19-08); the private-flag
  conflict of a note is one shared helper `resolveNotePrivateConflict`, and the page's saved notice repeats `/save`'s warning.
- **The phase added about 62 s to the whole `fuku/modules` run** by the plans' own reports (19-01 2.2, 19-02 2.5, 19-03 16,
  19-04 4.5, 19-05 4.5, 19-06 1, 19-07 9, 19-08 4.8, 19-09 9, 19-10 6, 19-11 2.3), against D-12's budget of about 40 s. The
  overshoot is the 0.3 s of a real settings environment per test (about 60 tests) and about 1 s each for the tests that install
  the real locale files; no new test is over 3 s.

**Record:** confirmed / changed, per item: _____

### 13. Follow-ups that stay out of scope

None of these was fixed (the goldens pin the commands as they are); each is a small separate change if you want it:

- **Unescaped HTML in command replies** (research C-10): `filters_added_success` (the keyword in `<code>`), `filters_removed_success`,
  `notes_save_success` (the name three times), `notes_removed_success` and the already-blacklisted list splice user text unescaped.
  The pages escape every user text.
- **`/save` as a photo caption panics** (`slice bounds out of range`, `content.go` reads `msg.Text` only) and `/filter` hides the
  same panic behind its recover. The pages' typed extraction is the reply-form one and does not have it.
- **The two-word reply quirk** (research C-10, measured): `/filter hello world` replying to a text stores the text `world`. The
  pages use the reply-form extraction without it.
- **The help text says the default blacklist mode is `none`** ("will just delete the messages") while the code stores `warn`
  (research C-02 d). With A19-3 a new word takes the shared action, else `warn`; the help text is unchanged.
- **`/blacklistaction <action>` on an empty list** writes nothing and still answers its success text (pinned by the golden).
- **`/clear` replies `notes_removed_success` even when the quoted form matched nothing**, and a lost insert race of `/filter` and
  `/save` is answered as a success.
- **Paging is in Go over a fresh unpaged read** (research A1): if a very long list ever makes a draw slow, the fallback is
  `ORDER BY lower(text), id LIMIT 8 OFFSET n` plus a count with the same screens.
- **`RemoveAllBlacklist`, `/stopall` and `/clearall`** stay outside the apply functions (they are not page controls).
- **Rolling deploy** (research Pitfall 6): a list press on a replica still on Phase 18 code is `common_callback_invalid_request`,
  and a typed list reply that lands on such a replica passes through unanswered while the wait stays live for new replicas;
  `/cancel` works on old code. Accepted, like the earlier phases' caveats.
- **The repository's `[Database] AddBlacklist` error lines** still carry the database error text, which on PostgreSQL can quote a word.

### 14. Accepted limits found while building

Read these as the known edges; none needs a decision unless you want it changed.

- **A word, keyword or reply that starts with a command (`/` and a non-space) cannot be added from the page** (the command can);
  a content message handled before the step-1 message's advance completes is judged as a step-1 keyword (two messages in the same
  instant): rare, visible to you, accepted.
- **A long text the client splits** into several messages is handled message by message: the first part that claims the wait is
  stored, the others are ignored without a reply.
- **Simultaneous adds may exceed the 150-filter cap by one or two**, as with the command; the cap is re-checked at step 2.
- **The Filters and Notes pages refuse an existing name instead of offering the commands' overwrite**; to change a stored filter
  or note delete it from the page first, or use the command.
- **A row whose ID has more than 17 digits** is drawn without its delete button and counted in the page's omitted total (no real
  table gets there); the delete Yes is exactly 64 bytes at a 17-digit ID.
- **Opening a list needs only the live admin check**, which is looser than `/blacklists` and `/filters` (they need the bot as an
  administrator); viewing writes nothing and a group menu shows nothing.
- **No spike result is assumed anywhere in the code or the requirements**: PAGE-07, PAGE-08 and PAGE-09 are complete on the
  automatic gate (the end-to-end test of plan 19-11 and the phase gate of plan 19-12).

### 15. CI: lint, the full test run and the PostgreSQL integrity target

**Why:** `make lint` cannot run in the build sandbox (the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0) and
the `fuku/modules` harness does not run on PostgreSQL. CI covers both. This phase adds one PostgreSQL guarantee,
`TestBlacklistActionValuesOnPostgres`, in `make test-postgres-integrity`. The plans ran a lint stand-in (`dupl`, `gocyclo`,
`godox`, staticcheck and errcheck on the added lines) that exits 0, but not govet's `inline` analyzer or gosec.

**Steps:** after the branch is pushed, read the CI run: `make lint` (including `dupl`, `gocyclo`, `godox` and govet's `inline`
analyzer), gosec, `make test` (the `fuku/modules` binary measured 1730 s to 1970 s here, against the 40 minute per-package limit)
and `make test-postgres-integrity`.

**Record:** green / findings: _____
