# Phase 20: conversations, live checks and test-group walk-through

This file is for you, the owner. It lists what needs your keys, a live Telegram client or a real group. **None of it
blocks the phase**: the code is complete and its automatic gate passes without any of it. Record your answers here.

Nothing here changes whether the code is safe. Conversations are off on the whole server until `ENABLE_CHAT=true`, and
then off in every group until that group's creator turns them on from the bot's private chat. A provider that fails,
refuses or is not configured never makes the bot post anything but one canned line, at most once in five minutes per group.

## 1. Persona check (both providers)

Run `20-PERSONA-CHECK.md` with `scripts/chat-spike` on Anthropic (`claude-haiku-5-5`, the default) and on Gemini
(`gemini-4-hydrogen`). It decides whether Haiku holds the character and the canon rules. If it does not, tell me rather than
changing the default (plan 12.5). Write down the token counts the tool prints: section 2 needs them.

- [ ] Anthropic run recorded
- [ ] Gemini run recorded (paid-tier key)

## 2. Recompute the cost table

The table on `docs/.../api-reference/environment.md` is the planning estimate, labelled as such. Replace it with the
numbers from your runs (input, cached and output tokens per reply) before you publish the docs.

- [ ] Table recomputed or kept as an estimate on purpose

## 3. Gemini wire details (from `GEMINI-API.md`, unverified)

The client is built to tolerate both answers; your key settles them.

- [ ] `thinkingLevel` lowercase `low` is accepted (try `-thinking-level LOW` if not)
- [ ] the blocked-prompt answer arrives in a shape the parser reads as refused (ask something the safety filter blocks)

## 4. Test-group walk-through

On a test supergroup where the bot is an administrator, with `ENABLE_CHAT=true` and a key set:

- [ ] `/settings` in private, open the group, Conversations: only you (the creator) see the row; an administrator does not.
- [ ] Turn it on: the card shows the data notice; Yes turns it on. Off is one tap.
- [ ] Mention the bot: one short reply, as a reply, in your language. Reply to that answer: it carries on the conversation.
- [ ] Reply to a ban notice or a welcome: no answer.
- [ ] `/ask what is tempe?` and `/xeno status`: answered. `/ask` alone: the usage line. `/ask` in private: the private line.
- [ ] Post a blacklisted word with a mention: the message is deleted and gets no answer.
- [ ] Audience "Admins and approved": a plain member gets nothing, an approved member is answered.
- [ ] Ask for a link or to ping someone: links are replaced by `[link removed]`, `@names` do not notify anyone.
- [ ] Ask it to ban someone: it says it cannot, and nothing happens.
- [ ] Twelve questions in one hour from one account: the thirteenth gets the allowance line once.
- [ ] Turn it off: the next mention gets nothing; turning it on again starts with no memory.

## 5. Decisions to confirm or change

These were built as the plan's section 12 decided, plus three changes the code required. Each is a small change.

1. A reply is never sent without its target (`allow_sending_without_reply` false), where the plan said true. The blacklist
   and the locks delete a message and still pass it on, so this is what keeps the bot from answering a deleted message.
   Cost: when a member deletes their own question before the answer, the answer is dropped too.
2. aispam judges asynchronously, so a spam message that also mentions the bot can be answered if aispam deletes it only
   after the reply. Accepted as a limit.
3. The request builder lives in the leaf package `fuku/utils/chatprompt` (not in `fuku/modules/chat_prompt.go` alone) so
   that `scripts/chat-spike` measures exactly what the bot sends.
4. The Conversations page is not part of `TestSettingsGateAllDoors`: that test needs every door to admit the administrators
   the owner lets in, and this page admits the creator alone. Its own authority tests cover it.
