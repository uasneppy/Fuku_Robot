# 261117-w10t: Wave 10 summary

Branch `wave10-settings`, base `a09b91c`.

## Fixed

| ID | Commit | What |
|----|--------|------|
| T1 | `37b2e46` | `runControl` and `runPageAction` refuse every control and action of a hidden page (`settingsPage.Hidden`) with `common_callback_invalid_request` before the right is judged (`settingsPress.pageHidden`). `settingsListGuardHidden` is removed (folded into the framework check). Tests: a new subtest of `TestSettingsFwdBlockOpens` (a plain administrator's forged write presses in a plain group answered `staff_fwdblock_owner_only` before) and of `TestSettingsDeletionMemberStaffGroup` (an administrator without `can_change_info` in a Staff Group answered `settings_no_change_info` before). AGENTS.md updated (Deletion page, Blocked sources page). |
| T2 | `780a167` | The claim script extends a wait with less than `settingsInputClaimMinTTL` (60 s) left to 60 s, rewrites its `x`, and rewrites the marker (`c|p|t`) to outlive it by the usual 5 minutes. A wait with more time left is untouched. Test: `TestSettingsInputClaimExtendsANearlyExpiredWait`. AGENTS.md (Data, the claim script) updated. |
| T3 | `9a7905d` | `listStepOne`: the step-2 prompt is sent with a request bound of its own (`settingsListStepSendTimeout`, 10 s, through the new `settingsInputReply.sendWithin`) and `m` is recorded on a fresh bounded context detached from the advance's (`settingsInputRedisTimeout`). Before, the record reused the advance's 5 s context, so a slower send left the wait without its step-2 prompt and the prompt's Cancel answered stale. The advance's bound became the variable `settingsListStepAdvanceTimeout` (same value) so the test can shorten it. Test: `TestSettingsListTwoStepSlowPrompt`. AGENTS.md (door bullet) updated. |

## Not reproduced

None.

## Owner decisions

- **T4** `admin_anon_admin_enabled_now` / `admin_anon_admin_disabled_now` are backwards in all 8 locales: mode on means
  `checkAnonAdmin` lets an anonymous admin act with no proof (`fuku/utils/chat_status/chat_status.go`, `if admin.GetAdminSettings(chat.Id).AnonAdmin { return true, true }`),
  yet `_enabled_now` says the bot will now ask anonymous admins to verify, and `_disabled_now` says it will stop asking.
  The Ukrainian bytes are pinned by the golden `TestLegacyAnonAdminRequests` (cases `on_by_creator`, `on_by_creator_no_row`,
  `on_yes`, `on_true`, `off_by_creator`, `off_no`, `off_false`), which must never be re-recorded, so no locale was changed
  (fixing the seven unpinned ones alone would leave the default language wrong and the languages disagreeing).
  Exact difference the owner would approve (uk, the pinned language):
  - `_enabled_now` second line: `Відтепер я проситиму анонімних адмінів підтверджувати права.` →
    `Відтепер анонімні адміни діятимуть без підтвердження прав.`
  - `_disabled_now` second line: `Відтепер я не проситиму анонімних адмінів підтверджувати права.` →
    `Відтепер анонімні адміни мають натиснути кнопку, щоб підтвердити свої права.`
  The same swap in the other seven: en (`From now on, I will ask anonymous admins to verify their permissions.` ↔
  `... I will not ask ...`), es, fr, hi, id, pt, ru each say "will ask" for enabled and "will not ask" for disabled; the
  fix is the opposite meaning in each, mirroring the status texts `admin_anon_admin_enabled` / `_disabled`. Approving it
  means re-recording those seven golden cases, which only the owner may allow.

## Existing tests changed

- `TestSettingsFwdBlockOpens`: one subtest added.
- `TestSettingsDeletionMemberStaffGroup`: one subtest added.
No existing assertion was changed; no golden touched.

## User-visible behaviour changes

- A forged press of a hidden page's control or action (Blocked sources outside a Staff Group, Deletion inside one) now
  always answers the invalid-request alert, whoever presses it, instead of a rights alert for presser without the right.
- A typed reply that claims a wait in its last minute keeps the wait (and its "wait ended" marker) for at least a minute
  more, so a failed save still keeps the wait and a saved one clears its marker.
- The step-2 prompt of a Filters or Notes add is recorded even when its send is slow, so its Cancel button works.

## Checks

Per item the named tests and nearby families (`TestSettingsFwdBlock*`, `TestSettingsDeletion*`, `TestSettingsList*`,
`TestSettingsInput*`, `TestSettingsEditTyped*`, `TestSettingsAnonAdmin*`, `TestLegacyAnonAdminRequests`,
`TestSettingsGateAllDoors`, `TestSettingsControlActionAllowList`, `TestSettingsPhase19Codes`) pass with `-race`;
`CGO_ENABLED=0 go build ./...`, `make check-translations` and golangci-lint (`./...`, 0 issues) pass. `make test` was not
run (shared machine); Wave 11 runs it.
