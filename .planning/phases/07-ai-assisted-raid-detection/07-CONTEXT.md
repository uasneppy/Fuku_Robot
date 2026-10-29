# Phase 7: AI-Assisted Raid Detection - Context

**Gathered:** 2026-10-29
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous); the owner accepted every recommended answer. The Gemini spike gate was deferred by the owner ("Plan now, spike later").

<domain>
## Phase Boundary

AI helps the rules without ever deciding alone. Borderline text is checked with the existing TypeSafe check, borderline photos, GIFs and stickers with Gemini (`gemini-3.5-flash-lite`, owner's key). A burst of AI spam verdicts in a short window starts the same lockdown as `/lockdown`, `join_surge` and `flood`. The trigger is on by default and an admin can turn it off or change its threshold.

Requirements: RAID-03, RAID-04, RAID-05, PLAT-02.

Out of scope:
- Deleting or banning on an AI verdict from the raid path. The lockdown stops a raid, as in Phase 6 (D-08). The existing `/aispam` deletion stays exactly as it is.
- The settings menu (Phase 9) and the web captcha (Phase 8).
- Running the Gemini spike. It needs the owner's key and is deferred; this phase ships the kit the owner runs (D-17).
- Multi-frame sampling of animated media, which the spike decides.

</domain>

<decisions>
## Implementation Decisions

### What is sent to AI and what counts as a spam verdict (RAID-04, RAID-05)
- **D-01:** "Borderline" means: the item comes from a **new member** (joined this group in the last 24 h, the `alita:raid:{<chat>}:joined` set the flood counter uses) and a cheap rule also finds it suspicious. For text the rule is a link, a forward, or text the same sender repeated. A photo, GIF or sticker from a new member is always a candidate. Messages from established members are never sent to any AI.
- **D-02:** Never sent: admins, approved users, anonymous or channel senders (`sender_chat`), automatic forwards, Telegram service IDs, the bot, members with a pending captcha. Supergroups only, like the other detectors. Checks run cheapest first and the candidate test happens before any AI call.
- **D-03:** The existing `/aispam` stays unchanged: per-chat opt-in, deletes at 0.8 (English) or 0.9. Its delete verdicts also feed the burst counter. For a text `/aispam` already judged, the raid path reuses that verdict, so there is no second TypeSafe call. The raid path itself never deletes, restricts or bans.
- **D-04:** A **spam verdict** is: text, a TypeSafe delete probability at the `/aispam` bars (0.8 English, 0.9 other); image, a Gemini `spam_probability` of at least 0.85, a code constant to tune after the owner's spike. Anything slow, failed, unreadable or not configured is "no verdict" and counts for nothing.

### The burst rule and "AI never decides alone" (RAID-03, RAID-05)
- **D-05:** Default: at least **5 spam verdicts from at least 3 different accounts within a fixed 120 s window**. The threshold is 3..500 like the other triggers and the window and account floor are code constants. One spammer posting five spam messages never locks a group. On by default in every group.
- **D-06:** Counting is Redis-only, like the join and flood counters: one Lua script per chat counts each message once (member = message ID) and each account once, trims to the window, caps the sets and sets TTLs; the chat ID is the hash tag. Without Redis the trigger does nothing and logs once. A count never bans, restricts or deletes.
- **D-07:** AI cannot stall or decide alone. Checks run in a bounded worker queue off the update path, so no handler waits on AI, and a full queue sheds the check. Every failure, timeout, missing key or open circuit breaker means "no verdict". The join and flood triggers never depend on AI. One verdict only adds to a count, and only the rule (verdicts ≥ threshold from ≥ 3 accounts in the window) fires.
- **D-08:** A firing calls the shared `raidFire` with `trigger_kind` `ai_burst` (8 characters; the column is 16, no CHECK), the verdict count and the window in the existing `trigger_count` and `trigger_window`, and `trigger_percent` 0. Exactly one lockdown and one alert still come from `uk_chat_lockdowns_active`, `lockdown.Start` and `BeginAlert`; the fire latch, the active check and the 10-minute quiet period apply unchanged.
- **D-09:** The alert line reads "AI spam: N spam verdicts in 120 s" (`lockdownTriggerSentence`, from the trigger columns, never from `reason`). The accounts behind the counted verdicts are saved as the alert's burst (a snapshot like Phase 6's flooders, D-17 there), so the sample lists them and "Ban N recent joiners" can ban them, with the usual Phase 5 exclusions.

### The Gemini integration (RAID-04, PLAT-02)
- **D-10:** Plain REST over `net/http` to `generateContent`, in the style of the existing TypeSafe client: no new dependency, the endpoint is a swappable variable so tests use an `httptest` server. The key goes in the `x-goog-api-key` header, never the URL.
- **D-11:** Config: `GEMINI_API_KEY` registered with `logredact.RegisterSecret` (PLAT-02) and `GEMINI_MODEL`, default `gemini-3.5-flash-lite`. Inert without the key. Errors are wrapped so neither the key nor the Telegram file URL (it holds the bot token) is ever logged, including debug output.
- **D-12:** Media bytes: a photo, the size nearest 1024 px; a static sticker, its WebP as is; a GIF, video, animated sticker or video sticker, Telegram's own JPEG thumbnail (one frame) because multi-frame sampling is unproven; files over 5 MB are skipped. Bytes are downloaded through `getFile` off the update path and sent inline; nothing but the image goes to Google.
- **D-13:** Request shape: code-owned instructions, a structured JSON answer (`spam_probability` 0..1 and a category enum), temperature 0, 8 s timeout, one retry on 429 within 2 s, and a per-chat circuit breaker like `/aispam`'s. Workers are bounded and share the queue with the text checks.

