import type { ImportedRelease } from '../shared/contracts';
import type { TrustStore } from './trust';
import { strictJson } from './trust';
import { mkdir, mkdtemp, lstat, realpath, readFile, writeFile, copyFile, rm } from 'node:fs/promises';
import { constants } from 'node:fs';
import { dirname, join } from 'node:path';
import { z } from 'zod';
import { PROFILES } from './manifest';
import { importRelease } from './import';
import { cancelled, InstallerError, requireThat } from '../shared/errors';
import { downloadVerifiedFile, type DownloadProgress } from '../shared/download';
import { digest } from '../shared/files';
import bundles from '../../resources/profiles/bundles.json';
export type { DownloadProgress };
export interface GithubDownloadOptions {
  trust: TrustStore; cache: string; signal?: AbortSignal; fetcher?: typeof fetch;
  expectedManifestSha256?: string; onProgress?: (progress: DownloadProgress) => void;
}
const tagPattern = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,99}$/;
const assetPattern = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,159}\.zip$/;
export function parseGithubReleaseUrl(value: string): { recipe: keyof typeof PROFILES; tag?: string; asset?: string } {
  requireThat(value.length <= 1000 && !/[\\%\x00-\x20\x7f]/.test(value), 'invalid_github_url');
  let url: URL; try { url = new URL(value); } catch { throw new InstallerError('invalid_github_url'); }
  requireThat(url.origin === 'https://github.com' && !url.username && !url.password && !url.search && !url.hash, 'invalid_github_url');
  const parts = url.pathname.replace(/\/$/, '').split('/').slice(1);
  const recipe = (Object.keys(PROFILES) as (keyof typeof PROFILES)[]).find(key => parts[0]?.toLowerCase() === 'cboc-vita-lab'
    && parts[1]?.toLowerCase().replace(/\.git$/, '') === PROFILES[key].repo);
  requireThat(recipe, 'unsupported_github_repo');
  if (parts.length === 2 || parts.length === 3 && parts[2] === 'releases'
      || parts.length === 4 && parts[2] === 'releases' && parts[3] === 'latest') return { recipe };
  requireThat(parts[2] === 'releases' && tagPattern.test(parts[4] ?? ''), 'invalid_github_url');
  if (parts.length === 5 && parts[3] === 'tag') return { recipe, tag: parts[4] };
  requireThat(parts.length === 6 && parts[3] === 'download' && assetPattern.test(parts[5]), 'invalid_github_url');
  return { recipe, tag: parts[4], asset: parts[5] };
}
const metadataSchema = z.object({ tag_name: z.string().regex(tagPattern), draft: z.literal(false), prerelease: z.boolean(), html_url: z.string(),
  assets: z.array(z.object({ name: z.string(), browser_download_url: z.string(), size: z.number().int().positive(), state: z.string(), digest: z.string().nullable().optional() })).max(100) });
