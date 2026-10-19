# Galaxy Preparation Validation

Status: private, installation held. No device files were changed during this
validation. Passing synthetic tests does not clear the hold.

## Verified

- Installed title `SMGV00001` uses the exact recovered `thp48d` runtime.
- USA Rev 0 (RMGE01) extraction is checked against 2,374 payload records and
  two metadata records. Two reviewed boot/FST variants accommodate offsets
  rewritten by the extraction tools; the game payloads match.
- The full cooker completed, including audio, movies, executable tables and
  Vita bucket layout. Expected output contains 2,374 records plus the FST.
- All nine quality-55 reference movies match fresh full-file Vita readbacks.
  Their 61,137 frames retain the original audio, dimensions and frame timing;
  every frame chain ends at the expected file boundary.
- Optimized movie size is 1,371,225,472 bytes. Expected complete game data is
  2,472,689,845 bytes plus the source FST. These are data sizes, not free-space
  requirements: extraction, temporary output and backups need extra space.
- Automated checks reject a different runtime, missing source files, altered
  textures/audio, incomplete conversion, missing optimizer, wrong movie output
  and unexpected output files. Failed preparation is not an install candidate.

## Remaining Encoder Difference

The recovered `tools/thp_transcode.cpp` is run at quality 55. A full preparation
run correctly failed its first optimized movie hash check. In a bounded
1,000-frame EndingA comparison, the ordinary recovered encoder matched 989
video payloads and all 1,000 audio payloads against the installed reference.

The same difference persisted with libjpeg-turbo 2.1.5 and 3.1.4.1, Apple Clang
and Linux Clang/GCC probes. Scalar/NEON and floating-point compiler variants
changed the mismatches but did not eliminate them. That is not evidence for
silently selecting another output hash or shipping an obsolete codec.

Next: establish the actual reference encoding recipe, reproduce all nine
movies, package the verified native helper with its source/notices, and rerun
complete preparation through the packaged app. Only then enable a matching
trusted release and perform the approved backup-protected Galaxy-only install
test. Keep other games, existing game data, saves and settings protected.

The current Galaxy manifest remains `held`, with no installable package. The
native movie helper is intentionally absent from the packaged tool catalog
until its output is verified. No public release or Windows acceptance is
implied by this record.
