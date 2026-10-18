import { afterEach, expect, it } from 'vitest';
import { rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { createInstallPlan } from '../../src/jobs/plan';
import { jobFixture } from './fixture';
import { digest } from '../../src/shared/files';
import { sfo } from '../fixtures/releases';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture() { const f = await jobFixture(); roots.push(f.root); return f; }
async function plan(f: Awaited<ReturnType<typeof fixture>>, mode: 'first_install' | 'explicit_update' = 'first_install') {
  return createInstallPlan(f.release, f.prepared, f.device, mode, { transport: f.transport, artwork: f.artwork });
}
it('creates a strictly read-only plan and keeps verified identical files', async () => {
  const f = await fixture(), path = f.prepared.files[0].destination; f.transport.files.set(path, Buffer.from('game'));
  const result = await plan(f);
  expect(f.transport.writes).toEqual([]); expect(result.blockers).toEqual([]);
  expect(result.files.find(p => p.destination === path)?.action).toBe('keep');
  expect(result.requiredBytes).toBeGreaterThan(f.artwork.inventory.members.reduce((n, m) => n + m.bytes, 0));
});
it('blocks different existing files and installed applications by default', async () => {
  const f = await fixture(); f.transport.files.set(f.prepared.files[0].destination, Buffer.from('different'));
  f.transport.files.set(`ux0:app/${f.release.manifest.titleId}/eboot.bin`, Buffer.from('old app'));
  const result = await plan(f); expect(result.blockers).toContain('existing_app'); expect(result.blockers).toContain('file_conflict');
  expect(f.transport.writes).toEqual([]);
});
it('does not turn unknown or insufficient capacity into ready', async () => {
  const f = await fixture(); f.transport.free = undefined; expect((await plan(f)).blockers).toContain('capacity_unknown');
  f.transport.free = 2; expect((await plan(f)).blockers).toContain('insufficient_space');
});
it('invalidates changed data, package bytes or artwork previews', async () => {
  const f = await fixture(); await writeFile(f.prepared.files[0].source, 'changed');
  await expect(plan(f)).rejects.toMatchObject({ code: 'preparation_changed' });
  await writeFile(f.prepared.files[0].source, 'game');
  const image = join(f.root, 'cover.png'); await writeFile(image, 'changed image');
  f.artwork.images.retroflow = { path: image, sourceSha256: digest('original'), outputSha256: digest('original') };
  await expect(plan(f)).rejects.toMatchObject({ code: 'artwork_changed' });
  f.artwork.images = {}; await writeFile(f.artwork.path, 'bad package');
  await expect(plan(f)).rejects.toMatchObject({ code: 'package_hash_mismatch' });
});
it('includes update backups, saved state and app-private BIOS in the immutable inventory', async () => {
  const f = await fixture(), title = f.release.manifest.titleId;
  f.transport.files.set(`ux0:app/${title}/private_bios.bin`, Buffer.from('private bios'));
  f.transport.files.set(`ux0:user/00/savedata/${title}/save.dat`, Buffer.from('save'));
  const result = await plan(f, 'explicit_update');
  expect(result.backupFiles.map(p => p.path)).toEqual(expect.arrayContaining([`ux0:app/${title}/private_bios.bin`, `ux0:user/00/savedata/${title}/save.dat`]));
  expect(result.protectedFiles.find(p => p.path.endsWith('/save.dat'))?.sha256).toBe(digest('save'));
  expect(result.blockers).toContain('private_app_file_conflict');
  expect(f.transport.writes).toEqual([]);
});
it('preserves existing BIOS and stages ur0 prerequisites separately for a card', async () => {
  const f = await fixture(); f.device = { ...f.device, transport: 'card', adapter: 'volume', capabilities: { ...f.device.capabilities, ur0: false } };
  f.transport.device = f.device;
  const source = join(f.root, 'compiler'); await writeFile(source, 'module');
  f.prepared.files.push({ source, destination: 'ur0:data/libshacccg.suprx', bytes: 6, sha256: digest('module') });
  const result = await plan(f); expect(result.requiresUr0Action).toBe(true);
  expect(result.files.find(p => p.source === source)).toMatchObject({ action: 'manual', destination: expect.stringContaining('/prerequisites/libshacccg.suprx') });
});
it('rejects unrelated destinations even when prepared-file hashes are valid', async () => {
  const f = await fixture(); f.prepared.files[0].destination = 'ux0:data/VitaShell/config.txt';
  await expect(plan(f)).rejects.toMatchObject({ code: 'protected_destination' });
});
it('keeps unknown space visibly unverified after explicit acknowledgement', async () => {
  const f = await fixture(); f.transport.free = undefined;
  const result = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', {
    transport: f.transport, artwork: f.artwork, acknowledgeUnknownSpace: true,
  });
  expect(result.blockers).not.toContain('capacity_unknown'); expect(result.freeBytes).toBeUndefined();
  expect(result.warnings).toContain('capacity_unverified');
});
it('preserves an existing cover by default and requires explicit backup-backed replacement', async () => {
  const f = await fixture(), source = join(f.root, 'cover.png'); await writeFile(source, 'custom cover');
  f.artwork.images.retroflow = { path: source, sourceSha256: digest('custom cover'), outputSha256: digest('custom cover') };
  const destination = `ux0:data/RetroFlow/COVERS/Homebrew/${f.release.manifest.titleId}.png`;
  f.transport.files.set(destination, Buffer.from('old cover'));
  const options = { transport: f.transport, artwork: f.artwork, retroflow: { category: 'homebrew' as const } };
  const keep = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', options);
  expect(keep.files.find(f => f.destination === destination)?.action).toBe('preserve');
  expect(keep.protectedFiles.find(f => f.path === destination)?.sha256).toBe(digest('old cover'));
  const replace = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { ...options, replaceRetroflow: true });
  expect(replace.files.find(f => f.destination === destination)?.action).toBe('replace');
  expect(replace.backupFiles.find(f => f.path === destination)?.sha256).toBe(digest('old cover'));
});
it('checks the installed title before allowing an explicit update', async () => {
  const f = await fixture(), root = `ux0:app/${f.release.manifest.titleId}`;
  f.transport.files.set(`${root}/sce_sys/param.sfo`, sfo('INFC00001'));
  expect((await plan(f, 'explicit_update')).blockers).toContain('installed_title_mismatch');
  f.transport.files.set(`${root}/sce_sys/param.sfo`, sfo(f.release.manifest.titleId));
  expect((await plan(f, 'explicit_update')).blockers).not.toContain('installed_title_mismatch');
});
it('binds the preparation tool fingerprints into the plan', async () => {
  const f = await fixture(); f.prepared.toolFingerprints = { nod: 'a'.repeat(64) };
  expect((await plan(f)).toolFingerprints).toEqual(f.prepared.toolFingerprints);
});
