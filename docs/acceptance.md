# Preview acceptance

This is an internal preview, not an accepted public installer release.

| Area | Evidence/status |
| --- | --- |
| Import, preparation, artwork, transport, recovery, UI | Automated synthetic tests; current private VPK import checks |
| macOS Apple Silicon | Packaged app launched with empty PATH; real bundle import/artwork UI and packaged-worker tests passed |
| macOS Intel / Windows x64 | Native runner validation pending |
| Actual installation | No device writes authorized during installer development; end-to-end hardware acceptance pending |
| Signing/notarization | No public signing identity selected; unsigned internal previews only |
| Third-party redistribution | Component/license inventory included; complete libvips/native-library source and notice review still pending |
| Strikers | Selected runtime preserved; current package/source/rights/hardware release gates still held |
| Infected | Private-only artwork; public release held |
| VitAdhoc | GPL compatibility clarification and current package acceptance pending |
| Galaxy | No selected final VPK; always held |

Passing app tests does not clear the game release gates. Public release requires
exact-platform package tests, clean-environment startup, device installation
and save-preservation evidence, signing/privacy review and verified downloads.

## Review follow-up

The independent review identified three substantive issues, now covered by
regression tests: consent carrying across installs, missing RetroFlow follow-up
steps after automatic installation, and recovery after a pre-install disconnect.
Update consent now binds to the reviewed game, device and existing file hashes.

Deferred: ordinary desktop shutdown does not dispose of abandoned temporary
workspaces. They may retain substantial local data. Recovery data must remain
available until a separate cleanup and retention policy is implemented.
