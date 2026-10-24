# Ideas For Later

Features we'd like to add. Nothing here is promised or scheduled.

## Loading screen artwork

**What:** a fourth slot in the **Artwork** picker, next to the bubble icon, background and logo. It would change the picture a game shows while it starts, using the same three sources (your picture, libretro-thumbnails, SteamGridDB) and a full-screen 960×544 preview.

**Why it isn't there yet:** every port keeps its loading screen in its own place and format, so the installer can't find it by looking at the app:

| Game | Where | Format |
| --- | --- | --- |
| Mario Strikers Charged | `boot/loading.bin` inside the app (optional; without it the loading screen is plain) | 960×544 RGB565 with a 12-byte header |
| Super Mario Galaxy | `sce_sys/loading-title.rgba` | raw RGBA |
| Infected | `sce_sys/livearea/contents/logo0.png` (LiveArea title card) | PNG |

**How it would work:**

- A release lists its loading screen in `vita-install.json`: the file inside the app, its size and its format (`png`, `rgba`, or `rgb565` with a header). The installer converts the chosen picture to match and adds it to the app, like the other artwork.
- Our own games get that entry built in, so existing releases work without changes.
- The choice is remembered per game and put back on every update, like the rest of the artwork.
