# 20-04 Summary: storage and apply functions

- Migration `20261119120000_add_chat_conversations.sql`: `chat_conversations` (enabled, provider, audience), inline checks.
- `fuku/db/chatconv`: `Get` (cached, `chat_conv` key in `skipLocal`), `GetFresh`, three column-targeted upserts.
- `fuku/modules/chat_settings.go`: `applyChatEnabled`, `applyChatProvider`, `applyChatAudience` behind `chatConvWrites`;
  Off deletes the transcript and the message-ID set.
- Kept out of backup (`TestChatConversationsStayOutOfBackup`); PostgreSQL guarantees added to `test-postgres-integrity`.
