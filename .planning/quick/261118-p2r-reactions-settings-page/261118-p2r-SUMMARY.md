---
type: quick
id: 261118-p2r
status: complete
---
# Summary: Reactions page in /settings

- `TestLegacyReactionRequests` committed alone first.
- `applyReactionAdd` (overwrite for the command, refuse for the page), `applyReactionRemove`, `applyReactionRemoveByID` through `reactionWrites`.
- Repository: `AddReactionContext`, `ListReactionEntriesFresh`, `GetReactionEntryFresh`, `CountReactionsFresh`, `ReactionExistsFresh`, `RemoveReactionByID`.
- Cap of 100 reactions per group (own commit), `/addreaction` answers `reactions_limit_reached`.
- Page `x` (`settings_reactions.go`), codes `xg xx xy xa xc`, part `(x,r)`, Order 73, icon 👍 (deviation: 🎭 is the Anonymous admins page's icon).
- Engine: optional `settingsListEntry.Detail` drawn after ` → `; shared `settingsListCapGate` (Filters uses it too).
- 16 `settings_` keys in 8 locales with limits rows; Main grows to 19 cells (12 owner rows).
