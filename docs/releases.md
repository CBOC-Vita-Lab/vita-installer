# Release bundles

Each game repository points to the same CBOC Vita Installer. Import a reviewed
release ZIP or folder containing `cboc-release.json` and its VPK. A parent
folder can hold multiple releases. Source archives are not install releases.

The installer trusts exact manifest SHA-256 fingerprints or an Ed25519
signature from a built-in publisher key. A signed envelope has only `keyId`,
`manifest` (base64 of the original UTF-8 JSON bytes), and `signature` (base64
Ed25519 signature over those exact bytes). Do not reserialize before signing.
Duplicate keys, comments, executable instructions and unknown schemas fail.
Keys supplied by a downloaded bundle never establish trust. Private releases
are selected locally; no GitHub password or token is shipped with the app.

VPK, runtime and title hashes are verified before use. Custom artwork is a
local derivative: retain the original bundle and record image/output hashes,
then verify all non-art VPK members are unchanged. Never rebuild game code.

## Adding a release

Add a reviewed data-only profile and its exact raw-byte fingerprint to the
built-in trust store, or sign it with an already trusted Ed25519 publisher key.
Keep private keys outside the repository. Verify the VPK SHA-256, `eboot.bin`
SHA-256, SFO title, input revision, destination and protected paths. The
source-records file identifies documentation/source snapshots; it does not
prove that their binaries reproduce the selected runtime.

New recipe IDs need an app update with bounded conversion and transfer tests.
A bundle cannot introduce commands, executable paths, network URLs or storage
roots. Test malformed archives, same-name conflicts, disconnected targets,
insufficient/unknown space, save preservation and restart recovery. Keep the
release held until the exact package passes device and rights checks.

Publisher-only outer bundles can be assembled with `node scripts/bundle.mjs NEW_OUTPUT_DIRECTORY`
and `VITA_PUBLISHING_ROOT` pointing to the private publishing workspace.
The pinned original ZIPs are never replaced. The new ZIP adds
`cboc-release.json`, `START-INSTALLER.md` and regenerated checksums; the VPK,
source and license payloads stay unchanged. Import readback is required.

GitHub checks are optional, unauthenticated and limited to the four fixed
public release endpoints. Offline/private/mismatched metadata never invalidates
a trusted local bundle. The app does not download or execute release assets
based on GitHub metadata. Private manifests must be removed from a future
public installer unless their release gates are explicitly cleared.
