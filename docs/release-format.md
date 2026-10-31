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
| `vpk` | VPK file, relative to this file (default: the only VPK in the folder; not needed when `build` makes the app) |
| `vpkSha256` | Refuse a damaged or incomplete download |
| `copy` | `[{"from": "assets", "to": "ux0:data/mygame"}]` replaces the default `data/`/`ux0/` rules |
| `titleId` | The app's title ID. Required when the app is made by `build`, checked against the VPK otherwise |
| `needs` | Files the player supplies (below) |
| `build` | Makes the app on the player's computer from one of their files ([below](#building-the-app-on-the-players-computer-build)) |
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

## Building the app on the player's computer (`build`)

Some ports can't ship the game's own code or data, so nothing derived from the game may be published. With `build`, the release ships a **builder kit** instead of an app. The installer runs it on the player's computer, on the player's own disc, and installs what it makes. The player gets no terminal, no SDK and no scripts, and nothing leaves their computer.

```json
{
  "name": "Infected",
  "titleId": "INFC00001",
  "build": {
    "from": "disc",
    "kit": {
      "windows-amd64": { "file": "Infected-Vita-builder-windows.tar.gz",   "sha256": "…64 hex digits…", "size": 312000000 },
      "darwin-amd64":  { "file": "Infected-Vita-builder-mac-intel.tar.gz", "sha256": "…", "size": 0 },
      "darwin-arm64":  { "file": "Infected-Vita-builder-mac-arm.tar.gz",   "sha256": "…", "size": 0 },
      "linux-amd64":   { "file": "Infected-Vita-builder-linux.tar.gz",     "sha256": "…", "size": 0 }
    },
    "run": "infected-build",
    "args": ["--progress", "--out", "{out}", "{in}"],
    "app": "infected.vpk",
    "label": "Building Infected from your disc",
    "minutes": 10
  },
  "needs": [
    { "id": "disc", "type": "psp-iso", "gameId": "ULUS-10054", "to": "ux0:data/infected/Infected.iso",
      "label": "Your Infected UMD backup (PSP, USA, ISO)", "prepare": [{ "step": "build" }] },
    { "id": "shader", "type": "shader-compiler", "optional": true }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `from` | The `id` of the need whose file is `{in}`: a `psp-iso`, `wii-disc`, `gc-disc`, `file` or `folder` need that is not optional. That need is still copied to the Vita as usual |
| `kit` | One kit archive per kind of computer, named `<os>-<arch>` as Go names them (`windows-amd64`, `windows-386`, `darwin-arm64`, `linux-arm64`…). Each has the `file` name in the same GitHub release, its `sha256`, and optionally its `size`. A Mac without an entry of its own uses the other Mac's (or `darwin-universal`; an Apple silicon Mac runs the Intel kit under Rosetta); there is no other fallback, and a computer with no entry is told the game can't be built there yet |
| `run` | The program inside the kit (a relative path; `.exe` is added on Windows) |
| `args` | Its arguments. `{in}` is the player's file, `{out}` an empty folder for the result and `{work}` an empty scratch folder. Must contain `{in}` and `{out}`. Default: `["{in}", "{out}"]` |
| `app` | The name of the VPK the program writes into `{out}`. The app in it must have the release's `titleId` |
| `label` | Shown while the build starts (default: "Building NAME from your disc") |
| `minutes` | About how long it takes, shown to the player before they start |

**The kit** is one archive per kind of computer, a file of the same release. The program, its tools and data are all inside; the installer unpacks it as it is, restoring the executable bits and links the archive stores (links must stay inside the kit). It is also the folder the program runs in. **Publish kits as `.tar.gz`** (a gzip-compressed tar, for example `Infected-Vita-builder-windows.tar.gz`); a ZIP works too. The installer tells which it is by looking inside the file, not at its name.

Why `.tar.gz`: installers older than 1.8 try every `.zip` file of a GitHub release as a possible app, so they would download all the kits (hundreds of megabytes each) before saying that no app was found. They skip `.tar.gz` files, so they download nothing and simply say that the release has no app. Installers from 1.8 on never download a kit while adding a game, whatever its name, and leave out files named like `…-builder-<windows|mac|linux>….tar.gz` (or `.tgz`, `.zip`) and any file a loose `vita-install.json` lists under `kit`.

**The program** reads `{in}`, writes the VPK to `{out}/<app>` and exits with 0. On failure it exits non-zero and writes **one plain-English line** to stderr, which is shown to the player as it is (for example "This is not the US version of Infected (ULUS-10054, 1.02)."). With `--progress` it also prints these lines on stdout; any other line is ignored:

```
@step Compiling (this is the long part)      a new phase starts
@progress 120 540                            progress in the current phase (whole numbers)
@done /full/path/to/infected.vpk             the last line when it worked
```

The installer shows the step text and a progress bar with the time left, and the program's temporary files are kept in the installer's cache (`TMPDIR`/`TMP`/`TEMP` point there) so they are cleaned up with it. Stopping the install stops the program and everything it started.

**What the player sees:**

- Before installing, the game says it is built on their computer from their own disc, and about how long it takes.
- They must press **Allow** first, like for a `tool` step. The text says that the game is built on this computer from their own disc, about how many minutes it takes, that it needs about 2 GB of free disk space, and that nothing is uploaded.
- Only then is the kit for their computer downloaded from the release (a progress bar, "Downloading the build tools"), checked against its `sha256` (a mismatch is refused and nothing is unpacked), unpacked once and kept. Browsing a release or adding it to the list downloads no kit. A release folder or ZIP that already holds the kit file next to `vita-install.json` is used instead, so a release works offline.
- The installer checks for about 2.5 GB of free space before it starts.
- The finished VPK is kept in the installer's cache, keyed by the kit, the player's file (path, size and date) and the arguments. A retry after a failed install, or a reinstall, doesn't build again; a different disc file or a new kit does. Older builds of the same game are removed.
- The built VPK then goes through the normal install exactly like a shipped one: artwork, PROM/Companion/USB install, the receipt, and update detection (the app on the Vita counts as this release's when the kit, program and arguments are the same).
- The disc is asked for again on every install, even when a copy is on the Vita, because the build needs it on the computer.
- Artwork can be chosen for a built game once it has been built the first time (before that there are no pictures to preview). It is applied to every later build.

`vpk` may be given as well (for example a working app for older installers); a release with `build` always builds. The `{"step": "build"}` on the `from` need is optional: when a release folder or ZIP is opened, installers older than 1.8 refuse an unknown step and say "a newer installer may be required", while without it they only say the release's app is missing. Given the release by its GitHub link, an older installer ignores `.tar.gz` kits and reports that the release has no app; with ZIP kits it would download them all first (see above).

## Artwork

Players can replace a game's bubble icon, LiveArea background and start image with their own pictures, libretro-thumbnails or SteamGridDB art before installing. For a game made by `build` the pictures are known once it has been built. The installer only replaces the pictures the VPK already has: `sce_sys/icon0.png`, the background and `startup-image` named in `sce_sys/livearea/contents/template.xml`, and `sce_sys/pic0.png`. Other LiveArea items (buttons such as a Settings shortcut) stay as you made them.

## File names

Name your files however you like. The installer goes by what a file is, not what it's called:

- **An app** is any ZIP with `sce_sys/param.sfo` inside, whether it ends in `.vpk`, `.zip`, `.bin` or nothing.
- **A release file** (`vita-install.json` or `cboc-release.json`) is recognised by its contents, even under another name.
- **If the app's name in the release file is out of date**, the app is found by its `vpkSha256` / `package.sha256`, then by its title ID.
- **On GitHub**, a complete release ZIP is tried first, then the app with its release file (matched by GitHub's checksum of each file), then the app alone. Source-only ZIPs are skipped once opened, and every download is checked against GitHub's checksum when there is one.

## Updates

Publish each version as a GitHub release whose tag is the version (`v1.0`, `v1.1`…). Keep the file names the same every time, for example `MyGame.vpk` and `MyGame.zip` (the VPK with its `vita-install.json` and anything else it needs); then `…/releases/latest/download/MyGame.zip` always links to the newest one. The installer takes the ZIP, or the VPK if there is no ZIP, and never a source ZIP (a name containing `source` or `src`).

With the Vita connected, the installer shows **Update** next to a game when its repository has a newer release.
