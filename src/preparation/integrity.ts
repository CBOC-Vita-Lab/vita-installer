import { join } from 'node:path';
import { lstat, realpath } from 'node:fs/promises';
import { checkedChild } from '../releases/import';
import { safeRelative } from '../shared/files';
import { cancelled, requireThat } from '../shared/errors';
import { FileIntegrityError, verifyFile } from '../shared/integrity';
export interface ExpectedFile { path: string; bytes: number; sha256: string }
export async function verifyInventory(root: string, files: ExpectedFile[], signal?: AbortSignal, hashes = true) {
  requireThat(!(await lstat(root)).isSymbolicLink(), 'unsafe_source');
  const canonical = await realpath(root);
  for (const file of files) {
    cancelled(signal); requireThat(safeRelative(file.path), 'unsafe_path');
    const path = join(root, file.path);
    try { await checkedChild(canonical, file.path); }
    catch (error) {
      const code = (error as { code?: string }).code;
      throw new FileIntegrityError(path, code === 'ENOENT' || code === 'ENOTDIR' ? 'source_missing' : 'source_unreadable');
    }
    await verifyFile(path, { bytes: file.bytes, sha256: hashes ? file.sha256 : undefined }, signal);
  }
}
