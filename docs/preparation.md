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
| Galaxy | Held | Full converter is included, but no assumed disc revision or substitute VPK is enabled |

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

Galaxy preserves the full recovered cooker, including movies and audio. The
wrapper requests the Vita layout and verifies it after conversion. Failed
conversions remove only their newly created output folder. The app still
returns `release_held` until the supported source identity and final runtime
are established.

References: [nod](https://github.com/encounter/nod/releases/tag/v2.0.0-alpha.12),
[PyCdlib](https://clalancette.github.io/pycdlib/),
[gpSP BIOS reference](https://docs.libretro.com/library/gpsp/).
