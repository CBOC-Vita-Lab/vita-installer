import { afterEach, expect, it } from 'vitest';
import { readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { FileJournal } from '../../src/jobs/journal';
import { createInstallPlan } from '../../src/jobs/plan';
import { executeTransfer } from '../../src/jobs/transfer';
import { recoverJob } from '../../src/jobs/recovery';
import { stageRoot } from '../../src/devices/transport';
import { jobFixture, MemoryTransport } from './fixture';
import { digest } from '../../src/shared/files';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture(update = false) {
  const f = await jobFixture(); roots.push(f.root);
  const save = `ux0:user/00/savedata/${f.release.manifest.titleId}/save.dat`; f.transport.files.set(save, Buffer.from('save'));
  const plan = await createInstallPlan(f.release, f.prepared, f.device, update ? 'explicit_update' : 'first_install', { transport: f.transport, artwork: f.artwork });
  const journal = new FileJournal(join(f.root, 'jobs')); return { ...f, save, plan, journal };
}
it('verifies final data and VPK, preserves saves and never promotes from transfer', async () => {
  const f = await fixture(), progress: string[] = [];
  const result = await executeTransfer(f.plan, f.transport, f.journal, s => progress.push(s.phase));
  expect(result.phase).toBe('verifying'); expect(result.completionMode).toBeUndefined();
  expect(result.verifiedFiles).toBe(f.plan.files.length); expect(progress).toContain('transferring');
  for (const file of f.plan.files) expect(digest(f.transport.files.get(file.destination)!)).toBe(file.sha256);
  expect(f.transport.files.get(f.save)?.toString()).toBe('save'); expect((await f.journal.load(f.plan.jobId)).promotion).toBe('not_started');
});
it('only advances verified byte progress after readback, not when data is queued', async () => {
  const f = await fixture(), snapshots: import('../../src/shared/contracts').JobSnapshot[] = [];
  const result = await executeTransfer(f.plan, f.transport, f.journal, s => snapshots.push(s));
  const uploading = snapshots.find(s => s.bytesDone > 0 && s.verifiedFiles === 0)!;
  expect(uploading.verifiedBytes).toBe(0);
  expect(result.verifiedBytes).toBe(f.plan.files.reduce((n, file) => n + file.bytes, 0));
});
it('makes verified local backups before any explicit update can finish', async () => {
  const f = await fixture(true); await executeTransfer(f.plan, f.transport, f.journal, () => {});
  const state = await f.journal.load(f.plan.jobId); expect(state.backups).toHaveLength(1);
  expect(await readFile(state.backups[0].local)).toEqual(Buffer.from('save')); expect(state.backups[0].sha256).toBe(digest('save'));
});
it('rejects a changed source or plan before writing a job marker', async () => {
  const f = await fixture(); await writeFile(f.prepared.files[0].source, 'mutated');
  await expect(executeTransfer(f.plan, f.transport, f.journal, () => {})).rejects.toMatchObject({ code: 'source_changed' });
  expect(f.transport.writes).toEqual([]); await writeFile(f.prepared.files[0].source, 'game');
  f.plan.files[0].destination = 'ux0:data/unrelated/file';
  await expect(executeTransfer(f.plan, f.transport, f.journal, () => {})).rejects.toMatchObject({ code: 'plan_changed' });
});
it('resumes verified files after disconnect and retries an incomplete partial from zero', async () => {
  const f = await fixture(); f.transport.failAfterWrites = 5;
  expect((await executeTransfer(f.plan, f.transport, f.journal, () => {})).phase).toBe('paused');
  f.transport.online = true; f.transport.failAfterWrites = undefined;
  const firstWrites = f.transport.writes.filter(p => p === f.plan.files[0].destination).length;
  const pending = f.plan.files[1].stage; f.transport.files.set(pending, Buffer.from('bad partial'));
  const result = await recoverJob(f.plan.jobId, f.transport, f.journal, () => {});
  expect(result.phase).toBe('verifying');
  expect(f.transport.writes.filter(p => p === f.plan.files[0].destination)).toHaveLength(firstWrites);
  expect(f.transport.writes).toContain(`remove:${pending}`);
  expect(digest(f.transport.files.get(f.plan.files[1].destination)!)).toBe(f.plan.files[1].sha256);
});
it('refuses another device at the same IP or a missing marker without creating one', async () => {
  const f = await fixture(); f.transport.failAfterWrites = 3;
  await executeTransfer(f.plan, f.transport, f.journal, () => {});
  const other = new MemoryTransport();
  await expect(recoverJob(f.plan.jobId, other, f.journal, () => {})).rejects.toMatchObject({ code: 'missing_job_marker' });
  expect(other.writes).toEqual([]);
});
it('rechecks target conflicts and saved-state changes after preflight', async () => {
  const f = await fixture(); f.transport.files.set(f.plan.files[0].destination, Buffer.from('other data'));
  const result = await executeTransfer(f.plan, f.transport, f.journal, () => {});
  expect(result.errorCode).toBe('target_changed'); expect(f.transport.files.get(f.plan.files[0].destination)?.toString()).toBe('other data');
  const g = await fixture(); g.transport.files.set(g.save, Buffer.from('new save'));
  await expect(executeTransfer(g.plan, g.transport, g.journal, () => {})).rejects.toMatchObject({ code: 'protected_files_changed' });
  expect(g.transport.writes).toEqual([]);
});
it('a fresh plan after changed saves reuses verified copies without changing the current save', async () => {
  const f = await fixture();
  await executeTransfer(f.plan, f.transport, f.journal, () => {});
  f.transport.files.set(f.save, Buffer.from('new save'));
  await expect(executeTransfer(f.plan, f.transport, f.journal, () => {})).rejects.toMatchObject({ code: 'protected_files_changed' });
  const next = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork });
  const copied = f.plan.files[0].destination, writes = f.transport.writes.filter(path => path === copied).length;
  expect(next.files.find(file => file.destination === copied)?.action).toBe('keep');
  expect(next.protectedFiles.find(file => file.path === f.save)?.sha256).toBe(digest('new save'));
  expect((await executeTransfer(next, f.transport, f.journal, () => {})).phase).toBe('verifying');
  expect(f.transport.writes.filter(path => path === copied)).toHaveLength(writes);
  expect(f.transport.files.get(f.save)?.toString()).toBe('new save');
});
it('does not resume a different mount or blindly repeat an uncertain promotion', async () => {
  const f = await fixture(); await executeTransfer(f.plan, f.transport, f.journal, () => {});
  f.transport.device = { ...f.device, identity: 'different-card' };
  await expect(recoverJob(f.plan.jobId, f.transport, f.journal, () => {})).rejects.toMatchObject({ code: 'device_changed' });
  f.transport.device = f.device; const state = await f.journal.load(f.plan.jobId); state.promotion = 'requested'; await f.journal.save(state);
  const count = f.transport.writes.length;
  expect((await recoverJob(f.plan.jobId, f.transport, f.journal, () => {})).completionMode).toBe('needs_attention');
  expect(f.transport.writes).toHaveLength(count);
});
it('cancels without deleting unrelated files or claiming completion', async () => {
  const f = await fixture(), abort = new AbortController();
  f.transport.files.set('ux0:data/other/save.dat', Buffer.from('untouched'));
  const result = await executeTransfer(f.plan, f.transport, f.journal, s => { if (s.phase === 'transferring') abort.abort(); }, abort.signal);
  expect(result.phase).toBe('cancelled'); expect(result.completionMode).toBeUndefined();
  expect(f.transport.files.get('ux0:data/other/save.dat')?.toString()).toBe('untouched');
  expect(f.transport.writes.every(p => !p.startsWith('remove:') || p.startsWith(`remove:${stageRoot(f.plan.jobId)}/`))).toBe(true);
});
it('refuses an application that appeared after the read-only plan', async () => {
  const f = await fixture(); f.transport.files.set(`ux0:app/${f.plan.titleId}/eboot.bin`, Buffer.from('newly installed'));
  await expect(executeTransfer(f.plan, f.transport, f.journal, () => {})).rejects.toMatchObject({ code: 'installed_app_changed' });
  expect(f.transport.writes).toEqual([]);
});
it('backs up and replaces a cover, including recovery after it was moved aside', async () => {
  const f = await fixture(), source = join(f.root, 'cover.png'); await writeFile(source, 'new cover');
  f.artwork.images.retroflow = { path: source, sourceSha256: digest('new cover'), outputSha256: digest('new cover') };
  const cover = `ux0:data/RetroFlow/COVERS/Homebrew/${f.plan.titleId}.png`; f.transport.files.set(cover, Buffer.from('old cover'));
  const plan = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', {
    transport: f.transport, artwork: f.artwork, retroflow: { category: 'homebrew' }, replaceRetroflow: true,
  });
  const rename = f.transport.renameOwned.bind(f.transport); let fail = true;
  f.transport.renameOwned = async (source, destination) => {
    if (destination === cover && fail) { fail = false; throw new Error('lost connection'); } await rename(source, destination);
  };
  const stopped = await executeTransfer(plan, f.transport, f.journal, () => {}); expect(stopped.phase).toBe('paused');
  const state = await f.journal.load(plan.jobId), old = state.backups.find(b => b.path === cover)!;
  expect((await readFile(old.local)).toString()).toBe('old cover');
  expect(f.transport.files.get(plan.files.find(f => f.destination === cover)!.backup!)?.toString()).toBe('old cover');
  const result = await recoverJob(plan.jobId, f.transport, f.journal, () => {});
  expect(result.phase).toBe('verifying'); expect(f.transport.files.get(cover)?.toString()).toBe('new cover');
  expect((await readFile(old.local)).toString()).toBe('old cover');
});
it('preserves a private BIOS byte for byte instead of replacing it with the selected BIOS', async () => {
  const f = await jobFixture('vitadhoc'); roots.push(f.root);
  const source = join(f.root, 'bios.bin'); await writeFile(source, 'selected bios');
  const destination = 'ux0:data/GBAdhoc/gba_bios.bin'; f.transport.files.set(destination, Buffer.from('private bios'));
  f.prepared.files.push({ source, destination, bytes: 13, sha256: digest('selected bios') });
  const plan = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork });
  expect(plan.files.find(f => f.destination === destination)?.action).toBe('preserve');
  const result = await executeTransfer(plan, f.transport, new FileJournal(join(f.root, 'jobs')), () => {});
  expect(result.phase).toBe('verifying'); expect(f.transport.files.get(destination)?.toString()).toBe('private bios');
  expect(f.transport.writes).not.toContain(destination);
});
it('rechecks free space before the first device write', async () => {
  const f = await fixture(); f.transport.free = 1;
  await expect(executeTransfer(f.plan, f.transport, f.journal, () => {})).rejects.toMatchObject({ code: 'insufficient_space' });
  expect(f.transport.writes).toEqual([]);
});
