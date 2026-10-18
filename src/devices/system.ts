import type { LocalInterface, VolumeEvidence } from './discover';
import { parse as parsePlist } from 'plist';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { networkInterfaces } from 'node:os';
import { access, lstat, readdir, readFile, realpath, statfs } from 'node:fs/promises';
import { constants } from 'node:fs';
import { join } from 'node:path';
import { requireThat } from '../shared/errors';
import { readSfo } from '../releases/sfo';
import { checkedChild } from '../releases/import';

const exec = promisify(execFile);
const object = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
const array = (value: unknown): unknown[] => Array.isArray(value) ? value : value ? [value] : [];
const text = (value: unknown): string => typeof value === 'string' ? value : '';
export function parseMacVolumes(listing: string, details: Record<string, string>): VolumeEvidence[] {
  const list = object(parsePlist(listing)); requireThat(Array.isArray(list.AllDisks), 'invalid_volume_inventory');
  const result: VolumeEvidence[] = [];
  for (const id of list.AllDisks) {
    if (typeof id !== 'string' || !details[id]) continue;
    const info = object(parsePlist(details[id]));
    const root = text(info.MountPoint), protocol = text(info.BusProtocol);
    if (info.Internal !== false || info.VirtualOrPhysical !== 'Physical' || !['USB', 'Secure Digital', 'SD'].includes(protocol)
      || !root.startsWith('/Volumes/') || root.includes('\0')) continue;
    result.push({ id: text(info.VolumeUUID) || `unbound:${id}`, root, name: text(info.VolumeName) || 'Connected storage',
      kind: protocol === 'USB' ? 'usb' : 'card', readable: false, vitaLayout: false, role: 'unknown' });
  }
  return result;
}
export function parseWindowsVolumes(listing: string): VolumeEvidence[] {
  const result: VolumeEvidence[] = [];
  for (const value of array(JSON.parse(listing || '[]'))) {
    const info = object(value), drive = text(info.DriveLetter);
    if (!/^[A-Z]$/i.test(drive) || !['USB', 'SD', 'MMC'].includes(text(info.BusType))) continue;
    result.push({ id: text(info.UniqueId) || `unbound:${drive}`, root: `${drive.toUpperCase()}:\\`,
      name: text(info.FileSystemLabel) || 'Connected storage', kind: info.BusType === 'USB' ? 'usb' : 'card',
      readable: false, vitaLayout: false, role: 'unknown', freeBytes: typeof info.SizeRemaining === 'number' ? info.SizeRemaining : undefined });
  }
  return result;
}
async function command(path: string, args: string[]): Promise<string> {
  return (await exec(path, args, { timeout: 4000, maxBuffer: 1024 ** 2, windowsHide: true, encoding: 'utf8' })).stdout;
}
const powershell = (script: string) => command('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script]);

export async function systemVolumes(): Promise<VolumeEvidence[]> {
  let volumes: VolumeEvidence[] = [];
  if (process.platform === 'darwin') {
    const listing = await command('/usr/sbin/diskutil', ['list', '-plist', 'external', 'physical']);
    const ids = array(object(parsePlist(listing)).AllDisks), details: Record<string, string> = {};
    for (const id of ids.slice(0, 32)) if (typeof id === 'string' && /^disk\d+(s\d+)?$/.test(id))
      details[id] = await command('/usr/sbin/diskutil', ['info', '-plist', id]);
    volumes = parseMacVolumes(listing, details);
  } else if (process.platform === 'win32') {
    volumes = parseWindowsVolumes(await powershell('Get-Partition | ForEach-Object { $p = $_; $d = $p | Get-Disk; $v = $p | Get-Volume -ErrorAction SilentlyContinue; if ($v) { [pscustomobject]@{UniqueId=$v.UniqueId;DriveLetter=[string]$v.DriveLetter;FileSystemLabel=$v.FileSystemLabel;BusType=[string]$d.BusType;SizeRemaining=$v.SizeRemaining} } } | ConvertTo-Json -Compress'));
  }
  for (const volume of volumes) {
    try {
      const root = await realpath(volume.root);
      await access(root, constants.R_OK);
      for (const folder of ['app', 'data']) { const stat = await lstat(join(root, folder)); requireThat(stat.isDirectory() && !stat.isSymbolicLink(), 'not_vita_storage'); }
      const apps = await readdir(join(root, 'app'), { withFileTypes: true });
      for (const app of apps.filter(a => a.isDirectory() && /^[A-Z0-9]{9}$/.test(a.name)).slice(0, 16)) {
        try {
          const sfo = await checkedChild(root, `app/${app.name}/sce_sys/param.sfo`);
          requireThat((await lstat(sfo)).size <= 65536, 'invalid_sfo');
          if (readSfo(await readFile(sfo)).titleId === app.name) { volume.vitaLayout = true; break; }
        } catch { /* A directory with a similar name is not device evidence. */ }
      }
      volume.readable = true; volume.root = root;
      const storage = await statfs(root); volume.freeBytes = storage.bavail * storage.bsize;
    } catch { volume.readable = false; }
  }
  return volumes;
}

export async function systemInterfaces(): Promise<LocalInterface[]> {
  const result: LocalInterface[] = [];
  let physicalNames = new Set<string>();
  if (process.platform === 'win32') physicalNames = new Set(array(JSON.parse(await powershell('Get-NetAdapter -Physical | Where-Object Status -eq Up | Select-Object -ExpandProperty Name | ConvertTo-Json -Compress'))).map(text));
  for (const [name, faces] of Object.entries(networkInterfaces())) for (const face of faces ?? []) {
    if (face.family !== 'IPv4') continue;
    result.push({ name, address: face.address, netmask: face.netmask, internal: face.internal,
      physical: process.platform === 'darwin' ? /^en\d+$/.test(name) : physicalNames.has(name) });
  }
  return result;
}
export async function systemNeighbors(): Promise<string[]> {
  if (process.platform === 'win32') return array(JSON.parse(await powershell('Get-NetNeighbor -AddressFamily IPv4 | Select-Object -ExpandProperty IPAddress | ConvertTo-Json -Compress'))).map(text);
  if (process.platform === 'darwin') {
    // macOS arp has no structured-output mode; addresses are filtered against local interfaces next.
    const output = await command('/usr/sbin/arp', ['-an']);
    return [...output.matchAll(/\((\d{1,3}(?:\.\d{1,3}){3})\)\s+at/g)].map(m => m[1]);
  }
  return [];
}
