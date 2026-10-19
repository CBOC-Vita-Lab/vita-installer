# Galaxy Preparation Validation

Status: private, release held. Host preparation passes; physical playback and
the distributable package's installation acceptance remain open.

## Verified

- Installed title `SMGV00001` uses the exact recovered `thp48d` runtime.
- USA Rev 0 (RMGE01) extraction is checked against 2,374 payload records and
  two metadata records. Two reviewed boot/FST variants accommodate offsets
  rewritten by extraction tools; the game payloads match.
- Complete preparation passes through the real bundled worker and native
  movie helper: 2,375 output files, 2,472,771,153 bytes for the tested extraction.
  One measured run took 199.99 seconds, excluding disc extraction and transfer.
- All nine installed quality-55 references were verified by full Vita readback.
  Their 61,137 frames retain original audio, dimensions and timing.
- The portable quality-55 recipe also processes all nine movies. Audio remains
  byte-identical and every frame chain is complete. Combined size matches the
  installed references: 1,371,225,472 bytes. Individual sizes differ by at most
  96 bytes.
- Full source/output checks reject missing or altered files, incomplete
  conversion, an unreviewed helper, a different runtime or incorrect movie
  hashes. There is no unoptimized fallback.

## Two Exact Recipes

The historical encoder's precise compiler settings have not been recovered.
The portable recipe is validated separately, not described as byte-identical.
Of 61,137 compressed video frames, 59,558 match. Decoding both outputs with the
game's decoder changes 93,370 of 21,598,479,360 samples; aggregate mean squared
error is 0.000060013484, worst frame 0.037650023, largest sample difference 38.
These measurements are not physical playback proof.

The original cooked catalog is unchanged. A separate catalog pins the portable
helper and every output exactly. The helper uses the recovered converter and
decoder, portable floating-point compilation and static libjpeg-turbo 3.1.4.1.
Source, build command and notices are retained. Arbitrary near-matches are never
accepted.

During an approved update, a movie matching the exact installed reference is
kept and protected with hash checks instead of being replaced by the portable
version. Unknown or damaged files still block. Saves and settings are backed up
before any approved reinstallation.

## Remaining Acceptance

Keep the public profile held until the distributable package, optimized movies,
ordinary playback and protected saves/settings pass on hardware. A private test
using the owner's exact backup package is not acceptance of a generic public
package. Windows/Intel recipes, signing and release rights are separate gates.
