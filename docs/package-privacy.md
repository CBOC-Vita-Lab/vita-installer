# Package privacy checks

Final-package verification rejects owner-home paths and token patterns. Windows
paths are checked with either separator, case-insensitively, including their
JSON-escaped form. Compressed Python members have a separate privacy audit.

One exact upstream artifact has a recorded build-path exception:

- Package: `@img/sharp-libvips-darwin-x64` 1.3.4.
- Member: `package/lib/libvips-cpp.8.18.7.dylib`.
- SHA-256: `fb2101ad991f683a46204ea0ac96a4269c4c93ecef023a0f841c93ebc9843353`.

Its official npm archive matches the lockfile's SHA-512 integrity. The native
CI package contains the identical library. Six embedded paths belong to the
upstream `sharp-libvips` build under `/Users/runner/work/sharp-libvips/`, which
collides with our generic Intel CI home. They are not the publisher's paths.

The exception applies only when the current home is exactly `/Users/runner`,
the package member has the exact expected path, and its full bytes have the
reviewed hash. Changed bytes, a renamed artifact, another home, or any token
pattern still fail. Verification receipts list every applied exception and
its source provenance. A dependency update requires another review.

This does not approve signing, redistribution rights or physical installation.
