import { createHash } from 'node:crypto';
import { lstat, readFile, readdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { z } from 'zod';

const platforms = z.enum(['darwin-arm64', 'darwin-x64', 'win32-x64']);
const hash = (data: Uint8Array) => createHash('sha256').update(data).digest('hex');
const candidateSchema = z.object({ schemaVersion: z.literal(1), platform: platforms,
  acceptedForRelease: z.literal(false), catalogSha256: z.string().regex(/^[a-f0-9]{64}$/) }).strict();

export function verifyCandidateReceipt(value: unknown, platform: string, catalogSha256: string) {
  const receipt = candidateSchema.parse(value);
  if (receipt.platform !== platform || receipt.catalogSha256 !== catalogSha256) throw new Error('Native candidate catalog mismatch.');
  return receipt;
}

export async function packageWorkerSelection(root: string, platform: string, args: string[]) {
  platforms.parse(platform);
  if (!args.length) return { root: join(root, 'workers/bin', platform) };
  if (args.length !== 1 || args[0] !== '--native-candidate') throw new Error('Unknown package staging option.');
  const candidate = join(root, 'out/native-package-workers', platform);
  const receipt = verifyCandidateReceipt(JSON.parse(await readFile(join(candidate, 'native-candidate.json'), 'utf8')),
    platform, hash(await readFile(join(candidate, 'catalog.json'))));
  return { root: candidate, candidate: receipt };
}

export async function writeWorkerCatalog(output: string, sourceRoot: string, platform: string) {
  platforms.parse(platform);
  const exe = platform.startsWith('win32-') ? '.exe' : '';
  async function file(path: string) {
    const info = await lstat(path);
    if (!info.isFile() || info.isSymbolicLink()) throw new Error('Worker catalog requires regular files.');
    return hash(await readFile(path));
  }
  const entry = async (path: string) => ({ path, sha256: await file(join(output, path)) });
  const catalog = { schemaVersion: 1, platform, worker: await entry(`vita-prepare${exe}`),
    volumeWorker: await entry(`vita-volume/vita-volume${exe}`), movieWorker: await entry(`vita-movies${exe}`),
    sources: [] as { path: string; sha256: string }[] };
  async function collect(directory: string, relative = '') {
    for (const item of (await readdir(directory, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
      if (['bin', 'build', '__pycache__'].includes(item.name)) continue;
      const path = join(directory, item.name), name = relative + item.name;
      if (item.isDirectory()) await collect(path, name + '/');
      else catalog.sources.push({ path: name, sha256: await file(path) });
    }
  }
  await collect(sourceRoot);
  await writeFile(join(output, 'catalog.json'), JSON.stringify(catalog, null, 2) + '\n');
  return catalog;
}
