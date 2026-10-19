import { afterEach, expect, it } from 'vitest';
import { mkdtemp, mkdir, writeFile, readFile, rm, rename, symlink, lstat, realpath, chmod } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawn, type ChildProcess } from 'node:child_process';
import { createInterface } from 'node:readline';
import { once } from 'node:events';
import { openTransport, stageRoot, type Transport } from '../../src/devices/transport';
import type { DeviceCandidate } from '../../src/shared/contracts';
import type { VolumeEvidence } from '../../src/devices/discover';
import { hashFile } from '../../src/shared/files';
import { observeTrees } from '../../src/jobs/observe';
const roots: string[] = [], transports: Transport[] = [], children: ChildProcess[] = [];
const jobId = 'a'.repeat(32), partial = `${stageRoot(jobId)}/one.partial`, destination = 'ux0:data/game/new.bin';
const scope = { jobId, destinations: [destination] };
afterEach(async () => {
  for (const t of transports.splice(0)) await t.close();
  for (const child of children.splice(0)) { const closed = once(child, 'close'); child.kill(); await closed; }
  for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true });
});
async function* bytes(value = 'payload') { yield Buffer.from(value); }
async function all(stream: AsyncIterable<Uint8Array>) { const b = []; for await (const c of stream) b.push(c); return Buffer.concat(b); }
async function volume(kind: 'usb' | 'card' = 'card') {
  const parent = await realpath(await mkdtemp(join(tmpdir(), 'transport-'))); roots.push(parent);
  const root = join(parent, 'CARD'); await mkdir(join(root, 'data/game'), { recursive: true });
  await writeFile(join(root, 'data/game/save.dat'), 'save');
  let current: VolumeEvidence[] = [{ id: 'one', root, name: 'VITA', kind, readable: true, vitaLayout: true, role: 'unknown' }];
  const device: DeviceCandidate = { id: 'volume:one', identity: 'volume:one', root, label: 'VITA', transport: kind, storageRole: 'ux0',
    adapter: 'volume', state: 'ready', evidence: [], capabilities: { transfer: true, install: false, ur0: false } };
  const python = await realpath(resolve('.venv-workers', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python'));
  const volumeWorker = { path: python, sha256: await hashFile(python), args: [resolve('workers/volume.py')] };
  return { root, parent, device, options: { scope, volumes: async () => current, volumeWorker }, setVolumes: (v: VolumeEvidence[]) => current = v };
}
async function opened(device: DeviceCandidate, options: Parameters<typeof openTransport>[1]) {
  const t = await openTransport(device, options); transports.push(t); return t;
}
it.each(['usb', 'card'] as const)('copies through %s without an extra ux0 directory or save changes', async kind => {
  const v = await volume(kind), t = await opened(v.device, v.options);
  await t.writeOwnedPartial(partial, bytes()); await t.renameOwned(partial, destination);
  expect((await all(t.read(destination))).toString()).toBe('payload');
  expect((await readFile(join(v.root, 'data/game/save.dat'))).toString()).toBe('save');
  expect(await t.stat(partial)).toBeNull(); expect(await lstat(join(v.root, 'ux0')).catch(() => null)).toBeNull();
  expect(await t.freeSpace()).toBeGreaterThan(0);
});
it('rejects unknown or secondary storage roles and unreadable media', async () => {
  const v = await volume();
  await expect(opened({ ...v.device, storageRole: 'unknown' }, v.options)).rejects.toMatchObject({ code: 'storage_role_unknown' });
  v.setVolumes([]); await expect(opened(v.device, v.options)).rejects.toMatchObject({ code: 'device_changed' });
});
it('never recreates a removed mountpoint or writes to a replacement card', async () => {
  const v = await volume(), t = await opened(v.device, v.options);
  await rename(v.root, join(v.parent, 'removed'));
  await expect(t.writeOwnedPartial(partial, bytes())).rejects.toMatchObject({ code: 'device_changed' });
  expect(await lstat(v.root).catch(() => null)).toBeNull();
  await mkdir(v.root); await mkdir(join(v.root, 'data'));
  await expect(t.writeOwnedPartial(partial, bytes())).rejects.toMatchObject({ code: 'device_changed' });
  expect(await lstat(join(v.root, 'data/CBOC-Installer')).catch(() => null)).toBeNull();
});
it('stops a mid-file copy when the volume identity changes', async () => {
  const v = await volume(), t = await opened(v.device, v.options);
  async function* interrupted() { yield Buffer.from('first'); v.setVolumes([]); yield Buffer.from('second'); }
  await expect(t.writeOwnedPartial(partial, interrupted())).rejects.toMatchObject({ code: 'device_changed' });
  expect(['', 'first']).toContain((await readFile(join(v.root, partial.slice(4))).catch(() => Buffer.alloc(0))).toString());
});
it('rejects symlink escapes and never replaces existing data, including concurrent final conflicts', async () => {
  const v = await volume(), t = await opened(v.device, v.options), outside = join(v.parent, 'outside'); await mkdir(outside);
  await symlink(outside, join(v.root, 'data/CBOC-Installer'));
  await expect(t.writeOwnedPartial(partial, bytes())).rejects.toMatchObject({ code: 'unsafe_destination' });
  await rm(join(v.root, 'data/CBOC-Installer')); await t.writeOwnedPartial(partial, bytes());
  await writeFile(join(v.root, destination.slice(4)), 'existing');
  await expect(t.renameOwned(partial, destination)).rejects.toMatchObject({ code: 'destination_conflict' });
  expect((await readFile(join(v.root, destination.slice(4)))).toString()).toBe('existing');
});
it('limits writes and deletion to the owned job and approved absent destinations', async () => {
  const v = await volume(), t = await opened(v.device, v.options);
  for (const path of ['ux0:data/game/save.dat', `${stageRoot('b'.repeat(32))}/one`, 'ur0:data/x', 'ux0:../x', 'ux0:data/x\nDELE save', 'ux0:data/x;DELE'])
    await expect(t.writeOwnedPartial(path, bytes())).rejects.toThrow();
  await expect(t.removeOwned('ux0:data/game/save.dat')).rejects.toThrow();
  const readonly = await opened(v.device, { volumes: v.options.volumes });
  await expect(readonly.writeOwnedPartial(partial, bytes())).rejects.toMatchObject({ code: 'write_not_authorized' });
});
it('does not treat a cancelled or failed source stream as a completed file', async () => {
  const v = await volume(), t = await opened(v.device, v.options), abort = new AbortController();
  async function* cancel() { yield Buffer.from('first'); abort.abort(); yield Buffer.from('last'); }
  await expect(t.writeOwnedPartial(partial, cancel(), abort.signal)).rejects.toMatchObject({ code: 'cancelled' });
  if (await t.stat(partial)) await t.removeOwned(partial);
  async function* failed() { yield Buffer.from('first'); throw new Error('source failure'); }
  await expect(t.writeOwnedPartial(partial, failed())).rejects.toThrow('source failure');
});
it('requires the verified native volume worker, never falls back to path-based writes', async () => {
  const v = await volume(), t = await opened(v.device, { scope, volumes: v.options.volumes });
  await expect(t.writeOwnedPartial(partial, bytes())).rejects.toMatchObject({ code: 'volume_worker_missing' });
  expect(await t.stat(partial)).toBeNull();
});
it('moves an approved old cover to an owned backup, but never an unrelated file', async () => {
  const v = await volume(); await writeFile(join(v.root, destination.slice(4)), 'old cover');
  const sha256 = await hashFile(join(v.root, destination.slice(4)));
  const t = await opened(v.device, { ...v.options, scope: { ...scope, replacements: [{ destination, sha256 }] } });
  const backup = `${stageRoot(jobId)}/backup/cover.png`;
  await expect(t.moveAside(destination, backup, '0'.repeat(64))).rejects.toThrow();
  await expect(t.moveAside('ux0:data/game/save.dat', backup, sha256)).rejects.toThrow();
  await t.moveAside(destination, backup, sha256);
  expect(await t.stat(destination)).toBeNull(); expect((await all(t.read(backup))).toString()).toBe('old cover');
});
it('does not write through read-only storage permissions', async () => {
  const v = await volume(), t = await opened(v.device, v.options);
  await chmod(join(v.root, 'data'), 0o500);
  try { await expect(t.writeOwnedPartial(partial, bytes())).rejects.toMatchObject({ code: 'EACCES' }); }
  finally { await chmod(join(v.root, 'data'), 0o700); }
});
async function ftp(writable = true, fault?: string, prom = false) {
  const root = await mkdtemp(join(tmpdir(), 'transport-ftp-')); roots.push(root);
  await mkdir(join(root, 'ux0/data/game'), { recursive: true }); await mkdir(join(root, 'ux0/app'));
  const python = resolve('.venv-workers', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python');
  const child = spawn(python, ['tests/fixtures/ftp_server.py', root, ...(writable ? ['--writable'] : []), ...(fault ? ['--fault', fault] : []), ...(prom ? ['--prom'] : [])], { stdio: ['ignore', 'pipe', 'ignore'] }); children.push(child);
  const lines = createInterface({ input: child.stdout! });
  const [line] = await once(lines, 'line'); lines.close(); const { port } = JSON.parse(line as string);
  const device: DeviceCandidate = { id: 'ftp:test', identity: 'unbound:127.0.0.1', transport: 'ftp', label: 'Test', host: '127.0.0.1', port,
    storageRole: 'ux0', adapter: 'ftp', state: 'ready', evidence: [], capabilities: { transfer: true, install: false, ur0: false } };
  return { root, device };
}
it('streams and verifies real socket transfers without append or a fabricated capacity', async () => {
  const f = await ftp(), t = await opened(f.device, { scope }), payload = Buffer.alloc(1024 * 1024, 0x32);
  await t.writeOwnedPartial(partial, (async function* () { yield payload; })());
  await t.renameOwned(partial, destination);
  expect(await all(t.read(destination))).toEqual(payload); expect(await t.freeSpace()).toBeUndefined();
  expect(await t.stat(destination)).toMatchObject({ kind: 'file', bytes: payload.length });
  for (const path of ['ux0:../escape', 'ux0:data/a\r\nDELE x', 'ux0:data/a;b', 'ftp://outside/file'])
    await expect(t.stat(path)).rejects.toMatchObject({ code: 'unsafe_device_path' });
}, 10000);
it('checks FTP files, directories and absence without opening listing data connections', async () => {
  const f = await ftp(), t = await opened({ ...f.device, adapter: 'companion' }, { scope });
  await writeFile(join(f.root, 'ux0/data/game/existing.bin'), 'existing');
  expect(await t.stat('ux0:data/game/existing.bin')).toMatchObject({ kind: 'file', bytes: 8 });
  expect(await t.stat('ux0:data/game')).toMatchObject({ kind: 'directory' });
  expect(await t.stat(destination)).toBeNull();
  const commands = (await readFile(join(f.root, 'commands.jsonl'), 'utf8')).trim().split('\n').map(line => JSON.parse(line).command);
  expect(commands.filter(command => ['LIST', 'MLSD'].includes(command))).toEqual([]);
  expect(commands.length).toBeLessThanOrEqual(5);
  await writeFile(join(f.root, 'ux0/data/game/new.bin'), 'appeared');
  expect(await t.stat(destination)).toMatchObject({ kind: 'file', bytes: 8 });
  await expect(t.writeOwnedPartial(partial, bytes())).resolves.toBeUndefined();
  await expect(t.renameOwned(partial, destination)).rejects.toMatchObject({ code: 'destination_conflict' });
  expect(await readFile(join(f.root, 'ux0/data/game/new.bin'), 'utf8')).toBe('appeared');
});
it('creates and reuses FTP parent directories without listing them for every file', async () => {
  const f = await ftp(), t = await opened({ ...f.device, adapter: 'companion' }, { scope });
  await t.writeOwnedPartial(partial, bytes()); await t.renameOwned(partial, destination);
  await t.writeOwnedPartial(`${stageRoot(jobId)}/two.partial`, bytes('second'));
  const commands = (await readFile(join(f.root, 'commands.jsonl'), 'utf8')).trim().split('\n').map(line => JSON.parse(line).command);
  expect(commands.filter(command => ['LIST', 'MLSD'].includes(command))).toEqual([]);
  expect(await all(t.read(destination))).toEqual(Buffer.from('payload'));
  await expect(t.writeOwnedPartial(partial.replace('one.partial', 'two.partial'), bytes('overwrite'))).rejects.toMatchObject({ code: 'destination_conflict' });
});
it.each(['ftp', 'companion'] as const)('preserves directory traversal when a %s server returns SIZE for directories', async adapter => {
  const f = await ftp(true, 'size-directory'), t = await opened({ ...f.device, adapter }, { scope });
  await mkdir(join(f.root, 'ux0/data/game/saves'), { recursive: true });
  const save = join(f.root, 'ux0/data/game/saves/save.dat'); await writeFile(save, 'current save');
  expect(await t.stat('ux0:data/game')).toMatchObject({ kind: 'directory' });
  expect(await observeTrees(t, ['ux0:data/game'])).toEqual([
    { path: 'ux0:data/game/saves/save.dat', bytes: 12, sha256: await hashFile(save) },
  ]);
  expect(await t.stat('ux0:data/game/saves/save.dat')).toMatchObject({ kind: 'file', bytes: 12 });
  expect(await t.stat(destination)).toBeNull();
});
it('falls back to fresh directory listings when SIZE is unsupported', async () => {
  const f = await ftp(true, 'size-unsupported'), t = await opened(f.device, { scope });
  expect(await t.stat(destination)).toBeNull();
  await writeFile(join(f.root, 'ux0/data/game/new.bin'), 'new');
  expect(await t.stat(destination)).toMatchObject({ kind: 'file', bytes: 3 });
  const commands = (await readFile(join(f.root, 'commands.jsonl'), 'utf8')).trim().split('\n').map(line => JSON.parse(line).command);
  expect(commands.filter(command => command === 'SIZE')).toHaveLength(1);
  expect(commands.filter(command => ['LIST', 'MLSD'].includes(command)).length).toBeGreaterThanOrEqual(2);
});
it('does not confuse denied SIZE metadata with an absent file', async () => {
  const f = await ftp(true, 'size-denied'), t = await opened({ ...f.device, adapter: 'companion' }, { scope });
  await writeFile(join(f.root, 'ux0/data/game/new.bin'), 'protected');
  expect(await t.stat(destination)).toMatchObject({ kind: 'file', bytes: 9 });
  await t.writeOwnedPartial(partial, bytes());
  await expect(t.renameOwned(partial, destination)).rejects.toMatchObject({ code: 'destination_conflict' });
  expect(await readFile(join(f.root, 'ux0/data/game/new.bin'), 'utf8')).toBe('protected');
});
it.each([undefined, 'promote-reject', 'promote-drop'])('sends one scoped VitaShell PROM request (%s)', async fault => {
  const f = await ftp(true, fault, true);
  const t = await opened({ ...f.device, adapter: 'vitashell', capabilities: { ...f.device.capabilities, install: true } }, { scope, promotionTimeoutMs: 1000 });
  expect(await t.promoteVpk!()).toBe(fault === 'promote-reject' ? 'rejected' : fault === 'promote-drop' ? 'unknown' : 'accepted');
  const commands = (await readFile(join(f.root, 'promotions.jsonl'), 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  expect(commands).toEqual([`${stageRoot(jobId)}/install.vpk`]);
}, 10000);
it.each(['readonly', 'full', 'list', 'disconnect', 'mid-file'])('reports FTP %s failures without placing a final file', async fault => {
  const f = await ftp(fault !== 'readonly', fault === 'readonly' ? undefined : fault);
  const t = await opened(f.device, { scope, timeoutMs: 1000 });
  if (fault === 'list') await expect(t.list('ux0:data')).rejects.toThrow();
  else await expect(t.writeOwnedPartial(partial, bytes('never-final'.repeat(100000)))).rejects.toThrow();
  expect(await lstat(join(f.root, destination.replace(':', '/'))).catch(() => null)).toBeNull();
  if (fault === 'mid-file') expect((await lstat(join(f.root, partial.replace(':', '/')))).size).toBeGreaterThan(0);
}, 10000);
