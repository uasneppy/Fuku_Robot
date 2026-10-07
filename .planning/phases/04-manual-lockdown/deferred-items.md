# Deferred items, phase 04-manual-lockdown

## From plan 04-02

- **Anonymous-admin re-entry loses the chat for shared checks (likely pre-existing, outside this phase).**
  `verifyAnonymousAdmin` (`alita/modules/bot_updates.go`) sets `ctx.CallbackQuery = nil`, which clears the callback
  query of the embedded `*gotgbot.Update`. After that `chat_status.extractChatFromContext` finds no chat, so
  `helpers.RequireGroup()` returns false and `PermissionResponder.Respond` drops its reply, both silently. Every command
  that lists `RequireGroup` in its checks and re-enters through `anonPipelineHandler` (`ban`, `dban`, `sban`, `tban`,
  `unban`, `skick`, `restrict`, `unrestrict` in `alita/modules/bans.go`) should therefore do nothing after the proof.
  Proven only for the lockdown commands (debug output showed `RequireGroup` returning false after the proof); a ban
  flow was not run. Plan 04-02 worked around it inside the lockdown commands (`requireLockdownGroup`,
  `lockdownRefuse`). Making `extractChatFromContext` fall back to `ctx.EffectiveChat` fixes the flow but breaks
  `TestUnapproveAllCallbackCancelInvalidAndUnavailableMessage` (a callback with no message must find no chat), so a
  fix needs that test's expectation reviewed or a narrower fallback.
