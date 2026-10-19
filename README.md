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
3. Select the matching release ZIP or folder. Pick a game if several are found.
4. Choose your game file and any missing required files.
5. Keep the default artwork or choose your own, then click **Install**.

The app prepares the folders; no separate Python or Dolphin installation is
needed. It shows the exact remaining VitaShell step when automatic installation
is unavailable. Existing different files are not silently replaced.
Installed titles are labelled before setup. Installing over one requires a
separate, unchecked update approval; choosing a ZIP never approves an update.

[Connection options](docs/install.md) / [Artwork](docs/artwork.md) /
[Recovery](docs/recovery.md)

## Releases

| Game/app | Guide | Current private bundle |
| --- | --- | --- |
| Mario Strikers Charged | [Install](docs/games/strikers.md) | `Strikers-a8d4ad6-desktop-private.zip` |
| Infected | [Install](docs/games/infected.md) | `Infected-current-desktop-private.zip` |
| VitAdhoc | [Install](docs/games/vitadhoc.md) | `VitAdhoc-build21-desktop-private.zip` |
| Galaxy | [Status](docs/games/galaxy.md) | Held; no final VPK |

## Development

Use Node 24, run `npm ci`, then `npm test`, `npm run typecheck` and `npm start`.
Native worker builds are described in [Preparation](docs/preparation.md).
[Release profiles](docs/releases.md) describe trust and contribution rules.
The app is MIT licensed; bundled components retain their own licenses.
