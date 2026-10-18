import type { DeviceCandidate } from '../shared/contracts';
export interface ServiceEvidence {
  host: string; port: number; banner: string; ux0: boolean; ur0: boolean;
  features: string[]; vitaShellInstalled?: boolean;
  companion?: { version: string; help: string }; identity?: string;
}
export function profileService(evidence: ServiceEvidence): DeviceCandidate | null {
  if (!evidence.ux0) return null;
  const companion = evidence.companion && /^Vita ?Companion\s+v?\d+\.\d+/i.test(evidence.companion.version.trim())
    && /^promote\s+Promote an extracted app directory\s*$/m.test(evidence.companion.help);
  const promotesVpk = evidence.features.some(f => /^PROM(?:\s|$)/i.test(f.trim()));
  const adapter = companion ? 'companion' : promotesVpk ? 'vitashell' : 'ftp';
  return { id: `ftp:${evidence.host}:${evidence.port}`, transport: 'ftp', label: `Vita (${evidence.host})`,
    identity: evidence.identity ?? `unbound:${evidence.host}:${evidence.port}`, host: evidence.host, port: evidence.port,
    storageRole: 'ux0', adapter, capabilities: { transfer: true, install: !!companion || promotesVpk, ur0: evidence.ur0 },
    evidence: ['ux0 namespace readable', ...(companion ? ['Companion version and promote help verified']
      : promotesVpk ? ['PROM advertised by active FTP service'] : ['Remote installation not established'])], state: 'ready' };
}
export function selectDevice(candidates: DeviceCandidate[], selectedIdentity?: string): DeviceCandidate | undefined {
  const available = candidates.filter(c => c.state !== 'offline');
  const identity = selectedIdentity ?? (new Set(available.map(c => c.identity)).size === 1 ? available[0]?.identity : undefined);
  if (!identity) return undefined;
  return available.filter(c => c.identity === identity).sort((a, b) =>
    Number(b.transport === 'ftp' && b.capabilities.install) - Number(a.transport === 'ftp' && a.capabilities.install)
    || Number(b.state === 'ready') - Number(a.state === 'ready'))[0];
}
