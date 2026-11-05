# Phase 12: Rules media, Media under text and Greet once, real-device and live checklist

This file collects the checks that need a live Telegram client or a real group. Each check is its own level-2 section so
a later plan can add its checks around it without renumbering these.

## EDIT-07 limit: a GIF stored before Phase 12

EDIT-07 limit (D-07, D-08): a GIF stored before Phase 12 is still a document, shows exactly as before and gets no Media under text switch until the owner sets that GIF again, which stores it as an animation and makes the switch appear.

### Live check

1. Pick a group whose welcome GIF was saved before Phase 12 (stored as a document), or save one with the previous build.
2. Open the group's settings in private, then the Welcome page. Expected: the media line reads File or GIF, the page shows
   the file hint and no Media under text switch.
3. Set the same GIF again through Set media (or `/setwelcome` by reply to the GIF).
4. Open the Welcome page again. Expected: the media line now reads GIF and the Media under text switch and its line
   appear.
5. Turn the switch on, press Full preview, then have a test account join the group. Expected: both show the caption above
   the GIF.
