import type { DeviceCandidate } from '../shared/contracts';
import type { ServiceEvidence } from './profile';
import { profileService } from './profile';
import { requireThat } from '../shared/errors';
import { isIPv4 } from 'node:net';
export interface LocalInterface { name: string; address: string; netmask: string; internal: boolean; physical: boolean }
export interface VolumeEvidence { id: string; root: string; name: string; kind: 'usb' | 'card'; readable: boolean; vitaLayout: boolean; role: 'ux0' | 'unknown'; freeBytes?: number }
export interface DiscoveryIO {
  interfaces(): Promise<LocalInterface[]>; volumes(): Promise<VolumeEvidence[]>; neighbors(): Promise<string[]>;
  probe(host: string, port: number, signal: AbortSignal): Promise<ServiceEvidence | null>;
}
export interface DiscoveryOptions {
  io: DiscoveryIO; savedHosts?: string[]; manual?: { host: string; port: number };
  onStatus?: (state: 'complete' | 'timeout' | 'network_denied' | 'usb_active' | 'cancelled') => void;
  timeoutMs?: number;
}
const number = (ip: string) => ip.split('.').reduce((n, part) => n * 256 + Number(part), 0) >>> 0;
const address = (n: number) => [24, 16, 8, 0].map(shift => (n >>> shift) & 255).join('.');
function privateAddress(ip: string) {
  if (!isIPv4(ip)) return false;
  const [a, b] = ip.split('.').map(Number);
  return a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254);
}
export function probeAddresses(interfaces: LocalInterface[], preferred: string[]): string[] {
  const allowed = new Set<string>();
  for (const face of interfaces) {
    if (!face.physical || face.internal || /^(utun|tun|tap|wg|tailscale|zt|ppp|ipsec|vbox|vmnet|bridge|docker|br-)/i.test(face.name)
      || !privateAddress(face.address) || !isIPv4(face.netmask)) continue;
    const mask = number(face.netmask), span = (~mask >>> 0) + 1;
    if (span < 4 || span > 256 || (span & (span - 1)) !== 0) continue;
    const base = (number(face.address) & mask) >>> 0;
    for (let i = 1; i < span - 1; i++) { const host = address(base + i); if (host !== face.address) allowed.add(host); }
  }
  return [...new Set([...preferred.filter(h => allowed.has(h)), ...allowed])].slice(0, 256);
}

function volumeCandidate(volume: VolumeEvidence): DeviceCandidate | null {
  if (!volume.readable || !volume.vitaLayout) return null;
  return { id: `volume:${volume.id}`, identity: `volume:${volume.id}`, root: volume.root, transport: volume.kind,
    label: volume.name, storageRole: volume.role, adapter: 'volume', capabilities: { transfer: true, install: false, ur0: false },
    evidence: ['Vita storage layout found', 'Storage role requires confirmation'],
    state: volume.role === 'ux0' ? 'ready' : 'needs_confirmation', freeBytes: volume.freeBytes };
}

async function bounded<T>(operation: Promise<T>, signal: AbortSignal, fallback: T): Promise<T> {
  if (signal.aborted) { void operation.catch(() => {}); return fallback; }
  let cancel: () => void = () => {};
  const stopped = new Promise<T>(resolve => { cancel = () => resolve(fallback); signal.addEventListener('abort', cancel, { once: true }); });
  try { return await Promise.race([operation, stopped]); } finally { signal.removeEventListener('abort', cancel); }
}

export async function* discoverDevices(options: DiscoveryOptions, signal?: AbortSignal): AsyncIterable<DeviceCandidate> {
  if (signal?.aborted) { options.onStatus?.('cancelled'); return; }
  const controller = new AbortController(); let timedOut = false, denied = false;
  const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, Math.min(15000, options.timeoutMs ?? 15000));
  const cancel = () => controller.abort(); signal?.addEventListener('abort', cancel, { once: true });
  try {
    const volumes = await bounded(options.io.volumes(), controller.signal, []);
    for (const volume of volumes) { const candidate = volumeCandidate(volume); if (candidate) yield candidate; }
    if (volumes.some(v => v.kind === 'usb' && v.readable && v.vitaLayout)) { options.onStatus?.('usb_active'); return; }
    const faces = await bounded(options.io.interfaces(), controller.signal, []);
    const neighbors = await bounded(options.io.neighbors(), controller.signal, []);
    let hosts = probeAddresses(faces, [...(options.manual ? [options.manual.host] : []), ...neighbors, ...(options.savedHosts ?? [])]);
    const port = options.manual?.port ?? 1337;
    requireThat(Number.isInteger(port) && port >= 1 && port <= 65535, 'invalid_ftp_port');
    if (options.manual) { requireThat(hosts.includes(options.manual.host), 'not_on_local_subnet'); hosts = [options.manual.host]; }
    for (let start = 0; start < hosts.length && !controller.signal.aborted; start += 16) {
      const batch = await Promise.all(hosts.slice(start, start + 16).map(async host => {
        if (controller.signal.aborted) return null;
        try { return await bounded(options.io.probe(host, port, controller.signal), controller.signal, null); }
        catch (error) {
          if (['EACCES', 'EPERM'].includes((error as NodeJS.ErrnoException).code ?? '')) { denied = true; controller.abort(); }
          return null;
        }
      }));
      for (const evidence of batch) if (evidence) { const candidate = profileService(evidence); if (candidate) yield candidate; }
    }
    options.onStatus?.(signal?.aborted ? 'cancelled' : denied ? 'network_denied' : timedOut ? 'timeout' : 'complete');
  } finally { clearTimeout(timeout); signal?.removeEventListener('abort', cancel); controller.abort(); }
}
