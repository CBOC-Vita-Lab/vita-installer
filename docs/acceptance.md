# Preview acceptance

The existing Apple Silicon 0.1.9 package and Strikers quiet package were made
public at the publisher's request. No additional tests were run for publication.
This does not mark the outstanding acceptance or licensing checks complete.

| Area | Evidence/status |
| --- | --- |
| Import, preparation, artwork, transport, recovery, UI | Automated synthetic tests; current private VPK import checks |
| GitHub URL import and repair | Automated URL, exact-version, checksum, redirect, interruption, cancellation and local-input preservation tests. No approved public game release is available for a full live URL-to-install test |
| Live download check | Public extraction helper downloaded and SHA-256 verified; deliberately damaged disposable cache restored to the same hash. Private game URL rejected without credentials or leftover files. No helper execution or Vita writes |
| Download UI | Actual renderer with a synthetic bridge checked at 1200x813, 800x600 and 390x844. URL entry, progress, cancellation and loaded logo verified; no horizontal overflow. Progress occupies its own row rather than covering controls |
| Progress and time | Automated live-worker line streaming, preparation frame/count reporting, verified-byte transfer estimates, stale/pause reset and final readback progress tests. No validated full Galaxy installation-time claim |
| Progress preview | Actual renderer with a synthetic bridge checked at 1200x813, 800x600 and 390x844. Movie/verification estimates, cancel/pause and text wrapping passed with no console errors. Packaged cooker emitted 150 live progress events on the owned Galaxy extraction; basic cooking took 39.7 seconds including worker startup, not movie optimization |
| macOS Apple Silicon | Packaged app launched with empty PATH; real bundle import/artwork UI and packaged-worker tests passed |
| Owned inputs on Apple Silicon | Strikers revision and all 2,359 prepared reference files match; incomplete extraction rejected. Infected inspection passes through the rebuilt app's helper and preserves the original ISO hash. Nine selected backend/integrity checks pass; no installation or gameplay inferred |
| Branding UI | Current logo and duplicate warning checked in a browser-only fixture at 1200x813, 800x600 and 390x844; no horizontal overflow or unloaded logo. Fresh native GUI inspection unavailable because desktop control timed out |
| macOS Intel / Windows x64 | Refreshed internal packages include all 1,108 notice files and pass backend startup, notice-integrity, preparation/volume and 28 real-IO checks. Native source suites pass (371 Intel and 369 Windows tests). Interactive runs, full native owned-input comparisons and physical acceptance remain pending |
| Actual installation | No installed game updated; full installation/gameplay acceptance remains pending |
| Real Vita safety check | Companion detected all four installed titles; three available profiles matched their installed runtime hashes. Scoped 128 KiB transfer, controlled interruption, reconnect, hash readback and test-file cleanup passed; all four runtime hashes unchanged afterward |
| Duplicate prevention | Existing titles labelled before preparation; automatic update approval remains off. Regression tests cover all four profiles; real-device execution without an approved plan blocked |
| App-only updates | Automated completed-receipt, compatibility, consecutive-update inventory, missing/corrupt data, wrong target/runtime, missing remote marker and pre-write mutation checks. The UI skips original-game preparation; manual update approval remains required |
| Desktop installer updates | Stable-version comparison, platform-specific browser downloads, private/offline/error states, bounded metadata, strict links and busy-operation gates covered by automated tests. No in-place replacement; public discovery and cross-version platform acceptance remain pending |
| Desktop update UI | Version 0.1.7 packaged Mac app launched; actual private-release status and manual check inspected. Synthetic available/busy/offline states checked at 1200x813, 800x600 and 390x844 with no horizontal overflow or console errors. No Vita writes |
| Signing/notarization | No public signing identity selected; the current Apple Silicon download is unsigned |
| Third-party redistribution | Native notice collection now includes metadata and available notices from 357 checksum-verified Rust archives. All three platform packages verify 1,108 notice files. Exact native build dependencies, four missing standalone Rust notices and the remaining source/relinking review are still open |
| Strikers | Quiet runtime, source and install ZIP uploaded with matching checksums; installer 0.1.8+ accepts the exact package. Runtime matches the separate device-update receipt. Full VPK/desktop installation, quiet-build gameplay and public rights gates remain held |
| Infected | Private-only artwork; public release held |
| VitAdhoc | The newer covers runtime matches the October 19 device readback. A local-only candidate removes the private BIOS and preserves all 16 existing notices; its content gate passes. Matching updated source, GPL compatibility and installation acceptance remain pending. The trusted private bundle still contains the earlier build21 runtime |
| Galaxy | The October 19 thp48d-based quiet runtime has executable-only readback and matching source at `24e7eb9`; its local generic-art VPK remains held. Nine historical installed movie references are verified. A separately pinned portable q55 recipe passes complete host preparation: 2,375 files in 199.99 seconds on one Mac. Historical encoder reproduction and distributable-package/playback acceptance remain open |

The full VitAdhoc destination review was stopped during its large protected
media inventory; that hardware step remains unaccepted. The safe live test did
not check update approval or launch any game. It wrote only disposable files in
a unique installer test directory, then removed them.

Passing app tests does not clear the game release gates. Full acceptance requires
exact-platform package tests, clean-environment startup, device installation
and save-preservation evidence, signing/privacy review and verified downloads.

## Review follow-up

The independent review identified three substantive issues, now covered by
regression tests: consent carrying across installs, missing RetroFlow follow-up
steps after automatic installation, and recovery after a pre-install disconnect.
Update consent now binds to the reviewed game, device and existing file hashes.
The branding review also caught overlapping device selections displaying a stale
installed-title warning. Device selection is now serialized, with a regression
test covering two devices and a delayed response.
The download review caught repair dropping the user's original scan folder and
older local releases losing their version locator. Repairs now retain selected
local roots, including extracted ZIP workspaces, and use published manifest
digests for bounded exact-version lookup. Downloaded payload extras remain excluded.

Deferred: ordinary desktop shutdown does not dispose of abandoned temporary
workspaces. They may retain substantial local data. Recovery data must remain
available until a separate cleanup and retention policy is implemented.
The app-only receipt search examines at most 100 local jobs before sorting;
large retained histories can miss a newer valid receipt and fall back to Full setup.
