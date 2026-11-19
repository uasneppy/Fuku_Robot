# 20-01 Summary: Persona and archive

Status: code and tests done; waiting for the owner's review of `persona.md` and `archive.md` before plan 20-02 starts.

- `fuku/utils/xenopersona`: `persona.md` (about 1,970 tokens by runes/4, a little above the brief's "about 1,200 to 1,800" because it carries every point of PLAN.md 7.2 and all ten canon rules), `archive.md` (about 9,700 tokens), `StaticSystemText`. Total about 11,600 tokens.
- Tests (`-tags testtools`): stable bytes across calls; no placeholder, format verb or date of the run; 6,000 to 12,000 tokens; no Fuku, Alita or DivideProjects; the exact canon numbers and the Contradictions and Not on record sections reach the model; no web address; standard-library leaf. golangci-lint v2 (built with Go 1.26): 0 issues.
- Judgement calls for the owner: the archive names the official site and the other places to read without their addresses, because the persona forbids links; Konny Diamond's full wiki name is an Indonesian vulgarity, so the archive tells the model to say "Konny"; the voice cast is cut to the lines a fan is likely to ask about; the archive's source comment is stripped from the text sent to the model.
- COMIC-DIGEST.md was used only where it agrees with LORE.md (chapter 2's announcement, broadcast and crisis meeting; chapter 6's countdown, "He killed me too", "That's why you don't fight a hungry wolf"; chapter 9's closing narration). Its misnamed speakers and unclear late chapter 9 pages were left out.
