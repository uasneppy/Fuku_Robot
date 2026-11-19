# 20-06 Summary: runtime

- `chat.go` (module `Chat`, priority 245), `chat_watch.go` (watcher at group 13), `chat_run.go` (pool, guards, breakers,
  counters, `StopChat`), `chat_history.go` (transcript and own-message set), `chat_prompt.go` (provider interface, two
  adapters, request), `chat_send.go` (filter, pacer, reply, canned lines). Canned lines in all 8 locales. `StopChat` in
  `main.go` after the AI spam drain; `/stats` gets a Conversations line.
- Verified first, through the real dispatcher with every module loaded: a mention in a text or a caption and a reply to the
  bot reach group 13 (`TestChatWatcherGroupReachedByMention`).
- Finding and deviation: the blacklist (and the locks, aispam, antiflood) delete a message and pass the update on, so group
  order alone does not stop the bot answering a deleted message. The reply is sent with `AllowSendingWithoutReply` false
  (plan 3.2 said true), so Telegram refuses a reply to a deleted message and the bot stays silent
  (`TestChatDeletedMessageGetsNoReply`). Known limit: aispam judges asynchronously, so a spam message that also mentions the
  bot can be answered if aispam deletes it after the reply.
- Tests: answers (mention, caption, reply to own message, history carried), ignores (unaddressed with no I/O, another bot's
  mention, a command, a reply to a moderation notice, private chat, bot, anonymous admin, channel, feature off), audience,
  guards (cooldown, budgets with the gated line, breaker with the gated line, refusal, blank answer, rejected key, no Redis,
  ENABLE_CHAT off), output filter, shutdown, both real adapters against httptest (no IDs in the body, same static block),
  log sweep. Mutation checks confirmed the reply check, the slash check, the reply-target flag and the log sweep are tested.
- Verified: chat tests pass with -race, lint clean, translations and docs checks clean, shard check clean.
