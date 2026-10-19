# Native Image Libraries

This collection contains unchanged notices selected from the source releases
listed in `manifest.json`. It covers 28 declared components of sharp's native
image library packages. Packaging checks the installed package and component
versions, and verifies every listed notice's SHA-256.

The source selection follows the sharp-libvips build recipe at
`ebb95f8add54eee8bed840e3fb587e4cbec857d7`. Fontconfig 2.18.3 and TIFF 4.7.2
were recovered from their official Git tags; the manifest records those commits
and source archive hashes, not a claim that they equal the unavailable tarballs.
Windows sharp 0.35.5 declares the same component versions as the Mac libvips
1.3.4 packages. Matching version metadata alone does not prove binary provenance.

## Acknowledgments

This software is based in part on the work of the Independent JPEG Group.

Portions of this software are copyright (C) 2026 The FreeType Project
(https://freetype.org). All rights reserved. FreeType's license options and
additional notices are retained in `freetype/`; the FTL option applies here.

Includes software developed by the University of California, Berkeley.

## Review Status

This is not a complete redistribution audit or a replacement for the individual
licenses. File-level and Rust dependency notices, Unicode data, a historical
mutable patch, complete corresponding source and relinking materials still need
review. Python's bundled native libraries are reviewed separately. The installer
remains an internal preview until those release checks are complete.
