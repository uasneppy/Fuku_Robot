# Phase 15: AI spam-name check, live checks and test-group walk-through

This file collects the checks that need your Anthropic key, a live Telegram client or a real group. It is for you, the
owner. One tool exists for the measuring checks (`scripts/claude-spike`, names mode, built in plan 15-09); the rest you do by
hand on a test supergroup. Record each answer here.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 14 ("Plan and build now,
check later"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting
values afterwards. **The live Haiku accuracy spike cannot run in this build: it has no Anthropic key.** Nothing here changes
whether the code is safe: for every outcome, an unknown answer, a verdict below the bar, an AI that is not configured, a
missing Redis and an unreachable provider all punish nobody (D-08, D-18). The checks change wording, the bar and your
confidence, not safety. The bar, the budgets and the model are one-line constants or settings to retune afterwards.

This file records no result. The measuring is yours.

## Before you enable Mute, Kick or Ban for the AI rule in a real group

Enforcement is ON in code. The kill switch `memberReqEnforcementLive` has been true by default since plan 13-13, so the
watcher, the join check, the "I fixed it" button and the captcha guard run in every group, and Phase 15 added the AI rule
to all of them. A group still screens nobody until its owner sets a rule, because **every rule defaults to Off** (the
migration adds `ai_action` with the value 0, so every existing group reads the rule Off). Nothing changes anywhere until
someone opens Requirements and picks a punishment for the AI rule.

- **With no `ANTHROPIC_API_KEY` on the bot, or with `ENABLE_AISPAM=false`, the AI rule is off.** The rule's screen says
  "AI not configured", offers `[Off]` alone, and refuses a punishing press. No name leaves the server. This is the state
  of any bot that has not been given a key.
- **Warn and Off need no spike.** Warn goes through the warn module and counts toward the group's warn limit.
- **Before Mute, Kick or Ban:** run checks **1** and **2** below (the false-positive rate on real, non-Latin and business
  names, and the stability of the verdicts). A single false positive on a Mute, Kick or Ban is a real person. **Warn or
  Mute is recommended; Ban is never suggested** for this rule (the page's hint says so).
- **Before Mute:** also run the Phase 13 checks A2 to A5 and A8 of `13-SPIKE.md`; the "I fixed it" button and the mute
  marker rest on them exactly as for the other rules.
- **The model must accept a forced tool call.** Use a Haiku-class model in `ANTHROPIC_MODEL`. Claude Opus 5.5, Sonnet 5.5
  and the Fable 5.1 family answer 400 to a forced tool call; the breaker reads that as a provider failure and pauses the
  checks after five of them, so nobody is judged and nobody is punished. An empty or invalid `ANTHROPIC_MODEL` falls back
  to `claude.DefaultModel` with a warning.
- **What "new member" means.** A new member is one the bot **recorded as joining while the rule is on**: the join entry
  writes a per-chat marker (`fuku:memberreq:nu:<chat>:<user>`, 30 days, Redis, best effort) before its Check-on-join
  branch, whether or not Check on join is on. A new member is judged at the join or, when Check on join is off, at their
  first message. A member who was **already in the group** when you switched the rule on is first recorded (a baseline of
  their names, no call) and is judged only when they **rename** (D-02, D-21). So a spammer who was in the group before the
  switch is judged when they change their name. With **no Redis**, a failed marker write, or a join the bot never saw (a
  captcha failure, a lockdown ban), a real new member is baselined at their first message and gets a free pass until they
  rename. That is the safe direction (UI-CHECK flag F-b), never a punishment.
- **A basic group is not screened** (mute and kick are supergroup methods); the page says so while any rule is on.

## What the automatic tests prove, and what they cannot

The suite fakes Telegram with a hand-written bot client and Anthropic with `httptest` servers or a fake transport; it runs on
SQLite and miniredis (PostgreSQL in CI). It proves the server logic: that the names request carries only the three
canonical strings (`TestClassifyNames*`, with injection names kept as data), that an unknown, a refusal, a paused breaker,
a spent budget, a shed call and a shutdown punish nobody and leave no hold, notice or cached verdict, that one string of
names costs one provider call across callers and replicas, that a rename is judged once and a baseline never judges, that
the held release and "I fixed it" read the cache and never re-roll, that the Requirements page and the press gate refuse a
punishment while the AI is not configured, and that every screen fits 3800 UTF-16 units and 64-byte buttons in 8
languages with no member name, count or Staff Group in them. One test, `TestMemberReqAIEndToEnd`, walks the whole slice
through the real dispatcher with only Telegram and the provider faked (the owner's page press and card Yes, a muted
joiner who renames and taps the button, an established member recorded and then judged on a rename, the AI not
configured, a rejected key and the pause), which is the automatic twin of check 6. It **cannot** prove what the live model says about a real
name, how often it changes its mind, how long and how dear a call is, whether the configured model accepts the request,
how a screen renders on a phone, whether a native speaker finds a translation natural, or how two real processes share
the budgets. Those are the checks below.

## Before you start

1. **An Anthropic API key, kept out of files.** Put it in the environment only (`export ANTHROPIC_API_KEY=...`); the tool
   never takes it as a flag and never prints it. Never paste it into this file, a ticket or a screenshot. Every name in a
   run is sent to Anthropic and billed to the account of that key.
2. **The model.** A Haiku-class model id in `ANTHROPIC_MODEL` (and `-model` for the tool). Check the current list prices
   and pass them with `-price-in` and `-price-out` (dollars per million tokens; the tool's defaults, 0.10 and 0.50, are
   placeholders you must verify).
3. **A labelled names file of at least 200 rows**, UTF-8 CSV with the header `label,first_name,last_name,username` and an
   optional `note` column; `label` is `spam` or `ham`. Aim for:
   - at least **100 legitimate** names, **half non-Latin** (Ukrainian, Russian, Arabic, Hindi, Chinese, Spanish, French),
     plus **30 business or brand accounts that only name themselves** (a shop, a restaurant, a studio: not spam);
   - at least **60 spam** names (adverts, crypto, casino, betting, adult, "write me", a phone number, link or messenger
     handle used as a name, emoji spam, mixed-script, templated botnet names);
   - **10 injection names** (a name that tells the model what to answer, or addresses an AI or a reviewer; the instruction
     treats those as strong spam, so label them `spam`).
   Use real names from your own groups only where you are happy for them to be sent to Anthropic.
4. **The run.** From the repository root:

   ```
   go run ./scripts/claude-spike -names names.csv -repeat 5 -csv out.csv
   ```

   It sends each row through the bot's own canonical form (`namecanon.Of`) and the bot's own names request
   (`claude.Client.ClassifyNames`), so it measures exactly what the bot sends. It prints, on standard output: the counts by
   label, the accuracy, precision, recall and false-positive rate at the bar (0.9 in this mode unless you give `-bar`), a
   sweep at 0.5, 0.7, 0.8, 0.85, 0.9, 0.95 and 0.97, the latency (p50, p95, max), the total and mean tokens, the estimated
   cost per 1,000 names, and, with `-repeat`, the stability report. `out.csv` has one row per name (row number, label, the
   three strings as you wrote them, the probability, the finish text, milliseconds, tokens, and the minimum, maximum and
   spread over the repeats). **The names appear only in that CSV**; the tool prints no name on stderr, and rows are named by
   their number. `-concurrency` (1 to 8) and `-limit` apply; the tool refuses a non-https `-endpoint` except for localhost.
5. **A test supergroup** with the bot as administrator (Restrict members and Delete messages rights) and, for check 6, the
   key set on the bot's own environment, plus a spare account whose name you can change, and an administrator account.
6. **Phones set to Hindi, Russian and Ukrainian** for check 7, and the bot's log open (it logs chat and user IDs, outcome
   names, a provider status and error type; never a name, an instruction, a probability or the key).
7. **Restore anything you change** in a borrowed group (the rule, the switches, the key).

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not". Tick the box when it
passes or when you have recorded what you changed.

### 1. Accuracy at the 0.9 bar

- [ ] done

Steps: run the command of "Before you start" (the `-repeat` can be 1 for this check). Read the summary at 0.9 and the
sweep. Open `out.csv` and read every legitimate name with a probability near or above 0.9 and every spam name below it.

Pass looks like: the **false-positive rate on legitimate names is 0.5% or lower** (about one in 200; business and
non-Latin names are the ones to watch) and the recall on spam is high enough for you to be useful. A false positive on a
Mute or Ban is a real person, so the bar errs high.

If not: pick a new bar from the sweep (the row with an acceptable false-positive rate) and change the constant
`memberReqAIBar` in `fuku/modules/memberreq_ai.go` (0.9 today) in one line; no key and no migration are involved, and cached
verdicts (24 h) are probabilities, so a retuned bar applies to them at once. If no bar gives an acceptable rate, keep the rule
on Warn or Off, and consider a different instruction (bump the verdict key tag `mrv1` in `fuku/utils/namecanon` in the same
commit as any change to `claude.NamesInstruction`). Record the numbers.

### 2. Non-determinism (stability of the verdicts)

- [ ] done

Steps: `-repeat 5` (it judges each name five times). Read the repeat lines: how many names have judgements on both sides of
the bar, and the largest and the mean spread. Open the minimum, maximum and spread columns of `out.csv` for the crossing
names.

Pass looks like: few or no names cross the bar between repeats, and the spreads are small. The default model takes no
sampling parameter (`temperature`, `top_p` and `top_k` are rejected with a 400), so the verdict cannot be made
deterministic; the 24 hour verdict cache (D-11) is what makes one string's answer stable in practice.

If not: if many names cross, a lucky or unlucky first judgement sticks for 24 hours for every group. Decide whether to keep
that stickiness (D-11) and whether to keep "no re-roll" (D-23: "I fixed it" and the held release read the cache and never
ask again). Alternatives: lower the verdict TTL (`memberReqAIVerdictTTL`), raise the bar so crossing names stay below it, or
keep the AI rule on Warn. Record how many crossed and by how much.

### 3. Latency, tokens and cost

- [ ] done

Steps: read the latency line (p50, p95, max), the mean tokens line and the cost per 1,000 names from the run of check 2.

Pass looks like: **p95 under about 2.5 s**, so most joins fit the join entry's 4 s lookup budget (a slower call is
welcomed and judged at the first message, never punished), mean tokens close to the estimate of **about 760 input and 20
output tokens**, and a cost per 1,000 names you accept (a call is about a hundredth of a cent at list prices; the hourly
budgets are abuse limits, not cost controls).

