import { afterEach, expect, it } from 'vitest';
import { mkdtemp, readdir, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { downloadGithubRelease, parseGithubReleaseUrl } from '../../src/releases/download';
import { digest, manifest, runtime, sfo, zip } from '../fixtures/releases';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
const repo = 'https://github.com/CBOC-Vita-Lab/strikers-charged-vita';
async function fixture(options: { manifestChanges?: object; untrusted?: boolean; galaxy?: boolean } = {}) {
  const repository = options.galaxy ? 'https://github.com/CBOC-Vita-Lab/galaxy-vita' : repo;
  const cache = await mkdtemp(join(tmpdir(), 'github-release-test-')); roots.push(cache);
  const vpk = await zip({ 'eboot.bin': runtime, 'sce_sys/param.sfo': sfo(options.galaxy ? 'SMGV00001' : undefined) });
  const raw = JSON.stringify({ ...manifest(digest(vpk)), ...(options.galaxy ? { recipeId: 'galaxy', titleId: 'SMGV00001', destinations: { data: 'ux0:data/smg/game' } } : {}), ...options.manifestChanges });
  const bundle = await zip({ 'release/cboc-release.json': raw, 'release/Strikers.vpk': vpk, 'release/game.iso': 'not an approved input', 'release/notes.txt': 'not imported' });
  const asset = `${repository}/releases/download/v1.0/cboc-release.zip`;
  const metadata = { tag_name: 'v1.0', draft: false, prerelease: false, html_url: `${repository}/releases/tag/v1.0`,
    assets: [{ name: 'cboc-release.zip', browser_download_url: asset, size: bundle.length, state: 'uploaded', digest: `sha256:${digest(bundle)}` }] };
  const calls: string[] = [];
  const fetcher: typeof fetch = async (url, init) => {
    calls.push(String(url)); expect(init).toMatchObject({ credentials: 'omit', redirect: 'manual' });
    expect(JSON.stringify(init?.headers ?? {})).not.toMatch(/authorization|cookie/i);
    if (String(url).endsWith('/releases?per_page=30')) return new Response(JSON.stringify([metadata]));
    if (String(url).startsWith('https://api.github.com/')) return new Response(JSON.stringify(metadata));
    if (url === asset) return new Response(Uint8Array.from(bundle));
    throw new Error('Unexpected destination');
  };
  return { cache, vpk, bundle, metadata, asset, raw, calls, fetcher,
    trust: { fingerprints: new Set(options.untrusted ? [] : [digest(raw)]), keys: new Map() } };
}
it('accepts repository, release page, tag and asset links without accepting arbitrary URLs', () => {
  for (const url of [repo, `${repo}/`, `${repo}.git`, `${repo}/releases`, `${repo}/releases/latest`])
    expect(parseGithubReleaseUrl(url)).toEqual({ recipe: 'strikers' });
  expect(parseGithubReleaseUrl(`${repo}/releases/tag/v1.0`)).toMatchObject({ recipe: 'strikers', tag: 'v1.0' });
  expect(parseGithubReleaseUrl(`${repo}/releases/download/v1.0/cboc-release.zip`)).toMatchObject({ tag: 'v1.0', asset: 'cboc-release.zip' });
  for (const url of [`http://github.com/CBOC-Vita-Lab/strikers-charged-vita`, `${repo}?token=secret`, `${repo}#token`,
    'https://user:secret@github.com/CBOC-Vita-Lab/strikers-charged-vita', 'https://github.com.evil.test/CBOC-Vita-Lab/strikers-charged-vita',
    'https://github.com/other/project', `${repo}/tree/main`, `${repo}/releases/tag/%2e%2e`, `${repo}/releases/tag/a%2Fb`])
    expect(() => parseGithubReleaseUrl(url)).toThrow();
});
it('downloads, authenticates and imports only the reviewed package, never bundled user inputs', async () => {
  const f = await fixture(), progress: number[] = [];
  const result = await downloadGithubRelease(repo, { ...f, onProgress: p => progress.push(p.bytes) });
  expect(result.manifest.titleId).toBe('MSCN00003'); expect(result.downloadUrl).toBe(f.asset);
  expect(await readFile(result.vpkPath!)).toEqual(f.vpk);
  expect((await readdir(result.sourceRoot)).sort()).toEqual(['Strikers.vpk', 'cboc-release.json']);
  expect(f.calls).toEqual(['https://api.github.com/repos/CBOC-Vita-Lab/strikers-charged-vita/releases/latest', f.asset]);
  expect(progress.at(-1)).toBe(f.bundle.length);
});
it('pins repairs to the exact authenticated manifest instead of silently updating', async () => {
  const f = await fixture();
  await expect(downloadGithubRelease(repo, { ...f, expectedManifestSha256: 'e'.repeat(64) })).rejects.toMatchObject({ code: 'repair_version_unavailable' });
  expect(await readdir(f.cache)).toEqual([]);
  expect((await downloadGithubRelease(repo, { ...f, expectedManifestSha256: digest(f.raw) })).manifestSha256).toBe(digest(f.raw));
});
it('finds an older exact repair by its published manifest digest without downloading newer bundles', async () => {
  const f = await fixture(), older = { ...f.metadata, assets: [...f.metadata.assets, {
    name: 'cboc-release.json', size: Buffer.byteLength(f.raw), state: 'uploaded', digest: `sha256:${digest(f.raw)}`,
    browser_download_url: `${repo}/releases/download/v1.0/cboc-release.json`
  }] };
  const newer = { ...f.metadata, tag_name: 'v2.0', html_url: `${repo}/releases/tag/v2.0`, assets: [] };
  const calls: string[] = [];
  const result = await downloadGithubRelease(repo, { ...f, expectedManifestSha256: digest(f.raw), fetcher: async (url, init) => {
    calls.push(String(url));
    if (String(url).endsWith('/releases?per_page=30')) return new Response(JSON.stringify([{ ...newer, tag_name: 'archive/v0' }, newer, older]));
    if (String(url).endsWith('/latest')) return new Response(JSON.stringify(newer));
    return f.fetcher(url, init);
  } });
  expect(result.manifestSha256).toBe(digest(f.raw)); expect(result.downloadUrl).toBe(f.asset);
  expect(calls).toEqual(['https://api.github.com/repos/CBOC-Vita-Lab/strikers-charged-vita/releases?per_page=30', f.asset]);
});
it('does not trust a matching published manifest marker without authenticating the actual bundle', async () => {
  const f = await fixture(); f.metadata.assets.push({ name: 'cboc-release.json', size: 10, state: 'uploaded',
    digest: `sha256:${'e'.repeat(64)}`, browser_download_url: `${repo}/releases/download/v1.0/cboc-release.json` });
  await expect(downloadGithubRelease(repo, { ...f, expectedManifestSha256: 'e'.repeat(64) })).rejects.toMatchObject({ code: 'repair_version_unavailable' });
  expect(await readdir(f.cache)).toEqual([]);
});
it('refuses untrusted, held, wrong-repository or corrupt payloads and cleans its temporary files', async () => {
  for (const options of [{ untrusted: true }, { manifestChanges: { releaseState: 'held' } },
    { manifestChanges: { recipeId: 'vitadhoc', titleId: 'GBAD00001', destinations: { data: 'ux0:data/GBAdhoc/roms' } } }]) {
    const f = await fixture(options);
    await expect(downloadGithubRelease(repo, f)).rejects.toThrow(); expect(await readdir(f.cache)).toEqual([]);
  }
  const f = await fixture(); f.metadata.assets[0].digest = `sha256:${'b'.repeat(64)}`;
  await expect(downloadGithubRelease(repo, f)).rejects.toMatchObject({ code: 'download_hash_mismatch' }); expect(await readdir(f.cache)).toEqual([]);
});
it('does not guess among multiple bundles, follow metadata redirects or expose inaccessible releases', async () => {
  const f = await fixture(); f.metadata.assets.push({ ...f.metadata.assets[0], name: 'Strikers-a8d4ad6-desktop-private.zip' });
  await expect(downloadGithubRelease(repo, f)).rejects.toMatchObject({ code: 'ambiguous_release_download' });
  for (const status of [302, 403, 404, 429]) await expect(downloadGithubRelease(repo, { ...f,
    fetcher: async () => new Response('', { status, headers: { Location: 'https://evil.test/' } }) })).rejects.toMatchObject({ code: 'github_release_unavailable' });
  expect(await readdir(f.cache)).toEqual([]);
});
it('validates GitHub asset redirects and rejects changed size, unsafe targets and cancellation', async () => {
  const f = await fixture();
  for (const location of ['http://release-assets.githubusercontent.com/file', 'https://evil.test/file', 'http://127.0.0.1/file', 'https://u:p@release-assets.githubusercontent.com/file']) {
    const calls: string[] = [];
    await expect(downloadGithubRelease(repo, { ...f, fetcher: async (url, init) => {
      calls.push(String(url)); return String(url).startsWith('https://api.github.com/') ? f.fetcher(url, init) : new Response('', { status: 302, headers: { Location: location } });
    } })).rejects.toMatchObject({ code: 'untrusted_download_redirect' });
    expect(calls).not.toContain(location); expect(await readdir(f.cache)).toEqual([]);
  }
  const redirect = 'https://release-assets.githubusercontent.com/github-production-release-asset/file?signature=temporary';
  const result = await downloadGithubRelease(repo, { ...f, fetcher: async (url, init) => url === f.asset
    ? new Response('', { status: 302, headers: { Location: redirect } }) : url === redirect ? new Response(Uint8Array.from(f.bundle)) : f.fetcher(url, init) });
  expect(result.downloadUrl).toBe(f.asset);
  const abort = new AbortController(); abort.abort();
  await expect(downloadGithubRelease(repo, { ...f, signal: abort.signal })).rejects.toMatchObject({ code: 'cancelled' });
  f.metadata.assets[0].size++;
  await expect(downloadGithubRelease(repo, f)).rejects.toMatchObject({ code: 'download_size_mismatch' });
});
it('cleans an interrupted or cancelled download and can retry from a fresh transfer', async () => {
  const f = await fixture();
  for (const mode of ['interrupted', 'cancelled']) {
    const abort = new AbortController();
    await expect(downloadGithubRelease(repo, { ...f, signal: abort.signal, fetcher: async (url, init) => {
      if (url !== f.asset) return f.fetcher(url, init);
      return new Response(new ReadableStream({ start(controller) {
        controller.enqueue(Uint8Array.from(f.bundle.subarray(0, 20)));
        if (mode === 'cancelled') abort.abort(); else controller.error(new Error('connection lost'));
      } }));
    } })).rejects.toMatchObject({ code: mode === 'cancelled' ? 'cancelled' : 'download_failed' });
    expect(await readdir(f.cache)).toEqual([]);
  }
  expect((await downloadGithubRelease(repo, f)).manifest.titleId).toBe('MSCN00003');
});
it('rejects oversized metadata/assets and latest prereleases before fetching a payload', async () => {
  const f = await fixture(); f.metadata.prerelease = true;
  await expect(downloadGithubRelease(repo, f)).rejects.toMatchObject({ code: 'github_release_unavailable' });
  f.metadata.prerelease = false; f.metadata.assets[0].size = 512 * 1024 ** 2 + 1;
  await expect(downloadGithubRelease(repo, f)).rejects.toMatchObject({ code: 'release_download_unavailable' });
  expect(f.calls).not.toContain(f.asset);
  await expect(downloadGithubRelease(repo, { ...f, fetcher: async () => new Response(' '.repeat(1024 * 1024 + 1)) })).rejects.toMatchObject({ code: 'github_release_unavailable' });
  expect(await readdir(f.cache)).toEqual([]);
});
it('downloads only an authenticated ready Galaxy release and still refuses a held one', async () => {
  const f = await fixture({ galaxy: true });
  expect((await downloadGithubRelease('https://github.com/CBOC-Vita-Lab/galaxy-vita', f)).manifest.titleId).toBe('SMGV00001');
  const held = await fixture({ galaxy: true, manifestChanges: { releaseState: 'held' } });
  await expect(downloadGithubRelease('https://github.com/CBOC-Vita-Lab/galaxy-vita', held)).rejects.toMatchObject({ code: 'release_held' });
});
it('blocks a different release tag instead of choosing a substitute', async () => {
  const f = await fixture();
  await expect(downloadGithubRelease(`${repo}/releases/tag/v2.0`, f)).rejects.toMatchObject({ code: 'github_release_unavailable' });
  expect(f.calls).toEqual(['https://api.github.com/repos/CBOC-Vita-Lab/strikers-charged-vita/releases/tags/v2.0']);
});
