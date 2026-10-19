# Release bundles

Each game repository points to the same CBOC Vita Installer. Paste its public
repository/release URL, or import a reviewed ZIP/folder containing
`cboc-release.json` and its VPK. A parent
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

## GitHub downloads and repair

Downloads are unauthenticated and limited to the four fixed CBOC repositories;
Galaxy remains blocked. Repository, release-page, tag and asset URLs are
supported. A repository URL selects the latest stable release. A tag URL pins
that release. Private releases still require a locally selected bundle.

After release gates are cleared, publish one outer bundle as `cboc-release.zip`
and attach its exact `cboc-release.json` separately. The existing pinned bundle
filenames are also accepted, but do not publish both names on the same release.
GitHub asset metadata must include its SHA-256 digest and uploaded state.
Source-code archives are not install bundles.

The manifest attachment lets the app find an older exact version among the
latest 30 published releases without downloading unrelated ZIPs. Legacy releases
without that attachment can repair from matching latest metadata only; an
older local ZIP remains the fallback. URL imports retain their exact asset URL.
Every repair still authenticates the downloaded manifest and compares its exact
hash, VPK hash, runtime and title. A metadata digest alone cannot authorize code.

Downloads enforce HTTPS, approved GitHub/CDN hosts, time/size limits and checksum
verification. Only the authenticated manifest and VPK enter input discovery;
extra downloaded files never become user game/BIOS/artwork inputs. Partial or
failed downloads are removed. Original local folders are never repaired in place.
The pinned extraction helper can also repair its own cache; bundled helpers
require reinstalling the app if damaged.

Offline or private metadata does not invalidate an intact trusted local bundle.
No GitHub credentials are shipped or requested. Private manifests must be removed
from a future public installer unless their release gates are explicitly cleared.
