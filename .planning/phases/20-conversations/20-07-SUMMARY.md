# 20-07 Summary: /ask, help, locales, docs

- `/ask <text>` and `/xeno` through `helpers.WrapCommand`, both disableable (`/xeno` honours `ask` too). Private chat:
  `chat_private_refused`; conversations off: `chat_off_hint`; no text: `chat_ask_usage`; otherwise the same checks as a
  mention (`chatAccept`, now shared with the watcher).
- Help page `chat_help_msg`, `help_module_name_chat`, icon 🗨; five keys in all 8 locales, EN and UK to the style guide.
- Docs: generated `commands/chat/`, environment page notes (paid-tier Gemini key, supported models, limits, cost estimates
  labelled as unmeasured), handler-groups row for group 13.
- Tests: `TestChatAskCommand` (answer, /xeno, usage, private, disable both, off hint); a mutation removing `/xeno`'s `ask`
  check is caught.
