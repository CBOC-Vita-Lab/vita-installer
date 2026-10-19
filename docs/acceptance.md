# Preview acceptance

This is an internal preview, not an accepted public installer release.

| Area | Evidence/status |
| --- | --- |
| Import, preparation, artwork, transport, recovery, UI | Automated synthetic tests; current private VPK import checks |
| macOS Apple Silicon | Packaged app launched with empty PATH; real bundle import/artwork UI and packaged-worker tests passed |
| Branding UI | Current logo and duplicate warning checked in a browser-only fixture at 1200x813, 800x600 and 390x844; no horizontal overflow or unloaded logo. Fresh native GUI inspection unavailable because desktop control timed out |
| macOS Intel / Windows x64 | Native runner validation pending |
| Actual installation | No installed game updated; full installation/gameplay acceptance remains pending |
| Real Vita safety check | Companion detected all four installed titles; three available profiles matched their installed runtime hashes. Scoped 128 KiB transfer, controlled interruption, reconnect, hash readback and test-file cleanup passed; all four runtime hashes unchanged afterward |
| Duplicate prevention | Existing titles labelled before preparation; automatic update approval remains off. Regression tests cover all three installable profiles; real-device execution without an approved plan blocked |
| Signing/notarization | No public signing identity selected; unsigned internal previews only |
| Third-party redistribution | Component/license inventory included; complete libvips/native-library source and notice review still pending |
| Strikers | Selected runtime preserved; current package/source/rights/hardware release gates still held |
| Infected | Private-only artwork; public release held |
| VitAdhoc | GPL compatibility clarification and current package acceptance pending |
| Galaxy | No selected final VPK; always held |

The full VitAdhoc destination review was stopped during its large protected
media inventory; that hardware step remains unaccepted. The safe live test did
not check update approval or launch any game. It wrote only disposable files in
a unique installer test directory, then removed them.

Passing app tests does not clear the game release gates. Public release requires
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

Deferred: ordinary desktop shutdown does not dispose of abandoned temporary
workspaces. They may retain substantial local data. Recovery data must remain
available until a separate cleanup and retention policy is implemented.
