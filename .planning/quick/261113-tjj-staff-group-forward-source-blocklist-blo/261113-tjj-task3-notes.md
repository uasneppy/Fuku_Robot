# 261113-tjj Task 3 notes (uncommitted progress note)

Commit: b907052 `feat(settings): private-only Blocked sources page for the Staff Group forward blocklist` on claude/ecstatic-goldberg-qsss05. Not pushed. Nothing under .planning/ is in it.

## Built

- `fuku/modules/settings_fwdblock.go`: page `v` (`Order` 11, icon 🚷, `settings_page_fwdblock`) as the fourth table of the list engine. Codes `vg vx vy va vc ve vs`, `CountMax` 200, `Hidden` = `settingsFwdBlockHidden` over the seam `settingsFwdBlockStaffGate` (default `staff.GetStaffGroup(id) != nil`). Reads are the variables `settingsFwdBlockRead` (stamps the list's action on every entry), `settingsFwdBlockEntry`, `settingsFwdBlockCount`, `settingsFwdBlockActionRead`, all over the fresh `staff.*Fresh` repository functions. Delete is `settingsFwdBlockRemove` over `applyForwardBlockRemoveByID`; the typed add is `settingsFwdBlockTyped` over `applyForwardBlockAdd`; the action screen is `renderSettingsFwdBlockAction` (drawn for an empty list too) and `vs` calls `applyForwardBlockAction`.
- `settings_menu.go`: tenth right `settingsRightStaffOwner` (appended after `settingsRightRestrictBotRestrict`), member part: creator allowed, everyone else the alert `staff_fwdblock_owner_only`. No bot half.
- `settings_list.go`: `settingsListDef.Hidden`, passed to the page by `settingsListPage`, plus `settingsListGuardHidden` which wraps the `Run` of every action of a table that has the hook.
- `settings_list_input.go`: part `(v,s)` in `settingsListParts`.
- 13 locale keys in all 8 files (`settings_page_fwdblock`, `settings_fwd_intro|count|empty|delete_ask|removed|wait|wait_rules|action_hint|saved|refuse_one`, `settings_btn_fwd_add`, `settings_entries_part_source`), rows in `settingsLocaleLimits`, the page in the label-length group, `staff_fwdblock_owner_only` in `settingsAlertKeys`, the typed-add keys in `settingsListExtraKeys`.
- Tests: `settings_fwdblock_test.go` (toolkit `newSettingsFwdEnv`; `TestSettingsFwdBlockOpens`, `...MainShape`, `...Add`, `...Delete`, `...Action`, `TestSettingsAuthorityParityFwdBlock`, `TestSettingsParityFwdBlock`, `TestSettingsDriftFwdBlock`) and `settings_fwdblock_screens_test.go` (`TestSettingsListFwdBlockCallbackBytes`, `TestSettingsListFwdBlockPageFitsEveryLocale`, `TestSettingsFwdBlockReadsCreateNothing`, `TestSettingsFwdBlockPageKeys`).
- AGENTS.md: part table (Handlers), Main grid note (Callbacks), a Blocked sources page bullet (Callbacks), the tenth-right bullet with the creator-only reasoning (Permissions), a sentence in the commands bullet (Subsystem traps), a Testing bullet and the updated Phase 19 testing sentence.

## Expected-set edits to existing tests (set edits only, no assertion loosened, no golden touched)

- `settings_list_framework_test.go` `TestSettingsRightBotHalves`: ten rights, the new one with no bot half.
- `settings_edit_hooks_test.go` `TestSettingsRightRefusalForParity`: seven rows for the tenth right (creator allowed; administrator, member, restricted, left, kicked refused with `staff_fwdblock_owner_only`).
- `settings_phase19_test.go`: `TestSettingsPhase19Codes` ranges over `settingsPhase19ListPages` (the three pages plus `v`), seven more codes, six parts, `v` in the registered page keys; `TestSettingsMainPhase19` skips a page whose `Hidden` says so for its chat (the chat is not a Staff Group).
- `settings_phase18_test.go`: registered page keys gain `v`; `TestSettingsPhase18InStaffGroup` now counts nineteen state lines for a Staff Group (Deletion hidden, Blocked sources shown).
- `settings_main_test.go` `TestSettingsMainGrid` end-to-end subtest, `settings_menu_test.go` `TestSettingsNavigation` (two loops) and `settings_test.go` `TestSettingsTracer`: skip a page hidden for the (non-Staff-Group) chat under test.
- `settings_phase19_e2e_test.go` staff group subtest: the new page is added to the pages that open for a Staff Group's own menu and name no Staff Group.
- `settings_list_locale_test.go`: the new typed-add keys in `settingsListExtraKeys`.

## Deviations from the plan

1. `settingsListGuardHidden` (not in the plan): page actions do not consult `Hidden` (`runPageAction` never did), so a forged `va` or `vs` from a creator of a plain group would have reached `Run`. The engine now wraps every action of a table with a `Hidden` hook; the forged-press cases of `TestSettingsFwdBlockOpens` fail without it (checked by mutation).
2. More existing tests needed expected-set edits than the three the plan named (list above), all the same kind: a hidden page is not in a non-Staff-Group chat's Main, and the page count of a Staff Group's Main moves from "every page but Deletion" to the same nineteen lines as any chat.
3. No file under `docs/` changed: `make generate-docs` regenerates nothing for this task (the settings strings are not in the generated docs; the staff help was regenerated in Task 1), so the commit holds no docs file; `make check-docs` passes.
4. `settings_blacklist.go` is not touched (its pre-existing G115 `int64(row.ID)` would have been flagged for this PR); the action screen has its own 12-line `settingsFwdBlockChoiceRow`. The models use `int64` IDs, so the new page has no conversion to guard.
5. `TestSettingsStaffPrivacy` is not edited: it renders no Staff Group's own menu for the list pages. The same privacy fact is held by a sentinel check in the group-menu subtest of `TestSettingsFwdBlockOpens` (no source title, ID or sentinel in any edit, send or answer) and by the page joining the e2e staff group subtest.
6. Entry numbering follows the engine's sort by entry text (the ID's text, then the title), while `/fwdblocklist` orders by row ID, so the two lists number differently for the same sources. Documented in AGENTS.md.
7. The page's one-token rule uses `strings.Fields` on the message text or caption; an album caption is handled once by the door's album gate, like the other list pages.

