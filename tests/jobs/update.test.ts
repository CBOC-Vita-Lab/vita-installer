import { afterEach, expect, it } from 'vitest';
import { rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { jobFixture } from './fixture';
import { createInstallPlan } from '../../src/jobs/plan';
import { executeTransfer } from '../../src/jobs/transfer';
import { FileJournal } from '../../src/jobs/journal';
import { digest } from '../../src/shared/files';
import { runtime, sfo } from '../fixtures/releases';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture() {
  const f = await jobFixture(); roots.push(f.root);
  const plan = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork });
  const journal = new FileJournal(join(f.root, 'jobs'));
  await executeTransfer(plan, f.transport, journal, () => {});
  const record = await journal.load(plan.jobId);
  record.promotion = 'reconciled'; record.snapshot.phase = 'complete'; record.snapshot.completionMode = 'installed';
  f.transport.files.set(`ux0:app/${f.release.manifest.titleId}/eboot.bin`, runtime);
  f.transport.files.set(`ux0:app/${f.release.manifest.titleId}/sce_sys/param.sfo`, sfo(f.release.manifest.titleId));
  await journal.save(record);
  return { ...f, journal, record };
}
it('plans an app-only update without original game files, retaining full data checks', async () => {
  const f = await fixture(), path = f.prepared.files[0].destination;
  await rm(f.prepared.files[0].source);
  const prepared = { ...f.prepared, files: [] };
  const plan = await createInstallPlan(f.release, prepared, f.device, 'explicit_update', {
    transport: f.transport, artwork: f.artwork, appUpdate: f.record,
  });
  expect(plan.appOnly).toBe(true); expect(plan.blockers).toEqual([]);
  expect(plan.protectedFiles.find(file => file.path === path)?.sha256).toBe(digest('game'));
  expect(plan.backupFiles.some(file => file.path === path)).toBe(false);
  expect(plan.files.some(file => file.destination === path)).toBe(false);
  f.transport.writes = [];
  expect((await executeTransfer(plan, f.transport, f.journal, () => {})).phase).toBe('verifying');
  expect(f.transport.writes).not.toContain(path);
});
it.each(['missing', 'corrupt', 'other-device', 'other-runtime', 'unconfirmed'])('blocks an app-only update with %s baseline', async failure => {
  const f = await fixture(), path = f.prepared.files[0].destination;
  if (failure === 'missing') f.transport.files.delete(path);
  if (failure === 'corrupt') f.transport.files.set(path, Buffer.from('evil'));
  if (failure === 'other-device') f.transport.device = { ...f.device, identity: 'another-vita' };
  if (failure === 'other-runtime') f.transport.files.set(`ux0:app/${f.release.manifest.titleId}/eboot.bin`, Buffer.from('other runtime'));
  if (failure === 'unconfirmed') f.record.promotion = 'requested';
  f.transport.writes = [];
  await expect(createInstallPlan(f.release, { ...f.prepared, files: [] }, f.device, 'explicit_update', {
    transport: f.transport, artwork: f.artwork, appUpdate: f.record,
  })).rejects.toThrow();
  expect(f.transport.writes).toEqual([]);
});
it('requires reviewed data compatibility for another release', async () => {
  const f = await fixture(); f.release.manifestSha256 = digest('new release');
  f.prepared.recipeFingerprint = digest(`preparation-v1\n${f.release.manifestSha256}`);
  const options = { transport: f.transport, artwork: f.artwork, appUpdate: f.record };
  await expect(createInstallPlan(f.release, { ...f.prepared, files: [] }, f.device, 'explicit_update', options))
    .rejects.toMatchObject({ code: 'incompatible_update' });
  f.release.manifest.appUpdateFrom = [f.record.plan.manifestSha256];
  expect((await createInstallPlan(f.release, { ...f.prepared, files: [] }, f.device, 'explicit_update', options)).appOnly).toBe(true);
});
it('rechecks retained game data immediately before any update write', async () => {
  const f = await fixture();
  const plan = await createInstallPlan(f.release, { ...f.prepared, files: [] }, f.device, 'explicit_update', {
    transport: f.transport, artwork: f.artwork, appUpdate: f.record,
  });
  f.transport.files.delete(f.prepared.files[0].destination); f.transport.writes = [];
  await expect(executeTransfer(plan, f.transport, f.journal, () => {})).rejects.toMatchObject({ code: 'protected_files_changed' });
  expect(f.transport.writes).toEqual([]);
});
it('retains the data inventory across consecutive app-only updates', async () => {
  const f = await fixture(), prepared = { ...f.prepared, files: [] };
  const plan = await createInstallPlan(f.release, prepared, f.device, 'explicit_update', {
    transport: f.transport, artwork: f.artwork, appUpdate: f.record,
  });
  await executeTransfer(plan, f.transport, f.journal, () => {});
  const record = await f.journal.load(plan.jobId);
  record.promotion = 'reconciled'; record.snapshot.phase = 'complete'; record.snapshot.completionMode = 'installed';
  await f.journal.save(record);
  const next = await createInstallPlan(f.release, prepared, f.device, 'explicit_update', {
    transport: f.transport, artwork: f.artwork, appUpdate: record,
  });
  expect(next.retainedData).toEqual(plan.retainedData);
  expect(next.retainedData).toHaveLength(f.prepared.files.length);
  expect(next.blockers).toEqual([]);
});
it('requires the on-device receipt before an app-only update', async () => {
  const f = await fixture();
  f.transport.files.delete(`ux0:data/CBOC-Installer/jobs/${f.record.plan.jobId}/marker.json`);
  f.transport.writes = [];
  await expect(createInstallPlan(f.release, { ...f.prepared, files: [] }, f.device, 'explicit_update', {
    transport: f.transport, artwork: f.artwork, appUpdate: f.record,
  })).rejects.toMatchObject({ code: 'missing_job_marker' });
  expect(f.transport.writes).toEqual([]);
});
