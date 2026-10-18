import { expect, it } from 'vitest';
import { discoverDevices, probeAddresses, type DiscoveryIO, type LocalInterface, type VolumeEvidence } from '../../src/devices/discover';
import { profileService, selectDevice, type ServiceEvidence } from '../../src/devices/profile';
import { inspectDevice } from '../../src/devices/native';
const wifi: LocalInterface = { name: 'en0', address: '192.168.2.3', netmask: '255.255.255.0', internal: false, physical: true };
const service = (host = '192.168.2.7'): ServiceEvidence => ({ host, port: 1337, banner: 'VitaShell', ux0: true, ur0: true, features: [], vitaShellInstalled: true });
function io(overrides: Partial<DiscoveryIO> = {}): DiscoveryIO {
  return { interfaces: async () => [wifi], volumes: async () => [], neighbors: async () => [], probe: async () => null, ...overrides };
}
async function collect(options: Parameters<typeof discoverDevices>[0], signal?: AbortSignal) {
  const results = []; for await (const device of discoverDevices(options, signal)) results.push(device); return results;
}
const card: VolumeEvidence = { id: 'card-one', root: '/Volumes/CARD', name: 'CARD', kind: 'card', readable: true, vitaLayout: true, role: 'unknown' };
it('requires service capabilities, not a greeting or installed VitaShell files', () => {
  expect(profileService({ ...service(), ux0: false })).toBeNull();
  for (const evidence of [service(), { ...service(), banner: 'Companion', companion: { version: 'Vita Companion 1.0.0', help: 'help\nversion' } }])
    expect(profileService(evidence)).toMatchObject({ adapter: 'ftp', capabilities: { install: false, transfer: true } });
});
it('recognizes only an advertised package or extracted-directory installation capability', () => {
  expect(profileService({ ...service(), features: ['PROM'] })).toMatchObject({ adapter: 'vitashell', capabilities: { install: true } });
  expect(profileService({ ...service(), companion: { version: 'Vita Companion 1.0.0', help: 'help\tDisplay help\nversion\tDisplay version\npromote\tPromote an extracted app directory\n' } }))
    .toMatchObject({ adapter: 'companion', capabilities: { install: true } });
  expect(profileService({ ...service(), companion: { version: 'unknown server', help: 'promote blah' } }))
    .toMatchObject({ adapter: 'ftp', capabilities: { install: false } });
});
it('prefers_verified_automatic_ftp for the same device only', () => {
  const manual = profileService({ ...service(), identity: 'device-one' })!;
  const automatic = profileService({ ...service(), port: 2121, identity: 'device-one', features: ['PROM'] })!;
  expect(selectDevice([manual, automatic])).toBe(automatic);
});
it('does_not_choose_another_device_for_automation', () => {
  const first = profileService({ ...service(), identity: 'first' })!, other = profileService({ ...service('192.168.2.8'), identity: 'second', features: ['PROM'] })!;
  expect(selectDevice([first, other], 'first')).toBe(first); expect(selectDevice([first, other])).toBeUndefined();
  expect(selectDevice([other], 'first')).toBeUndefined();
});
it('never_probes_public_vpn_or_wide_ranges or a saved address outside the current local interface', () => {
  const addresses = probeAddresses([wifi, { ...wifi, name: 'utun2', address: '10.0.0.2', physical: false },
    { ...wifi, address: '8.8.8.2' }, { ...wifi, address: '10.2.3.4', netmask: '255.255.0.0' }], ['8.8.8.8', '192.168.9.5', '192.168.2.10']);
  expect(addresses[0]).toBe('192.168.2.10'); expect(addresses.length).toBeLessThanOrEqual(256);
  expect(addresses.every(a => a.startsWith('192.168.2.'))).toBe(true);
  expect(addresses).not.toContain('192.168.2.0'); expect(addresses).not.toContain('192.168.2.255'); expect(addresses).not.toContain(wifi.address);
});
it('exposes configured cards, not blank media, misleading labels or unreadable cards', async () => {
  const results = await collect({ io: io({ volumes: async () => [card, { ...card, id: 'blank', vitaLayout: false, name: 'PS VITA' }, { ...card, id: 'locked', readable: false }] }) });
  expect(results).toHaveLength(1); expect(results[0]).toMatchObject({ transport: 'card', storageRole: 'unknown', state: 'needs_confirmation', capabilities: { install: false } });
});
it('does not infer ux0 role for a mounted USB Vita', async () => {
  const results = await collect({ io: io({ volumes: async () => [{ ...card, kind: 'usb' }] }) });
  expect(results[0]).toMatchObject({ transport: 'usb', storageRole: 'unknown', state: 'needs_confirmation' });
});
it('active_usb_blocks_ftp_filesystem_checks without unmounting anything', async () => {
  let probes = 0; const statuses: string[] = [];
  await collect({ io: io({ volumes: async () => [{ ...card, kind: 'usb' }], probe: async () => { probes++; return service(); } }), onStatus: s => statuses.push(s) });
  expect(probes).toBe(0); expect(statuses).toContain('usb_active');
});
it('keeps multiple cards and an unrelated network Vita separate', async () => {
  const results = await collect({ io: io({ volumes: async () => [card, { ...card, id: 'second', root: '/Volumes/SECOND' }],
    probe: async host => host === '192.168.2.7' ? service(host) : null }) });
  expect(results).toHaveLength(3); expect(selectDevice(results)).toBeUndefined();
});
it('bounds concurrency, uses one FTP port and gives neighbor/saved addresses priority', async () => {
  let active = 0, peak = 0; const seen: string[] = [], ports = new Set<number>();
  await collect({ savedHosts: ['192.168.2.12'], io: io({ neighbors: async () => ['192.168.2.11'], probe: async (host, port) => {
    active++; peak = Math.max(peak, active); seen.push(host); ports.add(port); await new Promise(r => setTimeout(r, 1)); active--; return null;
  } }) });
  expect(peak).toBeLessThanOrEqual(16); expect(seen.length).toBeLessThanOrEqual(256); expect(ports).toEqual(new Set([1337]));
  expect(new Set(seen.slice(0, 2))).toEqual(new Set(['192.168.2.11', '192.168.2.12']));
});
it('discovery_timeout_exposes_retry_and_usb and cancel_stops_queued_probes', async () => {
  const statuses: string[] = []; let probes = 0;
  const network = io({ probe: async (_host, _port, signal) => { probes++; await new Promise(r => signal.addEventListener('abort', r, { once: true })); return null; } });
  expect(await collect({ io: network, timeoutMs: 20, onStatus: s => statuses.push(s) })).toEqual([]);
  expect(probes).toBeLessThanOrEqual(16); expect(statuses).toContain('timeout');
  const controller = new AbortController(); controller.abort(); probes = 0;
  await collect({ io: network, onStatus: s => statuses.push(s) }, controller.signal);
  expect(probes).toBe(0); expect(statuses).toContain('cancelled');
});
it('handles an offline saved endpoint and permission denial without inventing a connection', async () => {
  const statuses: string[] = [];
  const results = await collect({ savedHosts: ['192.168.2.30'], io: io({ probe: async () => { throw Object.assign(new Error('denied'), { code: 'EACCES' }); } }), onStatus: s => statuses.push(s) });
  expect(results).toEqual([]); expect(statuses).toContain('network_denied');
});
it('rechecks a selected endpoint, revokes lost promotion capability and respects active USB', async () => {
  const candidate = profileService({ ...service(), features: ['PROM'] })!;
  const refreshed = await inspectDevice(candidate, undefined, io({ probe: async () => service() }));
  expect(refreshed.capabilities.install).toBe(false);
  expect(await inspectDevice(candidate, undefined, io())).toMatchObject({ state: 'offline' });
  let probes = 0;
  expect(await inspectDevice(candidate, undefined, io({ volumes: async () => [{ ...card, kind: 'usb' }], probe: async () => { probes++; return service(); } }))).toMatchObject({ state: 'offline' });
  expect(probes).toBe(0);
});
it('does not reuse a confirmed mount path after another card appears there', async () => {
  const devices = await collect({ io: io({ volumes: async () => [card] }) });
  const selected = { ...devices[0], storageRole: 'ux0' as const };
  expect(await inspectDevice(selected, undefined, io({ volumes: async () => [card] }))).toMatchObject({ state: 'ready' });
  expect(await inspectDevice(selected, undefined, io({ volumes: async () => [{ ...card, id: 'other-card' }] }))).toMatchObject({ state: 'offline' });
});
