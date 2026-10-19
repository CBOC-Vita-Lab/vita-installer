# Changes

## 0.1.10 public preview

- Recognize the Strikers LiveArea settings update from its GitHub URL, ZIP or folder.
- Allow app-only updates from compatible movie/quiet releases with a completed
  installer receipt; keep existing game data, saves and settings.
- Retain the transfer-speed and resume fixes from 0.1.9.
- Refresh the Apple Silicon app and bundled dependency notices. Signing,
  redistribution review and full installation acceptance remain pending.
- No new tests or Vita changes were made for this packaging update.

## 0.1.9 preview

- Reduce repeated FTP directory listings and redundant full-data readbacks.
- Offer **Review installation again** when protected files changed, rather
  than repeatedly resuming an outdated installation review.
- Keep parallel FTP disabled; no full-install speed estimate is claimed.

## 0.1.5 private preview

- Add app-only updates for verified, compatible installations. Skip ISO selection
  and media preparation while retaining complete installed-data checks.
- Preserve the checked data inventory across consecutive updates; missing or
  altered files require full setup. Update approval remains separate.
- Bundle the pinned Mac arm64 Galaxy quality-55 movie helper, source and notices.
  Complete preparation passes for all nine movies and 2,375 output files.
- Preserve exact known installed Galaxy optimized movies, saves and settings.
  Galaxy's distributable profile remains held for hardware/release acceptance.

## 0.1.4 private preview

- Show measured preparation, movie, copying and final verification progress.
- Add elapsed time and phase-specific estimates, suppressing unreliable counts
  during stalls, pauses and steps without known totals.

## 0.1.3 private preview

- Import approved public CBOC releases directly from a GitHub URL.
- Restore missing or damaged release files and the extraction tool automatically,
  with checksum checks, progress and cancellation.
- Keep repairs on the exact reviewed version, preserve original local inputs,
  and require a fresh review if a release changes after install planning.
- Never download replacement commercial games, BIOS or custom artwork.
- Private releases still need a local ZIP/folder; no GitHub credentials are included.

## 0.1.2 private preview

- Add the CBOC logo, desktop app icon and compact wordmark.
- Label titles already on the selected Vita or card before preparation.
- Keep updates blocked until the separate manual approval is checked.
- Accept the supported Infected PSP disc with either valid BOOT.BIN naming
  form; reject ambiguous records and retain the executable hash check.
- Verify real-Vita detection for all four titles, scoped FTP transfer and
  reconnect without updating installed games. Galaxy installation stays held.

## 0.1.1 private preview

- Stop on missing, moved, unreadable or changed required files.
- Return to the relevant picker and identify the file that needs replacing.
- Check every required Strikers extracted file against the supported revision.
- Recheck release files before preparation and installation, and prepared files
  before copying. Valid files continue through the normal flow.
- Offer file reselection for paused jobs with missing sources.

## 0.1.0 private preview

- Import a verified release ZIP or folder and choose a game visually.
- Detect Vita FTP, USB storage and configured memory cards.
- Prepare supported game files without a separate Python or Dolphin install.
- Preview default or custom bubble icons, bubble backgrounds and RetroFlow covers.
- Fit or crop custom artwork, adjust its position, and restore the default.
- Protect existing games, saves and covers; require fresh confirmation for updates.
- Resume interrupted transfers and verify installation before reporting success.
- Keep any remaining VitaShell or RetroFlow steps visible after transfer.

Apple Silicon Mac preview only. Windows, Intel Mac, physical installation,
public signing and redistribution acceptance remain pending. Galaxy is held.

Known issue: abandoned preparation workspaces are not automatically cleaned up
when the desktop app closes. Recovery data is retained; a bounded cleanup and
retention policy remains to be implemented.
