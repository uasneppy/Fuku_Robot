---
type: quick
id: 261118-p4s
status: complete
---
# Summary: formatting sample batch

- Reworded in Ukrainian and English: help_pm_intro, help_all_commands_usage, helpers_module_help_header, bans_help_msg,
  locks_help_msg, reactions_help_msg, lockdown_help_msg, bans_ban_normal_ban, bans_ban_tban, mutes_mute_message,
  mutes_tmute_message, mutes_reason_suffix, warns_user_warning, warns_warning_reason.
- Skipped because TestLegacyLockRequests pins them: locks_locked_successfully, locks_unlocked_successfully (the /lock and
  /unlock replies).
- A help page with its own bold title gets no generic header; the header shows the module's icon and translated name.
- Style guide and glossary: docs/src/content/docs/contributing/locale-style.md.
- TestLocaleStyle with an allow-list of 234 uk/en keys not yet converted.
- Waiting for the owner's approval of the sample before Part 4 step 3.
