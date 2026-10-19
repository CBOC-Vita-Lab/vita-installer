import { lstat } from 'node:fs/promises';
import { hashFile } from './files';
import { cancelled, InstallerError } from './errors';
export class FileIntegrityError extends InstallerError {
  constructor(public readonly path: string, code: 'source_missing' | 'source_changed' | 'source_unreadable') { super(code); }
}
export async function verifyFile(path: string, expected: { bytes?: number; sha256?: string }, signal?: AbortSignal) {
  try {
    cancelled(signal); const stat = await lstat(path);
    if (!stat.isFile() || stat.isSymbolicLink()) throw new FileIntegrityError(path, 'source_unreadable');
    if (expected.bytes !== undefined && stat.size !== expected.bytes || expected.sha256 && await hashFile(path, signal) !== expected.sha256)
      throw new FileIntegrityError(path, 'source_changed');
  } catch (error) {
    cancelled(signal); if (error instanceof FileIntegrityError) throw error;
    const code = (error as { code?: string }).code;
    throw new FileIntegrityError(path, code === 'ENOENT' || code === 'ENOTDIR' ? 'source_missing' : code === 'source_changed' ? 'source_changed' : 'source_unreadable');
  }
}
