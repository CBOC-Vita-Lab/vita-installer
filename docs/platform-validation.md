# Platform validation

## Current evidence

The Apple Silicon macOS app has been packaged and launched with an empty
`PATH`. Its real native file dialog imported the matching private VitAdhoc ZIP.
The app detected the Vita read-only, prepared the local package, displayed all
three artwork previews, accepted a custom bubble image and restored the default.
No Install command was sent and no Vita files were changed.

Separate packaged-worker checks cover real ISO parsing, missing/corrupt inputs,
incomplete Galaxy conversion, exclusive volume writes, symlink/root changes
and execution without Python on `PATH`. Loopback FTP and temporary-directory
tests exercise transfers and backups. They are not physical-card or Vita
installation acceptance.

Package checks enumerate resources, verify helper hashes/architectures,
reject owner paths/credentials and inspect compressed Python members. The
bundle includes a component inventory, license texts and known notice gaps.

## Not yet accepted

- Intel Mac and Windows native builds/runs.
- First install and reconnect/update acceptance on a separately approved Vita.
- Physical removable-media detection and card-swap tests on both operating systems.
- End-to-end preparation comparisons for every enabled game on each platform.
- Public signing/notarization and complete third-party redistribution review.

The current Mac artifact is an unsigned/ad-hoc internal preview. It is not
notarized. Do not disable Gatekeeper or other OS protections to distribute it.
No paid CI job, signing enrollment or certificate purchase has been started.

## Repeat on a native runner

Use the requested architecture, Node 24 and Python 3.14.4. Create
`.venv-workers`, install `workers/requirements.txt` and test-only
`pyftpdlib==2.2.0`, then run:

```sh
npm ci
node scripts/package-workers.mjs
npm test
npm run typecheck
npm run package
node scripts/verify-release.mjs
node scripts/smoke-installed.mjs PATH_TO_PACKAGED_APP
```

Set `VITA_WORKER_PYTHON` when the build-time interpreter is elsewhere. Players
do not install it. Set `VITA_TEST_WORKER` and `VITA_TEST_VOLUME_WORKER` to the
executables inside the packaged app to run the Python suites and real-IO tests.
Retain `out/package-verification.json`, `out/packaged-smoke.json` and test logs
with the exact app archive hash. Build on the actual target; renaming an arm64
artifact is not an Intel or Windows build.

The Windows paths and behavior remain unverified. The Mac validation receipt
cannot be reused to mark Windows accepted. No public game release is cleared
by an installer test.
