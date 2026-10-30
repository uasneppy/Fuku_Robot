# Phase 7: Gemini spike guide

This guide is for you, the owner. It tells you how to measure whether Gemini judges spam images well enough, what it costs, and which numbers in the code to change afterwards. The tool is `scripts/gemini-spike/`.

## Why this exists

The roadmap asked for a spike before the image checks are trusted: run Gemini on real samples and see the accuracy, speed and cost. You chose to defer it (D-17), because it needs your key and your own spam samples, and neither is available to the build.

So the build ships this kit instead. It also stands in for the AI-SPEC evaluation: per D-22, `/gsd-ai-integration-phase` is not run, and this guide plus the "Validation Architecture" section of `07-RESEARCH.md` carry the evaluation plan.

Until you have run it, the code uses safe starting values (bar 0.85, thinking MINIMAL, temperature 0, medium media resolution). The bot works with them. This guide is how you confirm or change them.

## Before you start

1. **Use a paid-tier key (D-21).** Create the key in a Google AI Studio project that has billing turned on. On the free tier Google may use the images you send to improve its products. On a paid tier it should not (this comes from third-party summaries of Google's terms, so read Google's current terms yourself).
2. **A run sends every sample image to Google.** Only use images you are happy to share. Do not include private photos of people.
3. **Keep the key out of your shell history.** Type it without echo:

   ```bash
   read -rs GEMINI_API_KEY && export GEMINI_API_KEY
   ```

   The tool reads the key only from this environment variable. There is no flag for it, and it never prints it. With no key set, the tool prints its usage, exits with a non-zero code and sends nothing.
4. **Check the prices.** The tool assumes $0.30 per million input tokens and $2.50 per million output tokens (`-price-in`, `-price-out`). These come from third-party price trackers. Check them at <https://ai.google.dev/gemini-api/docs/pricing> and pass the real numbers.
5. **Start small.** `-limit 20` sends only 20 images. The default `-concurrency 2` keeps the bill and the rate limits calm.

## Samples to collect

Make a folder with two subfolders, `spam/` and `ham/` (ham means ordinary, harmless images). A file name prefix also works: `spam_one.jpg` and `ham_two.png` count anywhere. Other files are skipped and counted.

Collect **at least 30 spam and 30 ordinary images for each type** the bot will send to Gemini:

| Type | How to get it |
|---|---|
| Photo | Save real photos from your groups, spam and ordinary |
| Static sticker | Save the WebP file |
| GIF | Save the JPEG thumbnail of the GIF, not the GIF itself |
| Video sticker | Save its JPEG thumbnail |
| Animated sticker (TGS) | Save its JPEG thumbnail |

The bot sends the thumbnail for animated media, so the samples must be thumbnails. Saved thumbnails of real raid GIFs and video stickers are the most useful samples. Only `.jpg`, `.jpeg`, `.png` and `.webp` files up to 5 MiB are read, and the type is checked from the file contents, not the name.

Use real raid images when you have them. Ordinary images should look like what your members post: memes, screenshots, selfies, product photos. A set of easy ordinary images will flatter the result.

## What to run, in order

Run these from the repository root.

1. **Keyless contract check.** No real key needed; no image of yours is sent:

   ```bash
   go run ./scripts/gemini-spike -probe -fake-key
   ```

   Every line should say `shape accepted, key refused`. A line starting `field rejected` means the API does not know a field name in `alita/utils/gemini/request.go`. Tell the developer, because that variant's spelling needs a one-line fix. (The tool exits 1 in that case.)
2. **The same check with your key.** It sends about 14 tiny 1x1 images and costs a fraction of a cent:

   ```bash
   go run ./scripts/gemini-spike -probe
   ```

   Now you learn model-level answers too: a `rejected: HTTP 400` line with Google's message names a setting the model does not accept (for example a thinking level).
3. **Model check:**

   ```bash
   go run ./scripts/gemini-spike -models-get
   ```

   It prints the model's name, its token limits and whether it takes `generateContent`. A 404 means `GEMINI_MODEL` (or the built-in default `gemini-3.5-flash-lite`, which came from search summaries only) is wrong.
4. **The labelled run:**

   ```bash
   go run ./scripts/gemini-spike -dir ~/spike-samples -csv base.csv > base-summary.txt
   ```

   Do this once per media type folder if you want separate numbers per type, or once on a mixed folder.
