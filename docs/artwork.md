# Local artwork

Before installing, choose **Default** or **Custom** for each image:

- **Bubble:** the round Vita home-screen icon.
- **LiveArea background:** the screen shown after tapping the bubble.
- **RetroFlow cover:** the image in RetroFlow.

PNG, JPEG and static WebP are accepted. Preview the result, use Fill or Fit,
and reset any image to the default. Custom files stay on the computer and Vita;
the installer does not upload them or change the original images.

Bubble and LiveArea dimensions come from the verified release. The default
RetroFlow image is the packaged cover, or the bubble icon if no cover is
included. RetroFlow itself uses app icons for homebrew without covers.

## Package checks

Default artwork keeps the original VPK unchanged. Custom bubble/background
art creates a separate local VPK. Only `sce_sys/icon0.png` and the verified
LiveArea background `sce_sys/livearea/contents/bg.png` may change. All other
member hashes must match, including the executable, SFO, template, shaders
and license notices. The original and derivative package hashes are retained
with the image receipts. Custom images have metadata removed.

The separate RetroFlow cover uses the title ID as its filename. The installer
must know whether RetroFlow categorizes the app as Homebrew or PS Vita before
choosing its folder. An existing different cover is a conflict, not permission
to overwrite it. An unknown category or a higher-priority game-name cover
requires manual import. Rescan in RetroFlow after adding a cover.

The app reads existing RetroFlow cache records and category overrides without
executing Lua or changing settings. A valid, unambiguous record selects the
correct cover folder automatically. Missing or conflicting records keep the
cover staged for manual import. Configuration changes during transfer stop
the job rather than silently choosing a different folder.

References: [RetroFlow source](https://github.com/jimbob4000/RetroFlow-Launcher/blob/ef4f14babbbde801f223e635789112e9f4775300/src/index.lua),
[custom-cover instructions](https://github.com/jimbob4000/RetroFlow-Launcher/blob/ef4f14babbbde801f223e635789112e9f4775300/docs/FAQs.md#custom-covers---how-do-i-add-custom-covers),
[sharp metadata handling](https://sharp.pixelplumbing.com/api-output/).
