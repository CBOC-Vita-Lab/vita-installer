# Synthetic Movie Fixtures

These twelve frames are generated flat colors, ramps, checkerboards and
deterministic block noise. The audio payload is a binary byte pattern used to
check preservation, not playable audio. There are no game frames or recordings.

`scripts/generate-movie-fixtures.mjs` created the input with Sharp and the golden
output with the already-reviewed Apple Silicon helper. `reference.json` records
both file hashes and that helper's hash. Regeneration requires a trusted helper;
the native CI jobs consume the frozen fixtures instead of creating new goldens.

The comparison checks exact output bytes, frame links, frame count, frame rate,
audio bytes, component metadata and unchanged input. It also checks normal
failure for missing/truncated input. Child processes run with an empty PATH.

This is a native build/codec regression check, not full game-movie or physical
Vita acceptance. No candidate hash is admitted to the release recipe here.
