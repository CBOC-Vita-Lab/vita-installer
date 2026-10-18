import type { DeviceCandidate, JobSnapshot } from '../shared/contracts';
import type { Transport } from '../devices/transport';
import type { InstallPlan } from './types';
import type { FileJournal } from './journal';
import { stageRoot } from '../devices/transport';
import { cancelled, requireThat } from '../shared/errors';
import { hashFile } from '../shared/files';
import { planFingerprint } from './plan';
import { verifyInstalledBaseline, verifyProtection, verifySources } from './transfer';
import { assertBinding } from './binding';
import { observeFile, readSmallFile } from './observe';
import { readSfo } from '../releases/sfo';
import { PACKAGE_LIMITS, walkZip } from '../releases/archive';
export type PromotionResult = 'accepted' | 'rejected' | 'unknown';
export interface InstalledEvidence { titleId: string; runtimeSha256: string; preserved: boolean; membersVerified: number }
export interface InstallationServices {
  inspect(device: DeviceCandidate): Promise<DeviceCandidate>;
  promote(device: DeviceCandidate, path: string): Promise<PromotionResult>;
  reconnect?(): Promise<Transport>;
}
export async function verifyInstalled(plan: InstallPlan, transport: Transport): Promise<InstalledEvidence | null> {
  if (transport.device.transport !== 'ftp') return null;
  const root = `ux0:app/${plan.titleId}`;
  if (!await transport.stat(`${root}/sce_sys/param.sfo`)) return null;
  const bytes = await readSmallFile(transport, `${root}/sce_sys/param.sfo`, 65536);
  let titleId: string; try { titleId = readSfo(bytes).titleId; } catch { return null; }
  if (titleId !== plan.titleId) return null;
  let verified = 0;
  for (const member of plan.package.inventory.members) {
    if (member.path.startsWith('sce_sys/package/')) continue;
    const path = `${root}/${member.path}`;
    if (!await transport.stat(path) || (await observeFile(transport, path)).sha256 !== member.sha256) return null;
    verified++;
  }
  const runtime = await observeFile(transport, `${root}/eboot.bin`);
  if (runtime.sha256 !== plan.package.runtimeSha256) return null;
  await verifyProtection(plan, transport);
  return { titleId, runtimeSha256: runtime.sha256, preserved: true, membersVerified: verified };
}

async function stageCompanion(plan: InstallPlan, transport: Transport, signal?: AbortSignal) {
  const root = `${stageRoot(plan.jobId)}/app`;
  requireThat(await hashFile(plan.package.path, signal) === plan.package.sha256, 'package_hash_mismatch');
  await walkZip(plan.package.path, PACKAGE_LIMITS, async (entry, stream) => {
    const member = plan.package.inventory.members.find(m => m.path === entry.fileName);
    requireThat(member, 'package_identity_mismatch');
    const path = `${root}/${member.path}`, staged = await transport.stat(path);
    if (staged) {
      requireThat(staged.kind === 'file', 'staging_conflict');
      if ((await observeFile(transport, path)).sha256 === member.sha256) { for await (const _chunk of stream) { /* Drain the authenticated entry. */ } return; }
      await transport.removeOwned(path);
    }
    await transport.writeOwnedPartial(path, stream, signal);
    requireThat((await observeFile(transport, path)).sha256 === member.sha256, 'transfer_verification_failed');
  }, signal);
  requireThat(await hashFile(plan.package.path, signal) === plan.package.sha256, 'source_changed');
  return root;
}

