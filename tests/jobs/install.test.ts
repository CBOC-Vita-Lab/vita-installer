import { afterEach, expect, it } from 'vitest';
import { rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { createInstallPlan } from '../../src/jobs/plan';
import { executeTransfer } from '../../src/jobs/transfer';
import { finishInstall, verifyInstalled, type InstallationServices } from '../../src/jobs/install';
import { FileJournal } from '../../src/jobs/journal';
import { jobFixture } from './fixture';
import { digest, runtime, sfo } from '../fixtures/releases';
import type { DeviceCandidate } from '../../src/shared/contracts';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture(adapter: DeviceCandidate['adapter'] = 'vitashell', kind: DeviceCandidate['transport'] = 'ftp') {
  const f = await jobFixture(); roots.push(f.root);
  const device: DeviceCandidate = { ...f.device, transport: kind, adapter, capabilities: { transfer: true, install: ['vitashell', 'companion'].includes(adapter), ur0: kind === 'ftp' } };
  f.transport.device = device;
  const plan = await createInstallPlan(f.release, f.prepared, device, 'first_install', { transport: f.transport, artwork: f.artwork });
  const journal = new FileJournal(join(f.root, 'jobs')); await executeTransfer(plan, f.transport, journal, () => {});
  const calls: string[] = [];
  const install = (title = plan.titleId, binary = runtime) => {
    f.transport.files.set(`ux0:app/${plan.titleId}/sce_sys/param.sfo`, sfo(title));
    f.transport.files.set(`ux0:app/${plan.titleId}/eboot.bin`, binary);
  };
  const services: InstallationServices = { inspect: async () => f.transport.device, promote: async (_device, path) => { calls.push(path); install(); return 'accepted'; } };
  return { ...f, device, plan, journal, calls, services, install };
}
it('reports installed only after VitaShell promotion and installed-title/runtime readback', async () => {
  const f = await fixture();
  const result = await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services);
  expect(result.completionMode).toBe('installed'); expect(f.calls).toEqual([f.plan.vpkDestination]);
  expect(await verifyInstalled(f.plan, f.transport)).toMatchObject({ titleId: f.plan.titleId, runtimeSha256: f.plan.package.runtimeSha256, preserved: true });
});
it('reads new game bytes once in staging and once at the final gate, not three times', async () => {
  const f = await jobFixture(); roots.push(f.root);
  const plan = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork });
  const journal = new FileJournal(join(f.root, 'jobs')), file = plan.files[0], reads: string[] = [];
  const read = f.transport.read.bind(f.transport);
  f.transport.read = async function* (path) { reads.push(path); yield* read(path); };
  expect((await executeTransfer(plan, f.transport, journal, () => {})).phase).toBe('verifying');
  const result = await finishInstall(plan, f.transport, journal, () => {}, {
    inspect: async () => ({ ...f.device, capabilities: { ...f.device.capabilities, install: false } }),
    promote: async () => { throw new Error('must not promote'); },
  });
  expect(result.completionMode).toBe('ready_on_vita');
  expect(reads.filter(path => path === file.stage || path === file.destination)).toEqual([file.stage, file.destination]);
});
it.each(['changed', 'deleted'] as const)('blocks promotion if final game data was %s after staging verification', async change => {
  const f = await fixture('companion'), file = f.plan.files[0];
  if (change === 'deleted') f.transport.files.delete(file.destination);
  else f.transport.files.set(file.destination, Buffer.alloc(file.bytes, 0x78));
  await expect(finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).rejects.toThrow();
  expect(f.calls).toEqual([]);
  expect((await f.journal.load(f.plan.jobId)).promotion).toBe('not_started');
});
it('reports final verification bytes separately and clears them before promotion readback', async () => {
  const f = await fixture(), snapshots: import('../../src/shared/contracts').JobSnapshot[] = [];
  const result = await finishInstall(f.plan, f.transport, f.journal, s => snapshots.push(s), f.services);
  const total = f.plan.files.reduce((n, file) => n + file.bytes, 0);
  expect(snapshots.some(s => s.phase === 'verifying' && s.verification?.completed === total && s.verification?.total === total)).toBe(true);
  expect(snapshots.find(s => s.phase === 'installing')?.verification).toBeUndefined();
  expect(result.completionMode).toBe('installed');
});
it('passes Companion a fully verified extracted directory, not a VPK filename', async () => {
  const f = await fixture('companion');
  f.services.promote = async (_device, path) => {
    f.calls.push(path); expect(path.endsWith('/app')).toBe(true); expect(f.transport.files.get(`${path}/eboot.bin`)).toEqual(runtime);
    expect(f.transport.files.get(`${path}/sce_sys/param.sfo`)).toEqual(sfo()); f.install(); return 'accepted';
  };
  expect((await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).completionMode).toBe('installed');
  expect(f.calls).toHaveLength(1);
});
it.each(['ftp', 'usb', 'card'] as const)('keeps %s completion truthful without an installation request', async kind => {
  const f = await fixture(kind === 'ftp' ? 'ftp' : 'volume', kind);
  const result = await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services);
  expect(result.completionMode).toBe(kind === 'card' ? 'ready_on_card' : 'ready_on_vita');
  expect(result.vpkDestination).toBe(f.plan.vpkDestination); expect(f.calls).toEqual([]);
});
it('revokes stale automatic-install capability after reinspection', async () => {
  const f = await fixture(); f.services.inspect = async () => ({ ...f.device, adapter: 'ftp', capabilities: { ...f.device.capabilities, install: false } });
  expect((await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).completionMode).toBe('ready_on_vita');
  expect(f.calls).toEqual([]);
});
it.each(['rejected', 'unknown', 'accepted'] as const)('does not claim success from a %s response without installed bytes', async status => {
  const f = await fixture(); f.services.promote = async (_device, path) => { f.calls.push(path); return status; };
  const first = await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services);
  expect(first.completionMode).toBe('needs_attention');
  const second = await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services);
  expect(second.completionMode).toBe('needs_attention'); expect(f.calls).toHaveLength(1);
});
it('reconciles a lost reply as installed when the exact installed bytes are present', async () => {
  const f = await fixture(); f.services.promote = async (_device, path) => { f.calls.push(path); f.install(); throw new Error('reply lost'); };
  expect((await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).completionMode).toBe('installed');
  expect(f.calls).toHaveLength(1);
});
it.each(['title', 'runtime'] as const)('rejects a mismatched installed %s', async mismatch => {
  const f = await fixture(); f.services.promote = async () => { f.install(mismatch === 'title' ? 'INFC00001' : f.plan.titleId, mismatch === 'runtime' ? Buffer.from('different') : runtime); return 'accepted'; };
  expect((await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).completionMode).toBe('needs_attention');
});
it('does not call a card ready after staged bytes changed', async () => {
  const f = await fixture('volume', 'card'); f.transport.files.set(f.plan.vpkDestination, Buffer.from('corrupt'));
  await expect(finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).rejects.toMatchObject({ code: 'transfer_verification_failed' });
  expect(f.calls).toEqual([]);
});
it('does not start installation before a complete verified transfer or on a modified plan', async () => {
  const f = await fixture(); const record = await f.journal.load(f.plan.jobId); record.verified = []; await f.journal.save(record);
  await expect(finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).rejects.toMatchObject({ code: 'transfer_incomplete' });
  f.plan.requiresUr0Action = true;
  await expect(finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).rejects.toMatchObject({ code: 'plan_changed' });
  expect(f.calls).toEqual([]);
});
it('leaves a necessary ur0 action explicit instead of claiming it was satisfied by a card', async () => {
  const f = await jobFixture(); roots.push(f.root);
  const device = { ...f.device, transport: 'card' as const, adapter: 'volume' as const, capabilities: { transfer: true, install: false, ur0: false } };
  f.transport.device = device;
  f.prepared.files.push({ ...f.prepared.files[0], destination: 'ur0:data/libshacccg.suprx' });
  const plan = await createInstallPlan(f.release, f.prepared, device, 'first_install', { transport: f.transport, artwork: f.artwork });
  const journal = new FileJournal(join(f.root, 'jobs')); await executeTransfer(plan, f.transport, journal, () => {});
  const result = await finishInstall(plan, f.transport, journal, () => {}, { inspect: async () => device, promote: async () => { throw new Error('must not install'); } });
  expect(result.completionMode).toBe('ready_on_card'); expect(result.nextActions).toContainEqual({ kind: 'copy_prerequisite',
    from: `ux0:data/CBOC-Installer/jobs/${plan.jobId}/prerequisites/libshacccg.suprx`, to: 'ur0:data/libshacccg.suprx' });
});
it('reconnects for readback after promotion without sending a second promotion', async () => {
  const f = await fixture(); let reconnects = 0;
  f.services.promote = async (_device, path) => { f.calls.push(path); f.install(); f.transport.online = false; return 'unknown'; };
  f.services.reconnect = async () => { reconnects++; f.transport.online = true; return f.transport; };
  expect((await finishInstall(f.plan, f.transport, f.journal, () => {}, f.services)).completionMode).toBe('installed');
  expect(f.calls).toHaveLength(1); expect(reconnects).toBe(1);
});
it('honors cancellation before the irreversible promotion request', async () => {
  const f = await fixture('companion'), abort = new AbortController();
  f.services.inspect = async () => { abort.abort(); return f.device; };
  await expect(finishInstall(f.plan, f.transport, f.journal, () => {}, f.services, abort.signal)).rejects.toMatchObject({ code: 'cancelled' });
  expect(f.calls).toEqual([]); expect((await f.journal.load(f.plan.jobId)).promotion).toBe('not_started');
});
it.each(['accepted', 'unknown'] as const)('retains manual cover instructions after %s automatic promotion', async status => {
  const f = await jobFixture(); roots.push(f.root);
  const path = join(f.root, 'cover.png'); await writeFile(path, 'cover');
  f.artwork.images.retroflow = { path, outputSha256: digest('cover'), sourceSha256: digest('cover') };
  const current = { ...f.device, adapter: 'vitashell' as const, capabilities: { ...f.device.capabilities, install: true } };
  f.transport.device = current;
  const plan = await createInstallPlan(f.release, f.prepared, current, 'first_install', { transport: f.transport, artwork: f.artwork });
  const journal = new FileJournal(join(f.root, 'jobs')); await executeTransfer(plan, f.transport, journal, () => {});
  const services: InstallationServices = { inspect: async () => current, promote: async () => {
    f.transport.files.set(`ux0:app/${plan.titleId}/eboot.bin`, runtime);
    f.transport.files.set(`ux0:app/${plan.titleId}/sce_sys/param.sfo`, sfo()); return status;
  } };
  const result = await finishInstall(plan, f.transport, journal, () => {}, services);
  expect(result.completionMode).toBe('installed');
  expect(result.nextActions).toEqual([{ kind: 'import_cover', from: `ux0:data/CBOC-Installer/jobs/${plan.jobId}/artwork/MSCN00003.png` }]);
});
