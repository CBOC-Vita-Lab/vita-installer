# Installer updates

All games use one desktop app, maintained in
[CBOC-Vita-Lab/vita-installer](https://github.com/CBOC-Vita-Lab/vita-installer).
Game releases and installer releases are separate.

## For players

1. Open the download icon beside the installer's version number.
2. Click **Download update** when a newer build is available.
3. Finish any current operation, quit the installer, then replace the app.

On Mac, open the ZIP and replace **CBOC Vita Installer.app** in the same
location. On Windows, run the new setup file after closing the old app.
Intel Mac and Windows builds remain validation candidates, not accepted player
updates. Use only the architecture and status listed in [Downloads](downloads.md).
Never remove the installer's app-data folder during an update. Its history,
receipts and recovery files are separate from the program and are reused.
Your Vita games, saves and selected local files are not changed by an app update.

The app checks public GitHub release information at startup and every 24 hours
while open. It does not send game files, device addresses or installation
history. GitHub still receives normal connection information, including your
IP address. There is no GitHub login or token inside the installer.
Offline checks, private repos and unpublished releases show **unavailable**,
not **up to date**. The releases button opens GitHub for manual downloads.
Drafts and prereleases are never offered automatically.

This preview opens the correct download in your browser; it does not silently
download, execute or replace software. Download buttons are blocked during a
Vita operation. Updates use the full desktop package, not a binary patch.
Game app-only updates are separate and can skip unchanged prepared game data.

## Shipping a new version

The publisher chose to make the existing Apple Silicon 0.1.9 package public
with its pending-approval notice, without another test run. This is a publication
decision, not evidence that the outstanding signing, rights or device checks
have passed. The checklist below remains the normal process for future versions.

1. Bump `version` in `package.json` and the lockfile. Keep the app name,
   `org.cboc.vita-installer` bundle ID and app-data location unchanged.
2. Run the full tests, typecheck, native packaging and package verification.
   Follow [platform validation](platform-validation.md); each platform needs
   its own accepted build. Test replacing an earlier desktop version with
   existing history/recovery data before public release.
3. Prepare a **draft** release in this repo and upload the verified package,
   checksums and short release notes. Use these exact asset names, replacing
   `VERSION` with the package version:

   - `CBOC-Vita-Installer-VERSION-darwin-arm64.zip`
   - `CBOC-Vita-Installer-VERSION-darwin-x64.zip`
   - `CBOC-Vita-Installer-VERSION-win32-x64.exe`

4. Download the assets again and compare SHA-256 values. GitHub metadata must
   show `uploaded`, a nonzero size and a SHA-256 digest. Upload only one package
   for each accepted platform. Source-code ZIPs are not desktop installers.
5. After signing, redistribution and platform gates pass, publish a stable
   release tagged `vVERSION` and mark it **Latest**. Public update discovery
   requires a public repo and published stable release. Do not change repo
   visibility as part of routine build automation.
6. From the previous version, verify the update notice, platform-specific
   download and retained history. Do not replace assets under an existing
   version: publish a new version for corrections.

Internal tags such as `v0.1.7-internal.1` stay draft/prerelease. A failed check
does not invalidate an intact local game bundle. A stable version with no
matching platform download is reported separately; another architecture is
never substituted.

The updater only opens a validated URL in this repository. It never trusts a
download URL provided by the renderer, a game bundle or release-note text.
GitHub's digest is required metadata, not a claim that a browser download has
been locally verified by the installer.

## Automatic replacement

Not enabled in the unsigned preview. Electron's supported Mac auto-updater
requires a signed app; the hosted GitHub update service also requires public
releases. Select a reviewed signing identity before enabling that path, and
keep restart/app replacement blocked until all Vita operations are finished.
Do not embed publisher credentials or disable OS security checks.
See [Electron's update guide](https://www.electronjs.org/docs/latest/tutorial/updates)
and [GitHub's release API](https://docs.github.com/en/rest/releases/releases#get-the-latest-release).
