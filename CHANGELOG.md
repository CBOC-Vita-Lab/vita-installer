# Changelog

## 1.0.0

**One small download that installs any PS Vita game from your computer.** A full rewrite of the installer.

| Computer | File |
| --- | --- |
| Mac, Intel or Apple Silicon (macOS 10.13+) | `CBOC-Vita-Installer-1.0.0-mac.zip` |
| Windows 10/11 | `CBOC-Vita-Installer-1.0.0-windows-x64.exe` |
| Windows 7/8 or 32-bit | `CBOC-Vita-Installer-1.0.0-windows-x86.exe` |
| Windows on ARM | `CBOC-Vita-Installer-1.0.0-windows-arm64.exe` |
| Linux | `CBOC-Vita-Installer-1.0.0-linux-amd64.tar.gz` / `-arm64` |

Mac: unzip, right-click the app > **Open** ([help](https://github.com/CBOC-Vita-Lab/vita-installer/blob/main/docs/mac-open.md)). Windows: if SmartScreen appears, **More info > Run anyway**.

1. Open the installer (it opens in your browser) and press **SELECT** in VitaShell. The Vita is found automatically.
2. Drop a `.vpk`, release `.zip` or folder, or paste a GitHub link. CBOC games are one click.
3. Add anything it asks for (for example your own disc), then **Install**.

What's new:

- Installs any VPK, release ZIP, folder or GitHub link. Several games install in one go.
- Finds the Vita on Wi-Fi automatically and remembers it. Also detects a Vita memory card or VitaShell USB drive.
- Installs remotely with no button presses on the Vita, through VitaShell or Vita Companion. With Vita Companion it can also launch the game when it's done.
- Sends files in parallel and skips files the Vita already has. Updates and retries are quick and recover from dropped connections.
- Your own Wii/GameCube disc as ISO, RVZ, WBFS… or a Dolphin-extracted folder, checked for the right game and revision. PSP ISOs too.
- `vita-install.json` lets any port describe its extra steps ([format](https://github.com/CBOC-Vita-Lab/vita-installer/blob/main/docs/release-format.md)); `cboc-release.json` still works.
- About 6 MB with nothing else to install, replacing the 150 MB, Apple-Silicon-only 0.1.x preview.

Written entirely by AI (Claude), fully vibecoded.
