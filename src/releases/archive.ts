import yauzl, { type Entry } from 'yauzl';
import { type Readable } from 'node:stream';
import { lstat } from 'node:fs/promises';
import { InstallerError, requireThat, cancelled } from '../shared/errors';
import { safeRelative } from '../shared/files';

export interface ArchiveLimits { maxFiles: number; maxFileBytes: number; maxTotalBytes: number }
export const PACKAGE_LIMITS = { maxFiles: 10000, maxFileBytes: 512 * 1024 ** 2, maxTotalBytes: 512 * 1024 ** 2 };
export async function walkZip(path: string, limits: ArchiveLimits,
  visit: (entry: Entry, stream: Readable) => Promise<void>, signal?: AbortSignal): Promise<{ entries: number; bytes: number }> {
  cancelled(signal);
  const stat = await lstat(path); requireThat(stat.isFile() && !stat.isSymbolicLink(), 'unsafe_archive');
  const zip = await new Promise<yauzl.ZipFile>((resolve, reject) => yauzl.open(path,
    { lazyEntries: true, validateEntrySizes: true, strictFileNames: true }, (err, file) => err ? reject(err) : resolve(file!)));
  let count = 0, total = 0;
  const seen = new Map<string, boolean>();
  const parents = new Set<string>();
  try {
    requireThat(zip.entryCount <= limits.maxFiles, 'archive_entry_limit');
    await new Promise<void>((resolve, reject) => {
      const stop = () => { zip.close(); reject(new InstallerError('cancelled')); };
      signal?.addEventListener('abort', stop, { once: true });
      const finish = (error?: unknown) => {
        signal?.removeEventListener('abort', stop); if (error) { zip.close(); reject(error); } else resolve();
      };
      zip.on('error', finish); zip.on('end', () => finish());
      zip.on('entry', async (entry: Entry) => {
        try {
          cancelled(signal);
          const directory = entry.fileName.endsWith('/'), name = directory ? entry.fileName.slice(0, -1) : entry.fileName;
          const key = name.normalize('NFKC').toLowerCase(), mode = (entry.externalFileAttributes >>> 16) & 0xf000;
          requireThat(safeRelative(name), 'unsafe_archive_path');
          requireThat(!seen.has(key), 'archive_path_collision');
          requireThat(!(entry.generalPurposeBitFlag & 1) && (mode === 0 || mode === 0x8000 || (directory && mode === 0x4000)), 'unsupported_archive_member');
          requireThat(directory || !parents.has(key), 'archive_path_collision');
          const parts = key.split('/');
          for (let i = 1; i < parts.length; i++) {
            const parent = parts.slice(0, i).join('/');
            requireThat(seen.get(parent) !== false, 'archive_path_collision'); parents.add(parent);
          }
          seen.set(key, directory);
          requireThat(++count <= limits.maxFiles, 'archive_entry_limit');
          total += entry.uncompressedSize;
          requireThat(entry.uncompressedSize <= limits.maxFileBytes && total <= limits.maxTotalBytes, 'archive_size_limit');
          if (!directory) {
            const stream = await new Promise<Readable>((res, rej) => zip.openReadStream(entry, (err, data) => err ? rej(err) : res(data!)));
            const abort = () => stream.destroy(new InstallerError('cancelled'));
            signal?.addEventListener('abort', abort, { once: true });
            try { await visit(entry, stream); } finally { signal?.removeEventListener('abort', abort); stream.destroy(); }
          }
          cancelled(signal); zip.readEntry();
        } catch (error) { finish(error); }
      });
      if (signal?.aborted) stop(); else zip.readEntry();
    });
    return { entries: count, bytes: total };
  } finally { zip.close(); }
}
