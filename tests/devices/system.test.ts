import { expect, it } from 'vitest';
import { build } from 'plist';
import { parseMacVolumes, parseWindowsVolumes } from '../../src/devices/system';
it('parses macOS physical external mounted media, ignoring virtual/internal/nonmounted volumes', () => {
  const common = { MountPoint: '/Volumes/VITA', VolumeName: 'VITA', DeviceIdentifier: 'disk9s1',
    VolumeUUID: 'card-uuid', Internal: false, VirtualOrPhysical: 'Physical', BusProtocol: 'Secure Digital' };
  const result = parseMacVolumes(build({ AllDisks: ['disk9s1', 'disk10s1', 'disk11s1', 'disk12s1'] }), {
    disk9s1: build(common), disk10s1: build({ ...common, Internal: true }),
    disk11s1: build({ ...common, VirtualOrPhysical: 'Virtual' }), disk12s1: build({ ...common, MountPoint: '' }),
  });
  expect(result).toHaveLength(1); expect(result[0]).toMatchObject({ id: 'card-uuid', root: '/Volumes/VITA', kind: 'card', role: 'unknown', vitaLayout: false });
});
it('parses a single Windows volume and does not mistake a label for Vita proof', () => {
  const raw = { UniqueId: 'volume-guid', DriveLetter: 'E', FileSystemLabel: 'PS VITA', DriveType: 'Removable', BusType: 'USB', SizeRemaining: 1024 };
  expect(parseWindowsVolumes(JSON.stringify(raw))).toMatchObject([{ id: 'volume-guid', root: 'E:\\', role: 'unknown', vitaLayout: false, freeBytes: 1024 }]);
  expect(parseWindowsVolumes(JSON.stringify([{ ...raw, BusType: 'NVMe' }, { ...raw, DriveLetter: null }]))).toEqual([]);
});
it('refuses malformed native metadata rather than guessing a mount point', () => {
  expect(() => parseWindowsVolumes('broken')).toThrow();
  expect(parseWindowsVolumes(JSON.stringify({ ...{ DriveLetter: '../' }, BusType: 'USB' }))).toEqual([]);
  expect(() => parseMacVolumes('not a plist', {})).toThrow();
});
