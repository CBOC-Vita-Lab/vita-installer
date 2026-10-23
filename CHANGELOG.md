# Changelog

## 1.0.0

A rewrite, built to be small, fast and simple.

- Installs any Vita VPK, release ZIP or folder, and can download straight from a GitHub link. Several games install in one go.
- Finds the Vita on Wi-Fi automatically and remembers it. Also detects a Vita memory card or VitaShell USB drive.
- Installs remotely with no button presses on the Vita, through VitaShell or Vita Companion. With Vita Companion it can also launch the game when it's done.
- Sends several files at once and skips files the Vita already has, so updates and retries are quick. Recovers from dropped connections.
- Unpacks Wii/GameCube disc images (ISO, RVZ, WBFS…) or uses a Dolphin-extracted folder, checking it's the right game and revision. Checks PSP ISOs too.
- New `vita-install.json` release format for port authors; `cboc-release.json` still works.
- One 6 MB download, nothing else to install: macOS 10.13+ (Intel and Apple Silicon), Windows 7+ (64-bit, 32-bit and ARM) and Linux.
- Replaces the 0.1.x Electron preview, which was Apple Silicon only.
