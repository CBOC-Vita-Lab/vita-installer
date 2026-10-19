# CBOC Vita Installer

<img src="resources/branding/cboc-mark.png" width="80" height="80" alt="CBOC logo">

A desktop installer for reviewed CBOC Vita bundles. Choose your release, add
your own game files, pick the artwork, then install.

**Private preview.** Public downloads are not available yet. This preview does
not include commercial games, a proprietary BIOS or the shader compiler.
Galaxy remains unavailable. Platform and device acceptance is tracked in
[Acceptance](docs/acceptance.md).

## Install a game

1. Open the [installer](https://github.com/CBOC-Vita-Lab/vita-installer/releases).
2. Connect your Vita, or attach its already configured memory card.
3. Paste the game's GitHub URL, or select its release ZIP/folder. Pick a game if several are found.
4. Choose your game file and any missing required files.
5. Keep the default artwork or choose your own, then click **Install**.

Download the game ZIP listed under **Releases > Assets**, not GitHub's
**Code > Download ZIP** or source-code archives. Keep the release ZIP intact;
the app opens it for you. Private releases must be downloaded in your browser
first, using an account with access.

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

Private previews still use browser downloads. The current unsigned preview
does not replace itself or restart automatically. [Update details](docs/updates.md).

## Releases

| Game/app | Guide | Current private bundle |
| --- | --- | --- |
| Mario Strikers Charged | [Install](docs/games/strikers.md) | `Strikers-quiet-20261019-install.zip` (installer 0.1.8+) |
| Infected | [Install](docs/games/infected.md) | `Infected-current-desktop-private.zip` |
| VitAdhoc | [Install](docs/games/vitadhoc.md) | `VitAdhoc-build21-desktop-private.zip` |
| Galaxy | [Status](docs/games/galaxy.md) | Held; no final VPK |

## Development

Use Node 24, run `npm ci`, then `npm test`, `npm run typecheck` and `npm start`.
Native worker builds are described in [Preparation](docs/preparation.md).
[Release profiles](docs/releases.md) describe trust and contribution rules.
The app is MIT licensed; bundled components retain their own licenses.