export async function finishInstall(plan: InstallPlan, transport: Transport, journal: FileJournal,
  onProgress: (snapshot: JobSnapshot) => void, services: InstallationServices, signal?: AbortSignal): Promise<JobSnapshot> {
  requireThat(plan.fingerprint === planFingerprint(plan), 'plan_changed');
  const record = await journal.load(plan.jobId); requireThat(record.plan.fingerprint === plan.fingerprint, 'plan_changed');
  await assertBinding(record, transport);
  const coverActions = plan.files.filter(f => f.destination.startsWith(`${stageRoot(plan.jobId)}/artwork/`))
    .map(file => ({ kind: 'import_cover' as const, from: file.destination }));
  const report = () => onProgress(structuredClone(record.snapshot));
  async function reconcile() {
    record.snapshot.phase = 'verifying'; record.snapshot.completionMode = undefined; record.snapshot.availableActions = []; report();
    let evidence: InstalledEvidence | null = null, errorCode = 'installed_bytes_unverified';
    try { evidence = await verifyInstalled(plan, transport); }
    catch (error) {
      errorCode = (error as { code?: string }).code ?? 'connection_lost';
      if (services.reconnect) {
        let next: Transport | undefined;
        try { next = await services.reconnect(); await assertBinding(record, next); evidence = await verifyInstalled(plan, next); }
        catch (e) { errorCode = (e as { code?: string }).code ?? 'connection_lost'; }
        finally { await next?.close(); }
      }
    }
    record.snapshot = { ...record.snapshot, phase: evidence ? 'complete' : 'paused', completionMode: evidence ? 'installed' : 'needs_attention',
      errorCode: evidence ? undefined : errorCode, availableActions: evidence ? ['done'] : ['retry', 'cancel'], nextActions: evidence ? coverActions : [] };
    if (evidence) record.promotion = 'reconciled';
    await journal.save(record); report(); return record.snapshot;
  }
  if (record.promotion !== 'not_started') return reconcile();
  cancelled(signal);
  requireThat(plan.files.every(f => record.verified.includes(f.destination)), 'transfer_incomplete');
  await verifySources(plan); await verifyProtection(plan, transport);
  for (const file of plan.files) if (file.action !== 'preserve')
    requireThat((await observeFile(transport, file.destination)).sha256 === file.sha256, 'transfer_verification_failed');
  const current = await services.inspect(plan.target);
  cancelled(signal);
  requireThat(current.state === 'ready' && current.identity === plan.target.identity && current.transport === plan.target.transport, 'device_changed');
  const manual = current.transport !== 'ftp' || !current.capabilities.install || plan.requiresUr0Action;
  if (manual) {
    const nextActions: NonNullable<JobSnapshot['nextActions']> = [];
    if (current.transport === 'card') nextActions.push({ kind: 'eject_card' });
    if (plan.requiresUr0Action) nextActions.push({ kind: 'copy_prerequisite',
      from: plan.files.find(f => f.action === 'manual' && f.destination.endsWith('/libshacccg.suprx'))?.destination, to: 'ur0:data/libshacccg.suprx' });
    nextActions.push({ kind: 'install_vpk', from: plan.vpkDestination });
    nextActions.push(...coverActions);
    record.snapshot = { ...record.snapshot, phase: 'complete', completionMode: current.transport === 'card' ? 'ready_on_card' : 'ready_on_vita',
      vpkDestination: plan.vpkDestination, nextActions, errorCode: undefined, availableActions: ['done'] };
    await journal.save(record); await transport.close(); report(); return record.snapshot;
  }
  await verifyInstalledBaseline(plan, transport);
  for (const backup of record.backups) requireThat(await hashFile(backup.local) === backup.sha256, 'backup_changed');
  requireThat(plan.backupFiles.every(f => record.backups.some(b => b.path === f.path && b.sha256 === f.sha256)), 'backup_required');
  const path = current.adapter === 'companion' ? await stageCompanion(plan, transport, signal) : plan.vpkDestination;
  await assertBinding(record, transport); await verifyInstalledBaseline(plan, transport); await verifyProtection(plan, transport);
  cancelled(signal);
  record.promotion = 'requested'; record.snapshot.phase = 'installing'; record.snapshot.availableActions = [];
  await journal.save(record); report();
  try { await services.promote(current, path); } catch { /* The readback, not the reply, decides the outcome. */ }
  return reconcile();
}
