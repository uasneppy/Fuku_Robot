# API Coverage — Google Gemini `generateContent` (v1beta REST)

> Full coverage by default. Opt-outs are explicit, reasoned decisions.
> Surface enumerated from the v1beta discovery document (revision 20261029), as listed in
> 07-RESEARCH.md "## API Coverage". Every INTEGRATE row names the plan task that builds it.
> The phase also keeps using the existing TypeSafe Jev client for text unchanged (no new
> surface), so only the Gemini surface is enumerated here.

| capability | decision | reason |
|---|---|---|
| Inline image input (`inlineData`, JPEG/PNG/WebP up to 5 MB) | INTEGRATE | Built in plan 07-02 Task 1 (`gemini.Client.Classify` request); fed real Telegram media by plan 07-08 Task 1 |
| Structured output (`responseMimeType` + `responseJsonSchema`) | INTEGRATE | Built in plan 07-02 Task 1 (schema with `spam_probability` 0..1 and the category enum, strict Go validation) |
| System instruction (`systemInstruction`) | INTEGRATE | Built in plan 07-02 Task 1 (code-owned classification rules; image text is content, never instructions) |
| Usage metadata (`usageMetadata` prompt, candidate and thought token counts) | INTEGRATE | Parsed in plan 07-02 Task 1 into the verdict; used by the spike's cost estimate in plan 07-05 Task 1 |
| Thinking config (`thinkingConfig.thinkingLevel` MINIMAL on `gemini-3*` models) | INTEGRATE | Built in plan 07-02 Task 1 (sent only for model ids starting with gemini-3) |
| Media resolution (`generationConfig.mediaResolution`) | INTEGRATE | Built in plan 07-02 Task 1 (MEDIA_RESOLUTION_MEDIUM constant, spike flag in plan 07-05) |
| Error envelope and retry (`error.status`, `RetryInfo.retryDelay`, 429 and 5xx) | INTEGRATE | Built in plan 07-02 Task 2 (one retry within 2 s, key-safe errors); provider-wide pause in plan 07-09 Task 2 |
| Model status (`modelStatus.modelStage`, `retirementTime`) | INTEGRATE | Parsed in plan 07-02 Task 1; one warning per process on a LEGACY, DEPRECATED or RETIRED model in plan 07-08 Task 1 |
| Safety feedback (`promptFeedback.blockReason`, candidate `finishReason`) | INTEGRATE | Read in plan 07-02 Task 1 as "no verdict" (blocked); counted as blocked in plan 07-08 Task 1 |
| Model metadata (`models.get`) | INTEGRATE | Spike tool only: `gemini.Client.GetModel` in plan 07-02 Task 2, `-models-get` mode in plan 07-05 Task 2 |
| Text-only prompts | OPT-OUT | Owner decision: TypeSafe handles text and Gemini is for images only (REQUIREMENTS out of scope "Gemini for text") |
| File upload API (`media.upload`, `files.*`, `fileData`) | OPT-OUT | Not needed under the 20 MB inline limit; an upload adds a round trip and keeps the image stored at Google |
| Video input | OPT-OUT | Multi-frame and video sampling is deferred (CONTEXT Deferred Ideas); one Telegram thumbnail frame is sent instead |
| Function calling, tools, grounding, URL context, code execution | OPT-OUT | Not needed for a classification answer and would widen the prompt-injection attack surface |
| Streaming (`streamGenerateContent`) | OPT-OUT | The answer is about 30 tokens, so streaming brings no latency benefit |
| Safety settings (`safetySettings` in the request) | OPT-OUT | Provider defaults in production (untestable without the owner's key); blocks are counted as no verdict; the spike exposes a `-safety` flag for the owner |
| Token counting (`countTokens`) | OPT-OUT | `usageMetadata` returned with every answer is enough for the counters and the spike's cost estimate |
| Context caching (`cachedContents`) | OPT-OUT | The instruction is about 200 tokens, below any useful cache size, and a cache would add server-side state |
| Batch mode (`batchGenerateContent`) | OPT-OUT | Asynchronous with minutes-to-hours latency; a raid needs a verdict within seconds |
| Service tiers (`serviceTier` flex or priority) | OPT-OUT | Flex is too slow for the 8 s budget and priority costs more; the standard tier fits |
| Request logging control (`store`) | OPT-OUT | Untestable without the owner's key; the spike exposes a `-store` flag so the owner can compare |
| Embeddings, tuned models, corpora, file search stores, Live API and auth tokens | OPT-OUT | Irrelevant to classifying one image |
| Sampling extras (`seed`, `candidateCount`, logprobs) | OPT-OUT | One deterministic candidate is all the classifier reads |
