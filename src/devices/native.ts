import type { DeviceCandidate } from '../shared/contracts';
import type { DiscoveryIO } from './discover';
import { probeAddresses } from './discover';
import { systemInterfaces, systemNeighbors, systemVolumes } from './system';
import { probeFtp } from './probe';
import { profileService } from './profile';
import { cancelled, requireThat } from '../shared/errors';

export const nativeDiscoveryIO: DiscoveryIO = { interfaces: systemInterfaces, volumes: systemVolumes, neighbors: systemNeighbors, probe: probeFtp };

export async function inspectDevice(candidate: DeviceCandidate, signal?: AbortSignal,
  io: DiscoveryIO = nativeDiscoveryIO): Promise<DeviceCandidate> {
  cancelled(signal);
  const volumes = await io.volumes();
  cancelled(signal);
  const offline = { ...candidate, state: 'offline' as const,
    capabilities: { transfer: false, install: false, ur0: false } };
  if (candidate.transport !== 'ftp') {
    const volume = volumes.find(v => `volume:${v.id}` === candidate.identity && v.root === candidate.root);
    if (!volume?.readable || !volume.vitaLayout) return offline;
    return { ...candidate, freeBytes: volume.freeBytes, capabilities: { transfer: true, install: false, ur0: false },
      state: candidate.storageRole === 'ux0' ? 'ready' : 'needs_confirmation' };
  }
  if (volumes.some(v => v.kind === 'usb' && v.readable && v.vitaLayout))
    return { ...offline, evidence: ['End USB mode before connecting by FTP'] };
  requireThat(!!candidate.host && Number.isInteger(candidate.port) && candidate.port! >= 1 && candidate.port! <= 65535, 'invalid_ftp_endpoint');
  const faces = await io.interfaces();
  cancelled(signal);
  if (!probeAddresses(faces, [candidate.host!]).includes(candidate.host!)) return offline;
  const deadline = AbortSignal.timeout(3000);
  const evidence = await io.probe(candidate.host!, candidate.port!, signal ? AbortSignal.any([signal, deadline]) : deadline);
  cancelled(signal);
  return evidence ? profileService(evidence) ?? offline : offline;
}
