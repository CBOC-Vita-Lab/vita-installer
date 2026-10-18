import { afterEach, expect, it } from 'vitest';
import { readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { FileJournal } from '../../src/jobs/journal';
import { createInstallPlan } from '../../src/jobs/plan';
import { jobFixture } from './fixture';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture() {
  const f = await jobFixture(); roots.push(f.root);
  const plan = await createInstallPlan(f.release, f.prepared, f.device, 'first_install', { transport: f.transport, artwork: f.artwork });
  return { ...f, plan, journal: new FileJournal(join(f.root, 'jobs')) };
}
it('persists the complete plan and unique device-binding token without device writes', async () => {
  const f = await fixture(), record = await f.journal.start(f.plan);
  expect(record.token).toMatch(/^[a-f0-9]{64}$/); expect(record.promotion).toBe('not_started');
  expect(await f.journal.load(f.plan.jobId)).toEqual(record); expect(f.transport.writes).toEqual([]);
  record.markerStarted = true; await f.journal.save(record); expect((await f.journal.load(f.plan.jobId)).markerStarted).toBe(true);
});
it('retains the last valid journal after interrupted replacement', async () => {
  const f = await fixture(), record = await f.journal.start(f.plan);
  const old = await readFile(join(f.journal.root, `${record.plan.jobId}.json`)); record.markerStarted = true;
  const interrupted = new FileJournal(f.journal.root, async () => { throw new Error('power loss'); });
  await expect(interrupted.save(record)).rejects.toThrow('power loss');
  expect(await readFile(join(f.journal.root, `${record.plan.jobId}.json`))).toEqual(old);
  expect(await readdir(f.journal.root)).toEqual([`${record.plan.jobId}.json`]);
});
it('rejects corrupt, stale or path-injected job records', async () => {
  const f = await fixture(), record = await f.journal.start(f.plan), stale = structuredClone(record);
  await expect(f.journal.load('../outside')).rejects.toMatchObject({ code: 'invalid_job_id' });
  await f.journal.save(record); await expect(f.journal.save(stale)).rejects.toMatchObject({ code: 'stale_job_state' });
  await writeFile(join(f.journal.root, `${record.plan.jobId}.json`), '{bad');
  await expect(f.journal.load(record.plan.jobId)).rejects.toMatchObject({ code: 'invalid_journal' });
});
