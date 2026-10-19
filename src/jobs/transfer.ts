import type { Transport, WriteScope } from '../devices/transport';
import type { JobSnapshot } from '../shared/contracts';
import type { InstallPlan } from './types';
import type { FileJournal } from './journal';
import { createReadStream } from 'node:fs';
import { cancelled, requireThat } from '../shared/errors';
import { hashFile } from '../shared/files';
import { planFingerprint } from './plan';
import { observeFile, observeTrees } from './observe';
import { ensureBackups } from './backups';
import { ensureBinding } from './binding';
import { verifyFile } from '../shared/integrity';

export function writeScope(plan: InstallPlan): WriteScope {
  requireThat(plan.fingerprint === planFingerprint(plan), 'plan_changed');
  return { jobId: plan.jobId, destinations: plan.files.map(f => f.destination), replacements:
    plan.files.filter(f => f.action === 'replace' && f.previous).map(f => ({ destination: f.destination, sha256: f.previous!.sha256 })) };
}

export async function verifyProtection(plan: InstallPlan, transport: Transport, signal?: AbortSignal) {
  const current = await observeTrees(transport, plan.protectedRoots, signal);
  requireThat(JSON.stringify(current) === JSON.stringify(plan.protectedFiles), 'protected_files_changed');
}
export async function verifyInstalledBaseline(plan: InstallPlan, transport: Transport, signal?: AbortSignal) {
  const root = `ux0:app/${plan.titleId}`;
  requireThat(!!await transport.stat(root) === plan.appExists, 'installed_app_changed');
  if (plan.appExists) {
    const current = await observeTrees(transport, [root], signal);
    requireThat(JSON.stringify(current) === JSON.stringify(plan.backupFiles.filter(f => f.path.startsWith(`${root}/`))), 'installed_app_changed');
  }
}
export async function verifySources(plan: InstallPlan, signal?: AbortSignal) {
  requireThat(plan.fingerprint === planFingerprint(plan), 'plan_changed'); cancelled(signal);
  for (const file of plan.files) {
    await verifyFile(file.source, file, signal);
  }
  for (const image of Object.values(plan.package.images)) await verifyFile(image.path, { sha256: image.outputSha256 }, signal);
}
export async function executeTransfer(plan: InstallPlan, transport: Transport, journal: FileJournal,
  onProgress: (snapshot: JobSnapshot) => void, signal?: AbortSignal): Promise<JobSnapshot> {
  await verifySources(plan, signal); requireThat(!plan.blockers.length, 'install_blocked');
  requireThat(await transport.identity() === plan.target.identity, 'device_changed');
  await verifyProtection(plan, transport, signal);
  await verifyInstalledBaseline(plan, transport, signal);
  const existing = await journal.load(plan.jobId).catch(e => { if (e.code === 'ENOENT') return null; throw e; });
  const completedBytes = plan.files.filter(f => ['copy', 'manual', 'replace'].includes(f.action) && existing?.verified.includes(f.destination))
    .reduce((n, f) => n + f.bytes, 0);
  const free = await transport.freeSpace();
  if (free === undefined || !Number.isSafeInteger(free)) requireThat(plan.warnings.includes('capacity_unverified'), 'capacity_unknown');
  else requireThat(free >= plan.requiredBytes - completedBytes * 2, 'insufficient_space');
  const record = existing ?? await journal.start(plan);
  requireThat(record.plan.fingerprint === plan.fingerprint && record.promotion === 'not_started', 'promotion_needs_reconciliation');
  const report = () => onProgress(structuredClone(record.snapshot));
  try {
    await ensureBackups(record, transport, journal, signal);
    await ensureBinding(record, transport, journal, signal);
    record.snapshot = { ...record.snapshot, phase: 'transferring', bytesDone: 0, verifiedFiles: 0, verifiedBytes: 0,
      errorCode: undefined, completionMode: undefined, availableActions: ['cancel'] }; report();
    for (const file of plan.files) {
      cancelled(signal); requireThat(await transport.identity() === plan.target.identity, 'device_changed');
      let destination = await transport.stat(file.destination);
      let replacing = false;
      if (file.action === 'preserve') requireThat(destination?.kind === 'file', 'target_changed');
      else if (destination) {
        requireThat(destination.kind === 'file', 'target_changed');
        const observed = await observeFile(transport, file.destination, signal);
        replacing = file.action === 'replace' && observed.sha256 === file.previous?.sha256;
        requireThat(observed.sha256 === file.sha256 || replacing, 'target_changed');
      }
      if (!destination || replacing) {
        requireThat(file.action !== 'keep' && !record.verified.includes(file.destination), 'target_changed');
        const staged = await transport.stat(file.stage);
        if (staged) {
          requireThat(staged.kind === 'file', 'staging_conflict');
          if (staged.bytes !== file.bytes || (await observeFile(transport, file.stage, signal)).sha256 !== file.sha256)
            await transport.removeOwned(file.stage);
        }
        if (!await transport.stat(file.stage)) {
          let sent = 0; const completedBytes = record.snapshot.bytesDone;
          async function* source() {
            for await (const chunk of createReadStream(file.source, { highWaterMark: 1024 * 1024, signal })) {
              cancelled(signal); sent += chunk.length;
              requireThat(sent <= file.bytes, 'source_changed');
              record.snapshot.bytesDone = completedBytes + sent; report(); yield chunk as Buffer;
            }
          }
          await transport.writeOwnedPartial(file.stage, source(), signal);
          record.snapshot.bytesDone = completedBytes;
        }
        requireThat((await observeFile(transport, file.stage, signal)).sha256 === file.sha256, 'transfer_verification_failed');
        requireThat(await hashFile(file.source, signal) === file.sha256, 'source_changed');
        destination = await transport.stat(file.destination);
        if (file.action === 'replace') {
          requireThat(file.previous && file.backup && record.backups.some(b => b.path === file.destination && b.sha256 === file.previous!.sha256), 'backup_required');
          if (destination && (await observeFile(transport, file.destination, signal)).sha256 === file.previous.sha256) {
            requireThat(await transport.stat(file.backup) === null, 'backup_conflict');
            await transport.moveAside(file.destination, file.backup, file.previous.sha256, signal);
            destination = null;
          }
          if (!destination) requireThat((await observeFile(transport, file.backup, signal)).sha256 === file.previous.sha256, 'backup_verification_failed');
        }
        if (destination) requireThat(destination.kind === 'file' && (await observeFile(transport, file.destination, signal)).sha256 === file.sha256, 'target_changed');
        else await transport.renameOwned(file.stage, file.destination, signal);
        // Staged bytes are already hashed. finishInstall hashes every final file before promotion.
        const placed = await transport.stat(file.destination);
        requireThat(placed?.kind === 'file' && placed.bytes === file.bytes, 'transfer_verification_failed');
      }
      if (!record.verified.includes(file.destination)) record.verified.push(file.destination);
      record.snapshot.bytesDone += file.bytes; record.snapshot.verifiedBytes! += file.bytes;
      record.snapshot.verifiedFiles++; await journal.save(record); report();
    }
    await verifyProtection(plan, transport, signal);
    record.snapshot.phase = 'verifying'; record.snapshot.vpkDestination = plan.vpkDestination;
    await journal.save(record); report(); return record.snapshot;
  } catch (error) {
    const stopped = signal?.aborted || (error as { code?: string }).code === 'cancelled';
    record.snapshot = { ...record.snapshot, phase: stopped ? 'cancelled' : 'paused', completionMode: undefined,
      errorCode: stopped ? 'cancelled' : (error as { code?: string }).code ?? 'connection_lost', availableActions: ['resume', 'cancel'] };
    await journal.save(record); report(); return record.snapshot;
  }
}
