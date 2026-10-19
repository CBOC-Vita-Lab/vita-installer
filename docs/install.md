# Connect and install

## Wi-Fi (preferred)

Use the same trusted local network as your Vita. The app detects a supported
FTP service automatically. If nothing appears, open VitaShell, set its SELECT
button to FTP, start FTP and click **Retry**. A manual address is available
if needed. Do not forward router ports or use public Wi-Fi for transfers.

Automatic VPK installation is used only when the detected service supports
it. Otherwise, the result shows the copied VPK to open in VitaShell. A plain
FTP connection alone does not mean automatic installation is available.

## USB or memory card

Start VitaShell USB, or connect a card already configured as the Vita's main
storage. Select the detected drive. If its role cannot be verified, confirm
that it is the Vita's **ux0** storage before continuing. The app does not
format drives, change storage mappings or configure adapters.

When finished, safely eject the drive. For a card reader, insert the card
into the powered-off Vita before turning it on. Open the copied VPK in
VitaShell. If a required shader file could not be placed in **ur0**, the
result gives its exact source and destination.

## Files and artwork

Paste a supported CBOC game repository or release URL, or choose a release ZIP,
extracted folder, or parent folder containing several releases. Private releases
need a local ZIP/folder. Select your own game files. Only selected folders and the
selected Vita's prerequisite location are checked, not your whole computer.

An existing title is labelled **Already installed**. Choosing it does not
approve a reinstall. Updates require the separate checkbox on the final review;
changing the game or destination clears that approval.

Files are checked automatically before preparation and again before copying.
Missing or damaged release files and the downloadable extraction tool repair
automatically when available. Repairs use verified copies in the app's cache,
not changes to your original folder, and never silently upgrade the release.
If repair is unavailable, choose the complete release ZIP/folder. Game files,
BIOS, shader files and custom artwork must come from you; missing or changed
inputs stop installation and show what to replace. Valid files need no extra
confirmation. A repair after final review requires a fresh review before copying.
Strikers' extracted folder
is checked against all 2,359 expected files, including their sizes and hashes.

Review the bubble icon, bubble background and RetroFlow cover. Keep defaults
or select PNG, JPEG or still WebP images. Custom images stay local. Existing
RetroFlow artwork is kept unless you explicitly choose a backed-up replacement.

Click **Install** after reviewing the destination. If free space cannot be
read over FTP, the app shows the required amount for you to confirm. Keep the
computer and Vita awake. On interruption, reconnect the same destination and
choose **Resume**; do not delete the job folder or local prepared files.
