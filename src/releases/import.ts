import { createWriteStream } from 'node:fs';
import { lstat, realpath, mkdtemp, mkdir, readdir, readFile, rm } from 'node:fs/promises';
import { dirname, join, resolve, sep } from 'node:path';
import { tmpdir } from 'node:os';
import { pipeline } from 'node:stream/promises';
import { Transform } from 'node:stream';
import { walkZip, type ArchiveLimits } from './archive';
import { authenticateManifest, type TrustStore } from './trust';
import { verifyVpk } from './manifest';
import { digest, safeRelative } from '../shared/files';
import { cancelled, requireThat } from '../shared/errors';
import type { ImportedRelease } from '../shared/contracts';

const IMPORT_LIMITS = { maxFiles: 10000, maxFileBytes: 512 * 1024 ** 2, maxTotalBytes: 2 * 1024 ** 3 };
interface Budget { entries: number; bytes: number }
export async function extractArchive(path: string, parent: string, limits = IMPORT_LIMITS,
  signal?: AbortSignal, budget: Budget = { entries: 0, bytes: 0 }): Promise<string> {
  cancelled(signal);
  requireThat(!(await lstat(parent)).isSymbolicLink(), 'unsafe_workspace');
  const output = await mkdtemp(join(await realpath(parent), 'vita-import-'));
  try {
    const usage = await walkZip(path, { ...limits, maxFiles: limits.maxFiles - budget.entries,
      maxTotalBytes: limits.maxTotalBytes - budget.bytes }, async (entry, source) => {
      const target = resolve(output, entry.fileName);
      requireThat(target.startsWith(output + sep), 'unsafe_archive_path');
      await mkdir(dirname(target), { recursive: true });
      let bytes = 0;
      const meter = new Transform({ transform(chunk, _encoding, done) {
        bytes += chunk.length;
        if (bytes > entry.uncompressedSize) done(new Error('archive_size_mismatch')); else done(null, chunk);
      }});
      await pipeline(source, meter, createWriteStream(target, { flags: 'wx', mode: 0o600 }), { signal });
      requireThat(bytes === entry.uncompressedSize, 'archive_size_mismatch');
    }, signal);
    budget.entries += usage.entries; budget.bytes += usage.bytes;
    return output;
  } catch (error) { await rm(output, { recursive: true, force: true }); throw error; }
}

export async function checkedChild(root: string, relative: string): Promise<string> {
  requireThat(safeRelative(relative), 'unsafe_path');
  let current = root;
  const parts = relative.split('/');
  for (let i = 0; i < parts.length; i++) {
    current = join(current, parts[i]); const stat = await lstat(current);
    requireThat(!stat.isSymbolicLink() && (i === parts.length - 1 ? stat.isFile() : stat.isDirectory()), 'unsafe_file');
  }
  const canonical = await realpath(current);
  requireThat(canonical.startsWith(root + sep), 'unsafe_path'); return canonical;
}

export async function importRelease(selection: string, store: TrustStore, signal?: AbortSignal): Promise<ImportedRelease[]> {
  cancelled(signal);
  const stat = await lstat(selection); requireThat(!stat.isSymbolicLink(), 'unsafe_source');
  const source = await realpath(selection), results: ImportedRelease[] = [], owned: string[] = [];
  const archiveBudget: Budget = { entries: 0, bytes: 0 }; let folderEntries = 0;
  async function archive(path: string, level: number) {
    requireThat(level <= 1, 'archive_depth_limit');
    const workspace = await extractArchive(path, await realpath(tmpdir()), IMPORT_LIMITS, signal, archiveBudget);
    owned.push(workspace); await search(workspace, 0, level, workspace);
  }
  async function search(root: string, depth: number, archiveLevel: number, workspace?: string): Promise<void> {
    cancelled(signal); requireThat(depth <= 4, 'folder_depth_limit');
    const entries = await readdir(root, { withFileTypes: true });
    const names = new Set<string>();
    for (const entry of entries) {
      cancelled(signal); requireThat(++folderEntries <= 10000, 'folder_entry_limit');
      const key = entry.name.normalize('NFKC').toLowerCase();
      requireThat(!names.has(key) && safeRelative(entry.name), 'folder_path_collision'); names.add(key);
      requireThat(!entry.isSymbolicLink() && (entry.isDirectory() || entry.isFile()), 'unsafe_source');
    }
    if (entries.some(e => e.name === 'cboc-release.json' && e.isFile())) {
      const path = await checkedChild(root, 'cboc-release.json');
      requireThat((await lstat(path)).size <= 1024 * 1024, 'invalid_manifest_size');
      const raw = await readFile(path), manifest = authenticateManifest(raw, store);
      const vpkPath = manifest.package ? await checkedChild(root, manifest.package.path) : null;
      const inventory = vpkPath ? await verifyVpk(vpkPath, manifest, signal) : null;
      results.push({ manifest, manifestSha256: digest(raw), sourceRoot: root, vpkPath, inventory, ownedWorkspace: workspace });
      return;
    }
    for (const entry of entries) {
      if (entry.isDirectory() && depth < 4) await search(join(root, entry.name), depth + 1, archiveLevel, workspace);
      else if (entry.isFile() && entry.name.toLowerCase().endsWith('.zip')) await archive(join(root, entry.name), archiveLevel + 1);
    }
  }
  try {
    if (stat.isDirectory()) await search(source, 0, -1);
    else { requireThat(stat.isFile(), 'unsafe_source'); await archive(source, 0); }
    requireThat(results.length, 'no_supported_release');
    const used = new Set(results.map(r => r.ownedWorkspace));
    for (const dir of owned) if (!used.has(dir)) await rm(dir, { recursive: true, force: true });
    return results;
  } catch (error) {
    for (const dir of owned) await rm(dir, { recursive: true, force: true });
    throw error;
  }
}
