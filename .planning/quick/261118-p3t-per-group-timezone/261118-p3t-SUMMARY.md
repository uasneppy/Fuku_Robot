---
type: quick
id: 261118-p3t
status: complete
---
# Summary: per-group timezone

- Migration 20261118120000 `chat_timezones`, model `ChatTimezone`, repository `fuku/db/timezone` (cached `chat_tz:<chat>`).
- `DEFAULT_TIMEZONE` (default Europe/Kyiv, invalid stops startup); `time/tzdata` embedded from `fuku/config` (deviation: the plan said main.go; config reads the variable before main's imports are guaranteed to be initialised).
- `chatClock` and `formatChatTime` (`chat_clock.go`); display sites converted: lockdown status, alerts, reminders, footers and cards, Staff Group panel and history, settings refresh and wait prompts, captcha held messages, approval date, export caption. The literal UTC left three locale strings in 8 files.
- `/timezone` and `applyTimezone` (`timezone.go`); Timezone page `z` (`settings_timezone.go`), presets, Use default, Type another through a new single-value branch of the typed-reply door (`settingsTypedPages`).
- Settings help mentions /timezone and the Reactions page in 8 languages.
- Not converted: the last-seen date of the staff ambiguous-username list (a date in UTC, no chat at hand).
