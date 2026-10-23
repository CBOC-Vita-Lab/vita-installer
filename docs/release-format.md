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

### Converting the player's files (`prepare`)

Many ports read the game's data in a different form from the disc: decompressed, little-endian, smaller movies. Add `prepare` to a `wii-disc`, `gc-disc`, `files` or `folder` need, and the installer converts the player's files on their computer before copying them, so nobody has to run a script. The player's own files are never changed; the result is kept in the installer's cache, so a second install (or a retry) starts sending straight away. After a successful install the Vita gets a small `.cboc-prepared` marker, so later updates of the app don't redo or resend the game data.

```json
{
  "id": "game",
  "type": "wii-disc",
  "gameId": "RMGE01",
  "revision": 0,
  "to": "ux0:data/mygame/game",
  "prepare": [
    { "step": "yaz0", "path": "files/ObjectData" },
    { "step": "thp-le", "path": "files/MovieData" },
    { "step": "thp", "path": "files/MovieData", "quality": 55 },
    { "step": "verify", "catalog": "game-files.json.gz", "skip": ["files/MovieData/"] }
  ]
}
```

Steps run in order; `path` limits a step to a folder (default: everything).

| `step` | What it does |
| --- | --- |
| `galaxy` | The full Super Mario Galaxy (USA) conversion: every archive, model, collision, layout, sound and movie made little-endian, checked file by file against the expected result, movies re-encoded smaller. About 2–3 minutes |
| `yaz0` | Decompresses Yaz0 files in place |
| `thp-le` | Makes THP movies little-endian (headers swapped, picture and sound data untouched) |
| `thp` | Re-encodes THP movies at JPEG `quality` (default 55): usually about half the size, faster to copy and to read on the Vita |
| `buckets` | Splits folders with more than `min` files (default 64) into subfolders of `per` files (default 32), named by the FNV-1a hash of the file name, for ports that look files up that way |
| `verify` | Checks every file against `catalog` (a JSON list, may be gzipped, below); `skip` lists path prefixes not checked |
| `tool` | Runs a converter shipped in the release (below) |

A `verify` catalog is `{"files": [{"path": "sys/main.dol", "bytes": 123, "sha256": "…"}]}` or the compact `{"files": [["sys/main.dol", 123, "…"]]}`. `"sha256s": [...]` allows several versions of a file.

#### Your own converter (`tool`)

For anything the built-in steps don't cover, ship a converter for each computer and the installer runs it:

```json
{
  "step": "tool",
  "label": "Converting the game",
  "program": {
    "windows-amd64": "tools/convert.exe",
    "darwin-universal": "tools/convert-mac",
    "linux-amd64": "tools/convert-linux"
  },
  "sha256": {
    "windows-amd64": "…",
    "darwin-universal": "…",
    "linux-amd64": "…"
  },
  "args": ["{in}", "{out}"]
}
```

Keys are `<os>-<arch>` as Go names them (`windows-386`, `windows-arm64`, `darwin-arm64`, `darwin-amd64`, `linux-arm64`…); `darwin-universal` covers both Macs. `{in}` is the player's (or the previous step's) folder, which the tool must only read; `{out}` is an empty folder it fills, which the next steps and the Vita get. The program is refused if its SHA-256 doesn't match, and the player is asked to **Allow** it before anything runs. A non-zero exit stops the install and shows the last lines the tool wrote to stderr.

Older CBOC releases ship `cboc-release.json` instead; the installer reads that too.
