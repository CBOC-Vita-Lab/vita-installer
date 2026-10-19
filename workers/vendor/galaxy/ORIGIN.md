# Galaxy cooker

Unmodified Python cooker modules from `CBOC-Vita-Lab/galaxy-vita`, source
snapshot `5c4b331feb53f6e7631118a07cc062a20ccdbab1`. The full converter
includes audio, movies, executable data tables and the verified Vita layout.
The accompanying Unlicense is preserved. No retail files are included.

The installer keeps Galaxy disabled until the supported disc identity and
final package are verified. Bundling the converter does not clear that gate.

## Movie worker

`tools/thp_transcode.cpp` and `platform/src/sdk/thp_video.cpp` are unmodified
from that same recovered thp48d source snapshot. The host build uses the
portable floating-point decoder with contraction disabled, quality 55 and
libjpeg-turbo 3.1.4.1. The game executable is not rebuilt or changed.
The helper hash and all nine full output hashes are pinned in
`resources/integrity/galaxy-movies-q55-v1.json`. See
`docs/galaxy-validation.md` for the comparison against the installed movies.
The separate libjpeg-turbo notices accompany the static helper.
