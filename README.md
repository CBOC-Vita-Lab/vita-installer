<p align="center"><img src="packaging/icon-256.png" width="112" height="112" alt="CBOC Vita Installer"></p>

<h1 align="center">CBOC Vita Installer</h1>

<p align="center"><b>Put games on your PS Vita from your computer.</b><br>
It finds your Vita, copies everything to the right place and installs it.</p>

<p align="center"><a href="https://github.com/CBOC-Vita-Lab/vita-installer/releases/latest"><b>Download</b></a></p>

---

<p align="center"><img src="docs/img/start.png" width="720" alt="The installer's start screen"></p>

- **Any Vita game or app:** a `.vpk`, a release `.zip`, a folder or a GitHub link.
- **Your own game files:** Wii and GameCube discs (ISO, RVZ, WBFS or an extracted folder) and PSP ISOs. It checks you have the right game and puts it where the port expects it, converting it first when the port needs that (Super Mario Galaxy, for example).
- **Updates:** connect your Vita and it shows **Update** when a game has a new version. Only changed files are sent, and your saves are never touched.
- **No setup:** one small download, nothing else to install.

## Download

| Computer | File |
| --- | --- |
| Mac, Intel or Apple Silicon (macOS 10.13 High Sierra and later) | `…-mac.zip` |
| Windows 10 / 11 | `…-windows-x64.exe` |
| Windows 7 / 8, or 32-bit Windows | `…-windows-x86.exe` |
| Windows on ARM | `…-windows-arm64.exe` |
| Linux (PC or ARM) | `…-linux-amd64.tar.gz` / `…-linux-arm64.tar.gz` |

Get them from the [latest release](https://github.com/CBOC-Vita-Lab/vita-installer/releases/latest).

- **Mac:** unzip, right-click the app and choose **Open**. Allow it to find devices on your network. ([Help](docs/mac-open.md))
- **Windows:** if "Windows protected your PC" appears, click **More info > Run anyway**.

## How to use it

1. **Open the installer.** It opens in your web browser.
2. **On the Vita, open VitaShell and press SELECT.** The installer finds your Vita by itself.
3. **Add a game.** Click **Get** next to a CBOC game, drop a file on the window, or paste a GitHub link.
4. **Add anything it asks for,** such as your own game disc.
5. **Click Install.**

## Questions

**What does my Vita need?** HENkaku/Ensō and VitaShell. Some games also need the shader compiler (`libshacccg.suprx`); the installer asks for it only if your Vita doesn't have it.

**It can't find my Vita.** Keep VitaShell's FTP screen open (SELECT) and both on the same Wi-Fi. Or click the status button at the top right and type the address the Vita shows.

**Big games are slow over Wi-Fi.** Use USB: in VitaShell press **START**, set **SELECT button** to **USB**, plug in the cable and press **SELECT**. The installer finds it by itself. When it's done, press **X** on the game in VitaShell's `ux0:VPK` folder.

**The transfer stopped.** Keep the Vita awake and press **Install** again. Finished files are not sent again.

**Is anything uploaded?** No. Everything stays between your computer and your Vita. It only goes online to download games you ask for and to check for updates.

## For port authors and developers

- [Release format](docs/release-format.md): how a release tells the installer what it needs.
- Build: `go test ./...` and `scripts/build.sh 1.0.0` (Go 1.20, no other dependencies).
- Release: add a new `## x.y.z` section at the top of [CHANGELOG.md](CHANGELOG.md) and push to `main`.

Written entirely by AI (Claude). MIT licensed.
