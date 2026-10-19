import { Client } from 'basic-ftp';
import { PassThrough, Readable } from 'node:stream';
import type { DeviceCandidate } from '../shared/contracts';
import { cancelled, requireThat } from '../shared/errors';
import { devicePath, finalPath, ownedPath, replacementPath, stageRoot, type RemoteEntry, type Transport, type TransportOptions } from './transport';
import { createHash } from 'node:crypto';

export async function openFtp(device: DeviceCandidate, options: TransportOptions): Promise<Transport> {
  const client = new Client(options.timeoutMs ?? 15000); let closed = false, supportsSize = true;
  try { await client.access({ host: device.host!, port: device.port!, user: 'anonymous', password: 'anonymous@', secure: false }); }
  catch (error) { client.close(); throw error; }
  const remote = (path: string) => { const p = devicePath(path); requireThat(p.mount === 'ux0' || device.capabilities.ur0, 'mount_unavailable'); return `/${p.mount}:/${p.relative}`; };
  const guard = () => requireThat(!closed && !client.closed, 'transport_closed');
  async function list(path: string): Promise<RemoteEntry[]> {
    guard(); await client.cd(remote(path)); const entries = await client.list();
    return entries.map(e => {
      requireThat(e.isFile || e.isDirectory, 'unsafe_destination');
      devicePath(`${path}/${e.name}`);
      return { name: e.name, bytes: e.size, kind: e.isDirectory ? 'directory' as const : 'file' as const };
    });
  }
  async function stat(path: string): Promise<RemoteEntry | null> {
    const absolute = remote(path), slash = path.lastIndexOf('/');
    requireThat(slash >= 4, 'unsafe_device_path');
    guard();
    // Older Vita servers return SIZE for directories too; check CWD before treating a size as a file.
    try { await client.cd(absolute); return { name: path.slice(slash + 1), kind: 'directory', bytes: 0 }; }
    catch (error) { if ((error as { code?: number }).code !== 550) throw error; }
    if (supportsSize) {
      try {
        const bytes = await client.size(absolute);
        requireThat(Number.isSafeInteger(bytes) && bytes >= 0, 'unsafe_destination');
        return { name: path.slice(slash + 1), kind: 'file', bytes };
      } catch (error) {
        const code = (error as { code?: number }).code;
        if (code === 500 || code === 502 || code === 504) supportsSize = false;
        else if (code === 550) {
          if (device.adapter === 'companion' && (error as Error).message === "550 The file doesn't exist.") return null;
          // Other servers also use 550 for denied metadata access. A fresh listing must decide.
        } else throw error;
      }
    }
    try { return (await list(path.slice(0, slash))).find(e => e.name === path.slice(slash + 1)) ?? null; }
    catch (error) { if ((error as { code?: number }).code === 550) return null; throw error; }
  }
  async function parents(path: string) {
    const { mount, relative } = devicePath(path), parts = relative.split('/');
    guard();
    try { await client.cd(remote(path.slice(0, path.lastIndexOf('/')))); return; }
    catch (error) { if ((error as { code?: number }).code !== 550) throw error; }
    for (let i = 2; i < parts.length; i++) {
      const dir = `${mount}:${parts.slice(0, i).join('/')}`;
      const exists = await stat(dir); if (exists) { requireThat(exists.kind === 'directory', 'unsafe_destination'); continue; }
      guard(); await client.send(`MKD ${remote(dir)}`);
    }
  }
  async function* read(path: string, signal?: AbortSignal) {
    cancelled(signal); guard(); const stream = new PassThrough();
    const stop = () => { client.close(); stream.destroy(); }; signal?.addEventListener('abort', stop, { once: true });
    let error: unknown, finished = false;
    const result = client.downloadTo(stream, remote(path)).catch(e => { error = e; stream.destroy(e); });
    try { for await (const chunk of stream) { cancelled(signal); yield chunk as Buffer; } await result; cancelled(signal); if (error) throw error; finished = true; }
    finally { signal?.removeEventListener('abort', stop); if (!finished) { client.close(); stream.destroy(); } await result; }
  }
  return { device, identity: async () => { guard(); return device.identity; }, stat, list, read,
    writeOwnedPartial: async (path, data, signal) => {
      ownedPath(path, options.scope); cancelled(signal); guard(); requireThat(await stat(path) === null, 'destination_conflict'); await parents(path);
      const source = Readable.from(data), stop = () => { client.close(); source.destroy(); };
      signal?.addEventListener('abort', stop, { once: true });
      try { await client.uploadFrom(source, remote(path)); cancelled(signal); }
      catch (error) { cancelled(signal); throw error; }
      finally { signal?.removeEventListener('abort', stop); source.destroy(); }
    },
    renameOwned: async (source, destination, signal) => {
      ownedPath(source, options.scope); finalPath(destination, options.scope); cancelled(signal); guard();
      await parents(destination); requireThat(await stat(destination) === null, 'destination_conflict');
      await client.rename(remote(source), remote(destination)); cancelled(signal);
    },
    moveAside: async (source, backup, expectedHash, signal) => {
      replacementPath(source, expectedHash, options.scope); ownedPath(backup, options.scope); cancelled(signal); guard();
      const hash = createHash('sha256'); for await (const chunk of read(source, signal)) hash.update(chunk);
      requireThat(hash.digest('hex') === expectedHash, 'target_changed');
      await parents(backup); requireThat(await stat(backup) === null, 'destination_conflict');
      await client.rename(remote(source), remote(backup)); cancelled(signal);
    },
    removeOwned: async path => { ownedPath(path, options.scope); guard(); await client.remove(remote(path)); },
    freeSpace: async () => { guard(); return undefined; },
    ...(device.adapter === 'vitashell' && device.capabilities.install && options.scope ? { promoteVpk: async (): Promise<'accepted' | 'rejected' | 'unknown'> => {
      guard();
      requireThat((await client.features()).has('PROM'), 'promotion_not_supported');
      try {
        const response = client.send(`PROM ${stageRoot(options.scope!.jobId)}/install.vpk`);
        client.ftp.socket.setTimeout(Math.min(120000, Math.max(1, options.promotionTimeoutMs ?? 120000)));
        const reply = await response;
        return reply.code === 200 && reply.message.trim() === '200 OK PROMOTING' ? 'accepted' : 'unknown';
      } catch (error) {
        const code = (error as { code?: number }).code;
        return typeof code === 'number' && code >= 400 && code < 600 ? 'rejected' : 'unknown';
      }
    } } : {}),
    close: async () => { closed = true; client.close(); }
  };
}
