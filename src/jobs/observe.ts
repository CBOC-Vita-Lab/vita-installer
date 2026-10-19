import { createHash } from 'node:crypto';
import type { Transport } from '../devices/transport';
import { devicePath } from '../devices/transport';
import { cancelled, requireThat } from '../shared/errors';
import type { ObservedFile } from './types';

export async function readSmallFile(transport: Transport, path: string, limit: number, signal?: AbortSignal): Promise<Buffer> {
  const entry = await transport.stat(path); requireThat(entry?.kind === 'file' && entry.bytes <= limit, 'invalid_target_file');
  const chunks: Uint8Array[] = []; let bytes = 0;
  for await (const chunk of transport.read(path, signal)) { cancelled(signal); bytes += chunk.length; requireThat(bytes <= limit, 'invalid_target_file'); chunks.push(chunk); }
  requireThat(bytes === entry.bytes, 'target_changed'); return Buffer.concat(chunks);
}

export async function observeFile(transport: Transport, path: string, signal?: AbortSignal,
  onProgress?: (completed: number) => void): Promise<ObservedFile> {
  devicePath(path); cancelled(signal); const before = await transport.stat(path);
  requireThat(before?.kind === 'file' && before.bytes >= 0 && before.bytes <= 128 * 1024 ** 3, 'invalid_target_file');
  const hash = createHash('sha256'); let bytes = 0;
  for await (const chunk of transport.read(path, signal)) {
    cancelled(signal); bytes += chunk.length; requireThat(bytes <= before.bytes, 'target_changed'); hash.update(chunk); onProgress?.(bytes);
  }
  const after = await transport.stat(path);
  requireThat(bytes === before.bytes && after?.kind === 'file' && after.bytes === before.bytes, 'target_changed');
  return { path, bytes, sha256: hash.digest('hex') };
}
export async function observeTrees(transport: Transport, roots: string[], signal?: AbortSignal): Promise<ObservedFile[]> {
  const files: ObservedFile[] = [], visited = new Set<string>(); let entries = 0, bytes = 0;
  async function walk(path: string, depth: number) {
    cancelled(signal); devicePath(path); if (visited.has(path)) return; visited.add(path);
    requireThat(++entries <= 100000 && depth <= 32, 'target_inventory_limit');
    const entry = await transport.stat(path); if (!entry) return;
    if (entry.kind === 'file') {
      bytes += entry.bytes; requireThat(bytes <= 128 * 1024 ** 3, 'target_inventory_limit');
      files.push(await observeFile(transport, path, signal)); return;
    }
    for (const child of await transport.list(path)) await walk(`${path}/${child.name}`, depth + 1);
  }
  for (const root of roots) await walk(root, 0);
  return files.sort((a, b) => a.path.localeCompare(b.path));
}
