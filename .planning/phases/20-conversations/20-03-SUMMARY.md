# 20-03 Summary: Spike tool and persona check sheet

- `fuku/utils/chatprompt`: the request builder moved into a leaf (deviation in layout from PLAN.md 5.1, which put it in `fuku/modules/chat_prompt.go`): the spike must use the real builder and cannot import `fuku/modules`. `chat_prompt.go` in plan 20-06 will call it with the group's data.
- `scripts/chat-spike`: questions file in, question, reply, outcome, tokens and latency out, for either provider; tested against `httptest`.
- `20-PERSONA-CHECK.md` with 43 questions in Ukrainian, English and Indonesian, and `scripts/chat-spike/persona-check.txt`, held in step by a test.
- The owner can now run the check with real keys while the rest is built.