## Commands run (CI env = cienv.sh, `unset FUKU_TEST_DATABASE`, SQLite + miniredis)

- `gofmt -l` on changed .go files: empty. `CGO_ENABLED=0 go build ./...` ok. `go vet -tags testtools ./fuku/modules ./fuku/i18n` ok. `make check-translations`, `make generate-docs`, `make check-docs`: ok, no drift.
- Whole `TestSettings` and `TestLoadSettings` families in the race-built `fuku/modules` test binary, three processes: `^TestSettings[A-L]` (317 top-level names), `^TestSettings[M-R]` (77), `^(TestSettings[S-Z]|TestLoadSettings)` (26): the three together cover the 420 names `go test -list` gives for `./fuku/modules` and `./fuku/i18n`; all three exit 0 after the final edits. (A first run found four failing tests (five subtests), all expected-set cases, fixed as listed above, then a full clean re-run.)
- `go test -race ./fuku/i18n -run '^(TestSettings|TestLoadSettings)'`: ok. `-run '^(TestForwardBlock|TestApplyForwardBlock)' ./fuku/modules`: ok. `./scripts/generate_docs/...`: ok. The new tests bare (`env -i`, no cienv): ok.
- Mutation checks: with the page's right changed to `settingsRightAdmin`, `TestSettingsAuthorityParityFwdBlock` and `TestSettingsFwdBlockPageKeys` fail; with the `Hidden` guard disabled, `TestSettingsFwdBlockOpens` fails (both restored).
- Lint stand-in (`lint-local-fixed.py e713ed5`): no dupl, gocyclo or godox findings. gosec 2.29 JSON run: 18 findings, the same 18 pre-existing ones, none in a file added or changed by Task 3 (`settings_list.go`, `settings_list_input.go`, `settings_menu.go`, `settings_fwdblock.go` and the tests are clean).
