import { afterEach, expect, it, vi } from 'vitest';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import sharp from 'sharp';
import { InstallerController } from '../../src/main/controller';
import { digest, manifest, runtime, sfo, zip } from '../fixtures/releases';
import { MemoryTransport, device } from '../jobs/fixture';
import { FileJournal } from '../../src/jobs/journal';
import * as nativeDevice from '../../src/devices/native';
import { downloadGithubRelease } from '../../src/releases/download';
import * as runner from '../../src/preparation/runner';
const roots: string[] = [], controllers: InstallerController[] = [];
afterEach(async () => { vi.restoreAllMocks(); for (const c of controllers.splice(0)) await c.dispose(); for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture(downloads = false, recipe: 'vitadhoc' | 'infected' | 'galaxy' = 'vitadhoc') {
  const root = await mkdtemp(join(tmpdir(), 'controller-test-')); roots.push(root);
  const icon = await sharp({ create: { width: 128, height: 128, channels: 4, background: '#67e8df' } }).png().toBuffer();
  const bg = await sharp({ create: { width: 840, height: 500, channels: 4, background: '#b0ee52' } }).png().toBuffer();
  const title = recipe === 'vitadhoc' ? 'GBAD00001' : recipe === 'galaxy' ? 'SMGV00001' : 'INFC00001';
  const vpk = await zip({ 'eboot.bin': runtime, 'sce_sys/param.sfo': sfo(title), 'sce_sys/icon0.png': icon, 'sce_sys/livearea/contents/bg.png': bg,
    'sce_sys/livearea/contents/template.xml': '<livearea><livearea-background><image>bg.png</image></livearea-background></livearea>' });
  const m = { ...manifest(digest(vpk)), titleId: title, recipeId: recipe, displayName: recipe,
    requiredInputs: recipe !== 'vitadhoc' ? [{ id: 'game', label: 'Game file', kind: `${recipe}-disc` }] : [],
    optionalInputs: recipe === 'vitadhoc' ? [{ id: 'roms', label: 'Your games', kind: 'roms' }] : [],
    destinations: { data: recipe === 'vitadhoc' ? 'ux0:data/GBAdhoc/roms' : recipe === 'galaxy' ? 'ux0:data/smg/game' : 'ux0:data/infected' }, protectedPaths: [], prerequisites: [] };
  const raw = Buffer.from(JSON.stringify(m)); await writeFile(join(root, 'cboc-release.json'), raw); await writeFile(join(root, 'Strikers.vpk'), vpk);
  const bundle = await zip({ 'cboc-release.json': raw, 'Strikers.vpk': vpk });
  const repo = `https://github.com/CBOC-Vita-Lab/${recipe === 'vitadhoc' ? 'vitadhoc' : `${recipe}-vita`}`;
  const url = `${repo}/releases/download/build21/cboc-release.zip`;
  let downloadCount = 0;
  const trust = { fingerprints: new Set([digest(raw)]), keys: new Map() };
  const transport = new MemoryTransport();
  const c = new InstallerController({ userData: join(root, 'private'), trust,
    downloadRelease: downloads ? (value, expectedManifestSha256, signal, onProgress) => downloadGithubRelease(value, {
      cache: join(root, 'downloads'), trust, expectedManifestSha256, signal, onProgress, fetcher: async target => {
        if (target === url) { downloadCount++; return new Response(Uint8Array.from(bundle)); }
        if (String(target).endsWith('/releases?per_page=30')) return new Response('[]');
        if (String(target).startsWith(repo.replace('https://github.com', 'https://api.github.com/repos') + '/releases/')) return new Response(JSON.stringify({
          tag_name: 'build21', draft: false, prerelease: false, html_url: `${repo}/releases/tag/build21`,
          assets: [{ name: 'cboc-release.zip', size: bundle.length, state: 'uploaded', digest: `sha256:${digest(bundle)}`, browser_download_url: url }] }));
        throw new Error('Unexpected download');
      } }) : undefined,
    discover: async function* () { yield transport.device; }, inspect: async () => transport.device, open: async () => transport,
    tools: async () => recipe === 'infected' ? { dataWorker: { path: 'test-reader', sha256: digest('worker') } } : {} }, () => {});
  controllers.push(c); return { root, c, vpk, transport, downloads: () => downloadCount };
}
it('lets a verified Galaxy package reach the required-file and artwork steps without installing over an existing game', async () => {
  const f = await fixture(false, 'galaxy'); f.transport.files.set('ux0:app/SMGV00001/eboot.bin', runtime);
  await f.c.command({ command: 'discover' }); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]);
  expect(f.c.snapshot().error).toBeUndefined();
  expect(f.c.snapshot().releases).toMatchObject([{ recipe: 'galaxy', held: false, installed: true }]);
  expect(f.c.snapshot().inputs).toMatchObject([{ id: 'game', status: 'missing' }]);
  expect(f.c.snapshot().artwork).toHaveLength(3); expect(f.c.snapshot().plan).toBeUndefined(); expect(f.transport.writes).toEqual([]);
});
it('imports a GitHub URL through authenticated download and keeps duplicate approval off', async () => {
  const f = await fixture(true); f.transport.files.set('ux0:app/GBAD00001/eboot.bin', runtime);
  await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'importUrl', url: 'https://github.com/CBOC-Vita-Lab/vitadhoc' });
  expect(f.c.snapshot().error).toBeUndefined(); expect(f.c.snapshot().releases[0]).toMatchObject({ recipe: 'vitadhoc', installed: true });
  expect(f.c.snapshot().artwork).toHaveLength(3); expect(f.c.snapshot().plan).toBeUndefined();
  expect(f.downloads()).toBe(1); expect(f.transport.writes).toEqual([]);
});
it.each(['missing', 'corrupt'] as const)('repairs a %s release package during initial import without modifying the original folder', async kind => {
  const f = await fixture(true), path = join(f.root, 'Strikers.vpk');
  if (kind === 'missing') await rm(path); else await writeFile(path, 'broken');
  await f.c.command({ command: 'import', kind: 'folder' }, [f.root]);
  expect(f.c.snapshot().error).toBeUndefined(); expect(f.c.snapshot().artwork).toHaveLength(3); expect(f.downloads()).toBe(1);
  if (kind === 'missing') await expect(readFile(path)).rejects.toMatchObject({ code: 'ENOENT' });
  else expect(await readFile(path, 'utf8')).toBe('broken');
  expect(f.transport.writes).toEqual([]);
});
it('repairs a deleted release after selection and continues preparation automatically', async () => {
  const f = await fixture(true); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await rm(join(f.root, 'Strikers.vpk')); await f.c.command({ command: 'prepare' });
  expect(f.c.snapshot().error).toBeUndefined(); expect(f.c.snapshot().prepared).toBe(true); expect(f.c.snapshot().notice).toBe('release_repaired');
  expect(f.downloads()).toBe(1); expect(f.transport.writes).toEqual([]);
});
it('requires a new review if release repair is needed after an install plan was approved', async () => {
  const f = await fixture(true); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'prepare' }); await f.c.command({ command: 'plan', acknowledgeUnknownSpace: false, explicitUpdate: false, replaceRetroflow: false });
  const id = f.c.snapshot().plan!.id;
  await rm(join(f.root, 'Strikers.vpk')); await f.c.command({ command: 'execute', id });
  expect(f.c.snapshot().error).toBe('release_repaired_review'); expect(f.c.snapshot().repairStep).toBe('artwork');
  expect(f.c.snapshot().plan).toBeUndefined(); expect(f.c.snapshot().job).toBeUndefined(); expect(f.transport.writes).toEqual([]);
});
it.each(['before-import', 'after-import', 'zip'] as const)('keeps selected local game files discoverable when repairing %s', async when => {
  vi.spyOn(runner, 'runWorker').mockResolvedValue({ stdout: JSON.stringify({ bootSha256: '952d4bb2c08209f4cbc0020d95c4cc601e9944cad5c7ade4c1d060fe8daab1bc' }), stderr: '' });
  const f = await fixture(true, 'infected'), game = join(f.root, 'game.iso'), packagePath = join(f.root, 'Strikers.vpk');
  await writeFile(game, 'own game');
  const selection = when === 'zip' ? join(f.root, 'selected.zip') : f.root;
  if (when === 'zip') await writeFile(selection, await zip({ 'cboc-release.json': await readFile(join(f.root, 'cboc-release.json')), 'game.iso': 'own game' }));
  if (when === 'before-import') await rm(packagePath);
  await f.c.command({ command: 'import', kind: when === 'zip' ? 'file' : 'folder' }, [selection]);
  expect(f.c.snapshot().inputs).toMatchObject([{ id: 'game', status: 'ready' }]);
  if (when === 'after-import') await rm(packagePath);
  await f.c.command({ command: 'discover' }); await f.c.command({ command: 'prepare' });
  expect(f.c.snapshot().error).toBeUndefined(); expect(f.c.snapshot().prepared).toBe(true);
  await f.c.command({ command: 'selectRelease', id: f.c.snapshot().selectedRelease! });
  expect(f.c.snapshot().inputs).toMatchObject([{ id: 'game', status: 'ready' }]);
  expect(await readFile(game, 'utf8')).toBe('own game'); expect(f.downloads()).toBe(1); expect(f.transport.writes).toEqual([]);
});
it('does not download replacement user games', async () => {
  const f = await fixture(true), game = join(f.root, 'own-game.gba'); await writeFile(game, 'own game');
  await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'chooseInput', id: 'roms', kind: 'file' }, [game]); await rm(game);
  await f.c.command({ command: 'prepare' });
  expect(f.c.snapshot().prepared).toBe(false); expect(f.c.snapshot().repairStep).toBe('required'); expect(f.downloads()).toBe(0);
  expect(f.transport.writes).toEqual([]);
});
it('repairs without consuming another slot in a full eight-folder input selection', async () => {
  const f = await fixture(true); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]);
  for (let i = 0; i < 7; i++) {
    const folder = join(f.root, `selected-${i}`); await mkdir(folder);
    await f.c.command({ command: 'scanFolder' }, [folder]);
  }
  expect(f.c.snapshot().error).toBeUndefined();
  await f.c.command({ command: 'discover' }); await rm(join(f.root, 'Strikers.vpk'));
  await f.c.command({ command: 'prepare' });
  expect(f.c.snapshot().error).toBeUndefined(); expect(f.c.snapshot().prepared).toBe(true);
  expect(f.downloads()).toBe(1); expect(f.transport.writes).toEqual([]);
});
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
it.each(['before-import', 'after-import'] as const)('detects an existing installation %s without approving an update or writing', async when => {
  const f = await fixture(); f.transport.files.set('ux0:app/GBAD00001/eboot.bin', runtime);
  if (when === 'before-import') await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'import', kind: 'folder' }, [f.root]);
  if (when === 'after-import') await f.c.command({ command: 'discover' });
  expect(f.c.snapshot().releases[0]).toMatchObject({ installed: true });
  expect(f.c.snapshot().plan).toBeUndefined(); expect(f.transport.writes).toEqual([]);
  f.transport.files.delete('ux0:app/GBAD00001/eboot.bin');
  await f.c.command({ command: 'discover' });
  expect(f.c.snapshot().releases[0]).toMatchObject({ installed: false });
});
it('serializes target selection so an installed warning cannot belong to another Vita', async () => {
  const f = await fixture(), second = new MemoryTransport();
  second.device = { ...device, id: 'ftp:second', identity: 'unbound:second' };
  f.transport.files.set('ux0:app/GBAD00001/eboot.bin', runtime);
  let releaseList!: () => void, enteredList!: () => void;
  const entered = new Promise<void>(resolve => { enteredList = resolve; });
  const gate = new Promise<void>(resolve => { releaseList = resolve; });
  const originalList = f.transport.list.bind(f.transport);
  f.transport.list = async path => { enteredList(); await gate; return originalList(path); };
  const c = new InstallerController({ userData: join(f.root, 'switch-state'),
    trust: { fingerprints: new Set([digest(await readFile(join(f.root, 'cboc-release.json')))]), keys: new Map() },
    discover: async function* () { yield f.transport.device; yield second.device; }, inspect: async candidate => candidate,
    open: async candidate => candidate.id === second.device.id ? second : f.transport, tools: async () => ({}) }, () => {});
  controllers.push(c); await c.command({ command: 'import', kind: 'folder' }, [f.root]); await c.command({ command: 'discover' });
  const first = c.command({ command: 'selectDevice', id: device.id }); await entered;
  try {
    expect(c.snapshot().busy).toBe('scanning');
    await c.command({ command: 'selectDevice', id: second.device.id });
    expect(c.snapshot().selectedDevice).toBe(device.id);
  } finally { releaseList(); await first; }
  expect(c.snapshot().releases[0].installed).toBe(true);
  await c.command({ command: 'selectDevice', id: second.device.id });
  expect(c.snapshot().selectedDevice).toBe(second.device.id); expect(c.snapshot().releases[0].installed).toBe(false);
  expect(f.transport.writes).toEqual([]); expect(second.writes).toEqual([]);
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
it('blocks a deleted release package before preparation and offers the release step', async () => {
  const f = await fixture(); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await rm(join(f.root, 'Strikers.vpk'));
  await f.c.command({ command: 'prepare' });
  expect(f.c.snapshot()).toMatchObject({ prepared: false, repairStep: 'release', error: 'release_files_missing', errorFile: 'Strikers.vpk' });
  expect(f.transport.writes).toEqual([]);
  await writeFile(join(f.root, 'Strikers.vpk'), f.vpk);
  await f.c.command({ command: 'prepare' }); expect(f.c.snapshot().prepared).toBe(true);
});
it.each(['missing', 'changed'] as const)('blocks a %s package when reselecting a release', async state => {
  const f = await fixture(); await f.c.command({ command: 'import', kind: 'folder' }, [f.root]);
  const id = f.c.snapshot().selectedRelease!;
  if (state === 'missing') await rm(join(f.root, 'Strikers.vpk'));
  else await writeFile(join(f.root, 'Strikers.vpk'), 'changed');
  await f.c.command({ command: 'selectRelease', id });
  expect(f.c.snapshot()).toMatchObject({ prepared: false, repairStep: 'release', error: `release_files_${state}`, errorFile: 'Strikers.vpk' });
  expect(f.transport.writes).toEqual([]);
  await writeFile(join(f.root, 'Strikers.vpk'), f.vpk);
  await f.c.command({ command: 'selectRelease', id });
  expect(f.c.snapshot().repairStep).toBeUndefined(); expect(f.c.snapshot().error).toBeUndefined();
});
it.each(['before-plan', 'before-copy'] as const)('blocks deleted input %s and accepts a located replacement without device writes', async when => {
  const f = await fixture(), path = join(f.root, 'owned.gba'); await writeFile(path, 'game');
  await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'chooseInput', id: 'roms', kind: 'file' }, [path]); await f.c.command({ command: 'prepare' });
  const plan = { command: 'plan' as const, explicitUpdate: false, acknowledgeUnknownSpace: false, replaceRetroflow: false };
  if (when === 'before-copy') await f.c.command(plan);
  const id = f.c.snapshot().plan?.id; await rm(path);
  await f.c.command(when === 'before-plan' ? plan : { command: 'execute', id: id! });
  expect(f.c.snapshot()).toMatchObject({ prepared: false, repairStep: 'required', error: 'source_missing', errorFile: 'owned.gba' });
  expect(f.c.snapshot().plan).toBeUndefined(); expect(f.transport.writes).toEqual([]);
  const replacement = join(f.root, 'restored.gba'); await writeFile(replacement, 'game');
  await f.c.command({ command: 'chooseInput', id: 'roms', kind: 'file' }, [replacement]); await f.c.command({ command: 'prepare' }); await f.c.command(plan);
  expect(f.c.snapshot().plan?.blockers).toEqual([]); expect(f.c.snapshot().error).toBeUndefined(); expect(f.c.snapshot().repairStep).toBeUndefined();
});
it('does not retain ready status for a file deleted before preparation', async () => {
  const f = await fixture(), path = join(f.root, 'owned.gba'); await writeFile(path, 'game');
  await f.c.command({ command: 'import', kind: 'folder' }, [f.root]); await f.c.command({ command: 'discover' });
  await f.c.command({ command: 'chooseInput', id: 'roms', kind: 'file' }, [path]); await rm(path);
  await f.c.command({ command: 'prepare' });
  expect(f.c.snapshot().inputs).toMatchObject([{ id: 'roms', status: 'missing' }]);
  expect(f.c.snapshot().repairStep).toBe('required'); expect(f.c.snapshot().prepared).toBe(false); expect(f.transport.writes).toEqual([]);
});
