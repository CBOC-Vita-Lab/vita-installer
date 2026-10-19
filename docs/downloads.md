# Downloads

Strikers and the installer repositories are public source previews. Infected,
Galaxy and VitAdhoc remain private. All current release downloads are still
unpublished drafts, requiring repository write access; they do not appear in
the app's public update check. Public repository access alone does not expose
draft assets. Licensing and full installation review remain pending.

## Desktop app

Get the app only from [vita-installer releases](https://github.com/CBOC-Vita-Lab/vita-installer/releases).

| Download | Use |
| --- | --- |
| [`CBOC-Vita-Installer-0.1.9-darwin-arm64.zip`](https://github.com/CBOC-Vita-Lab/vita-installer/releases/tag/untagged-09e00aebad3d79563276) | Current unsigned Apple Silicon Mac preview |
| Files labelled `validation` | Internal package checks only; not accepted player updates |
| Source code ZIP | Developers only; not an application |

Keep any current transfer running until it finishes. Then quit the old app,
unzip the replacement and retain the app-data folder. Do not disable operating
system security protections. See [Updates](updates.md) and
[Platform validation](platform-validation.md) for status and advanced checks.

## Games and apps

| Repository | Current download | Manual / technical route |
| --- | --- | --- |
| [Strikers](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/releases) | `Strikers-quiet-20261019-install.zip`; standalone VPK also available | [Install](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/blob/main/INSTALL.md), [Build](https://github.com/CBOC-Vita-Lab/strikers-charged-vita/blob/main/BUILDING.md) |
| [Infected](https://github.com/CBOC-Vita-Lab/infected-vita/releases) | `Infected-source-20261019.zip`; no shareable VPK | [Install](https://github.com/CBOC-Vita-Lab/infected-vita/blob/main/INSTALL.md), [Build from your disc](https://github.com/CBOC-Vita-Lab/infected-vita/blob/main/BUILDING.md) |
| [VitAdhoc](https://github.com/CBOC-Vita-Lab/vitadhoc/releases) | `VitAdhoc-build21-source-20261019.zip`; earlier source, not the newer covers installer | [Install](https://github.com/CBOC-Vita-Lab/vitadhoc/blob/main/INSTALL.md), [Build](https://github.com/CBOC-Vita-Lab/vitadhoc/blob/main/BUILDING.md) |
| [Galaxy](https://github.com/CBOC-Vita-Lab/galaxy-vita/releases) | `Galaxy-quiet-source-20261019.zip`; current source and preparation tools, no accepted install bundle | [Install status](https://github.com/CBOC-Vita-Lab/galaxy-vita/blob/main/DESKTOP-INSTALL.md), [Technical notes](https://github.com/CBOC-Vita-Lab/galaxy-vita/blob/main/TECHNICAL.md) |

The Strikers install ZIP opens directly in the app. The complete Strikers
repository folder also works because it contains `cboc-release.json` and the
matching VPK. Source-only downloads for the other projects do not.

Each release includes checksums. On Mac, run `shasum -a 256 FILE`; on Windows,
run `Get-FileHash -Algorithm SHA256 FILE` and compare the result with the release
checksum file. This checks the download, not gameplay or redistribution rights.

Game dumps, proprietary BIOS/shader files, private artwork and unapproved HD
packs are not release assets. Older owner-only bundles stay local. No download
in this list is a promise of public-release or complete hardware acceptance.
