# What's New

## Version 1.8 — October 31, 2026

Infected installs straight from your disc.

- **Games built from your own disc.** Some games can't be shared ready-made, so the installer now builds them on your computer. Choose your own disc, press **Install**, and it builds the game for your Vita and installs it. No terminal, no tools to set up. The first build takes about 10 minutes (the game says how long before you start) and needs about 2 GB of free disk space.
- **You decide first.** The installer asks you to **Allow** it, and says what it will do. Nothing from your disc is uploaded; everything stays on your computer and your Vita.
- **Downloaded only when needed.** The build tools are fetched when you start the build, checked for damage, and kept for next time. Just looking at a game downloads nothing big.
- **Only built once.** If an install is interrupted, or you install again, the finished game is reused. You can press **Stop** during a build.
- **Clear messages.** A wrong disc version or too little free space shows a plain explanation on the game.
- **Artwork** for a built game can be chosen once it has been built for the first time.

For port authors: releases can now build their app on the player's computer. See [Release format](docs/release-format.md).

## Version 1.7 — October 25, 2026

Works with more Vita setups.

- **Fixes installs that stopped right after creating a folder.** Some Vita FTP setups (seen with an sd2vita and a Vita 1001 on firmware 3.60) answer "folder created" in their own way, and the installer mistook that for a failure. Any success reply is now accepted when creating, removing or renaming files and folders.

## Version 1.6 — October 24, 2026

Clearer when something goes wrong.

- **Errors stay on screen.** If an install stopped part-way, the reason used to disappear a moment later. It now stays on the game until you press Install again, so you can see (and tell us) exactly what happened.

## Version 1.5 — October 23, 2026

HD textures for Strikers and Galaxy.

- **Strikers and Galaxy HD packs.** Both games get an optional **HD textures** slot. Choose your `hdtex.pack` and it goes where the game looks for it.
- **Any shape works.** Choose the pack itself, a ZIP with it inside, the folder it's in, or the first of its split pieces (`.part00`, `.001`). It's unpacked or joined for you. Infected's HD packs work the same way.
- **Known by what's inside.** A renamed pack is still found, and a pack in a format the game can't read is turned away before it reaches your Vita.
- File slots now have a **Folder** button too.

## Version 1.4 — October 23, 2026

Your games, your artwork.

**New: Artwork**

- **Choose the look of every game.** Each game has an **Artwork** button for its home-screen bubble, LiveArea background and logo.
- **See it first.** A live preview shows exactly how the bubble and LiveArea will look on your Vita before you install.
- **Your own pictures.** Drop in a PNG or JPEG, or choose one. It's cropped and converted for the Vita automatically.
- **libretro-thumbnails.** Search box art, title screens and screenshots with no account. The installer guesses the system from the game.
- **SteamGridDB.** Icons, wide backgrounds and logos from its community, with your own free API key (kept on your computer).
- **Kept for updates.** Your choice is remembered and put back every time the game is updated.

**Under the hood**

- Pictures are written as 8-bit indexed PNGs at the Vita's exact sizes: 128×128 bubble, 840×500 background, 280×158 start image, and pic0 when the game has one. Only pictures the game already has are replaced.
- A game with new artwork counts as a new build, so it's installed again even if nothing else changed.

## Version 1.3 — October 23, 2026

Smarter about files.

**Easier**

- **Any file name works.** Apps and release files are recognised by what's inside them, so a renamed download installs just the same.
- **Always the right download.** On GitHub it picks the complete release, or the app plus its release file, matched by checksum. It skips source-code ZIPs.
- **Checked on arrival.** Every download from GitHub is checked against GitHub's own checksum, so a damaged file is caught before it reaches your Vita.

**Your games**

- **VitAdhoc:** the optional games slot now only offers the file types VitAdhoc reads (.gba, .gb, .gbc).

**Under the hood**

- **Self-update** finds this computer's download even if its name changes, and can check it with GitHub's checksum when a release has no checksum list.
- **A damaged app** next to its release file is never installed on its own.

## Version 1.2 — October 23, 2026

The installer now keeps itself up to date.

**Easier**

- New versions download in the background and are checked against the release's checksums before anything changes.
- Click **Update now** to switch straight away, or just open the installer next time: it's already on the new version.
- The page you have open reconnects to the new version by itself.
- If an update can't be installed on your computer, it says why and offers the download instead.

**Under the hood**

- The new version must start and report the right version number before it replaces the old one.
- Windows downloads now carry the right version number in their file details.

## Version 1.1 — October 23, 2026

The everything-better update. The installer has been rebuilt from the ground up.

**Works on more computers**

- Intel and Apple Silicon Macs back to macOS 10.13, Windows 7 to 11 (64-bit, 32-bit and ARM) and Linux. The old version only ran on Apple Silicon Macs.
- About 7 MB instead of 150 MB, with nothing else to install.

**Faster and easier**

- Finds your Vita by itself over Wi-Fi, USB or a memory card reader.
- Sends several files at once, skips what's already there, and picks up by itself after a Wi-Fi drop.
- Installs over Wi-Fi with nothing to press on the Vita, and launches the game with Vita Companion.
- CBOC games are one click on the start screen, and any GitHub link works too.

**Your games**

- Wii, GameCube and PSP games as disc images or extracted folders, checked and copied to the right place.
- Super Mario Galaxy: your disc is converted for the Vita on your computer, every file is checked, and the movies come out about half the size.
- Infected: optional slots for your own HD texture packs.
- Smart updates: shows **Update** when a new version is out, sends only what changed, and never touches your saves.

**Under the hood**

- A record of each install on the Vita, so updates catch every changed file.
- Release ZIPs with simple file names are preferred over a bare VPK.
- Release authors can describe extra steps in a `vita-install.json` file ([release format](docs/release-format.md)).

## Earlier versions

- **1.0.0 to 1.0.5** (October 23, 2026): the first releases of the rebuilt installer, which added everything above step by step.
- **0.1.x**: the original Apple Silicon preview.
