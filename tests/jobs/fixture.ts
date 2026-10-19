import { mkdtemp, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import type { DeviceCandidate, ImportedRelease, PreparationResult, PreparedPackage } from '../../src/shared/contracts';
import type { RemoteEntry, Transport } from '../../src/devices/transport';
import { manifestSchema, PROFILES, verifyVpk } from '../../src/releases/manifest';
import { digest, manifest, runtime, sfo, zip } from '../fixtures/releases';
export const device: DeviceCandidate = { id: 'ftp:fixture', identity: 'unbound:fixture', transport: 'ftp', label: 'Vita',
  storageRole: 'ux0', adapter: 'ftp', state: 'ready', evidence: [], capabilities: { transfer: true, install: false, ur0: true } };
export class MemoryTransport implements Transport {
  device = device; files = new Map<string, Buffer>(); writes: string[] = []; free: number | undefined = 1e10; online = true;
  failAfterWrites?: number;
  async identity() { if (!this.online) throw new Error('disconnected'); return this.device.identity; }
  async stat(path: string): Promise<RemoteEntry | null> {
    await this.identity(); const content = this.files.get(path), directory = [...this.files.keys()].some(p => p.startsWith(`${path}/`));
    return content ? { name: path.split('/').at(-1)!, kind: 'file', bytes: content.length }
      : directory ? { name: path.split('/').at(-1)!, kind: 'directory', bytes: 0 } : null;
  }
  async list(path: string) {
    const names = new Set([...this.files.keys()].filter(p => p.startsWith(`${path}/`)).map(p => p.slice(path.length + 1).split('/')[0]));
    const result: RemoteEntry[] = []; for (const name of names) result.push((await this.stat(`${path}/${name}`))!); return result;
  }
  async *read(path: string) { await this.identity(); if (!this.files.has(path)) throw new Error('missing'); yield this.files.get(path)!; }
  async writeOwnedPartial(path: string, data: AsyncIterable<Uint8Array>) {
    await this.identity(); if (this.files.has(path)) throw new Error('exists'); this.writes.push(path); const chunks: Uint8Array[] = [];
    for await (const chunk of data) { chunks.push(chunk); this.files.set(path, Buffer.concat(chunks)); }
    if (this.failAfterWrites === this.writes.length) { this.online = false; throw new Error('disconnected'); }
  }
  async renameOwned(source: string, destination: string) {
    await this.identity(); if (this.files.has(destination)) throw new Error('exists'); this.writes.push(destination);
    this.files.set(destination, this.files.get(source)!); this.files.delete(source);
  }
  async removeOwned(path: string) { await this.identity(); this.writes.push(`remove:${path}`); this.files.delete(path); }
  async moveAside(source: string, backup: string, hash: string) {
    if (digest(this.files.get(source)!) !== hash) throw new Error('target changed'); await this.renameOwned(source, backup);
  }
  async freeSpace() { return this.free; }
  async close() {}
}
export async function jobFixture(recipe: 'strikers' | 'infected' | 'vitadhoc' = 'strikers') {
  const root = await mkdtemp(join(tmpdir(), 'job-test-'));
  const profile = PROFILES[recipe];
  const bytes = await zip({ 'eboot.bin': runtime, 'sce_sys/param.sfo': sfo(profile.title) });
  const vpkPath = join(root, 'app.vpk'), source = join(root, 'data.bin'); await writeFile(vpkPath, bytes); await writeFile(source, 'game');
  const m = manifestSchema.parse({ ...manifest(digest(bytes)), recipeId: recipe, titleId: profile.title,
    destinations: { data: profile.data }, protectedPaths: recipe === 'strikers' ? manifest().protectedPaths : [], prerequisites: [] });
  const release: ImportedRelease = { manifest: m, manifestSha256: digest(JSON.stringify(m)), sourceRoot: root, vpkPath, inventory: await verifyVpk(vpkPath, m) };
  const artwork: PreparedPackage = { path: vpkPath, sha256: digest(bytes), originalSha256: digest(bytes), runtimeSha256: digest(runtime), inventory: release.inventory!, images: {} };
  const prepared: PreparationResult = { releaseId: m.releaseId, workspace: root, recipeFingerprint: digest(`preparation-v1\n${release.manifestSha256}`), inputFingerprints: { disc: digest('game') },
    files: [{ source, destination: `${m.destinations.data}/files/Art/data.bin`, bytes: 4, sha256: digest('game') }] };
  const transport = new MemoryTransport(); return { root, release, prepared, artwork, transport, device };
}