async function metadata<T>(url: string, schema: z.ZodType<T>, fetcher: typeof fetch, signal?: AbortSignal): Promise<T> {
  const timeout = AbortSignal.timeout(15_000), combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
  try {
    const response = await fetcher(url, { redirect: 'manual', credentials: 'omit', signal: combined, headers: { Accept: 'application/vnd.github+json' } });
    requireThat(response.status === 200 && response.body, 'github_release_unavailable');
    const reader = response.body.getReader(), chunks: Uint8Array[] = []; let bytes = 0;
    try { for (;;) { const next = await reader.read(); if (next.done) break;
      bytes += next.value.length; requireThat(bytes <= 1024 * 1024, 'github_release_unavailable'); chunks.push(next.value); }
    } finally { await reader.cancel(); }
    return schema.parse(strictJson(Buffer.concat(chunks)));
  } catch { cancelled(signal); throw new InstallerError('github_release_unavailable'); }
}
export async function downloadGithubRelease(value: string, options: GithubDownloadOptions): Promise<ImportedRelease> {
  cancelled(options.signal);
  const target = parseGithubReleaseUrl(value), { recipe } = target, fetcher = options.fetcher ?? fetch;
  const repo = `https://github.com/CBOC-Vita-Lab/${PROFILES[recipe].repo}`;
  const api = `https://api.github.com/repos/CBOC-Vita-Lab/${PROFILES[recipe].repo}/releases`;
  let exact: z.infer<typeof metadataSchema> | undefined;
  if (options.expectedManifestSha256 && !target.tag) {
    const records = await metadata(`${api}?per_page=30`, z.array(z.unknown()).max(30), fetcher, options.signal);
    const history = records.flatMap(record => { const parsed = metadataSchema.safeParse(record); return parsed.success ? [parsed.data] : []; });
    // The small manifest asset locates old releases without speculative ZIP downloads.
    exact = history.find(release => release.html_url === `${repo}/releases/tag/${release.tag_name}` && release.assets.some(asset =>
      asset.name === 'cboc-release.json' && asset.state === 'uploaded' && asset.size <= 1024 * 1024
      && asset.browser_download_url === `${repo}/releases/download/${release.tag_name}/cboc-release.json`
      && asset.digest === `sha256:${options.expectedManifestSha256}`));
  }
  const data = exact ?? await metadata(`${api}/${target.tag ? `tags/${target.tag}` : 'latest'}`, metadataSchema, fetcher, options.signal);
  requireThat(data.html_url === `${repo}/releases/tag/${data.tag_name}` && (target.tag ? data.tag_name === target.tag : exact || !data.prerelease), 'github_release_unavailable');
  const allowed = ['cboc-release.zip', bundles[recipe].output];
  if (target.asset) requireThat(allowed.includes(target.asset), 'unsupported_release_asset');
  const assets = data.assets.filter(a => allowed.includes(a.name) && (!target.asset || target.asset === a.name));
  requireThat(assets.length > 0, 'release_download_unavailable'); requireThat(assets.length === 1, 'ambiguous_release_download');
  const asset = assets[0], url = `${repo}/releases/download/${data.tag_name}/${asset.name}`;
  requireThat(asset.browser_download_url === url && asset.state === 'uploaded' && asset.size <= 512 * 1024 ** 2
    && /^sha256:[a-f0-9]{64}$/.test(asset.digest ?? ''), 'release_download_unavailable');
  await mkdir(options.cache, { recursive: true, mode: 0o700 }); requireThat(!(await lstat(options.cache)).isSymbolicLink(), 'unsafe_workspace');
  const workspace = await mkdtemp(join(await realpath(options.cache), 'release-'));
  let imported: ImportedRelease[] = [], succeeded = false;
  try {
    const archive = join(workspace, 'download.zip');
    await downloadVerifiedFile({ url, sha256: asset.digest!.slice(7), bytes: asset.size, maxBytes: 512 * 1024 ** 2 }, archive, options.signal, fetcher, options.onProgress);
    imported = await importRelease(archive, options.trust, options.signal);
    requireThat(imported.length === 1, 'ambiguous_release_download');
    const release = imported[0];
    requireThat(release.manifest.recipeId === recipe && release.manifest.releaseState === 'ready' && release.vpkPath, 'release_held');
    requireThat(!options.expectedManifestSha256 || release.manifestSha256 === options.expectedManifestSha256, 'repair_version_unavailable');
    // Only authenticated installer payload enters automatic input discovery.
    const root = join(workspace, 'payload'), packagePath = join(root, release.manifest.package!.path);
    await mkdir(dirname(packagePath), { recursive: true });
    const raw = await readFile(join(release.sourceRoot, 'cboc-release.json'));
    requireThat(digest(raw) === release.manifestSha256, 'source_changed');
    await writeFile(join(root, 'cboc-release.json'), raw, { flag: 'wx', mode: 0o600 });
    await copyFile(release.vpkPath, packagePath, constants.COPYFILE_EXCL);
    const [checked] = await importRelease(root, options.trust, options.signal);
    await rm(archive); cancelled(options.signal); succeeded = true;
    return { ...checked, ownedWorkspace: workspace, inputRoots: [], downloadUrl: url };
  } finally {
    for (const root of new Set(imported.map(r => r.ownedWorkspace).filter(Boolean))) await rm(root!, { recursive: true, force: true });
    if (!succeeded) await rm(workspace, { recursive: true, force: true });
  }
}
