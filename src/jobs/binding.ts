import type { Transport } from '../devices/transport';
import { stageRoot } from '../devices/transport';
import { digest } from '../shared/files';
import { requireThat } from '../shared/errors';
import type { JobRecord, FileJournal } from './journal';
import { observeFile } from './observe';

export function markerBytes(record: JobRecord) {
  return Buffer.from(JSON.stringify({ schemaVersion: 1, jobId: record.plan.jobId, token: record.token,
    fingerprint: record.plan.fingerprint, identity: record.plan.target.identity }));
}
export async function assertBinding(record: JobRecord, transport: Transport, signal?: AbortSignal) {
  requireThat(await transport.identity() === record.plan.target.identity, 'device_changed');
  const bytes = markerBytes(record), root = stageRoot(record.plan.jobId);
  for (const name of ['marker.json', 'marker.pending']) {
    const path = `${root}/${name}`, existing = await transport.stat(path);
    if (existing && existing.kind === 'file' && existing.bytes === bytes.length
      && (await observeFile(transport, path, signal)).sha256 === digest(bytes)) return path;
  }
  throw Object.assign(new Error('missing_job_marker'), { code: 'missing_job_marker' });
}
export async function ensureBinding(record: JobRecord, transport: Transport, journal: FileJournal, signal?: AbortSignal) {
  const root = stageRoot(record.plan.jobId), pending = `${root}/marker.pending`, target = `${root}/marker.json`;
  if (record.markerStarted) {
    const found = await assertBinding(record, transport, signal);
    if (found === pending) { requireThat(await transport.stat(target) === null, 'marker_conflict'); await transport.renameOwned(pending, target, signal); }
    return;
  }
  requireThat(await transport.identity() === record.plan.target.identity, 'device_changed');
  requireThat(await transport.stat(root) === null, 'staging_conflict');
  record.markerStarted = true; await journal.save(record);
  await transport.writeOwnedPartial(pending, (async function* () { yield markerBytes(record); })(), signal);
  await assertBinding(record, transport, signal); await transport.renameOwned(pending, target, signal);
  requireThat(await assertBinding(record, transport, signal) === target, 'marker_verification_failed');
}
