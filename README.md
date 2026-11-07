<p align="center"><img src="packaging/icon-256.png" width="112" height="112" alt="CBOC Vita Installer"></p>

<h1 align="center">CBOC Vita Installer</h1>

<p align="center"><b>Your games, on your Vita, in a few clicks.</b></p>

Putting a game on a PS Vita used to mean FTP apps, folder hunting and a lot of guesswork. CBOC Vita Installer does it for you. Pick a game, press **Install**, and it finds your Vita, sends everything to the right place and installs it. When it's done, the game is waiting on your home screen.

<p align="center"><img src="docs/img/start.png" width="720" alt="The installer's start screen, offering an update for Mario Strikers Charged"></p>

<p align="center"><b><a href="https://github.com/CBOC-Vita-Lab/vita-installer/releases/latest">Download the latest version</a></b> · <a href="#how-to-use-it">How to use it</a> · <a href="CHANGELOG.md">What's new</a> · <a href="#help">Help</a></p>

## New In Version 2.1

**Your artwork keeps itself up to date.** Play a game, take a screenshot, and the next time your Vita connects its home screen bubble and LiveArea pictures are refreshed from it. Super Mario Galaxy even uses the title screen it saves the first time you play. No reinstall, nothing else touched, and the old pictures are kept in case you want them back.

**Your own art stays yours.** Pictures you chose, or changed yourself on the Vita, are never replaced automatically. Turn the refresh off, or run it now, from the window you use to connect your Vita.

**Ready for the newest games:** Super Mario Galaxy 2.1 and VitAdhoc 1.3 with SNES.

## Recent Highlights

- **Games built from your own disc** (2.0): choose your disc and the installer builds the game on your computer, then installs it. One-click **Prepare your Vita** for the shader compiler, and artwork from your Vita's screenshots.
- **Bowser fixed in Super Mario Galaxy** (1.8): his battle intros no longer twist and spin. Install Galaxy again and only the four corrected files are sent.
- **More Vita setups** (1.7): installs no longer stop right after creating the first folder on Vitas whose FTP server words its replies differently.
- **HD textures for Strikers and Galaxy** (1.5): choose your `hdtex.pack` as the file, a ZIP, a folder or split pieces.
- **Custom artwork** (1.4): pick a new bubble, LiveArea background and logo from your own pictures, libretro-thumbnails or SteamGridDB, with a live preview first.
- **Any file name works** (1.3): downloads are recognised by what's inside them, and every GitHub download is checked before it reaches your Vita.
- **Keeps itself up to date** (1.2): new versions download in the background and switch over with one click.
- **Runs almost everywhere** (1.1): Intel and Apple Silicon Macs back to macOS 10.13, Windows 7 to 11 (including 32-bit and ARM) and Linux, in a 7 MB download.
- **Faster, hands-free installs:** several files at once, nothing to press on the Vita, and it picks up after a Wi-Fi drop.
- **Bring your own game:** Wii, GameCube and PSP games, checked and converted for the Vita when needed (Super Mario Galaxy included), or built on your computer from your disc (Infected). Homebrew like VitAdhoc gets slots for your own games and BIOS. Infected, Strikers and Galaxy take your own HD texture packs, as the file, a ZIP, a folder or split pieces.
- **Smart updates:** shows **Update** when a game has a new version and sends only what changed. Your saves are never touched.

## What To Expect

| | |
| --- | --- |
| **Computers** | Mac (Intel or Apple Silicon, macOS 10.13 and newer), Windows 7, 8, 10 and 11 (64-bit, 32-bit and ARM), Linux (PC and ARM) |
| **Your Vita** | Any homebrew-enabled Vita (HENkaku or Ensō) with VitaShell |
| **Wi-Fi speed** | The Vita's Wi-Fi manages about 2–4 MB/s, so a 2 GB game takes 10–20 minutes. USB is several times faster |
| **Game updates** | Usually seconds, because only changed files are sent |
| **Installer updates** | Automatic: downloaded in the background, switched over with one click or the next time you open it |
| **Super Mario Galaxy** | About 2–3 minutes to convert your disc on a recent computer, once |
| **Games built from your disc** | About 10 minutes to build the game on your computer the first time (Infected), and about 1 GB of free disk space while it works. The finished game is reused afterwards. Building works on Windows (64-bit Intel/AMD), macOS 14 or newer, and Linux (64-bit Intel/AMD). On any other computer, build the game on one of those and choose the `.vpk` it makes |
| **Artwork** | Optional. Your own pictures, libretro-thumbnails, or SteamGridDB with your free API key |
| **Your files** | Nothing is uploaded anywhere. Everything stays between your computer and your Vita |

## How To Use It

**You'll need:** your Vita with VitaShell, the same Wi-Fi as your computer (or a USB cable), and your own copy of any game that asks for one.

