# Release format (for port authors)

The installer works with any VPK. It handles a release ZIP or folder like this:

| In the release | Goes to |
| --- | --- |
| any `*.vpk` | installed as an app (several VPKs = several apps) |
| `data/…` next to the VPK | `ux0:data/…` |
| `ux0/…` next to the VPK | `ux0:…` (but never `ux0:tai` or `ux0:app`) |
| `ur0/data/…` next to the VPK | `ur0:data/…` |
| everything else (readme, source, licenses) | not copied |

Files already on the Vita with the same size are skipped, so updates only send what changed. If the app uses vitaGL (it mentions `libshacccg` or `vitagl`), the installer checks the Vita for `ur0:data/libshacccg.suprx` and asks for it when it's missing.

## vita-install.json

If players have to supply files (their own disc, a BIOS…), or some paths must never be overwritten, put a `vita-install.json` next to the VPK:

```json
{
  "name": "Mario Strikers Charged",
  "vpk": "Strikers-settings-20261019.vpk",
  "vpkSha256": "43543e5bce2f162f6678bc2baacbdcbeb03a4593493c0e401a5ad6534623400a",
  "needs": [
    {
      "id": "game",
      "type": "wii-disc",
      "label": "Your Mario Strikers Charged disc (Wii, USA, Rev 1)",
      "gameId": "R4QE01",
      "revision": 1,
      "verify": { "sys/main.dol": "8f811628ee6646c60620a9c599ef08ed929d2eccc98a779c211847f1500f81fd" },
      "to": "ux0:data/mscharged/game"
    },
    { "type": "shader-compiler" }
  ],
  "keep": ["ux0:data/mscharged/np-data/NAND", "ux0:data/mscharged/np-player.txt"]
}
```

Every field is optional.

| Field | Meaning |
| --- | --- |
| `name` | Name to show (default: the VPK's title) |
| `vpk` | VPK file, relative to this file (default: the only VPK in the folder) |
| `vpkSha256` | Refuse a damaged or incomplete download |
| `copy` | `[{"from": "assets", "to": "ux0:data/mygame"}]` replaces the default `data/`/`ux0/` rules |
| `needs` | Files the player supplies (below) |
| `keep` | Vita paths that are never written, such as saves and settings |
| `notes` | One short message shown before installing |
| `held` | If set, the release is shown but cannot be installed; the text says why |

### Need types

| `type` | Player chooses | Copied to `to` |
| --- | --- | --- |
| `wii-disc`, `gc-disc` | ISO, RVZ, WIA, WBFS, CISO, GCZ, or a Dolphin/nodtool extracted folder. `gameId`, `revision` and `verify` (SHA-256 of files in the extracted disc) are checked | the disc's `sys/` and `files/` |
| `psp-iso` | a PSP `.iso`; `gameId` (for example `ULUS-10054`) is checked | the file, at the exact path in `to` |
| `file` | one file (`ext` limits the extensions) | the exact path in `to` |
| `files` | a file or a folder of files (`ext` filters them); existing files are kept | the folder in `to` |
| `folder` | a folder | the folder in `to` |
| `shader-compiler` | `libshacccg.suprx`, only asked for if the Vita doesn't have it | `ur0:data/libshacccg.suprx` |

Add `"optional": true` to a need that may be skipped, and `"help"` for one line of advice.

Older CBOC releases ship `cboc-release.json` instead; the installer reads that too.