If not: if p95 is much higher, joins are judged at the first message instead of at the join (safe, slower). If the tokens
differ a lot from 760 and 20, the estimate in the plans is off, so note it. If the price is wrong, pass the real prices with
`-price-in` and `-price-out` (list prices change; verify them on Anthropic's pricing page).

### 4. Request acceptance (the configured model takes a forced tool call with thinking disabled)

- [ ] done

Steps: a short live run (`-limit 3`) with the model you configured. `-probe` exercises the **image** request only, so use the
names run itself for this check. Also try `-models-get` to read the model's metadata.

Pass looks like: the run answers with probabilities, not HTTP 400. Haiku 5.5 accepts the forced tool call and
`thinking: disabled`.

If not: if the finish text says `failed HTTP 400 invalid_request_error`, the model does not accept a forced tool call (Opus
5.5, Sonnet 5.5 and the Fable 5.1 family answer 400). Use a Haiku-class model. In the bot such answers count as provider
failures and pause the name checks for 5 minutes after five of them, with one warning in the log; nobody is punished.

### 5. Refusals on adult-content spam names

- [ ] done

Steps: include a few adult-content spam names in the file and read how many rows have the finish text `blocked` in `out.csv`
(the model's `stop_reason: refusal`).

Pass looks like: few or none. A refusal is counted as no verdict: it is never a breach and never cached.

If not: if many adult-content names are refused, those spam accounts are never caught by this rule (the safe direction).
Record the count; the lockdown and the other rules are the fallback.

### 6. In a test supergroup (the key is on the bot)

- [ ] done

Steps, in a test supergroup with the bot as administrator and the key set on the bot:

1. Open Requirements and set the AI rule to **Warn**. Join a test account whose name is spam-like. Pass: the account is
   warned and the notice names the AI rule and ends with the remedy (change the name or username, or ask an admin to
   approve).
2. Rename the account to another spam-like name and write. Pass: it is judged once; renaming back and forth to a string
   already judged makes **no second call** (the verdict cache).
3. Rename it to an ordinary name and write. Pass: the hold is released (a clean message ends a Warn hold).
4. Set the rule to **Mute**, join and mute a spam-like account, rename it to an ordinary name and press **I fixed it**.
   Pass: the member is unmuted (the button reads the cached verdict for the new names, or judges them once).
5. Unset the key on the bot and restart. Pass: the Requirements page says **AI not configured**, the AI rule's screen
   offers `[Off]` alone, a stored punishment stays visible and inert, and a press for a punishment is refused with the
   same line.
6. Put a **wrong key** on the bot. Pass: the provider answers 401, the name checks pause for 5 minutes for every group, one
   warning appears in the log, and nobody is punished.

Pass looks like: every step as above.

If not: record which step failed with the log lines (they carry IDs and outcome names only). A rename that is judged twice
points at the verdict key (check that `ANTHROPIC_MODEL` did not change between the two). A punishment that follows an
unanswered call is a bug: report it, because the code is meant to punish nobody on an unknown.

### 7. The new cell and the Off-only screen in Hindi, Russian and Ukrainian on a phone

- [ ] done

Steps: on a phone set to hi, ru and uk, open Requirements in a test group and look at the AI cell (the eighth cell, the last
row of the grid), the AI rule screen with the key set and with it unset (the Off-only form), and the Mute confirmation card.

Pass looks like: each cell is on one line (hi "AI स्पैम नाम", ru "ИИ-спам", uk "ШІ-спам"), no row wraps, and the confirmation
says new members are judged when they join and members already in the group only after a name change.

If not: if a cell wraps (Hindi combining marks can look wider than their count), shorten the key in place in that locale
(`settings_btn_mreq_ai`, `settings_mreq_rule_ai`, at most 18 code points); no key is added.

### 8. Two live replicas over one PostgreSQL and one Redis

- [ ] done

Steps: run two bot processes with the same database, the same Redis and the same key. Send the same spam-like name through
both (join the account, then have it write, so both replicas see messages). Watch the logs.

Pass looks like: one string of names is **judged once across both replicas** (one provider call, the second replica reads the
verdict cache); the hourly budgets (5 per account, 100 per group, 400 in all) are counted fleet-wide in Redis, and one
punishment happens, not two.

If not: a double judgement of one string points at the verdict key or at a Redis fault; a double punishment would be a
serious bug in the hold claim (the Phase 13 guarantee), so report it with the log lines.

### 9. A restart during a burst of joins

- [ ] done

Steps: with the AI rule on Warn in a test group, have several test accounts with spam-like names join together, and send
SIGTERM to the bot in the middle. Start it again.

Pass looks like: the process stops inside the 60 s shutdown budget with no new drain (the provider call derives from the
member requirements' root context, which the existing drain cancels), no hold is left half made, nobody is punished for a
cut-off call, and a member whose call was cut is judged again at their next message.

If not: record what was left (a hold, a notice, a cached verdict). A cut-off call must leave none of them.

### 10. A native-speaker read of the ten new keys

- [ ] done

The seven non-English texts were written from the English source and the register of the existing keys, not by a native
speaker. Have a native reader check these before relying on them (es, fr, hi, id, pt, ru, uk):

| Key | Why it needs a native reader |
|-----|------------------------------|
| `mreq_rule_ai` | fills a slot after a colon and joins other fragments with a comma; in ru ("имя, похожее на спам"), uk ("ім'я, схоже на спам") and hi ("स्पैम जैसा नाम") it must stay a noun phrase that does not agree with the addressee; it must not read as an accusation of the member in a public message. The grammar-sensitive one |
| `mreq_remedy_ai` | a second-person sentence appended to a notice, sometimes after the photo remedy; must name changing the name or username and the admin's approval (ru, uk use the plural-polite imperative "смените ... либо попросите", "змініть ... чи попросіть") |
| `settings_mreq_desc_ai`, `settings_mreq_hint_ai` | facts the owner will repeat to members: only the first name, last name and username go to Anthropic's Claude, never messages; members already here are checked only after a name change; new members when they join or at their first message |
| `settings_mreq_here_ai` | must match the wording of `lockdown_auto_ai_unavailable` and use the language's usual word for AI (IA in es, fr, pt; ИИ in ru; ШІ in uk; AI in hi, id) |
| `settings_mreq_confirm_ai_kick`, `_ban`, `_mute` | the consequence texts the owner reads at the destructive confirmation; the mute text quotes the `mreq_btn_fixed` words verbatim (the test enforces it) |
| `settings_btn_mreq_ai`, `settings_mreq_rule_ai` | cell name at most 18 code points; A15-A2 phone check of the Hindi ("AI स्पैम नाम", 14 code points rendered), Russian ("ИИ-спам") and Ukrainian ("ШІ-спам") cells, because Hindi combining marks can look wider than their count |

Pass looks like: each text reads naturally and says what the English says. If not: fix the text in place in that locale
file; no key is added, and the length tests (`TestMemberReqLocaleKeys`, `TestSettingsLocaleKeys`) hold the limits.

### 11. CI: lint, the full test run and the PostgreSQL integrity target

- [ ] done

Steps: let CI run `make lint`, `make test` and `make test-postgres-integrity`; the integrity target includes
`TestMemberRequirementAIMigrationDefaults` (the `ai_action` column defaults to 0, its named CHECK refuses 5 and the statement
runs twice without error). The sandbox that built this phase could run neither `make lint` nor the PostgreSQL target of CI
(`gosec` and `golangci-lint` could not run there either; a local stand-in covered `dupl`, `gocyclo`, `godox`, `staticcheck`
and `errcheck` on the added lines).

Pass looks like: all three green on the branch.

If not: a lint finding names the file and the line; an unused helper, a gosec G101 on a name containing "token" or "secret",
or a clone are the usual ones. Fix in place.

## Follow-ups (decisions for you, none blocks the phase)

- **D-22: sticky unknowns do not defer.** A provider failure, a paused breaker or a spent budget does not hold a weaker
  rule's punishment back (only an unknown that is not sticky does). Keep or change.
- **D-23: "I fixed it" and the held release read the verdict cache and never re-roll.** The way out of a false positive is a
  changed name or an admin's approval. The alternative (a Fresh read bypasses the cache) turns the button into a re-roll of
  a model that cannot be made deterministic; check 2 tells you how much that matters.
- **The baseline in the watcher or in the job.** The watcher reads the baseline and the marker and writes nothing; the job
  writes them (the research's Open Question 2). The rule (security SEC-01): a member whose baseline equals their names and who has no
  marker stays satisfied in the watcher, and the job passes them before it reads the verdict cache, even when another account's
  judgement of the same string has since cached a breach. It errs toward not punishing.
- **A15-A4: the hourly-budget wording.** An "I fixed it" tap answered `mreq_fixed_check_failed` ("Try again in a moment")
  because the member's hourly unit (5 per account) is spent is worded for a moment, not up to an hour. An admin's `/unmute`
  is the fallback. One new key would fix it.
- **A15-A6: a Redis outage is silent.** The AI rule is inert without Redis and the page says nothing. A line would cost a
  Redis read on every draw and one key.
- **A15-A9: the public phrase.** The notice says the name looks like spam, in a public message about a real person. The
  remedy and the admin's approval are in the same notice. A softer phrase ("a name flagged by the AI check") is over the
  22-code-point ceiling.
- **Names use no Telegram pacer.** The provider call is an HTTPS call to Anthropic; only the punishment and the notice go
  through the member requirements' Telegram pacer.
- **The model, the bar and the budgets** (`ANTHROPIC_MODEL`, `memberReqAIBar`, `memberReqAIUserHourlyBudget`,
  `memberReqAIChatHourlyBudget`, `memberReqAIGlobalHourlyBudget`) are one-line values to retune after checks 1 to 3.

## Accepted limits

- A spammer **already in the group** when the rule was switched on is judged only when they rename; a new member is the one
  recorded as joining while the rule is on (flag F-b). With no Redis, a failed marker write or a join the bot never saw, a
  real new member gets a free pass until they rename.
- The **baseline lives 180 days** (absolute, nothing refreshes it: the watcher is read-only and an equal baseline is a
  no-write pass; the marker lives 30). A member **dormant for over 180 days** is first-sighted again and baselined with no
  call, whatever names they carry then. Accepted, the same D-02 limit as a spammer already in the group when the rule was
  switched on (review WR-02 raised it from 30 days, which reopened the hole for every member every month).
- A stale verdict stays for **24 hours** (the verdict cache), bounded; a retuned bar applies to it at once.
- **The AI can be wrong.** A false positive is freed by a rename or an approval (and "I fixed it" for a mute).
- One call costs about a hundredth of a cent; the budgets (5 per account per hour, 100 per group, 400 in all) are abuse
  limits, not cost controls.
- A **Redis outage is silent** on the page (no baseline, no call, no punishment).
- **Check on join off** judges at the first message, not at the join.
- Rolling-deploy caveats, accepted like the others in AGENTS.md: a replica on the previous code answers a `t=ai` press as
  an invalid request and ignores the new column until it is replaced.

**None of these checks blocks the phase.** Plan 15-11 read this file against the finished code: the keys, the meaning of
"new member", the budgets (5, 100 and 400 an hour), the 5 minute pauses, the notice and confirmation texts and the model
rule all match the code, and the end-to-end test agrees with the steps of check 6. Nothing needed correcting.
