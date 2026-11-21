# Gemini Developer API facts (read 2026-11-19)

All facts below were read on Google's official docs on 2026-11-19. `ai.google.dev/gemini-api/docs/*` and
`ai.google.dev/api/*` now redirect to `aistudio.google.com/docs/*` and `aistudio.google.com/api-reference/*`.

Two things that changed and matter for the design:

- The docs now default to a new **Interactions API** (`client.interactions.create`, snake_case, `steps`, `previous_interaction_id`).
  `generateContent` is called "the previous generateContent API" but is still fully documented and supports the newest models.
  Every docs page has a generateContent variant behind `?generate-content=true`. Everything below is the generateContent API.
- The current generation is **Gemini 4** (Carbon, Neon, Hydrogen, Argon), not 2.5/3.x Flash/Pro naming.

Raw page markdown is available by appending `.md.txt` to a docs URL
(e.g. `https://aistudio.google.com/docs/text-generation.md.txt?generate-content=true`); that is where the REST curl tabs were read.

---

## 1. Models, status, limits, prices

Sources: https://aistudio.google.com/docs/models , https://aistudio.google.com/docs/deprecations ,
https://aistudio.google.com/docs/pricing , https://aistudio.google.com/docs/choosing-a-model ,
model pages `https://aistudio.google.com/docs/models/<id>`.

### Current general-purpose text models (all stable IDs, no shutdown date announced)

| Model ID | Released | Context | Max output | Free tier | Paid input / output per 1M | Cached input per 1M |
|---|---|---|---|---|---|---|
| `gemini-4-hydrogen` (recommended default, "Fast") | 2026-11-12 | 1,048,576 unified (input+thinking+output) | 65,536 per request (up to 1M with long decode) | Yes | $1.50 / $7.50 | $0.075 (implicit only) |
| `gemini-4-neon` | 2026-11-18 | 1M unified | 65,536 | No | $3.00 / $15.00 | $0.15 (implicit only) |
| `gemini-4-carbon` (strongest) | 2026-11-10 | 1M unified | 65,536 streaming; 32,768 on direct non-streaming REST | No | $2.00 / $10.00 through 2026-11-30, then $4.00 / $20.00 | $0.10, then $0.20 (implicit only) |
| `gemini-4-argon` | 2026-10-13 | 1M unified | same as Carbon | No | same as Carbon | same as Carbon |
| `gemini-3.8-flash` ("previous-generation Flash") | 2026-09-02 | 1,048,576 input | 65,536 | Yes | $0.75 / $3.75 through 2026-12-31, then $1.50 / $7.50 | $0.075, then $0.15; storage $0.50 then $1.00 per 1M tok/hour |
| `gemini-3.6-flash` | 2026-07-21 | not read | not read | Yes | same as 3.8 Flash | same as 3.8 Flash |
| `gemini-3.5-flash-lite` (cheapest, "Fastest") | 2026-07-21 | 1,048,576 input | 65,536 | Yes | $0.30 / $2.50 | $0.03; storage $1.00 per 1M tok/hour; context caching "Not available" on free tier |
| `gemini-3.1-flash-lite` | 2026-05-07 | not read | not read | Yes | $0.25 / $1.50 (audio in $0.50) | $0.025 |

Output price always includes thinking tokens. The model pages for Hydrogen, 3.8 Flash and 3.5 Flash-Lite each list
"Stable: `<id>`". There is no current "Pro" stable model; the Pro line is `gemini-3.1-pro-preview` (preview, paid only,
$2.00/$12.00 up to 200k prompt tokens, $4.00/$18.00 above).

