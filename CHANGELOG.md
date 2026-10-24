# What's New

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
