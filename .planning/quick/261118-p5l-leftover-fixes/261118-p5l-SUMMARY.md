---
type: quick
id: 261118-p5l
status: complete
---
# Summary: leftover fixes after PR #18

1. Done (9980a5a). The postInit test now stops the deletion worker and the admin index sweeper too; test-only race.
2. Done (ceb5966). Joined in uk/en: antiflood_setflood_disabled, bans_unban_unbanned_user, mutes_unmute_message,
   rules_need_text, notes_private_enabled, notes_private_disabled. TestLocaleStyle fails the old shape (a short opener of
   at most four words, then the fact alone on line 2). Left: filters_added_success, filters_removed_success
   (TestLegacyFilterRequests pins them; listed in localeStyleGoldenPinned).
3. Done (3a029de). /disabledel answers common_settings_save_failed on a failed save.
4. Done (c6d6f34). The ambiguous-username refusal shows last-seen dates in the Staff Group's zone with its label.
5. Done (ef11de0). formatting.Smarkdown and the Markdown constant removed.
6. Not done: TestLegacyLockRequests pins the `/locks` listing (" - url = true").
7. Not done: TestLegacyWarnSettingsRequests case mode_unknown_unescaped pins the unescaped value.
8. Done: the STATE.md placeholder "(this commit)" (row 261118-p4r; the p4s row already named daa6b1d) now names 890f1a8.
