# First Mac Launch

Use **0.1.11 or newer** on an Apple Silicon Mac (M1 or later).
Version 0.1.10 has an incomplete app signature and can be reported as damaged.

1. Download the named Mac ZIP from the
   [official releases](https://github.com/CBOC-Vita-Lab/vita-installer/releases/latest).
2. Unzip it. Quit any older installer after its current operation finishes,
   then move **CBOC Vita Installer.app** to Applications and replace the old app.
3. Open the app. If macOS says it cannot verify the developer, dismiss the warning.
4. Open **System Settings > Privacy & Security**, choose **Open Anyway** for
   CBOC Vita Installer, then confirm **Open**. Authenticate if macOS requests it.

Only approve an app obtained from the official release that you intend to run.
This approval is specific to this app; do not disable Gatekeeper globally or
run commands to strip quarantine. The preview is ad-hoc signed for integrity,
not signed by an Apple-identified developer or notarized by Apple.

If **0.1.11** still says **damaged**, or macOS says it **will damage your computer**,
stop rather than bypassing that warning. Confirm the version and compare the ZIP
with `INSTALLER-SHA256SUMS.txt` from the same release. Report the exact message,
macOS version and Mac chip. Do not attach personal logs, game files or saves.

Replacing the desktop app does not change Vita games. Keep its app-data folder
so installation history and recovery records are retained.

[Apple's instructions](https://support.apple.com/en-us/102445) explain per-app
approval and the different security warnings.
