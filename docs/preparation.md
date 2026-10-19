# Preparing game data

The desktop app uses its bundled tools. Players do not need Python or Dolphin.
Only the folders selected in the app are searched. Nothing is downloaded from
game sites, and no game images, BIOS files or shader compiler are bundled.
Selected game ZIPs expand into a bounded, app-owned workspace. The original
ZIP remains unchanged; ambiguous archives require choosing a single input.

| Release | Accepted input | Check |
| --- | --- | --- |
| Strikers | Own USA Rev 1 ISO/RVZ, or an existing full extraction | Outer disc R4QE01, Disc 1, Revision 1; exact main.dol hash after validated extraction |
| Infected | Own USA 1.02 ISO | Exact PSP_GAME/SYSDIR/BOOT.BIN hash using PyCdlib |
| VitAdhoc | No game file needed for app installation | Optional .gba, .gb and .gbc files retain their names; adjacent saves/settings are excluded |
| Galaxy | Own USA Rev 0 ISO/RVZ or complete extraction; release held | RMGE01, exact source inventory, thp48d runtime and complete optimized-output inventory |

Strikers uses the same extraction algorithm as the game's PREPARE-GAME.py:
nodtool 2.0.0-alpha.12, data partition only, with hash validation. Only sys/
and files/ enter the transfer plan. Existing validated extractions are indexed
without making another full copy. Raw image identity remains pending until
the extracted executable has been checked. Infected's ISO is referenced in
place; it is not unpacked for installation.

Optional inputs never become normal-install requirements. VitAdhoc has an
open BIOS built in. A separately selected private GBA BIOS must match the
known 16 KiB image; no download is offered and an existing target file is
never implicitly replaced. The shader compiler is also checked by content,
not just its filename. Unreviewed optional HD packs are not accepted yet.

## Developer Verification

`npm test` runs the TypeScript tests. Create a developer-only `.venv-workers`
and install `workers/requirements.txt` to run the Python worker tests:

```sh
python -m unittest discover -s tests/preparation -p 'test_*.py'
node scripts/package-workers.mjs
```

Set `VITA_TEST_WORKER` to the packaged `vita-prepare` executable and rerun the
Python tests to exercise the bundled runtime. On Windows use the platform's
Python executable via `VITA_WORKER_PYTHON`. Building on a Mac does not produce
or validate a Windows executable.

The worker catalog records source and executable hashes. Conversion tools
come from the installed app, never from an imported release ZIP. Every child
process uses argument arrays, a bounded runtime/output budget and a minimal
environment without account/API credentials. Cancellation waits for the child
to exit; partial output is not an installation candidate.

The movie helper currently has one validated host recipe: macOS arm64, Clang
portable floating-point mode and static libjpeg-turbo 3.1.4.1. The build script
checks its exact binary hash; another compiler or platform requires a separately
reviewed helper and full output validation, not an automatic hash update.

## Build-specific preparation

Prepare files on the computer, before transfer. Use the recipe belonging to
the selected runtime, not a generic "compress everything" pass. Preserve the
build's texture formats, dimensions, audio, timing and directory layout.
Custom bubble, LiveArea and RetroFlow artwork uses the separate validated
artwork conversion path; it does not replace in-game textures.

Galaxy's recipe runs the full recovered cooker: archives, models, textures,
layouts, collision data, audio, executable data tables and bucket folders.
Byte-stream formats stay unchanged where the runtime expects that. All nine
movies then require the separate quality-55 THP optimizer. Basic THP endian
conversion alone is not an acceptable install. The Vita plays prepared files;
it does not perform this conversion during gameplay.

Full source and output inventories are checked, not only filenames or a small
sample. Missing helpers, incomplete output or an optimization mismatch block
installation and remove the incomplete preparation workspace. There is no
fallback to original heavy movies. A different Galaxy runtime also requires
a reviewed matching recipe.

The current evidence and remaining work are recorded in
[Galaxy validation](galaxy-validation.md).

Galaxy remains held for physical acceptance. A separate portable quality-55
recipe now passes the complete preparation pipeline. Its helper and all nine
outputs have exact pins, while the original installed-movie catalog remains
unchanged. Known installed optimized movies are retained rather than replaced
solely for encoder rounding differences. Arbitrary near-matches are rejected.

Strikers retains its reviewed disc extraction; Infected retains the supported
ISO; VitAdhoc retains user-selected ROMs. Do not recompress their media or
invent texture resizing. Any additional offline transformation requires
evidence from that build and verification before its recipe is enabled.

References: [nod](https://github.com/encounter/nod/releases/tag/v2.0.0-alpha.12),
[PyCdlib](https://clalancette.github.io/pycdlib/),
[gpSP BIOS reference](https://docs.libretro.com/library/gpsp/).
