import { lstat, readdir, realpath } from 'node:fs/promises';
import { join } from 'node:path';
import { cancelled, requireThat } from '../shared/errors';
import { hashFile, safeRelative } from '../shared/files';
import type { PreparedFile } from '../shared/contracts';
import type { ArchiveLimits } from '../releases/archive';

export async function inventoryTree(root: string, destination: string, limits: ArchiveLimits,
  signal?: AbortSignal, include: (relative: string) => boolean = () => true): Promise<PreparedFile[]> {
  const files: PreparedFile[] = [], names = new Set<string>(); let bytes = 0, entries = 0;
  requireThat(!(await lstat(root)).isSymbolicLink(), 'unsafe_source');
  const canonical = await realpath(root);
  async function walk(folder: string, prefix: string, depth: number) {
    cancelled(signal); requireThat(depth <= 32, 'folder_depth_limit');
    for (const entry of (await readdir(folder, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
      cancelled(signal); requireThat(++entries <= limits.maxFiles, 'input_entry_limit');
      const relative = prefix + entry.name;
      requireThat(safeRelative(relative), 'unsafe_input_path');
      const key = relative.normalize('NFKC').toLowerCase(); requireThat(!names.has(key), 'input_path_collision'); names.add(key);
      const path = join(folder, entry.name), stat = await lstat(path);
      requireThat(!stat.isSymbolicLink() && (stat.isDirectory() || stat.isFile()), 'unsafe_source');
      if (stat.isDirectory()) await walk(path, `${relative}/`, depth + 1);
      else if (include(relative)) {
        bytes += stat.size; requireThat(stat.size <= limits.maxFileBytes && bytes <= limits.maxTotalBytes, 'input_size_limit');
        files.push({ source: path, destination: `${destination}/${relative}`, bytes: stat.size, sha256: await hashFile(path, signal) });
      }
    }
  }
  await walk(canonical, '', 0); return files;
}
