import type { InstallPlan } from './types';
import type { JobSnapshot } from '../shared/contracts';
import { randomBytes } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdir, open, rename, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { digest } from '../shared/files';
import { InstallerError, requireThat } from '../shared/errors';
import { planFingerprint } from './plan';
export interface JobRecord {
  schemaVersion: 1; revision: number; plan: InstallPlan; token: string; markerStarted: boolean;
  verified: string[]; backups: { path: string; sha256: string; local: string }[];
  promotion: 'not_started' | 'requested' | 'reconciled'; snapshot: JobSnapshot;
}
export class FileJournal {
  private pending: Promise<unknown> = Promise.resolve();
  constructor(public root: string, private beforeReplace?: () => Promise<void>) {}
  private path(jobId: string) {
    requireThat(/^[a-f0-9]{32}$/.test(jobId), 'invalid_job_id'); return join(this.root, `${jobId}.json`);
  }
  private serial<T>(operation: () => Promise<T>): Promise<T> {
    const next = this.pending.then(operation, operation); this.pending = next.catch(() => {}); return next;
  }
  async start(plan: InstallPlan): Promise<JobRecord> {
    return this.serial(async () => {
      requireThat(plan.fingerprint === planFingerprint(plan), 'plan_changed');
      requireThat(!await lstat(this.path(plan.jobId)).catch(e => { if (e.code === 'ENOENT') return null; throw e; }), 'job_exists');
      const record: JobRecord = { schemaVersion: 1, revision: 0, plan: structuredClone(plan), token: randomBytes(32).toString('hex'), markerStarted: false,
        verified: [], backups: [], promotion: 'not_started', snapshot: { jobId: plan.jobId, phase: 'checking', bytesDone: 0,
          bytesTotal: plan.files.reduce((n, f) => n + f.bytes, 0), verifiedFiles: 0, totalFiles: plan.files.length, availableActions: ['cancel'] } };
      await this.replace(record); return record;
    });
  }
  async load(jobId: string): Promise<JobRecord> {
    const path = this.path(jobId), handle = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
    try {
      const stat = await handle.stat(); requireThat(stat.isFile() && stat.size <= 64 * 1024 ** 2, 'invalid_journal');
      const envelope = JSON.parse(await handle.readFile('utf8')) as { payload: string; sha256: string };
      requireThat(typeof envelope.payload === 'string' && digest(envelope.payload) === envelope.sha256, 'invalid_journal');
      const record = JSON.parse(envelope.payload) as JobRecord;
      requireThat(record.schemaVersion === 1 && Number.isSafeInteger(record.revision) && record.revision >= 0 && record.plan?.jobId === jobId
        && record.plan.fingerprint === planFingerprint(record.plan) && /^[a-f0-9]{64}$/.test(record.token)
        && typeof record.markerStarted === 'boolean' && Array.isArray(record.verified) && Array.isArray(record.backups)
        && ['not_started', 'requested', 'reconciled'].includes(record.promotion) && record.snapshot?.jobId === jobId, 'invalid_journal');
      return record;
    } catch { throw new InstallerError('invalid_journal'); } finally { await handle.close(); }
  }
  async save(record: JobRecord): Promise<void> {
    await this.serial(async () => {
      const current = await this.load(record.plan.jobId);
      requireThat(current.revision === record.revision, 'stale_job_state');
      requireThat(record.plan.fingerprint === planFingerprint(record.plan) && record.plan.fingerprint === current.plan.fingerprint
        && record.token === current.token, 'plan_changed');
      const next = { ...record, revision: record.revision + 1 }; await this.replace(next); record.revision = next.revision;
    });
  }
  private async replace(record: JobRecord) {
    await mkdir(this.root, { recursive: true, mode: 0o700 });
    requireThat(!(await lstat(this.root)).isSymbolicLink(), 'unsafe_journal_path');
    const path = this.path(record.plan.jobId), temporary = `${path}.${randomBytes(8).toString('hex')}.tmp`;
    try {
      const handle = await open(temporary, 'wx', 0o600);
      try { const payload = JSON.stringify(record); await handle.writeFile(JSON.stringify({ payload, sha256: digest(payload) })); await handle.sync(); }
      finally { await handle.close(); }
      await this.beforeReplace?.(); await rename(temporary, path);
      if (process.platform !== 'win32') { const directory = await open(this.root, 'r'); try { await directory.sync(); } finally { await directory.close(); } }
    } finally { await rm(temporary, { force: true }); }
  }
}
