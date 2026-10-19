# Platform validation

## Current evidence

The Apple Silicon macOS app has been packaged and launched with an empty
`PATH`. Its real native file dialog imported the matching private VitAdhoc ZIP.
The app detected the Vita read-only, prepared the local package, displayed all
three artwork previews, accepted a custom bubble image and restored the default.
No Install command was sent and no Vita files were changed.

Separate packaged-worker checks cover real ISO parsing, missing/corrupt inputs,
incomplete Galaxy conversion, exclusive volume writes, symlink/root changes
and execution without Python on `PATH`. Loopback FTP and temporary-directory
tests exercise transfers and backups. They are not physical-card or Vita
installation acceptance.

The native source regression suites now pass on Intel macOS and Windows x64
(368 app tests on Intel Mac and 366 on Windows, with five and seven skipped
respectively). Native bundled
preparation and volume tools also build and pass their empty-PATH checks.
All 28 real-IO transfer/coordinator checks pass against the bundled tools on
both platforms. Windows-only POSIX/symlink cases remain skipped.
Evidence: [native worker checks](https://github.com/CBOC-Vita-Lab/vita-installer/actions/runs/39227449832)
and [native package checks](https://github.com/CBOC-Vita-Lab/vita-installer/actions/runs/39236605679).
These automated checks do not establish interactive installer or physical-device
acceptance; full native owned-input comparisons remain open.

Native movie candidates now build on Intel Mac and Windows x64. Each passes
three checks: matching reviewed source bytes, exact conversion of twelve
synthetic frames with preserved timing/audio bytes, and clean invalid-input
errors. The downloaded executable hashes match their build receipts.
Evidence: [native movie checks](https://github.com/CBOC-Vita-Lab/vita-installer/actions/runs/39223928122).

The same isolated build recipe on Apple Silicon reproduces all nine complete
owned movies exactly (1,371,225,472 bytes). The Intel candidate reproduces those
same outputs under Rosetta on Apple Silicon; this is not a full native Intel
movie test. Complete Windows owned-movie and Vita playback checks are still
open. Candidate helpers are not added to the accepted release pins.

Package checks enumerate resources, verify helper hashes/architectures,
reject owner paths/credentials and inspect compressed Python members. The
bundle includes a component inventory, license texts and known notice gaps.
Native image-library packaging checks all 28 declared component versions and
the selected upstream notice hashes before staging and inside the finished app.
The latest Apple Silicon package verifies 1,108 files, including unchanged
metadata/notices from all 357 registry archives in librsvg's release lockfile.
Recovered notices carry their own source hashes. Original attribution and
public-domain notices are retained too. This conservative collection is not
proof of the exact binary dependency set; four missing standalone Rust notices
and the corresponding-source/relinking review remain open.

Mac and Windows package metadata are checked separately; missing notices or
dependency upgrades require review. The completed Intel/Windows runs below
verified the earlier 81-file collection against their actual dependencies.
Their packages must be refreshed for this expanded collection. Finished
packages reject deliberately damaged notice copies; interactive installed-app
acceptance remains open.

A complete Intel Mac ZIP and Windows x64 setup program now build and pass final
verification. Each packaged backend starts with an empty PATH, all seven
notice-integrity cases pass, and the helpers inside the finished app pass
preparation, volume and 28 real-IO checks. The compressed-member privacy audit
checks 750 Intel and 754 Windows members. The Windows volume suite skips one
POSIX-only case. Neither package has been installed or tested interactively
on a user machine of its native platform.

The Intel privacy finding was an upstream library's generic build path, not a
publisher path. Its exact bytes match the official archive. A
[provenance-bound exception](package-privacy.md) records that one artifact;
changed files, different paths and token patterns still fail. Windows home-path
checks also cover mixed separators and JSON escaping.

One earlier Intel source run hit a 10-second
loopback-transfer timeout. Twenty local repeats and the unchanged native retry
passed, but its cause has not been established. No timeout or assertion was
weakened to obtain a pass.

## Not yet accepted

- Native interactive installation and app runs on Intel Mac and Windows.
- First install and reconnect/update acceptance on a separately approved Vita.
- Physical removable-media detection and card-swap tests on both operating systems.
- End-to-end preparation comparisons for every enabled game on each platform.
- Public signing/notarization and complete third-party redistribution review.

The current Mac artifact is an unsigned/ad-hoc internal preview. It is not
notarized. Do not disable Gatekeeper or other OS protections to distribute it.
No paid CI job, signing enrollment or certificate purchase has been started.

## Repeat on a native runner

The manually triggered **Native regression checks** workflow runs the app and
source-worker suites on Intel macOS and Windows x64. It also builds the native
preparation/volume executables, tests them with no Python on PATH, and runs
the real card/USB and coordinator fixtures against those packaged workers.
It uses read-only repository
access, pinned official actions, sequential standard runners and a 20-minute
timeout per platform. It does not publish releases, access a Vita, upload game
inputs or clear packaging acceptance. Run only within the included allowance
with the organization's zero-spend Actions budget in place.

The movie-helper builder still rejects unvalidated platforms and unknown
executable hashes. Native regression results do not bypass that release gate.

The separate **Native movie candidate checks** workflow builds the unchanged
converter with pinned libjpeg-turbo 3.1.4.1 and compares generated test movies
against the reviewed helper's exact output. It retains isolated candidates
under `out/movie-candidate`, never `workers/bin`, and does not update trust
pins. Synthetic tests cannot replace complete owned-movie comparisons or Vita
playback acceptance. No game assets are uploaded. To run the same check locally
with CMake and Clang available:

```sh
node scripts/build-movie-candidate.mjs
node --test tests/preparation/movie-candidate.test.mjs
```

Set `CMAKE` to an explicit CMake executable when it is not on PATH. These are
build-time tools, not additional player requirements.

The manual **Native package validation** workflow builds complete Intel Mac
and Windows app packages using isolated candidate helpers. It checks resource
integrity, deliberately damaged copies, backend startup with an empty PATH,
and real file operations through the helpers inside the finished app.
It retains internal artifacts for one day, never publishes a release and never
changes approved movie hashes. Its two sequential jobs each have a 20-minute
timeout, allowing up to 40 runner-minutes per dispatch; the zero-spend budget
must remain enabled. Both native package jobs pass; public, interactive and
physical acceptance remain held as described above.

To build the same internal candidate locally:

```sh
node scripts/bootstrap-packaging.mjs
node scripts/package-native-candidate.mjs
npm run build
node scripts/stage-package.mjs --native-candidate
npx --no-install electron-forge make
node scripts/verify-release.mjs
node --test tests/packaging/packaged-notices.test.mjs
node scripts/smoke-installed.mjs
```

The candidate receipt says `acceptedForRelease: false` and binds the selected
platform and helper catalog. The normal packaging path below remains gated by
the approved movie helper; this build-only option does not clear GUI, game,
device, signing or redistribution acceptance.

Use the requested architecture, Node 24 and Python 3.14.4. Create
`.venv-workers`, install `workers/requirements.txt` and test-only
`pyftpdlib==2.2.0`, then run:

```sh
npm ci
node scripts/bootstrap-packaging.mjs
node scripts/package-workers.mjs
npm test
npm run typecheck
npm run package
node scripts/verify-release.mjs
node --test tests/packaging/packaged-notices.test.mjs
node scripts/smoke-installed.mjs PATH_TO_PACKAGED_APP
```

Set `VITA_WORKER_PYTHON` when the build-time interpreter is elsewhere. Players
do not install it. Set `VITA_TEST_WORKER` and `VITA_TEST_VOLUME_WORKER` to the
executables inside the packaged app to run the Python suites and real-IO tests.
Retain `out/package-verification.json`, `out/packaged-smoke.json` and test logs
with the exact app archive hash. Build on the actual target; renaming an arm64
artifact is not an Intel or Windows build.

The packaged-notice test checks a disposable copy of the app. It deliberately
removes or changes notices, acknowledgments, the notice manifest and component
metadata, then requires the real package verifier to reject each case.

Native source/worker regression evidence does not replace the missing complete
Windows and Intel Mac app acceptance. The Apple Silicon receipt cannot be
reused to mark another platform accepted. No public game release is cleared
by an installer test.