5. **Variant runs.** Repeat step 4 on the same samples, changing one thing at a time, and compare:

   ```bash
   go run ./scripts/gemini-spike -dir ~/spike-samples -temperature default
   go run ./scripts/gemini-spike -dir ~/spike-samples -thinking LOW
   go run ./scripts/gemini-spike -dir ~/spike-samples -media-res MEDIA_RESOLUTION_LOW
   go run ./scripts/gemini-spike -dir ~/spike-samples -safety BLOCK_NONE
   go run ./scripts/gemini-spike -dir ~/spike-samples -model <another model id>
   ```

   Each variant is a full run, so it costs about the same as the first.

## What to read in the output

- **Accuracy, precision, recall at the 0.85 bar.** Precision is the share of flagged images that really are spam. Recall is the share of spam images that were flagged. For the raid trigger, precision matters most: a false flag adds a verdict toward a lockdown. Recall can be modest because several accounts must be flagged anyway.
- **The sweep of bars (0.50 to 0.95).** Look for the lowest bar where precision is still high.
- **Blocked and failed counts.** Blocked means Google's safety filter refused to answer. Failed means an HTTP or parse problem, shown with its status and code. A high blocked rate on spam means adult spam is escaping the check.
- **Latency p50, p95 and max.** The bot gives each call 8 seconds. If p95 is near that, the check will often be dropped as "not checked".
- **Tokens and cost per 1000 images.** Compare with the hourly budgets (150 images per group, 500 in all). The estimate uses the prices you passed.

## Decisions

| Question | Keep the current value if | Change it if |
|---|---|---|
| Bar 0.85 | precision at 0.85 is high (few ordinary images flagged) and recall is acceptable | precision is poor: raise the bar. Recall is poor and precision is excellent: lower it |
| Thumbnail or skip, per media type | thumbnails of GIFs and video or animated stickers score about like photos | a type scores like a coin flip: skip that type in the media rules |
| Temperature 0 or default | `-temperature 0` is as good or better | `-temperature default` is clearly better (some models answer worse at 0) |
| Thinking MINIMAL or LOW | MINIMAL is as accurate | LOW is clearly more accurate and its cost and latency are acceptable |
| Media resolution MEDIUM or LOW | LOW loses accuracy | LOW is as accurate and cheaper and faster |
| Which model | the default model meets the numbers above | another model is more accurate or cheaper at the same quality |
| Should a safety block count as a spam signal? (research Open Question 1) | blocked images are rare among spam | most blocked images are real spam. Today a block counts as nothing. Counting it is a code change, not a constant: ask for a new plan |

## Which constants to change afterwards

| What | Where |
|---|---|
| Image bar (`0.85`) | `raidAIImageBar` in `alita/modules/raid_ai_image.go` |
| Media resolution | `defaultMediaRes` in `alita/utils/gemini/request.go` |
| Thinking level | `thinkingMinimal` and the rule in `thinkingLevelFor` in `alita/utils/gemini/request.go` |
| Temperature | `gen[fieldTemperature] = 0` in `buildRequest`, `alita/utils/gemini/request.go` |
| Hourly budgets (150 per group, 500 in all) | `raidAIChatHourlyBudget` and `raidAIGlobalHourlyBudget` in `alita/modules/raid_ai_image.go` |
| Thumbnail or skip per media type | the media rules in `alita/modules/raid_ai_media.go` |
| Model | the `GEMINI_MODEL` environment variable (no code change) |

After any code change run `make test` and `make lint`.

## Checks only a live run can do

`07-VALIDATION.md` lists what the automatic tests cannot prove. These need you:

1. Gemini accepts the exact request: model id, `thinkingLevel`, `responseJsonSchema`, `mediaResolution`, `image/webp` and thumbnails. Steps 1 to 3 above cover this.
2. Spam accuracy at the 0.85 bar, latency, token use, cost, the safety-block rate and the quota of your key. Steps 4 and 5 cover this.
3. A real burst of suspicious images from three or more accounts that joined in the last 24 hours locks a real test group, and the alert names "AI spam". Needs the bot running with the key set.
4. With no `GEMINI_API_KEY`, `/autolockdown` says AI checks are not set up and no image leaves the server. Start the bot without the key, send images from a new account, and check the notice.
5. Read the new strings in es, fr, hi, id, pt and ru (the `lockdown_*ai*` and `autolockdown_*ai*` keys), if you can, or ask a native speaker.

## What the tool never does

- It does not read the key from a flag, and it does not print it.
- It does not send anything without the key. `-probe -fake-key` uses a built-in fake key and no real image.
- It does not touch your bot, your database or your groups. It uses only the Gemini client in `alita/utils/gemini`.
