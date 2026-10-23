<p align="center"><img src="packaging/icon-256.png" width="112" height="112" alt="CBOC Vita Installer"></p>

<h1 align="center">CBOC Vita Installer</h1>

<p align="center"><b>Your games, on your Vita, in a few clicks.</b></p>

Putting a game on a PS Vita used to mean FTP apps, folder hunting and a lot of guesswork. CBOC Vita Installer does it for you. Pick a game, press **Install**, and it finds your Vita, sends everything to the right place and installs it. When it's done, the game is waiting on your home screen.

<p align="center"><img src="docs/img/start.png" width="720" alt="The installer's start screen, showing an update for Mario Strikers Charged"></p>

<p align="center"><b><a href="https://github.com/CBOC-Vita-Lab/vita-installer/releases/latest">Download the latest version</a></b> · <a href="#how-to-use-it">How to use it</a> · <a href="CHANGELOG.md">What's new</a> · <a href="#help">Help</a></p>

## New In Version 1.2

**It keeps itself up to date.** When a new version is out, the installer downloads it in the background and checks it's genuine. Click **Update now**, or just open the installer next time and you're on the new version. Your page even reconnects by itself.

## Version 1.1 Highlights

The installer was rebuilt from the ground up: smaller, faster, and it runs on almost any computer.

- **Runs almost everywhere.** Intel and Apple Silicon Macs back to macOS 10.13, Windows 7 to 11 (including 32-bit and ARM) and Linux. The old version only ran on Apple Silicon Macs.
- **A fraction of the size.** About 7 MB instead of 150 MB, with nothing else to install.
- **Faster transfers.** It sends several files at once, skips what your Vita already has, and picks up by itself if the Wi-Fi drops.
- **Hands-free installs.** Over Wi-Fi, nothing to press on the Vita. With Vita Companion it can even launch the game for you.
- **Bring your own game.** Wii, GameCube and PSP games as disc images or extracted folders. It checks you have the right version and puts everything where the port expects it.
- **Super Mario Galaxy ready.** It converts your own Galaxy disc for the Vita on your computer, checks every file, and makes the movies about half the size.
- **Smart updates.** Connect your Vita and it shows **Update** when a new version is out. Only what changed is sent, and your saves are never touched.
- **Infected HD textures.** Optional slots for the HD texture packs you made from your own disc.

## What To Expect

| | |
| --- | --- |
| **Computers** | Mac (Intel or Apple Silicon, macOS 10.13 and newer), Windows 7, 8, 10 and 11 (64-bit, 32-bit and ARM), Linux (PC and ARM) |
| **Your Vita** | Any homebrew-enabled Vita (HENkaku or Ensō) with VitaShell |
| **Wi-Fi speed** | The Vita's Wi-Fi manages about 2–4 MB/s, so a 2 GB game takes 10–20 minutes. USB is several times faster |
| **Game updates** | Usually seconds, because only changed files are sent |
| **Installer updates** | Automatic: downloaded in the background, switched over with one click or the next time you open it |
| **Super Mario Galaxy** | About 2–3 minutes to convert your disc on a recent computer, once |
| **Your files** | Nothing is uploaded anywhere. Everything stays between your computer and your Vita |

## How To Use It

**You'll need:** your Vita with VitaShell, the same Wi-Fi as your computer (or a USB cable), and your own copy of any game that asks for one.

1. **Download** the file for your computer from the [latest release](https://github.com/CBOC-Vita-Lab/vita-installer/releases/latest) and open it. It opens in your web browser.
2. **On your Vita, open VitaShell and press SELECT.** The installer finds it by itself.
3. **Pick a game.** Click **Get** next to a CBOC game, drop in a `.vpk` or `.zip`, or paste a GitHub link.
4. **Press Install.** Add your own game file first if it asks for one.

Updating a game? Open the installer with your Vita connected and press **Update**. The installer keeps itself up to date too.

**First launch:** on a Mac, right-click the app and choose **Open** ([more help](docs/mac-open.md)). On Windows, if a blue "Windows protected your PC" box appears, click **More info > Run anyway**.

## Help

**The update banner says to download it myself.** On a Mac, move the app into your **Applications** folder first; macOS doesn't let apps update themselves while they run straight from Downloads.

**It can't find my Vita.** Keep VitaShell's FTP screen open and both devices on the same Wi-Fi. You can also click the status button at the top right and type the address your Vita shows.

**I want it faster.** Use USB: in VitaShell press **START**, set **SELECT button** to **USB**, plug in the cable and press **SELECT**. When it's done, press **X** on the game in VitaShell's `ux0:VPK` folder.

**The transfer stopped.** Press **Install** again. It picks up where it left off.

**My Mac or PC is older and it asks for an extracted folder.** Unpacking disc images needs macOS 12 or Windows 10. On older systems, extract the disc in Dolphin first (right-click the game > **Properties** > **Filesystem** > **Extract Entire Disc**) and choose that folder.

## Download Contents

| File | What it is |
| --- | --- |
| `CBOC-Vita-Installer-1.2-mac.zip` | The Mac app, for Intel and Apple Silicon |
| `CBOC-Vita-Installer-1.2-windows-x64.exe` / `-x86.exe` / `-arm64.exe` | Windows: most PCs, older or 32-bit PCs, and ARM PCs |
| `CBOC-Vita-Installer-1.2-linux-amd64.tar.gz` / `-arm64.tar.gz` | Linux |
| `SHA256SUMS.txt` | Checksums, if you want to check your download |

## For Developers

| | |
| --- | --- |
| [Release format](docs/release-format.md) | How a Vita release tells the installer what it needs, including converting the player's files |
| Build and test | `go test ./...`, then `scripts/build.sh 1.2` (Go 1.20, no other dependencies) |

The app is one Go program with a plain HTML page in `ui/`. The code is in `internal/`.

## Credits

Built by AI agents (Claude) and directed by a human. Disc images are unpacked with [nodtool](https://github.com/encounter/nod). Free and open source under the MIT license.

An unofficial fan project, not affiliated with Nintendo or Sony.
