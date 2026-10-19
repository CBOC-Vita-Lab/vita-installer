# Changes

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