1. **Download** the file for your computer from the [latest release](https://github.com/CBOC-Vita-Lab/vita-installer/releases/latest) and open it. It opens in your web browser.
2. **On your Vita, open VitaShell and press SELECT.** The installer finds it by itself.
3. **Pick a game.** Click **Get** next to a CBOC game, drop in a `.vpk` or `.zip`, or paste a GitHub link.
4. **Add what it asks for,** like your own game disc. Choose a file or a folder, or just drag it onto the line.
5. **Press Install.** That's it.

<p align="center"><img src="docs/img/game.png" width="640" alt="A game ready to install: it asks for your own disc and the shader compiler, with an Artwork button"></p>

**First launch:** on a Mac, right-click the app and choose **Open** ([more help](docs/mac-open.md)). On Windows, if a blue "Windows protected your PC" box appears, click **More info > Run anyway**.

### Change a Game's Artwork

1. Press **Artwork** on the game, before you install.
2. Pick what to change: **Bubble icon**, **Background** or **Logo**.
3. Pick a source:
   - **Your picture:** drop in a PNG or JPEG.
   - **libretro-thumbnails:** search box art, title screens and screenshots. No account needed.
   - **SteamGridDB:** paste your free API key once, then search icons, backgrounds and logos.
   - **Your Vita:** screenshots you took in the game (PS + Start) and, for Super Mario Galaxy, the title screen the game saves of itself after its first start. Needs Wi-Fi.
4. Watch the preview change, then press **Done** and **Install**.

<p align="center"><img src="docs/img/artwork.png" width="560" alt="The artwork picker, previewing the home-screen bubble and LiveArea"></p>

Your choice is remembered and goes back on every time the game is updated. **Use the game's own artwork** undoes it.

### Dumping Your UMD

A Vita has no UMD drive, so a game on a UMD (Infected, for example) has to be copied to a computer from a PSP 1000, 2000 or 3000 that runs custom firmware such as [ARK](https://github.com/PSP-Archive/ARK-4).

1. Put the UMD in the PSP and connect it to your computer with a USB cable.
2. On the PSP home screen, press **SELECT** to open the VSH menu and set **USB Device** to **UMD Disc**.
3. In **Settings**, open **USB Connection**.
4. Your computer shows the disc as a drive holding one `.iso`. Copy it to your computer.

Choose that `.iso` in the installer. CSO and PBP files aren't supported.

### Prepare Your Vita

Some games need a file from Sony called the shader compiler (`libshacccg.suprx`) on your Vita, and it can't be shipped with them. If your Vita doesn't have it, the game shows **Prepare your Vita**:

1. Press **Prepare your Vita**. The installer puts ShaRKBR33D, a free Vita app, on your Vita.
2. **On your Vita,** open ShaRKBR33D once and follow it. It needs Wi-Fi.
3. Come back and press **Check again**.

Already have the file? Press **Choose** instead, and pick `libshacccg.suprx`, or a ZIP or folder that has it. The installer looks in both `ur0:data/` and `ur0:data/external/`, and if the file there is empty or cut short it says so, so you know to get it again. Infected doesn't need it to install: without it, a few rarely used effects are skipped.

### Updating

- **Games:** connect your Vita and open the installer. Games with a new version show **Update**. Only what changed is sent, and your saves stay put.
- **The installer:** it gets new versions by itself. Click **Update now** on the banner, or just open it next time.

## Help

**The update banner says to download it myself.** On a Mac, move the app into your **Applications** folder first; macOS doesn't let apps update themselves while they run straight from Downloads.

**It can't find my Vita.** Keep VitaShell's FTP screen open and both devices on the same Wi-Fi. You can also click the status button at the top right and type the address your Vita shows.

**I want it faster.** Use USB: in VitaShell press **START**, set **SELECT button** to **USB**, plug in the cable and press **SELECT**. When it's done, press **X** on the game in VitaShell's `ux0:VPK` folder.

**The transfer stopped.** Press **Install** again. It picks up where it left off. If it says your Vita's memory card is full, free up space in VitaShell (press **SELECT** on `ux0:` to see what's free), or leave out an HD pack.

**A game says the shader compiler is missing, but I copied it.** Open the game in the installer with your Vita on Wi-Fi: it checks `ur0:data/` and `ur0:data/external/` and tells you if the file is damaged. If VitaShell can't open `ur0:data` at all, that folder is damaged: rename it, make a new `data` folder, and move the files into it.

**A game that is built from my disc stopped.** The message on the game says why, usually a damaged disc copy or too little free space (about 1 GB is needed). Fix that and press **Install** again. The build tools are only downloaded once.

**It says the game can't be built on my computer.** Building needs Windows (64-bit Intel/AMD), macOS 14 or newer, or Linux (64-bit Intel/AMD). Build the game on one of those and choose the `.vpk` it makes where the game says **Choose the .vpk**.

**It says my disc is not the right version.** Use the US version of the game, as a plain `.iso` dumped from your own UMD. CSO and PBP files aren't supported.

**My Mac or PC is older and it asks for an extracted folder.** Unpacking disc images needs macOS 12 or Windows 10. On older systems, extract the disc in Dolphin first (right-click the game > **Properties** > **Filesystem** > **Extract Entire Disc**) and choose that folder.

## Download Contents

| File | What it is |
| --- | --- |
| `CBOC-Vita-Installer-2.1-mac.zip` | The Mac app, for Intel and Apple Silicon |
| `CBOC-Vita-Installer-2.1-windows-x64.exe` / `-x86.exe` / `-arm64.exe` | Windows: most PCs, older or 32-bit PCs, and ARM PCs |
| `CBOC-Vita-Installer-2.1-linux-amd64.tar.gz` / `-arm64.tar.gz` | Linux |
| `SHA256SUMS.txt` | Checksums, if you want to check your download |

## For Developers

| | |
| --- | --- |
| [Ideas for later](docs/ROADMAP.md) | Features we'd like to add, such as custom loading screens |
| [Release format](docs/release-format.md) | How a Vita release tells the installer what it needs, including converting the player's files |
| Build and test | `go test ./...`, then `scripts/build.sh 2.1` (Go 1.20, no other dependencies) |

The app is one Go program with a plain HTML page in `ui/`. The code is in `internal/`.

## Credits

Built by AI agents (Claude) and directed by a human. Disc images are unpacked with [nodtool](https://github.com/encounter/nod). Artwork search uses [libretro-thumbnails](https://github.com/libretro-thumbnails/libretro-thumbnails) and [SteamGridDB](https://www.steamgriddb.com); their pictures belong to their creators and are downloaded straight to your computer, never stored here. Free and open source under the MIT license.

An unofficial fan project, not affiliated with Nintendo or Sony.
