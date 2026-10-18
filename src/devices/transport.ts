import type { DeviceCandidate } from '../shared/contracts';
import type { VolumeEvidence } from './discover';
import { openVolume } from './usb';
import { openFtp } from './ftp';
import { requireThat } from '../shared/errors';
import { safeRelative } from '../shared/files';
export interface RemoteEntry { name: string; kind: 'file' | 'directory'; bytes: number }
export interface WriteScope { jobId: string; destinations: string[]; replacements?: { destination: string; sha256: string }[] }
export interface Transport {
  device: DeviceCandidate;
  identity(): Promise<string>;
  stat(path: string): Promise<RemoteEntry | null>;
  list(path: string): Promise<RemoteEntry[]>;
  read(path: string, signal?: AbortSignal): AsyncIterable<Uint8Array>;
  writeOwnedPartial(path: string, data: AsyncIterable<Uint8Array>, signal?: AbortSignal): Promise<void>;
  renameOwned(source: string, destination: string, signal?: AbortSignal): Promise<void>;
  moveAside(source: string, backup: string, expectedSha256: string, signal?: AbortSignal): Promise<void>;
  removeOwned(path: string): Promise<void>;
  freeSpace(): Promise<number | undefined>;
  promoteVpk?(): Promise<'accepted' | 'rejected' | 'unknown'>;
  close(): Promise<void>;
}
export interface VolumeWorker { path: string; sha256: string; args?: string[] }
export interface TransportOptions {
  scope?: WriteScope; volumes?: () => Promise<VolumeEvidence[]>; timeoutMs?: number; volumeWorker?: VolumeWorker;
  promotionTimeoutMs?: number;
}
export async function openTransport(device: DeviceCandidate, options: TransportOptions = {}): Promise<Transport> {
  requireThat(device.state === 'ready' && device.capabilities.transfer, 'device_not_ready');
  requireThat(device.storageRole === 'ux0', 'storage_role_unknown');
  if (options.scope) {
    requireThat(/^[a-f0-9]{32}$/.test(options.scope.jobId), 'invalid_job_id');
    options.scope.destinations.forEach(devicePath);
  }
  return device.transport === 'ftp' ? openFtp(device, options) : openVolume(device, options);
}
export function stageRoot(jobId: string): string { return `ux0:data/CBOC-Installer/jobs/${jobId}`; }
export function devicePath(path: string): { mount: 'ux0' | 'ur0'; relative: string } {
  requireThat(/^(ux0|ur0):/.test(path) && safeRelative(path.slice(4)) && !/[;*?"<>|]/.test(path), 'unsafe_device_path');
  return { mount: path.slice(0, 3) as 'ux0' | 'ur0', relative: path.slice(4) };
}
export function ownedPath(path: string, scope?: WriteScope) {
  devicePath(path); requireThat(scope, 'write_not_authorized');
  requireThat(path.startsWith(`${stageRoot(scope.jobId)}/`), 'not_owned_path');
}
export function finalPath(path: string, scope?: WriteScope) {
  devicePath(path); requireThat(scope, 'write_not_authorized');
  requireThat(scope.destinations.includes(path) || path.startsWith(`${stageRoot(scope.jobId)}/`), 'write_not_authorized');
}
export function replacementPath(path: string, hash: string, scope?: WriteScope) {
  finalPath(path, scope);
  requireThat(scope?.replacements?.some(r => r.destination === path && r.sha256 === hash), 'replacement_not_authorized');
}
