import { constants } from 'node:fs';
import { lstat, open, readdir, realpath, statfs } from 'node:fs/promises';
import { join } from 'node:path';
import type { DeviceCandidate } from '../shared/contracts';
import { cancelled, requireThat } from '../shared/errors';
import { systemVolumes } from './system';
import { devicePath, finalPath, ownedPath, replacementPath, type RemoteEntry, type Transport, type TransportOptions } from './transport';
import { volumeOperation } from './volume-worker';
import { createHash } from 'node:crypto';

export async function openVolume(device: DeviceCandidate, options: TransportOptions): Promise<Transport> {
  requireThat(device.root, 'device_changed');
  const root = device.root, original = await lstat(root, { bigint: true }).catch(() => null);
  requireThat(original?.isDirectory() && !original.isSymbolicLink() && await realpath(root) === root, 'device_changed');
  let closed = false;
  async function guard() {
    requireThat(!closed, 'transport_closed');
    const now = await lstat(root, { bigint: true }).catch(() => null);
    requireThat(now?.isDirectory() && !now.isSymbolicLink() && now.dev === original!.dev && now.ino === original!.ino, 'device_changed');
    const volume = (await (options.volumes ?? systemVolumes)()).find(v => `volume:${v.id}` === device.identity && v.root === root);
    requireThat(volume?.readable && volume.vitaLayout, 'device_changed');
  }
  await guard();
  async function pathFor(path: string): Promise<string> {
    const parsed = devicePath(path); requireThat(parsed.mount === 'ux0', 'mount_unavailable');
    await guard();
    const parts = parsed.relative.split('/'); let current = root;
    for (let i = 0; i < parts.length; i++) {
      current = join(current, parts[i]);
      const entry = await lstat(current).catch((e: NodeJS.ErrnoException) => { if (e.code === 'ENOENT') return null; throw e; });
      requireThat(!entry?.isSymbolicLink() && (!entry || (i === parts.length - 1 ? entry.isFile() || entry.isDirectory() : entry.isDirectory())), 'unsafe_destination');
      if (!entry) break;
    }
    return join(root, ...parts);
  }
  async function stat(path: string): Promise<RemoteEntry | null> {
    const local = await pathFor(path), s = await lstat(local).catch((e: NodeJS.ErrnoException) => { if (e.code === 'ENOENT') return null; throw e; });
    return s ? { name: path.split('/').at(-1)!, kind: s.isDirectory() ? 'directory' : 'file', bytes: s.size } : null;
  }
  async function* read(path: string, signal?: AbortSignal) {
    cancelled(signal); const local = await pathFor(path), handle = await open(local, constants.O_RDONLY | constants.O_NOFOLLOW);
    try {
      requireThat((await handle.stat()).isFile(), 'not_a_file'); await guard();
      const buffer = Buffer.alloc(1024 * 1024);
      while (true) { cancelled(signal); await guard(); const { bytesRead } = await handle.read(buffer); if (!bytesRead) break; yield Buffer.from(buffer.subarray(0, bytesRead)); }
    } finally { await handle.close(); }
  }
  async function write(path: string, data: AsyncIterable<Uint8Array>, signal?: AbortSignal) {
    cancelled(signal); await pathFor(path); await guard();
    async function* checked() { for await (const chunk of data) { cancelled(signal); await guard(); yield chunk; } }
    await volumeOperation(options.volumeWorker, ['write', root, String(original!.dev), String(original!.ino), devicePath(path).relative], checked(), signal);
    await guard();
  }
  async function move(source: string, destination: string, expectedHash?: string, signal?: AbortSignal) {
    await pathFor(source); await pathFor(destination); requireThat(await stat(destination) === null, 'destination_conflict');
    const hash = createHash('sha256'); for await (const chunk of read(source, signal)) hash.update(chunk);
    const sha256 = hash.digest('hex'); requireThat(!expectedHash || expectedHash === sha256, 'target_changed');
    await guard(); await volumeOperation(options.volumeWorker, ['move', root, String(original!.dev), String(original!.ino),
      devicePath(source).relative, devicePath(destination).relative, sha256], undefined, signal);
  }
  return { device, identity: async () => { await guard(); return device.identity; }, stat, read,
    list: async path => {
      const local = await pathFor(path), result: RemoteEntry[] = [];
      for (const name of await readdir(local)) { const entry = await stat(`${path}/${name}`); if (entry) result.push(entry); }
      return result;
    },
    writeOwnedPartial: async (path, data, signal) => { ownedPath(path, options.scope); await write(path, data, signal); },
    renameOwned: async (source, destination, signal) => {
      ownedPath(source, options.scope); finalPath(destination, options.scope);
      await move(source, destination, undefined, signal);
    },
    moveAside: async (source, backup, hash, signal) => {
      replacementPath(source, hash, options.scope); ownedPath(backup, options.scope); await move(source, backup, hash, signal);
    },
    removeOwned: remove,
    freeSpace: async () => { await guard(); const s = await statfs(root); return s.bavail * s.bsize; },
    close: async () => { closed = true; }
  };
  async function remove(path: string) {
    ownedPath(path, options.scope); await pathFor(path); await guard();
    await volumeOperation(options.volumeWorker, ['remove', root, String(original!.dev), String(original!.ino), devicePath(path).relative]);
  }
}
