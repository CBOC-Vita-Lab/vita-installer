# CBOC Vita Installer

<img src="resources/branding/cboc-mark.png" width="80" height="80" alt="CBOC logo">

An **optional desktop setup helper** for CBOC Vita bundles. Choose your release,
add your own game files, pick the artwork, then install.

**You do not need this program to install or play Strikers.** The game runs on
the PS Vita. [Manual setup from Windows, Mac or Linux](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/blob/main/INSTALL.md)
uses the ready-made VPK, your extracted game files and VitaShell. No compiling
or Python is required. No computer or desktop app is needed while playing.
The Apple Silicon restriction applies only to this helper's current download.

**Public preview.** The Apple Silicon Mac app is available below.
Binary redistribution, Apple notarization and full installation testing are still under
review; publication is not a claim of third-party approval. This preview does
not include commercial games, a proprietary BIOS or the shader compiler.
Galaxy remains unavailable. Platform and device acceptance is tracked in
[Acceptance](docs/acceptance.md).

## Choose Your Method

| Method | Computer | What to do |
| --- | --- | --- |
| [Manual setup](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/blob/main/INSTALL.md) | Windows, macOS (Intel or Apple Silicon), Linux | Extract your own ISO/RVZ, copy `sys/` and `files/` to the Vita, install the VPK in VitaShell. USB, FTP and configured memory-card transfers are covered. |
| Optional CBOC helper | Apple Silicon Mac preview | Follow the steps below to automate preparation and transfer. |

Both routes install the same Strikers VPK. You still need your own supported
Wii USA Rev 1 (`R4QE01`) game and a legitimately obtained `libshacccg.suprx`.
Already extracted the game? The manual route needs only copying and VitaShell
installation. [Dolphin extraction instructions](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/blob/main/PREPARE-GAME.md#dolphin-no-python-needed)
are available for ISO/RVZ users; Dolphin is used for extraction, not Vita gameplay.

## Install With This Helper

1. Download [CBOC Vita Installer 0.1.11 for Apple Silicon Mac](https://github.com/CBOC-Vita-Lab/vita-installer/releases/download/v0.1.11/CBOC-Vita-Installer-0.1.11-darwin-arm64.zip), unzip it and move the app to Applications.
2. Connect your Vita, or attach its already configured memory card.
3. Paste `https://github.com/CBOC-Vita-Lab/strikers-charged-vita`, or select its [install ZIP](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/releases/latest) or complete folder.
4. Choose your game file and any missing required files.
5. Keep the default artwork or choose your own, then click **Install**.

Use the named game install ZIP under **Releases > Assets**, not an installer
source-code archive. Keep the release ZIP intact; the app opens it for you.
The complete Strikers repository folder also works.

**First Mac launch:** open the app. If macOS cannot verify its developer, use
**System Settings > Privacy & Security > Open Anyway**, then confirm Open.
This preview has a valid ad-hoc signature but is not Apple-notarized.
Version 0.1.11 fixes the broken signature in 0.1.10; replace the old app rather
than disabling security or removing quarantine attributes. [Mac help](docs/mac-open.md).

The app prepares the folders; no separate Python or Dolphin installation is
needed. It shows the exact remaining VitaShell step when automatic installation
is unavailable. Existing different files are not silently replaced.
Installed titles are labelled before setup. Installing over one requires a
separate, unchecked update approval; choosing a ZIP never approves an update.
Missing or damaged release files repair automatically when the same verified
version is available. Your game files stay yours to provide. Private releases
still use a local ZIP or folder; the app has no GitHub login.

For a compatible release previously installed by this app, choose **Update app**
to skip selecting and preparing the game again. The installed data is still
checked. Full setup remains available when data needs to change.

[Connection options](docs/install.md) / [Artwork](docs/artwork.md) /
[Recovery](docs/recovery.md)

Intel Mac and Windows x64 packages are retained separately for validation.
They have passed automated package checks, not native interactive installation
or full Vita acceptance. See [Downloads](docs/downloads.md) before choosing a file.
Only the Apple Silicon package is published for this release.
Full installation acceptance remains pending. Actions artifacts are validation
builds for developers, not player downloads.

## While it runs

Preparation happens on your computer before copying. The app shows the current
step, measured progress and elapsed time. A remaining-time estimate appears
once enough work has completed; it applies to that step, or the current movie,
not the whole installation. Checks can take time after copying finishes.

Keep both devices awake. If the connection drops, reconnect the same Vita and
choose **Resume**. Follow any final VitaShell action shown by the app. A copied
package is not called installed until installation has been verified.

## Update the installer

The installer has its own [repository and downloads](https://github.com/CBOC-Vita-Lab/vita-installer/releases), separate from the games.
It checks for a newer public version when opened and once a day while running.
Use the download icon beside its version number to check at any time.

When an update is available, click **Download update**. Once any installation
has finished, quit the app and replace it with the download. Keep the app's
data folder: your installation history and recovery records stay there.
Updating the desktop app does not update or reinstall games on your Vita.

Private previews still use browser downloads. The current unnotarized preview
does not replace itself or restart automatically. [Update details](docs/updates.md).

## Releases

| Game/app | Guide | Current bundle / status |
| --- | --- | --- |
| Mario Strikers Charged | [Manual](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/blob/main/INSTALL.md) / [Optional helper](docs/games/strikers.md) | Standalone VPK or `Strikers-settings-20261019-install.zip`; use helper 0.1.11 for the automated Mac route |
| Infected | [Status](docs/games/infected.md) | Owner-only local bundle; GitHub has source, not an install ZIP |
| VitAdhoc | [Status](docs/games/vitadhoc.md) | Earlier local build 21 only; newer covers build held |
| Galaxy | [Status](docs/games/galaxy.md) | Current quiet source available; desktop installation held |

Each game's repository has a separate manual install guide with exact Vita
paths, backups and technical build details. Source-only ZIPs are not install
bundles. [Downloads](docs/downloads.md) links to the correct repositories.

## Development

Use Node 24, run `npm ci`, then `npm test`, `npm run typecheck` and `npm start`.
Native worker builds are described in [Preparation](docs/preparation.md).
[Release profiles](docs/releases.md) describe trust and contribution rules.
The app is MIT licensed; bundled components retain their own licenses.
