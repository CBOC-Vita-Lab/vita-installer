import { afterEach, expect, it, vi } from 'vitest';
import { rm } from 'node:fs/promises';
import { join } from 'node:path';
import { jobFixture } from './fixture';
import { createInstallPlan } from '../../src/jobs/plan';
import { executeTransfer } from '../../src/jobs/transfer';
import { FileJournal } from '../../src/jobs/journal';
import { digest } from '../../src/shared/files';
vi.mock('../../resources/integrity/galaxy-cooked-rmge01-r0.json', async () => {
  const { digest, runtime } = await import('../fixtures/releases');
  return { default: { runtimeSha256: digest(runtime), files: [{ path: 'files/MovieData/opening.thp', bytes: 9, sha256: digest('old-movie') }] } };
});
vi.mock('../../resources/integrity/galaxy-movies-q55-v1.json', async () => {
  const { digest, runtime } = await import('../fixtures/releases');
  return { default: { runtimeSha256: digest(runtime), files: [{ path: 'files/MovieData/opening.thp', bytes: 4, sha256: digest('game') }] } };
});
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
it('preserves a verified installed q55 movie rather than replacing it for encoder rounding', async () => {
  const f = await jobFixture('galaxy'); roots.push(f.root);
  const path = 'ux0:data/smg/game/files/MovieData/opening.thp'; f.prepared.files[0].destination = path;
  f.transport.files.set(path, Buffer.from('old-movie'));
  const plan = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork });
  expect(plan.blockers).toEqual([]); expect(plan.files[0].action).toBe('preserve');
  expect(plan.protectedFiles).toContainEqual({ path, bytes: 9, sha256: digest('old-movie') });
  expect(plan.retainedData).toContainEqual({ path, bytes: 9, sha256: digest('old-movie') });
  expect((await executeTransfer(plan, f.transport, new FileJournal(join(f.root, 'jobs')), () => {})).phase).toBe('verifying');
  expect(f.transport.files.get(path)?.toString()).toBe('old-movie'); expect(f.transport.writes).not.toContain(path);
});
it('does not treat an unknown movie or a movie from another runtime as compatible', async () => {
  const f = await jobFixture('galaxy'); roots.push(f.root);
  const path = 'ux0:data/smg/game/files/MovieData/opening.thp'; f.prepared.files[0].destination = path;
  f.transport.files.set(path, Buffer.from('bad-movie'));
  const plan = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork });
  expect(plan.blockers).toContain('file_conflict');
  f.transport.files.set(path, Buffer.from('old-movie'));
  f.release.manifest.package!.runtimeSha256 = '9'.repeat(64);
  await expect(createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork }))
    .rejects.toMatchObject({ code: 'package_runtime_mismatch' });
  expect(f.transport.writes).toEqual([]);
});
