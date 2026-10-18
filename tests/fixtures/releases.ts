import { createHash } from 'node:crypto';
import { ZipFile } from 'yazl';
export const digest = (value: Uint8Array | string) => createHash('sha256').update(value).digest('hex');
export function sfo(title = 'MSCN00003') {
  const key = Buffer.from('TITLE_ID\0'), value = Buffer.from(`${title}\0`);
  const b = Buffer.alloc(36 + key.length + value.length);
  [0x46535000, 0x101, 36, 36 + key.length, 1].forEach((n, i) => b.writeUInt32LE(n, i * 4));
  b.writeUInt16LE(0x204, 22); b.writeUInt32LE(value.length, 24); b.writeUInt32LE(value.length, 28);
  key.copy(b, 36); value.copy(b, 36 + key.length); return b;
}
export async function zip(members: Record<string, Uint8Array | string>, forceZip64Format = false) {
  const archive = new ZipFile(), chunks: Buffer[] = [];
  const result = new Promise<Buffer>((resolve, reject) => {
    archive.outputStream.on('data', c => chunks.push(c));
    archive.outputStream.on('end', () => resolve(Buffer.concat(chunks)));
    archive.outputStream.on('error', reject);
  });
  for (const [name, value] of Object.entries(members)) archive.addBuffer(Buffer.from(value), name);
  archive.end({ forceZip64Format, comment: '' }); return result;
}
export const runtime = Buffer.from('SCE\0synthetic-runtime');
export function manifest(vpkHash = 'a'.repeat(64)) {
  return {
    schemaVersion: 1, releaseId: 'strikers-a8d4ad6', titleId: 'MSCN00003',
    displayName: 'Strikers', version: 'a8d4ad6', minInstallerVersion: '0.1.0',
    visibility: 'private', releaseState: 'ready', recipeId: 'strikers',
    package: { path: 'Strikers.vpk', sha256: vpkHash, runtimeSha256: digest(runtime) },
    requiredInputs: [{ id: 'game', label: 'Game file', kind: 'strikers-disc' }], optionalInputs: [],
    destinations: { data: 'ux0:data/mscharged/game' },
    protectedPaths: ['ux0:data/mscharged/np-data/NAND', 'ux0:data/mscharged/np-player.txt'],
    prerequisites: ['shader-compiler'],
    limits: { maxFiles: 10000, maxFileBytes: 17179869184, maxTotalBytes: 34359738368 }
  };
}
