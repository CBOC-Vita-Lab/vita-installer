# Opening the app on a Mac

The app is not notarized by Apple (that needs a paid Apple developer account), so macOS asks before the first launch. You only do this once.

1. Unzip `CBOC-Vita-Installer-…-mac.zip` and drag **CBOC Vita Installer** to **Applications**.
2. Right-click (or Control-click) the app and choose **Open**, then **Open** again.
3. If macOS still says it can't be opened (macOS 15 and later): open **System Settings > Privacy & Security**, scroll down to the message about CBOC Vita Installer and click **Open Anyway**.
4. When asked whether the app may find devices on your local network, click **Allow**. That is how it finds your Vita on Wi-Fi.

The app has no window of its own: it opens a page in your browser and shows no Dock icon. To quit it, click the status button at the top right of the page and choose **Quit installer**, or just close the tab. It quits by itself a minute or two later.

Please don't turn off Gatekeeper or remove quarantine flags with Terminal commands. The steps above are all you need.

Works on macOS 10.13 High Sierra and later, on both Intel and Apple Silicon Macs.
