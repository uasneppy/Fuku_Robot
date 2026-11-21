# 20-02 Summary: Provider clients

- `claude.Client.Converse` (`fuku/utils/claude/converse.go`): two system blocks (static with `cache_control` ephemeral, dynamic after), one user message, `output_config.effort` low, no thinking, sampling, tools or prefill; its own 40 s transport, one retry for a wait of at most 2 s; the server-side fallback only for `ServerFallback` models. `exec`/`send` now take `sendOptions` (transport, beta header); the classification calls pass the zero value and are unchanged.
- `fuku/utils/gemini`: new leaf, `Generate`, `HTTPError` (retryable, key rejected, RetryInfo, per-day quota), tolerant parser.
- Tests against `httptest` only: request shape and banned fields, the byte-identical static block across chats, every stop and finish reason, refusal, truncation, blank answer, retry, no redirect, the key never in an error, leaf. golangci-lint: 0 issues.
- Not settled without a key (owner, 20-PERSONA-CHECK.md and GEMINI-API.md): lowercase vs uppercase `thinkingLevel`, whether `gemini-3.5-flash-lite` accepts `low`, the blocked-prompt JSON shape (both documented shapes handled).
