import { lstat, mkdir, open, rename, rm, statfs } from 'node:fs/promises';
import { join } from 'node:path';
import { cancelled, requireThat } from '../shared/errors';
import { hashFile } from '../shared/files';
import type { Transport } from '../devices/transport';
import type { FileJournal, JobRecord } from './journal';
import { observeFile } from './observe';

export async function ensureBackups(record: JobRecord, transport: Transport, journal: FileJournal, signal?: AbortSignal) {
  if (!record.plan.backupFiles.length) return;
  const root = join(journal.root, record.plan.jobId, 'backups'); await mkdir(root, { recursive: true, mode: 0o700 });
  requireThat(!(await lstat(root)).isSymbolicLink(), 'unsafe_backup_path');
  const capacity = await statfs(root), total = record.plan.backupFiles.reduce((n, f) => n + f.bytes, 0);
  requireThat(capacity.bavail * capacity.bsize >= total + 16 * 1024 ** 2, 'backup_space_required');
  for (let i = 0; i < record.plan.backupFiles.length; i++) {
    cancelled(signal); const file = record.plan.backupFiles[i], local = join(root, `${i}.bin`), pending = `${local}.pending`;
    const existing = await lstat(local).catch(e => { if (e.code === 'ENOENT') return null; throw e; });
    if (existing) {
      requireThat(await hashFile(local, signal) === file.sha256, 'backup_changed');
      if (record.backups.some(b => b.path === file.path && b.local === local && b.sha256 === file.sha256)) continue;
      requireThat((await observeFile(transport, file.path, signal)).sha256 === file.sha256, 'protected_files_changed');
    }
    else {
      requireThat((await observeFile(transport, file.path, signal)).sha256 === file.sha256, 'protected_files_changed');
      await rm(pending, { force: true }); const handle = await open(pending, 'wx', 0o600);
      try { for await (const chunk of transport.read(file.path, signal)) { cancelled(signal); await handle.writeFile(chunk); } await handle.sync(); }
      finally { await handle.close(); }
      requireThat(await hashFile(pending, signal) === file.sha256, 'backup_verification_failed');
      await rename(pending, local);
    }
    if (!record.backups.some(b => b.path === file.path)) { record.backups.push({ path: file.path, local, sha256: file.sha256 }); await journal.save(record); }
  }
}
