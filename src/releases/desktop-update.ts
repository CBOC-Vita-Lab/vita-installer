import { gt, valid } from 'semver';
import { z } from 'zod';
import type { DesktopUpdateView } from '../shared/ui';

const releases = 'https://github.com/CBOC-Vita-Lab/vita-installer/releases';
const metadata = z.object({
  tag_name: z.string().regex(/^v\d+\.\d+\.\d+$/).max(100), draft: z.literal(false), prerelease: z.literal(false),
  html_url: z.string(), assets: z.array(z.unknown()).max(100)
});
const assetSchema = z.object({ name: z.string(), state: z.literal('uploaded'),
  size: z.number().int().positive().max(2 * 1024 ** 3), digest: z.string().regex(/^sha256:[a-f0-9]{64}$/),
  browser_download_url: z.string() });
interface Options {
  version: string; platform: string; arch: string; fetcher?: typeof fetch;
  open: (url: string) => Promise<void>; changed: () => void;
}

// Desktop updates never enter the Vita job worker or execute downloaded code.
export class DesktopUpdates {
  private view: DesktopUpdateView;
  private downloadUrl?: string;
  private checking?: Promise<void>;
  constructor(private options: Options) {
    const { version, platform, arch } = options;
    const label = platform === 'darwin' ? arch === 'arm64' ? 'macOS (Apple Silicon)' : 'macOS (Intel)'
      : platform === 'win32' ? `Windows (${arch})` : `${platform} (${arch})`;
    this.view = { status: 'idle', currentVersion: version, platform: label };
  }
  snapshot(): DesktopUpdateView { return { ...this.view }; }
  check(): Promise<void> {
    if (!this.checking) this.checking = this.lookup().finally(() => { this.checking = undefined; });
    return this.checking;
  }
  private async lookup(): Promise<void> {
    this.downloadUrl = undefined;
    this.view = { ...this.view, status: 'checking', latestVersion: undefined }; this.options.changed();
    try {
      const response = await (this.options.fetcher ?? fetch)('https://api.github.com/repos/CBOC-Vita-Lab/vita-installer/releases/latest', {
        redirect: 'error', credentials: 'omit', signal: AbortSignal.timeout(8000), headers: { Accept: 'application/vnd.github+json' }
      });
      if (!response.ok || !response.body) throw new Error('Unavailable');
      const reader = response.body.getReader(), chunks: Uint8Array[] = []; let size = 0;
      try {
        for (;;) {
          const part = await reader.read(); if (part.done) break;
          size += part.value.length; if (size > 256 * 1024) throw new Error('Oversized response');
          chunks.push(part.value);
        }
      } finally { await reader.cancel(); }
      const data = metadata.parse(JSON.parse(Buffer.concat(chunks).toString('utf8')));
      const version = data.tag_name.slice(1);
      if (!valid(version) || !valid(this.options.version) || data.html_url !== `${releases}/tag/${data.tag_name}`) throw new Error('Invalid release');
      this.view.latestVersion = version;
      if (!gt(version, this.options.version)) this.view.status = 'current';
      else {
        const { platform, arch } = this.options;
        const supported = (platform === 'darwin' && ['arm64', 'x64'].includes(arch)) || (platform === 'win32' && arch === 'x64');
        const names = supported ? [`CBOC-Vita-Installer-${version}-${platform}-${arch}.${platform === 'win32' ? 'exe' : 'zip'}`] : [];
        const assets = data.assets.flatMap(raw => {
          const parsed = assetSchema.safeParse(raw); if (!parsed.success) return [];
          const asset = parsed.data;
          return names.includes(asset.name) && asset.browser_download_url === `${releases}/download/${data.tag_name}/${asset.name}` ? [asset] : [];
        });
        this.downloadUrl = assets.length === 1 ? assets[0].browser_download_url : undefined;
        this.view.status = this.downloadUrl ? 'available' : 'no-build';
      }
    } catch { this.view = { ...this.view, status: 'unavailable', latestVersion: undefined }; }
    this.options.changed();
  }
  async openDownload(busy: boolean): Promise<void> {
    if (busy || this.view.status !== 'available' || !this.downloadUrl) throw new Error('Update unavailable');
    await this.options.open(this.downloadUrl);
  }
}
