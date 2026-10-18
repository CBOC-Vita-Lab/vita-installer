import { afterEach, expect, it, vi } from 'vitest';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import sharp from 'sharp';
import { InstallerController } from '../../src/main/controller';
import { digest, manifest, runtime, sfo, zip } from '../fixtures/releases';
import { MemoryTransport, device } from '../jobs/fixture';
import { FileJournal } from '../../src/jobs/journal';
import * as nativeDevice from '../../src/devices/native';
const roots: string[] = [], controllers: InstallerController[] = [];
afterEach(async () => { vi.restoreAllMocks(); for (const c of controllers.splice(0)) await c.dispose(); for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'controller-test-')); roots.push(root);
  const icon = await sharp({ create: { width: 128, height: 128, channels: 4, background: '#67e8df' } }).png().toBuffer();
  const bg = await sharp({ create: { width: 840, height: 500, channels: 4, background: '#b0ee52' } }).png().toBuffer();
  const vpk = await zip({ 'eboot.bin': runtime, 'sce_sys/param.sfo': sfo('GBAD00001'), 'sce_sys/icon0.png': icon, 'sce_sys/livearea/contents/bg.png': bg,
    'sce_sys/livearea/contents/template.xml': '<livearea><livearea-background><image>bg.png</image></livearea-background></livearea>' });
  const m = { ...manifest(digest(vpk)), titleId: 'GBAD00001', recipeId: 'vitadhoc', displayName: 'VitAdhoc', requiredInputs: [], optionalInputs: [{ id: 'roms', label: 'Your games', kind: 'roms' }],
    destinations: { data: 'ux0:data/GBAdhoc/roms' }, protectedPaths: [], prerequisites: [] };
  const raw = Buffer.from(JSON.stringify(m)); await writeFile(join(root, 'cboc-release.json'), raw); await writeFile(join(root, 'Strikers.vpk'), vpk);
  const transport = new MemoryTransport();
  const c = new InstallerController({ userData: join(root, 'private'), trust: { fingerprints: new Set([digest(raw)]), keys: new Map() },
    discover: async function* () { yield transport.device; }, inspect: async () => transport.device, open: async () => transport, tools: async () => ({}) }, () => {});
  controllers.push(c); return { root, c, vpk, transport };
}
it('imports one authenticated bundle, selects it and scans only its local required files', async () => {
  const f = await fixture(); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]);
  const state = f.c.snapshot(); expect(state.releases).toHaveLength(1); expect(state.selectedRelease).toBe(state.releases[0].id);
  expect(state.artwork.map(a => a.slot)).toEqual(['bubble', 'livearea', 'retroflow']);
  expect(JSON.stringify(state)).not.toContain(f.root); expect(state.prepared).toBe(false);
});
it('connects automatically to one ready FTP candidate without writing to it', async () => {
  const f = await fixture(); await f.c.command({ command: 'discover' });
  expect(f.c.snapshot().selectedDevice).toBe(device.id); expect(f.transport.writes).toEqual([]);
});
it('previews custom art, resets to the trusted default, and binds the exact preview before installing', async () => {
  const f = await fixture(); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  const before = f.c.snapshot().artwork[0].preview, path = join(f.root, 'private-art.png');
  await writeFile(path, await sharp({ create: { width: 256, height: 100, channels: 4, background: '#ff0000' } }).png().toBuffer());
  await f.c.command({ command: 'artwork', slot: 'bubble', action: 'choose' }, [path]);
  expect(f.c.snapshot().artwork[0].mode).toBe('custom'); expect(f.c.snapshot().artwork[0].preview).not.toBe(before);
  await f.c.command({ command: 'artwork', slot: 'bubble', action: 'default' }); expect(f.c.snapshot().artwork[0].preview).toBe(before);
  await f.c.command({ command: 'prepare' }); expect(f.c.snapshot().prepared).toBe(true);
  await f.c.command({ command: 'artwork', slot: 'bubble', action: 'choose' }, [path]);
  await f.c.command({ command: 'plan', acknowledgeUnknownSpace: false, explicitUpdate: false, replaceRetroflow: false });
  expect(f.c.snapshot().plan?.customArtwork).toEqual(['bubble']); expect(f.c.snapshot().plan?.blockers).toEqual([]);
  expect(f.transport.writes).toEqual([]); expect(await readFile(join(f.root, 'Strikers.vpk'))).toEqual(f.vpk);
  await writeFile(path, 'changed');
  await f.c.command({ command: 'plan', acknowledgeUnknownSpace: false, explicitUpdate: false, replaceRetroflow: false });
  expect(f.c.snapshot().plan).toBeUndefined(); expect(f.c.snapshot().error).toBe('artwork_changed');
});
it('rejects unknown release/device handles and never accepts a renderer path', async () => {
  const f = await fixture();
  await f.c.command({ command: 'selectRelease', id: 'b'.repeat(32) }); expect(f.c.snapshot().error).toBe('unknown_release');
  await f.c.command({ command: 'selectDevice', id: '/private/drive' }); expect(f.c.snapshot().error).toBe('unknown_device');
  expect(f.transport.writes).toEqual([]);
});
it('requires the current plan token and stops a cancelled transfer without reporting installed', async () => {
  const f = await fixture(); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'prepare' }); await f.c.command({ command: 'plan', acknowledgeUnknownSpace: false, explicitUpdate: false, replaceRetroflow: false });
  await f.c.command({ command: 'execute', id: 'b'.repeat(32) }); expect(f.c.snapshot().error).toBe('install_blocked'); expect(f.transport.writes).toEqual([]);
  const original = f.transport.writeOwnedPartial.bind(f.transport); let stopped = false;
  f.transport.writeOwnedPartial = async (path, data) => { await original(path, data); if (!stopped) { stopped = true; await f.c.command({ command: 'cancel' }); } };
  await f.c.command({ command: 'execute', id: f.c.snapshot().plan!.id });
  expect(f.c.snapshot().job?.phase).toBe('cancelled'); expect(f.c.snapshot().job?.completionMode).toBeUndefined();
});
it('pauses a Companion staging disconnect and resumes the same job without a lost promotion', async () => {
  const f = await fixture(); f.transport.device = { ...device, adapter: 'companion', capabilities: { ...device.capabilities, install: true } };
  vi.spyOn(nativeDevice, 'inspectDevice').mockImplementation(async () => f.transport.device);
  await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'prepare' }); await f.c.command({ command: 'plan', acknowledgeUnknownSpace: false, explicitUpdate: false, replaceRetroflow: false });
  const id = f.c.snapshot().plan!.id, original = f.transport.writeOwnedPartial.bind(f.transport);
  f.transport.writeOwnedPartial = async (path, data) => { await original(path, data); if (path.includes('/app/')) { f.transport.online = false; throw new Error('disconnected'); } };
  await f.c.command({ command: 'execute', id });
  expect(f.transport.writes.some(path => path.includes('/app/'))).toBe(true);
  expect(f.c.snapshot().job).toMatchObject({ phase: 'paused', availableActions: ['resume', 'cancel'] });
  const journal = new FileJournal(join(f.root, 'private/jobs'));
  expect((await journal.load(id)).promotion).toBe('not_started');
  await f.c.command({ command: 'pause' });
  expect((await journal.load(id)).snapshot.phase).toBe('paused');
  f.transport.online = true; f.transport.writeOwnedPartial = original;
  f.transport.device = { ...device };
  await f.c.command({ command: 'resume', id });
  expect(f.c.snapshot().job?.completionMode).toBe('ready_on_vita');
});
it('does not turn an idle pause into cancellation', async () => {
  const f = await fixture(); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'prepare' }); await f.c.command({ command: 'plan', acknowledgeUnknownSpace: false, explicitUpdate: false, replaceRetroflow: false });
  f.transport.failAfterWrites = 2;
  await f.c.command({ command: 'execute', id: f.c.snapshot().plan!.id });
  await f.c.command({ command: 'pause' });
  expect(f.c.snapshot().job?.phase).toBe('paused');
});
it('requires a reviewed matching conflict before accepting update consent', async () => {
  const f = await fixture(); f.transport.files.set('ux0:app/GBAD00001/sce_sys/param.sfo', sfo('GBAD00001'));
  f.transport.files.set('ux0:app/GBAD00001/eboot.bin', runtime);
  await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'prepare' });
  const request = { command: 'plan' as const, acknowledgeUnknownSpace: true, explicitUpdate: true, replaceRetroflow: true };
  await f.c.command(request);
  expect(f.c.snapshot().plan?.blockers).toContain('existing_app');
  await f.c.command(request);
  expect(f.c.snapshot().plan?.blockers).not.toContain('existing_app');
  f.transport.files.set('ux0:app/GBAD00001/eboot.bin', Buffer.from('another installed runtime'));
  await f.c.command(request);
  expect(f.c.snapshot().plan?.blockers).toContain('existing_app');
  expect(f.c.snapshot().plan?.consents?.update).toBe(false);
});