### Settings, the spike kit and observability (RAID-06 extended, RAID-03)
- **D-14:** `/autolockdown ai [N|on|off]` with the same grammar, live `getChatMember` authority and anonymous-admin proof as `joins` and `flood`. Two new columns on `chat_raid_detection` (`ai_enabled` default true, `ai_threshold` default 5, CHECK 3..500) in an append-only migration; the write is a column-targeted upsert followed by `cache.DeleteCache`; the key stays in `skipLocal`. Not part of backup, export, import or reset. `/autolockdown` alone shows the AI line, and says "AI checks are not set up on this bot" when the server has no keys. Authority check text for 'AI' ships in all 7 locales.
- **D-15:** Counters for checked, spam, kept, shed and failed feed the developer `/stats`, like `/aispam`. No message text or image bytes are logged. The docs say that suspicious images from new members go to Google Gemini and suspicious text to TypeSafe.
- **D-16:** The Gemini client, the candidate rules and the verdict counter sit behind interfaces or swappable variables, with hand-written fakes in tests (no mock libraries). The AI path is inert without keys and the whole phase passes `make test` with no network.
- **D-17 (spike deferred, owner's choice):** The Gemini spike gate is not run in this phase. Instead it ships `scripts/gemini-spike/` (a standalone Go tool that reads a folder of labelled sample images, calls the same client code with the owner's key, and prints accuracy, latency, tokens and an estimated cost) and `07-SPIKE.md` (what to measure and which constants to tune: the 0.85 bar, the thumbnail choice, the model). The tool is not part of `make test` and calls nothing without a key.

### Claude's Discretion
- The rule for "repeated text" (exact match against the sender's recent texts, reusing `observeAISpamSender`'s history) and the exact link and forward tests.
- Redis key names (`alita:raid:{<chat>}:ai...`), TTLs, caps and the snapshot key for the spammers.
- Whether the Gemini and TypeSafe checks share one queue type or two, how the breaker is shared, and the worker counts.
- Exact wording of the not-set-up notice, the alert line and the docs note, in all 7 locales.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **TypeSafe Jev client:** `alita/modules/aispam_jev.go` (swappable endpoint and HTTP client, timeout, one retry, verdict with `DeleteProbability`, `Language`) and `alita/modules/aispam.go` (bounded queue and workers, `aispamEnqueue`, per-chat breaker, `observeAISpamSender` history, `DrainAISpamChecks`). `aispam.IsAISpamEnabled` is opt-in, missing row = off.
- **Raid machinery (Phase 6):** `raid_flood.go` (`raidFloodCountUpdate`, the `:joined` 24 h set, the flood scripts and flooders snapshot), `raid_trigger.go` (`raidFire`, `raidTrigger`, `lockdownTriggerSentence`, `raidPostAsync`), `raid_counters.go` (`raidKey`, Redis helpers), `autolockdown.go` (grammar, live authority), `alita/db/raiddetection` (settings, `skipLocal`), `chat_lockdowns.trigger_*` columns.
- **Alert and burst:** `lockdownBurstSnapshot` and `lockdownMergeFlooders` already merge the flooder snapshot with the 10-minute tracker; the AI spammer snapshot follows the same pattern.
- **Secrets and shutdown:** `logredact.RegisterSecret`, `DrainAISpamChecks` and the shutdown manager's LIFO drains.

### Established Patterns
- Watchers return `ext.ContinueGroups`; heavy work runs off the update path in recovered, counted goroutines; Redis writes are best effort under 250 ms; counters are Lua scripts keyed by `{chat}`.
- Exactly-once side effects come from conditional database writes, never Redis latches.
- Settings columns are column-targeted upserts with no GORM `default:` on booleans, followed by `DeleteCache`.
- Migrations are append-only and timestamped; every new column goes into the test harness `AutoMigrate` lists; new locale keys go in all 7 files.

### Integration Points
- The flood watchers (group `-4`, `12`) or a new candidate step see each message and decide the AI candidate; the group `3` `checkAISpam` shares the text verdict.
- A new verdict counter fires `raidFire` with `ai_burst`; `lockdownTriggerSentence` and the alert trigger line learn the new kind.
- `/autolockdown` gains the `ai` word; config gains `GEMINI_API_KEY` and `GEMINI_MODEL`; `main.go` registers a drain for the new workers after DB-close; developer `/stats` gains the AI counters.

</code_context>

<specifics>
## Specific Ideas

- The alert's trigger line should read like "AI spam: 7 spam verdicts in 120 s".
- Owner decisions carried in: Gemini `gemini-3.5-flash-lite` with the owner's key for images, TypeSafe for text, rules decide when to lock, triggers on by default with conservative thresholds.
- Fail open everywhere: if an AI provider is slow, down or not configured, the rule-based triggers keep working and nothing waits on AI.

</specifics>

<deferred>
## Deferred Ideas

- Running the Gemini spike (accuracy, latency, cost, animated-media sampling) with the owner's key; the kit in D-17 is the way to do it.
- Multi-frame sampling of GIFs and video stickers (needs ffmpeg or a Lottie renderer).
- A per-group model or bar setting; the bars are code constants for v1.
- Counting verdicts from established members or using AI on ordinary messages. Not wanted: too slow and costly.

</deferred>