Preview text models: `gemini-3.1-pro-preview`, `gemini-3-flash-preview` ($0.50/$3.00, "legacy Flash").
Alias pattern `gemini-flash-latest` exists (hot-swapped with 2 weeks' email notice); the docs say production should use a specific stable ID.

### Deprecations and shutdowns

- `gemini-2.5-pro`: from 2026-12-01 requests are automatically routed to `gemini-3.8-flash` and billed at its rates.
- `gemini-2.5-flash`: from 2026-12-01 routed to `gemini-3.5-flash-lite`.
- `gemini-2.5-flash-lite`: not deprecated, but all 2.5 models are limited to users who used them before. Do not start on 2.5.
- `gemini-3.7-flash` and `gemini-3.5-flash`: endpoints stay but route to `gemini-3.8-flash` / `gemini-3.6-flash`.
- `gemini-3.1-flash-lite`: shutdown 2027-05-07 (replacement `gemini-3.5-flash-lite`).
- Shut down already: `gemini-2.0-flash`, `gemini-2.0-flash-lite` (2026-06-01), `gemini-3-pro-preview` (2026-03-09).
- Listed shutdown dates are "earliest possible dates".

### Data use (free vs paid)

Sources: https://aistudio.google.com/docs/pricing , https://aistudio.google.com/docs/terms (effective 2026-03-23).

- Free tier ("Unpaid Services"): content is "used to improve our products"; human reviewers may read, annotate and process
  API input and output (disconnected from account, key and project).
- Paid tier: prompts (including system instructions and cached content) and responses are not used to improve products;
  logged for a limited period only for abuse detection and legal disclosure; processed under the Data Processing Addendum.
- EEA, Switzerland, UK: "You may use only Paid Services when making API Clients available to users in the European Economic
  Area, Switzerland, or the United Kingdom." If you (the developer) are in the EEA/CH/UK, the paid-tier data terms apply to all
  services including free quota.
- Age: 18+; API clients must not be directed at or likely to be accessed by under-18s.

---

## 2. REST request (non-streaming)

Sources: https://aistudio.google.com/api-reference/generate-content/generate-content ,
https://aistudio.google.com/docs/text-generation?generate-content=true (and its `.md.txt`),
https://aistudio.google.com/docs/thinking?generate-content=true ,
https://aistudio.google.com/api-reference/generate-content.openapi.json ,
https://aistudio.google.com/api-reference/api-versions , https://aistudio.google.com/docs/troubleshooting .

- URL: `POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`
  (streaming: `:streamGenerateContent?alt=sse`, same body). The reference page and every curl example use **v1beta**.
  The API-versions page says `v1` is stable and "the latest SDKs default to v1"; system instructions and thinking are listed as
  supported in both; explicit context caching is v1beta only. The curl examples for Gemini 4 all use v1beta, so use v1beta.
- Auth header: `x-goog-api-key: <key>`. Content type `application/json`.
- Field naming: the reference and OpenAPI spec define **camelCase** (`systemInstruction`, `generationConfig`,
  `safetySettings`, `maxOutputTokens`, `thinkingConfig`). Google's own curl example for the system prompt uses snake_case
  `"system_instruction"`, and media examples use `inline_data` / `mime_type`, so both spellings are accepted on input.
  Responses are camelCase. Use camelCase.
- Request body top-level fields (OpenAPI `GenerateContentRequest`): `contents` (required), `systemInstruction`, `generationConfig`,
  `safetySettings`, `tools`, `toolConfig`, `cachedContent`, `serviceTier`, `store`, `continuationToken`, `labels`, `model`.
- `systemInstruction` is a `Content` object (`{"parts":[{"text":"..."}]}`), "Currently, text only".
- `contents[]`: `{"role":"user"|"model","parts":[{"text":"..."}]}`; roles "should alternate between user and model".
- `generationConfig` fields: `maxOutputTokens`, `stopSequences` (up to 5; the stop sequence is not included in the output),
  `temperature`, `topP`, `topK`, `seed`, `candidateCount`, `responseMimeType` (`text/plain` default, `application/json`,
  `text/x.enum`), `responseJsonSchema`, `presencePenalty`, `frequencyPenalty`, `thinkingConfig`, and a newer `responseFormat`.

### Sampling parameters are dead on current models

"Gemini 4 and Gemini 3.6+ models ignore custom values for `temperature`, `top_p`, and `top_k` and return deprecation warning
headers (and will return `400` errors in a future release). Remove `temperature`, `top_p`, and `top_k` from your request
configurations." Out-of-range values already return 400 on Gemini 4. `candidateCount` is "unsupported on Gemini 3 and 4".
So a persona bot cannot tune temperature on `gemini-4-hydrogen`, `gemini-3.8-flash`, `gemini-3.6-flash` (the reference says
"silently ignored on Gemini 3.6 Flash and newer"). Whether `gemini-3.5-flash-lite` honours temperature was not stated explicitly;
"Gemini 3.6+" by version number would not include it, but treat it as unverified.

### Thinking

- Gemini 4 and Gemini 3: `generationConfig.thinkingConfig.thinkingLevel` ("required for Gemini 4 and recommended for Gemini 3").
  Enum in the reference: `MINIMAL`, `LOW`, `MEDIUM`, `HIGH`, `XHIGH`; the curl examples send lowercase (`"low"`, `"high"`, `"xhigh"`).
- Gemini 2.5 only: `thinkingBudget` (integer; 0 disables on 2.5 Flash / Flash-Lite, -1 dynamic). On Gemini 4 `thinkingBudget` is
  deprecated: sent alone it is mapped to the nearest level with a warning; sent together with `thinkingLevel` it is a 400.
- Levels and defaults:

| Model | Default | Levels |
|---|---|---|
| gemini-4-hydrogen, gemini-4-neon | high | low, medium, high, xhigh |
| gemini-4-carbon, gemini-4-argon | medium | low, medium, high |
| gemini-3.8-flash | medium | low, medium, high (`minimal` returns an error) |
| gemini-3.6-flash | medium | minimal, low, medium, high |
| gemini-3.5-flash-lite, gemini-3.1-flash-lite | minimal | minimal, low, medium, high |

- Can thinking be disabled? No on Gemini 4 ("You cannot disable thinking for Gemini 4 Carbon, Argon, Neon, or Hydrogen"); lowest
  is `low`. Gemini 3 Flash / Flash-Lite have no full off either; `minimal` "means the model likely will not think".
- Thinking tokens count against `maxOutputTokens`: "sets the maximum number of tokens a response can generate, including thought
  tokens"; if the limit is hit while reasoning the response ends with `finishReason: MAX_TOKENS` and "truncated or empty output
  (while still billing for any thinking tokens generated)". Google's advice: lower `thinkingLevel` instead of a small
  `maxOutputTokens`. For a chat persona: set `thinkingLevel` `low` (Hydrogen) or `minimal` (Flash-Lite) and keep
  `maxOutputTokens` generous (thousands, not hundreds).
- `includeThoughts: true` returns thought summaries as parts with `"thought": true`; default is off.
- Thought signatures (https://aistudio.google.com/docs/thought-signatures?generate-content=true): Gemini 3/4 may put a
  `thoughtSignature` (base64 string) on any part, including text parts. For plain text chat returning them is recommended, not
  enforced: no error if omitted, "performance may degrade". Required only for function calling. Documented dummy values to skip
  validation: `"context_engineering_is_the_way_to_go"` or `"skip_thought_signature_validator"`. For a Go client: keep the model
  turn's parts verbatim (including `thoughtSignature`) when replaying history if practical.

### Complete curl (minimally adapted from the docs' system-instruction, chat, stop-sequence, thinking and safety examples)

```shell
curl "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-hydrogen:generateContent" \
  -H "x-goog-api-key: $GEMINI_API_KEY" \
  -H 'Content-Type: application/json' \
  -X POST \
  -d '{
    "systemInstruction": {
      "parts": [ { "text": "You are a cat. Your name is Neko." } ]
    },
    "contents": [
      { "role": "user",  "parts": [ { "text": "Hello." } ] },
      { "role": "model", "parts": [ { "text": "Hello! How can I help you today?" } ] },
      { "role": "user",  "parts": [ { "text": "Please write a four-line poem about the ocean." } ] }
    ],
    "generationConfig": {
      "maxOutputTokens": 1000,
      "stopSequences": [ "Title" ],
      "responseMimeType": "text/plain",
      "thinkingConfig": { "thinkingLevel": "low" }
    },
    "safetySettings": [
      { "category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "BLOCK_LOW_AND_ABOVE" }
    ]
  }'
```

Each piece is copied from a docs example; the combination was not run by me. The docs' own system-prompt example spells the key
`"system_instruction"`.

---

## 3. Response shape

Sources: https://aistudio.google.com/api-reference/generate-content (GenerateContentResponse, Candidate),
https://aistudio.google.com/docs/text-generation?generate-content=true , https://aistudio.google.com/docs/safety-settings?generate-content=true .

```json
{
  "candidates": [
    {
      "content": { "parts": [ { "text": "..." } ], "role": "model" },
      "finishReason": "STOP",
      "index": 0
    }
  ],
  "usageMetadata": { "promptTokenCount": 0, "candidatesTokenCount": 0, "totalTokenCount": 0 },
  "modelVersion": "gemini-4-hydrogen",
  "responseId": "..."
}
```

- Text: `candidates[0].content.parts[].text` (concatenate text parts; skip parts with `"thought": true`).
- Candidate fields: `index`, `content`, `finishReason`, `finishMessage`, `safetyRatings[]` (`category`, `probability`
  NEGLIGIBLE/LOW/MEDIUM/HIGH, `blocked`), `citationMetadata`, `tokenCount`, `continuationToken`.
- `finishReason` values: `FINISH_REASON_UNSPECIFIED`, `STOP`, `MAX_TOKENS`, `SAFETY`, `RECITATION`, `LANGUAGE`, `OTHER`,
  `BLOCKLIST`, `PROHIBITED_CONTENT`, `SPII`, `MALFORMED_FUNCTION_CALL`, `IMAGE_SAFETY`, `IMAGE_PROHIBITED_CONTENT`, `IMAGE_OTHER`,
  `NO_IMAGE`, `IMAGE_RECITATION`, `UNEXPECTED_TOOL_CALL`, `TOO_MANY_TOOL_CALLS`, `MISSING_THOUGHT_SIGNATURE`,
  `MALFORMED_RESPONSE`, `ESCALATION`, `PUP_LIMITED_DISABLED` (account limited for Prohibited Use Policy violations),
  `CONTINUATION` (per-request token limit reached, continue with `continuationToken`). Treat unknown values as a non-STOP ending.
- `promptFeedback`: `blockReason` (`BLOCK_REASON_UNSPECIFIED`, `SAFETY`, `OTHER`, `BLOCKLIST`, `PROHIBITED_CONTENT`,
  `IMAGE_SAFETY`) and `safetyRatings[]`.
- `usageMetadata`: `promptTokenCount` (includes cached tokens), `cachedContentTokenCount`, `candidatesTokenCount`,
  `toolUsePromptTokenCount`, `thoughtsTokenCount`, `totalTokenCount` (prompt + tool-use prompt + thoughts + candidates),
  plus `promptTokensDetails`, `cacheTokensDetails`, `candidatesTokensDetails` (arrays of `{modality, tokenCount}`), `serviceTier`.
- Also top-level: `modelVersion`, `responseId`, `modelStatus` (`modelStage`: EXPERIMENTAL/PREVIEW/STABLE/LEGACY/RETIRED,
  `retirementTime`, `message`) - usable to detect a model on its way out.
- Blocked prompt: HTTP 200 with no candidates. The reference: the API "Returns no candidates at all only if there was something
  wrong with the prompt (check `prompt_feedback`)"; `blockReason`: "If set, the prompt was blocked and no candidates are returned."
  The docs give no literal JSON sample; by the schema it is `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"},"usageMetadata":{...}}`
  with `candidates` absent. A blocked response candidate instead has `finishReason: "SAFETY"` (etc.) and no content
  ("The content that was blocked is not returned"). The Go client must handle: missing `candidates`, a candidate with no
  `content` or empty `parts`, and `MAX_TOKENS` with empty text (thinking used the budget).

---

## 4. safetySettings

Sources: https://aistudio.google.com/docs/safety-settings?generate-content=true , API reference (HarmCategory, SafetySetting).

- Categories: `HARM_CATEGORY_HARASSMENT`, `HARM_CATEGORY_HATE_SPEECH`, `HARM_CATEGORY_SEXUALLY_EXPLICIT`,
  `HARM_CATEGORY_DANGEROUS_CONTENT`, `HARM_CATEGORY_JAILBREAK` (new). The request-body text also lists
  `HARM_CATEGORY_CIVIC_INTEGRITY` as supported, but it is not in the HarmCategory enum on the same page (inconsistent; unverified).
- Thresholds: `OFF`, `BLOCK_NONE`, `BLOCK_ONLY_HIGH`, `BLOCK_MEDIUM_AND_ABOVE`, `BLOCK_LOW_AND_ABOVE`,
  `HARM_BLOCK_THRESHOLD_UNSPECIFIED` (use default).
- Default: "If the threshold is not set, the default block threshold is **Off** for Gemini 2.5 and later models." So for current
  models the adjustable filters are off unless you turn them on; one setting per category.
- Cannot be turned off: "built-in protections against core harms, such as content that endangers child safety. These types of
  harm are always blocked and cannot be adjusted." These surface as `PROHIBITED_CONTENT` / `BLOCKLIST` / `OTHER` / `SPII`.
- "Applications that use less restrictive safety settings may be subject to review."

---

## 5. Errors and retry

Sources: https://aistudio.google.com/docs/api-errors?generate-content=true , https://aistudio.google.com/docs/troubleshooting ,
https://aistudio.google.com/docs/rate-limits .

Error body (gRPC status as JSON):

```json
{
  "error": {
    "code": 400,
    "message": "API key not valid. Please pass a valid API key.",
    "status": "INVALID_ARGUMENT",
    "details": [
      { "@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "API_KEY_INVALID", "domain": "googleapis.com",
        "metadata": { "service": "generativelanguage.googleapis.com" } },
      { "@type": "type.googleapis.com/google.rpc.LocalizedMessage", "locale": "en-US", "message": "..." }
    ]
  }
}
```

| HTTP | status | Meaning | Retry? |
|---|---|---|---|
| 400 | INVALID_ARGUMENT | malformed body (note: an invalid API key is also a 400) | no |
| 400 | FAILED_PRECONDITION | free tier not available in your country; enable billing | no |
| 402 | RESOURCE_EXHAUSTED | prepay credit balance depleted | no |
| 403 | PERMISSION_DENIED | wrong/blocked/leaked key, project restricted, needs verification or billing | no |
| 404 | NOT_FOUND | resource (or model) not found | no |
| 429 | RESOURCE_EXHAUSTED | RPM/TPM/RPD or spend-based limit | yes, backoff |
| 499 | CANCELLED | client closed the connection | - |
| 500 | INTERNAL | Google-side error (docs example: input context too long) | yes |
| 503 | UNAVAILABLE | overloaded / out of capacity | yes |
| 504 | DEADLINE_EXCEEDED | not finished in time; raise the client timeout | yes |

Retry guidance (troubleshooting page): exponential backoff starting about 1 s (1, 2, 4, 8 s), add jitter, retry only `429`, `408`
and `5xx`, never `400`, `402`, `403`, cap the number of retries. The Python SDK retries up to four times, initial delay about 1 s,
maximum 60 s. RPD resets at midnight Pacific time; limits are per project, not per key.

`Retry-After` / `RetryInfo`: **not documented by Google.** The official pages mention neither. Community reports on Google's own
forum (not official docs) show 429 bodies with `details[]` containing `google.rpc.QuotaFailure` (with `quotaId` such as
`GenerateRequestsPerDayPerProjectPerModel-FreeTier`), `google.rpc.Help`, and `google.rpc.RetryInfo` with `"retryDelay":"27s"`,
plus "Please retry in 27.96s" in the message; other reports show a bare 429 with no details and no `Retry-After` header, and a
`retryDelay` of `1s` on a daily-quota error (misleading).
(https://discuss.ai.google.dev/t/429-errors-despite-waiting-after-retrydelay/96899 ,
https://discuss.ai.google.dev/t/429-resource-exhausted-at-0-03-of-quota-reproducible-across-projects-tiers-and-endpoints/177317)
Design: parse `RetryInfo.retryDelay` when present as a lower bound, never rely on it, and treat a `quotaId` containing `PerDay`
as not retryable until the daily reset.

Spend-based limit: Tier 1 is capped at $10 per rolling 10 minutes (429 when exceeded); Tier 1 billing cap $250.

---

## 6. Context caching

Sources: https://aistudio.google.com/docs/caching?generate-content=true , https://aistudio.google.com/docs/choosing-a-model ,
https://aistudio.google.com/docs/pricing .

Implicit caching:

- On by default for all Gemini 2.5 and newer models, nothing to configure, "no cost saving guarantee".
- Minimum prompt size: 4,096 tokens for Gemini 4 Carbon/Argon/Neon/Hydrogen, 3.8 Flash, 3.6 Flash, 3.1 Pro Preview; 2,048 for
  2.5 Flash/Pro. **`gemini-3.5-flash-lite` and `gemini-3.1-flash-lite` are not in the minimum-token table**, so whether implicit
  caching applies to Flash-Lite, and at what size, is unverified (the pricing page lists a cached price for it on the paid tier
  and "Not available" on the free tier).
- Discount: Gemini 4 "discounts cached input tokens by 95%" (Hydrogen $0.075 vs $1.50). 3.8 Flash cached $0.075 vs $0.75 (90%)
  until 2026-12-31. Flash-Lite $0.03 vs $0.30 (90%).
- To hit: "putting large and common contents at the beginning of your prompt" and "send requests with similar prefix in a short
  amount of time". The docs give no cache lifetime number.
- Gemini 4 multi-turn note: the full previous request is cached only if the new request adds at least 2,050 tokens; for smaller
  additions "Gemini re-processes the last 2,050 tokens of the previous request at the standard input rate".
- Visibility: `usageMetadata.cachedContentTokenCount` (and `cacheTokensDetails`).

Explicit caching (`cachedContents`, v1beta, Beta):

- **Not supported for Gemini 4 models** (implicit only). Supported on Gemini 3 and 2.5 models (e.g. `gemini-3.8-flash`).
- Create a cache (model, `systemInstruction`, `contents`, `ttl` such as `"300s"`), then pass `"cachedContent": "cachedContents/{id}"`
  in the request. Default TTL 1 hour, no minimum or maximum TTL; only `ttl` / `expireTime` can be updated.
- Billed: cached tokens at the reduced rate plus storage per hour (3.8 Flash $0.50 per 1M tokens/hour until 2026-12-31 then $1.00;
  Flash-Lite $1.00; 3.1 Pro $4.50). "Implicit caching doesn't apply when a request uses an explicit cache."
- Minimum token count for explicit caching: "varies by model"; the page gives no number (unverified).

For a stable 8k-15k token system prompt: it is above the 4,096 minimum, so implicit caching applies on `gemini-4-hydrogen`,
`gemini-3.8-flash` and `gemini-3.6-flash`, provided the system instruction is byte-identical and requests arrive close together.
Keeping the big stable system instruction first is exactly what Google recommends, but a hit is not guaranteed; verify with
`cachedContentTokenCount`. Anything that varies per chat (date, user name) must go after the stable prefix, in `contents`. For a
guarantee on a low-traffic bot, explicit caching on `gemini-3.8-flash` is the only option (12k tokens stored is about $0.006/hour
at $0.50 per 1M/hour). On Flash-Lite, implicit caching is unverified.

---

## 7. Languages and regions

- Regions: https://aistudio.google.com/docs/available-regions lists both **Ukraine** and **Indonesia** (and the EU countries, UK,
  US). Restriction is by the caller's IP/server location. Russia, Belarus and China are not on the list.
- EEA/CH/UK free-tier restriction: see section 1 (only Paid Services when serving users there). The error table also documents
  `400 FAILED_PRECONDITION` "Gemini API free tier is not available in your country. Please enable billing". Ukraine is not in the
  EEA; the docs do not say whether the free tier is offered in Ukraine (unverified).
- Language support for text generation: **I found no current page listing supported languages for text models.** The
  troubleshooting page says only "The API supports only a number of select languages. Submitting prompts in unsupported languages
  can produce unexpected or even blocked responses. See model detail pages for updates", and the model pages for Hydrogen, 3.8
  Flash and 3.5 Flash-Lite list no languages. `finishReason: LANGUAGE` exists for unsupported-language output. Ukrainian,
  Indonesian and English all appear in the language tables for Live Translate, Live API and Transcribe, which is indirect evidence
  only. Equal quality across the three is not claimed anywhere; unverified.

---

## 8. OpenAI-compatible endpoint

Yes: https://aistudio.google.com/docs/openai?generate-content=true . Base URL
`https://generativelanguage.googleapis.com/v1beta/openai/`, chat completions and embeddings, key as the OpenAI `api_key` (Bearer).
"Support for the OpenAI libraries is still in beta". Not used here.

---

## 9. Rate limits

Source: https://aistudio.google.com/docs/rate-limits .

**Google no longer publishes per-model RPM/TPM/RPD numbers in the docs.** The page says limits "can be viewed in Google AI Studio"
(https://aistudio.google.com/rate-limit) and gives numbers only for Omni, Genie and Decision models. So the free-tier and Tier 1
limits for `gemini-4-hydrogen`, `gemini-3.8-flash` and `gemini-3.5-flash-lite` could not be verified; third-party numbers
(e.g. "15 RPM / 1,500 RPD") were seen but are not from Google and should not be trusted. Read them from the project's AI Studio
rate-limit page.

What is documented:

- Dimensions: RPM, TPM (input tokens), RPD; any one exceeded gives 429; per project; RPD resets at midnight Pacific.
- Tiers: Free (active project; some need verification), Tier 1 (billing account linked; billing cap $250; spend limit $10 per
  10 minutes), Tier 2 ($100 paid + 3 days; $200 per 10 minutes), Tier 3 ($1,000 paid + 30 days).
- "Specified rate limits are not guaranteed and actual capacity may vary." Preview models have tighter limits.
- Free tier availability: Hydrogen, 3.8 Flash, 3.6 Flash, 3.5 Flash-Lite, 3.1 Flash-Lite yes; Carbon, Argon, Neon, 3.1 Pro no.
- A recent forum thread title reports repeated free-tier 429s on 3.8 Flash without quota details (community, not official).

---

## 10. API key handling

Sources: https://aistudio.google.com/docs/api-key , curl examples in the guides, the reference page.

- Every guide curl example sends the key in the **`x-goog-api-key` header**. The API reference "Example request" still shows
  `...:generateContent?key=$GEMINI_API_KEY`, so the query parameter still works, but the docs contain no explicit statement
  comparing the two. Use the header; it keeps the key out of URLs, proxy logs and Go `url.Error` strings.
- Key types: since 2026-05-28 new AI Studio keys are "authorization (auth) keys" bound to a service account and restricted to the
  Gemini API. Unrestricted standard keys are rejected; unrestricted keys dormant for a long time are blocked (since 2026-05-07).
- Leaked keys are detected and blocked: `403 PERMISSION_DENIED` "Your API key was reported as leaked. Please use another API key."
- Best practice listed: environment variable `GEMINI_API_KEY` (or `GOOGLE_API_KEY`, which wins if both are set), never in source
  control or client-side code, secret manager in production, IP-address application restrictions in the Cloud console, billing alerts.
- `GET /v1beta/models` only checks that the key exists; a 200 there does not prove `generateContent` is enabled for the project.

---

## Could not verify

1. Per-model RPM/TPM/RPD for any current text model, free or Tier 1 (AI Studio dashboard only).
2. `Retry-After` header and `RetryInfo.retryDelay` on 429: not in official docs; community evidence only, and inconsistent.
3. A supported-languages list for text generation, and any claim of equal Ukrainian/Indonesian/English quality.
4. Implicit caching on `gemini-3.5-flash-lite` / `gemini-3.1-flash-lite` (absent from the minimum-token table); explicit-caching
   minimum token counts; implicit cache lifetime.
5. Whether `gemini-3.5-flash-lite` honours `temperature`/`topP` ("ignored on Gemini 3.6+ and 4" is the only statement).
6. A literal JSON sample of a blocked-prompt response (schema description only).
7. Whether the free tier is offered to projects/callers in Ukraine.
8. `HARM_CATEGORY_CIVIC_INTEGRITY` status (mentioned in prose, missing from the enum).
9. Context window and max output for `gemini-3.6-flash`, `gemini-3.1-flash-lite`, Neon, Carbon model pages (not opened; values
   for Neon/Carbon come from the models overview and thinking guide).
10. Knowledge cutoff dates (not on the pages read).
11. The combined curl in section 2 was assembled from separate docs examples and not executed.
