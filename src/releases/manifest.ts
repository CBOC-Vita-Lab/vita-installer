import { createHash } from 'node:crypto';
import { z } from 'zod';
import { hashFile, safeRelative } from '../shared/files';
import { requireThat } from '../shared/errors';
import { walkZip, PACKAGE_LIMITS } from './archive';
import { readSfo } from './sfo';

export const PROFILES = {
  strikers: { title: 'MSCN00003', data: 'ux0:data/mscharged/game', repo: 'strikers-charged-vita' },
  infected: { title: 'INFC00001', data: 'ux0:data/infected', repo: 'infected-vita' },
  vitadhoc: { title: 'GBAD00001', data: 'ux0:data/GBAdhoc/roms', repo: 'vitadhoc' },
  galaxy: { title: 'SMGV00001', data: 'ux0:data/smg/game', repo: 'galaxy-vita' }
} as const;
const hash = z.string().regex(/^[a-f0-9]{64}$/);
const input = z.object({ id: z.string().regex(/^[a-z][a-z0-9-]{0,39}$/), label: z.string().min(1).max(100),
  kind: z.enum(['strikers-disc', 'infected-disc', 'galaxy-disc', 'roms', 'gba-bios', 'shader-compiler', 'hd-pack']) }).strict();
const devicePath = z.string().refine(p => p.startsWith('ux0:data/') && safeRelative(p.slice(4)));
export const manifestSchema = z.object({
  schemaVersion: z.literal(1), releaseId: z.string().regex(/^[a-z0-9][a-z0-9._-]{0,79}$/),
  titleId: z.enum(['MSCN00003', 'INFC00001', 'GBAD00001', 'SMGV00001']),
  displayName: z.string().min(1).max(100), version: z.string().min(1).max(40), minInstallerVersion: z.literal('0.1.0'),
  visibility: z.enum(['private', 'public']), releaseState: z.enum(['ready', 'held']),
  recipeId: z.enum(['strikers', 'infected', 'vitadhoc', 'galaxy']),
  package: z.object({ path: z.string().refine(safeRelative), sha256: hash, runtimeSha256: hash }).strict().nullable(),
  requiredInputs: z.array(input).max(10), optionalInputs: z.array(input).max(10),
  destinations: z.object({ data: devicePath }).strict(), protectedPaths: z.array(devicePath).max(30),
  prerequisites: z.array(z.enum(['shader-compiler'])).max(1),
  appUpdateFrom: z.array(hash).max(100).optional(),
  limits: z.object({ maxFiles: z.number().int().min(1).max(100000), maxFileBytes: z.number().int().min(1).max(64 * 1024 ** 3), maxTotalBytes: z.number().int().min(1).max(128 * 1024 ** 3) }).strict()
}).strict().superRefine((m, ctx) => {
  const p = PROFILES[m.recipeId];
  if (p.title !== m.titleId || p.data !== m.destinations.data || (!m.package && m.releaseState !== 'held'))
    ctx.addIssue({ code: 'custom', message: 'Profile mismatch' });
});
export type ReleaseManifest = z.infer<typeof manifestSchema>;
export interface PackageMember { path: string; bytes: number; sha256: string }
export interface PackageInventory { sha256: string; runtimeSha256: string; titleId: string; members: PackageMember[] }
export async function verifyVpk(path: string, manifest: ReleaseManifest, signal?: AbortSignal): Promise<PackageInventory> {
  const pkg = manifest.package; requireThat(pkg, 'release_held');
  requireThat(await hashFile(path, signal) === pkg.sha256, 'package_hash_mismatch');
  const members: PackageMember[] = []; let titleId = '', runtimeSha256 = '';
  await walkZip(path, PACKAGE_LIMITS, async (entry, stream) => {
    const hash = createHash('sha256'), chunks: Buffer[] = []; let size = 0;
    if (entry.fileName === 'sce_sys/param.sfo') requireThat(entry.uncompressedSize <= 65536, 'invalid_sfo');
    for await (const chunk of stream) {
      size += chunk.length; requireThat(size <= entry.uncompressedSize, 'archive_size_mismatch'); hash.update(chunk);
      if (entry.fileName === 'sce_sys/param.sfo') chunks.push(chunk);
    }
    requireThat(size === entry.uncompressedSize, 'archive_size_mismatch');
    const sha256 = hash.digest('hex'); members.push({ path: entry.fileName, bytes: size, sha256 });
    if (entry.fileName === 'sce_sys/param.sfo') titleId = readSfo(Buffer.concat(chunks)).titleId;
    if (entry.fileName === 'eboot.bin') runtimeSha256 = sha256;
  }, signal);
  requireThat(titleId === manifest.titleId, 'package_title_mismatch');
  requireThat(runtimeSha256 === pkg.runtimeSha256, 'package_runtime_mismatch');
  requireThat(await hashFile(path, signal) === pkg.sha256, 'source_changed');
  return { sha256: pkg.sha256, runtimeSha256, titleId, members };
}
