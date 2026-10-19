import { lstat, realpath, rename } from 'node:fs/promises';
import { join } from 'node:path';
import { checkedChild } from '../releases/import';
import { InstallerError, requireThat } from '../shared/errors';
import { hashFile } from '../shared/files';
import { FileIntegrityError, verifyFile } from '../shared/integrity';
import { verifyInventory } from './integrity';
import { inventoryTree } from './inventory';
import { runWorker, type VerifiedTool } from './runner';
import type { ArchiveLimits } from '../releases/archive';
import type { ReleaseManifest } from '../releases/manifest';
import inputCatalog from '../../resources/integrity/galaxy-rmge01-r0.json';
import cookedCatalog from '../../resources/integrity/galaxy-cooked-rmge01-r0.json';

export function checkGalaxyRecipe(manifest: ReleaseManifest) {
  requireThat(manifest.package?.runtimeSha256 === cookedCatalog.runtimeSha256, 'unsupported_preparation_build');
  const inputs = [...manifest.requiredInputs, ...manifest.optionalInputs];
  requireThat(manifest.requiredInputs.filter(input => input.kind === 'galaxy-disc').length === 1
    && !manifest.optionalInputs.some(input => input.kind === 'galaxy-disc')
    && inputs.every(input => ['galaxy-disc', 'shader-compiler', 'hd-pack'].includes(input.kind))
    && new Set(inputs.map(input => input.id)).size === inputs.length, 'invalid_preparation_recipe');
}

export function checkGalaxyInfo(info: string) {
  const header = info.split(/^Partition\s+\d+/m, 1)[0];
  requireThat(/^Game ID: RMGE01\s*$/m.test(header) && /^Disc 1, Revision 0\s*$/m.test(header), 'unsupported_game_revision');
}

export async function checkGalaxyExtraction(source: string, signal?: AbortSignal, hashes = false) {
  requireThat(!(await lstat(source)).isSymbolicLink(), 'unsafe_source');
  const canonical = await realpath(source);
  const hasSys = await lstat(join(canonical, 'sys')).catch(error => { if (error.code === 'ENOENT') return null; throw error; });
  const root = hasSys ? canonical : join(canonical, 'DATA');
  const stat = await lstat(root); requireThat(stat.isDirectory() && !stat.isSymbolicLink(), 'unsafe_source');
  const dol = inputCatalog.files.find(f => f.path === 'sys/main.dol')!;
  await verifyInventory(root, [dol], signal, true);
  // Disc extraction tools rewrite offsets. Accept only the two reviewed metadata variants.
  for (const file of inputCatalog.metadata) {
    const path = await checkedChild(root, file.path);
    await verifyFile(path, { bytes: file.bytes }, signal);
    if (!file.sha256s.includes(await hashFile(path, signal))) throw new FileIntegrityError(path, 'source_changed');
  }
  await verifyInventory(root, inputCatalog.files, signal, hashes);
  return root;
}

export async function prepareGalaxy(source: string, output: string, destination: string, limits: ArchiveLimits,
  worker: VerifiedTool, movieWorker: VerifiedTool, signal?: AbortSignal) {
  const root = await checkGalaxyExtraction(source, signal, true);
  const fst = await hashFile(join(root, 'sys/fst.bin'), signal);
  const result = await runWorker(worker, ['galaxy-cook', root, output], { cwd: source, signal });
  const receipt = JSON.parse(result.stdout);
  requireThat(receipt.prepared === true && receipt.movies === true && receipt.layoutVerified === true, 'galaxy_conversion_failed');
  for (const movie of cookedCatalog.files.filter(f => f.path.startsWith('files/MovieData/'))) {
    const path = await checkedChild(output, movie.path), optimized = `${path}.optimized`;
    await runWorker(movieWorker, [path, optimized, '55'], { cwd: output, signal });
    try { await verifyFile(optimized, movie, signal); }
    catch (error) {
      if (error instanceof FileIntegrityError) throw new InstallerError('optimization_verification_failed');
      throw error;
    }
    await rename(optimized, path);
  }
  await checkGalaxyExtraction(root, signal, true);
  requireThat(await hashFile(join(root, 'sys/fst.bin'), signal) === fst, 'source_changed');
  await verifyInventory(output, cookedCatalog.files, signal, true);
  await verifyFile(join(output, 'sys/fst.bin'), { sha256: fst }, signal);
  const files = await inventoryTree(output, destination, limits, signal);
  requireThat(files.length === cookedCatalog.files.length + 1, 'unexpected_conversion_output');
  return files;
}
