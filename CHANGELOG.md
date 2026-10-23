# Changelog

## 1.0.4

- Simpler instructions on the project page.
- Small fix to the progress display at the end of a transfer.

Works on Mac (Intel and Apple Silicon, macOS 10.13 and later), Windows 7 to 11 (64-bit, 32-bit and ARM) and Linux (PC and ARM).

## 1.0.3

**Smarter updates.**

- The installer now keeps a small install record on the Vita. Updates send every file that changed, even when its size didn't, and skip the rest.
- With your Vita connected, the start screen shows the CBOC games already on it and offers **Update** when a newer release is out. A game you add again says whether it's already installed or which version it replaces.
- Infected: optional slots for your own HD texture packs (2x and the Vita 4x pack), checked and put in the right place. The full 4x pack, too big for the Vita, is refused.
- Files you choose for a game are no longer blocked by the release's protected paths; those now only stop the release's own files from overwriting your saves and settings.

Downloads are the same as 1.0.0, with 1.0.3 in the name.

## 1.0.2

- Super Mario Galaxy and other ports: converts your game files on your computer before sending them (see 1.0.1).
- Cleans up its temporary folder on the Vita even if the Wi-Fi connection dropped during the install.
- `SHA256SUMS.txt` no longer lists itself, so `sha256sum -c` passes cleanly.

Downloads are the same as 1.0.0, with 1.0.2 in the name.

## 1.0.1

**Converts your game files for ports that need it, starting with Super Mario Galaxy.**

- Super Mario Galaxy (USA): choose your disc or extracted folder and the installer does the whole conversion on your computer in a few minutes: every archive, model, collision, layout and sound made ready for the Vita, every file checked against the expected result, and the movies re-encoded about half the size so they copy and play faster. Nothing to run by hand.
- Any port can now ask for its files to be converted: decompress Yaz0, convert or shrink THP movies, arrange big folders, check files against a list, or run its own converter (checked against a fixed hash, and only after you allow it). See [the format](https://github.com/CBOC-Vita-Lab/vita-installer/blob/main/docs/release-format.md#converting-the-players-files-prepare).
- Converted games are kept for a while, so installing again starts sending straight away, and a game already converted on your Vita isn't sent again when you update the app.
- Shows files and frames while converting, with time left.

Downloads are the same as 1.0.0, with 1.0.1 in the name.


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
- Your own Wii/GameCube disc as ISO, RVZ, WBFS… or an already extracted folder (dragged in or chosen), checked for the right game and revision and copied to exactly where the port expects it. PSP ISOs too.
- Notices game files that are already on the Vita and doesn't ask for them again, so updates only send the app.
- `vita-install.json` lets any port describe its extra steps ([format](https://github.com/CBOC-Vita-Lab/vita-installer/blob/main/docs/release-format.md)); `cboc-release.json` still works.
- About 6 MB with nothing else to install, replacing the 150 MB, Apple-Silicon-only 0.1.x preview.

Written entirely by AI (Claude), fully vibecoded.
