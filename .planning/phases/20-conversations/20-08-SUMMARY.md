# 20-08 Summary: end-to-end test, full checks, owner checklist

- `TestChatEndToEnd`: the owner turns conversations on from the private page (toggle, card, Yes); a member mentions the bot;
  the real Anthropic adapter answers through an httptest stand-in; the reply is posted as a reply; a second member replies to
  it and the next request carries the first exchange.
- `make test`: every package passed (fuku/modules 2,376 s, coverage 79.4%), and the only skips are the allowed PostgreSQL
  ones. The run's gate still reported a failure: `go: no such tool "covdata"` while building coverage for
  `fuku/utils/actionlog`, a package with no test files. The cause is this container's Go install, not the code.
- Lint clean (golangci-lint v2 built locally; `make lint` itself cannot run here), `make test-shard-check`,
  `make check-translations` and `make check-docs` clean.
- Owner checklist: `20-SPIKE.md` (persona check on both providers, cost table, Gemini wire details, test-group walk-through,
  decisions to confirm).
