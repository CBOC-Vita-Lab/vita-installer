import { createHash } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { lstat } from 'node:fs/promises';
import { requireThat, cancelled } from './errors';
export const digest = (bytes: Uint8Array | string) => createHash('sha256').update(bytes).digest('hex');
export function safeRelative(value: string): boolean {
  return value.length > 0 && value.length <= 1024 && !/[\\:\x00-\x1f\x7f]/.test(value)
    && value.split('/').every(p => p !== '' && p !== '.' && p !== '..'
      && !/[. ]$/.test(p) && !/^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(p));
}
export async function hashFile(path: string, signal?: AbortSignal): Promise<string> {
  cancelled(signal);
  const before = await lstat(path); requireThat(before.isFile() && !before.isSymbolicLink(), 'unsafe_file');
  const hash = createHash('sha256');
  const stream = createReadStream(path, { signal });
  try { for await (const chunk of stream) { cancelled(signal); hash.update(chunk); } }
  catch (error) { cancelled(signal); throw error; }
  cancelled(signal);
  const after = await lstat(path);
  requireThat(before.ino === after.ino && before.dev === after.dev && before.size === after.size
    && before.mtimeMs === after.mtimeMs && !after.isSymbolicLink(), 'source_changed');
  return hash.digest('hex');
}
