import type { ImportedRelease, DeviceCandidate } from '../shared/contracts';
import type { Transport } from '../devices/transport';
import type { JobRecord } from './journal';
import type { ObservedFile } from './types';
import { planFingerprint } from './plan';
import { assertBinding } from './binding';
import { observeFile, readSmallFile } from './observe';
import { readSfo } from '../releases/sfo';
import { requireThat } from '../shared/errors';
import { authenticateManifest, type TrustStore } from '../releases/trust';
import { digest } from '../shared/files';

export function trustedUpdateReceipt(record: JobRecord, trust: TrustStore) {
  if (!record.plan.manifestEvidence) return trust.fingerprints.has(record.plan.manifestSha256);
  const encoded = record.plan.manifestEvidence;
  if (encoded.length > 1400000) return false;
  const raw = Buffer.from(encoded, 'base64');
  if (raw.toString('base64') !== encoded || digest(raw) !== record.plan.manifestSha256) return false;
  try {
    const manifest = authenticateManifest(raw, trust);
    return manifest.releaseState === 'ready' && manifest.titleId === record.plan.titleId
      && manifest.package?.sha256 === record.plan.package.originalSha256
      && manifest.package?.runtimeSha256 === record.plan.package.runtimeSha256;
  } catch { return false; }
}

export function compatibleAppUpdate(release: ImportedRelease, record: JobRecord, device: DeviceCandidate) {
  return record.promotion === 'reconciled' && record.snapshot.phase === 'complete'
    && record.snapshot.completionMode === 'installed' && record.plan.titleId === release.manifest.titleId
    && record.plan.target.identity === device.identity && record.plan.target.transport === device.transport
    && record.plan.fingerprint === planFingerprint(record.plan)
    && (record.plan.manifestSha256 === release.manifestSha256 || release.manifest.appUpdateFrom?.includes(record.plan.manifestSha256) === true);
}

export async function verifyAppUpdate(release: ImportedRelease, record: JobRecord, transport: Transport,
  signal?: AbortSignal): Promise<ObservedFile[]> {
  requireThat(compatibleAppUpdate(release, record, transport.device), 'incompatible_update');
  await assertBinding(record, transport, signal);
  const app = `ux0:app/${release.manifest.titleId}`;
  requireThat(readSfo(await readSmallFile(transport, `${app}/sce_sys/param.sfo`, 65536, signal)).titleId === release.manifest.titleId
    && (await observeFile(transport, `${app}/eboot.bin`, signal)).sha256 === record.plan.package.runtimeSha256, 'installed_app_changed');
  // Optional ROM libraries are not a prerequisite for updating the emulator.
  if (release.manifest.recipeId === 'vitadhoc') return [];
  const data = `${release.manifest.destinations.data}/`;
  const files = record.plan.retainedData ?? record.plan.files.filter(file => file.destination.startsWith(data) && file.action !== 'preserve')
    .map(file => ({ path: file.destination, bytes: file.bytes, sha256: file.sha256 }));
  requireThat(files.length > 0 && files.every(file => file.path.startsWith(data)), 'incompatible_update');
  for (const file of files) {
    const present = await transport.stat(file.path);
    requireThat(present?.kind === 'file' && present.bytes === file.bytes, 'update_data_changed');
    requireThat((await observeFile(transport, file.path, signal)).sha256 === file.sha256, 'update_data_changed');
  }
  return structuredClone(files);
}
