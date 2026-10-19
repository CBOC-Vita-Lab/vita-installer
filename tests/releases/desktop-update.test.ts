import { expect, it } from 'vitest';
import { DesktopUpdates } from '../../src/releases/desktop-update';

const base = 'https://github.com/CBOC-Vita-Lab/vita-installer/releases';
function release(version = '0.2.0', platform = 'darwin', arch = 'arm64') {
  const name = `CBOC-Vita-Installer-${version}-${platform}-${arch}.${platform === 'win32' ? 'exe' : 'zip'}`;
  return { tag_name: `v${version}`, draft: false, prerelease: false, html_url: `${base}/tag/v${version}`,
    assets: [{ name, state: 'uploaded', size: 200, digest: `sha256:${'a'.repeat(64)}`,
      browser_download_url: `${base}/download/v${version}/${name}` }] };
}
function updater(data: unknown = release(), version = '0.1.7', platform = 'darwin', arch = 'arm64') {
  const opened: string[] = [], changes: string[] = [];
  const service = new DesktopUpdates({ version, platform, arch, fetcher: async (url, options) => {
    expect(url).toBe('https://api.github.com/repos/CBOC-Vita-Lab/vita-installer/releases/latest');
    expect(options).toMatchObject({ redirect: 'error', credentials: 'omit' });
    expect(options?.signal).toBeInstanceOf(AbortSignal);
    return Response.json(data);
  }, open: async url => { opened.push(url); }, changed: () => { changes.push(service.snapshot().status); } });
  return { service, opened, changes };
}
it('checks stable versions numerically and opens only the matching official asset on request', async () => {
  const { service, opened, changes } = updater(release('0.10.0'), '0.9.0');
  await service.check();
  expect(service.snapshot()).toMatchObject({ status: 'available', currentVersion: '0.9.0', latestVersion: '0.10.0' });
  expect(changes).toEqual(['checking', 'available']); expect(opened).toEqual([]);
  await service.openDownload(false);
  expect(opened).toEqual([`${base}/download/v0.10.0/CBOC-Vita-Installer-0.10.0-darwin-arm64.zip`]);
  expect(JSON.stringify(service.snapshot())).not.toContain('https:');
});
it.each(['0.2.0', '0.3.0'])('never offers a downgrade from %s', async version => {
  const { service, opened } = updater(release(), version); await service.check();
  expect(service.snapshot().status).toBe('current');
  await expect(service.openDownload(false)).rejects.toThrow(); expect(opened).toEqual([]);
});
it('does not open an update while a Vita operation is running', async () => {
  const { service, opened } = updater(); await service.check();
  await expect(service.openDownload(true)).rejects.toThrow(); expect(opened).toEqual([]);
});
it.each([
  release('0.2.0', 'darwin', 'x64'), release('0.2.0', 'win32', 'x64'),
  { ...release(), assets: [] }
])('does not substitute another architecture, platform or source archive', async data => {
  const { service, opened } = updater(data); await service.check();
  expect(service.snapshot().status).toBe('no-build');
  await expect(service.openDownload(false)).rejects.toThrow(); expect(opened).toEqual([]);
});
it('supports the versioned Windows setup name without treating it as a Mac build', async () => {
  const { service, opened } = updater(release('0.2.0', 'win32', 'x64'), '0.1.7', 'win32', 'x64');
  await service.check(); await service.openDownload(false);
  expect(opened[0]).toBe(`${base}/download/v0.2.0/CBOC-Vita-Installer-0.2.0-win32-x64.exe`);
});
it.each([
  { ...release(), draft: true }, { ...release(), prerelease: true },
  { ...release(), tag_name: 'v0.2.0-beta.1' }, { ...release(), tag_name: 'not-a-version' },
  { ...release(), html_url: 'https://example.com/download' },
  { ...release(), html_url: `${base}/tag/v0.2.0?token=private` },
  { ...release(), tag_name: 'v999999999999999999999999.0.0' }
])('rejects draft, prerelease, invalid or redirected metadata', async data => {
  const { service } = updater(data); await service.check(); expect(service.snapshot().status).toBe('unavailable');
});
it.each([
  { browser_download_url: 'https://evil.test/update.zip' },
  { browser_download_url: `${base}/download/v0.2.0/../../other.exe` },
  { browser_download_url: `${base}/download/v0.2.0/CBOC-Vita-Installer-0.2.0-darwin-arm64.zip?token=private` },
  { state: 'new' }, { digest: null }, { size: 0 }
])('does not offer an incomplete or substituted asset', async patch => {
  const data = release(); Object.assign(data.assets[0], patch);
  const { service, opened } = updater(data); await service.check();
  expect(service.snapshot().status).toBe('no-build');
  await expect(service.openDownload(false)).rejects.toThrow(); expect(opened).toEqual([]);
});
it('rejects ambiguous duplicate platform assets', async () => {
  const data = release(); data.assets.push(data.assets[0]);
  const { service } = updater(data); await service.check(); expect(service.snapshot().status).toBe('no-build');
});
it.each([404, 403, 429, 500])('reports HTTP %s as unavailable, never up to date', async status => {
  const service = new DesktopUpdates({ version: '0.1.7', platform: 'darwin', arch: 'arm64',
    fetcher: async () => new Response(null, { status }), open: async () => {}, changed: () => {} });
  await service.check(); expect(service.snapshot().status).toBe('unavailable');
});
it('bounds responses, handles offline checks, and allows a later retry', async () => {
  let attempt = 0;
  const service = new DesktopUpdates({ version: '0.1.7', platform: 'darwin', arch: 'arm64', fetcher: async () => {
    attempt++; if (attempt === 1) throw new Error('offline');
    if (attempt === 2) return new Response(' '.repeat(256 * 1024 + 1));
    if (attempt === 3) return new Response('{invalid');
    return Response.json(release());
  }, open: async () => {}, changed: () => {} });
  for (let i = 0; i < 3; i++) { await service.check(); expect(service.snapshot().status).toBe('unavailable'); }
  await service.check(); expect(service.snapshot().status).toBe('available');
});
it('coalesces overlapping checks and clears stale download actions on a failed recheck', async () => {
  let complete!: (response: Response) => void, requests = 0;
  const service = new DesktopUpdates({ version: '0.1.7', platform: 'darwin', arch: 'arm64',
    fetcher: () => { requests++; return new Promise(resolve => { complete = resolve; }); }, open: async () => {}, changed: () => {} });
  const a = service.check(), b = service.check(); expect(requests).toBe(1);
  complete(Response.json(release())); await Promise.all([a, b]); expect(service.snapshot().status).toBe('available');
  const c = service.check(); await expect(service.openDownload(false)).rejects.toThrow();
  complete(new Response(null, { status: 404 })); await c; expect(service.snapshot().status).toBe('unavailable');
  await expect(service.openDownload(false)).rejects.toThrow();
});
