---
type: quick
id: 261118-p1h
status: complete
---
# Summary: translated help module buttons

- 30 `help_module_name_<module>` keys in all 8 locale files.
- `/help` keyboard built per reader language, sorted by translated label with locale collation; global `markup` removed.
- Module help header shows the translated name; `/help <name>` accepts the translated name (multi-word too); the group PM link keeps the internal name.
- Filters, Greetings, Notes, Formatting and Reactions help buttons follow the reader's language (`SetModuleHelpFunc`); the Reactions labels come from `reactions_button_add/remove`; `/donate` and the configuration walkthrough no longer force Ukrainian.
- Tests: `TestHelpModuleNamesExist`, `TestHelpModuleLabelFallsBack`, `TestHelpTranslatedNameResolves`, `TestHelpTranslatedButtonOpensModule`, rewritten `TestInitHelpButtonsBuildsSortedKeyboard`.
