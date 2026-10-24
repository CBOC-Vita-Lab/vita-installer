# Release format (for port authors)

The installer works with any VPK. It handles a release ZIP or folder like this:

| In the release | Goes to |
| --- | --- |
| any `*.vpk` | installed as an app (several VPKs = several apps) |
| `data/…` next to the VPK | `ux0:data/…` |
| `ux0/…` next to the VPK | `ux0:…` (but never `ux0:tai` or `ux0:app`) |
| `ur0/data/…` next to the VPK | `ur0:data/…` |
| everything else (readme, source, licenses) | not copied |

Updates only send what changed. The installer keeps a small record of each install on the Vita (`ux0:data/cboc-installer/installed/TITLEID.json`) with a hash of every file the release shipped, so a changed file is sent even when its size stayed the same, and unchanged files are skipped. The player's own files (discs, ISOs) are skipped when the same size is already there. If the app uses vitaGL (it mentions `libshacccg` or `vitagl`), the installer checks the Vita for `ur0:data/libshacccg.suprx` and asks for it when it's missing.

## vita-install.json

If players have to supply files (their own disc, a BIOS…), or some paths must never be overwritten, put a `vita-install.json` next to the VPK:

```json
{
  "name": "Mario Strikers Charged",
  "vpk": "MarioStrikersCharged.vpk",
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
| `keep` | Vita paths the release's own files never overwrite, such as saves and settings (files the player chooses for a need still go where the need says) |
| `notes` | One short message shown before installing |
| `held` | If set, the release is shown but cannot be installed; the text says why |

### Need types

| `type` | Player chooses | Copied to `to` |
| --- | --- | --- |
| `wii-disc`, `gc-disc` | ISO, RVZ, WIA, WBFS, CISO, GCZ, or a Dolphin/nodtool extracted folder. `gameId`, `revision` and `verify` (SHA-256 of files in the extracted disc) are checked | the disc's `sys/` and `files/` |
| `psp-iso` | a PSP `.iso`; `gameId` (for example `ULUS-10054`) is checked | the file, at the exact path in `to` |
| `file` | one file (`ext` limits the extensions, `names` are looked for automatically next to the download, `maxBytes` refuses bigger files) | the exact path in `to` |
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

Older CBOC releases ship `cboc-release.json` instead; the installer reads that too. Infected releases get two optional HD texture slots (`texpack_2x.bin`, and `texpack_4x_mixed.bin` installed as `texpack_4x.bin`) automatically. Strikers and Galaxy releases (any release that asks for disc `R4QE01` or `RMGE01`) get an optional HD texture slot for `hdtex.pack`, installed to `ux0:data/mscharged/hdtex.pack` or `ux0:data/smg/hdtex.pack`.

A `file` need can set `magic`, the bytes the file starts with. The installer then recognises the file by its contents, whatever it is called. For every `file` need the player can choose the file itself, a ZIP that holds it, a folder that holds it, or the first of its split pieces (`.part00`, `.part01`, … or `.001`, `.002`, …); the installer unpacks or joins it before copying.

## Artwork

Players can replace a game's bubble icon, LiveArea background and start image with their own pictures, libretro-thumbnails or SteamGridDB art before installing. The installer only replaces the pictures the VPK already has: `sce_sys/icon0.png`, the background and `startup-image` named in `sce_sys/livearea/contents/template.xml`, and `sce_sys/pic0.png`. Other LiveArea items (buttons such as a Settings shortcut) stay as you made them.

## File names

Name your files however you like. The installer goes by what a file is, not what it's called:

- **An app** is any ZIP with `sce_sys/param.sfo` inside, whether it ends in `.vpk`, `.zip`, `.bin` or nothing.
- **A release file** (`vita-install.json` or `cboc-release.json`) is recognised by its contents, even under another name.
- **If the app's name in the release file is out of date**, the app is found by its `vpkSha256` / `package.sha256`, then by its title ID.
- **On GitHub**, a complete release ZIP is tried first, then the app with its release file (matched by GitHub's checksum of each file), then the app alone. Source-only ZIPs are skipped once opened, and every download is checked against GitHub's checksum when there is one.

## Updates

Publish each version as a GitHub release whose tag is the version (`v1.0`, `v1.1`…). Keep the file names the same every time, for example `MyGame.vpk` and `MyGame.zip` (the VPK with its `vita-install.json` and anything else it needs); then `…/releases/latest/download/MyGame.zip` always links to the newest one. The installer takes the ZIP, or the VPK if there is no ZIP, and never a source ZIP (a name containing `source` or `src`).

With the Vita connected, the installer shows **Update** next to a game when its repository has a newer release.
