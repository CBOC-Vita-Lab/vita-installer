import { lstat, open, realpath, rename } from 'node:fs/promises';
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
import type { PreparationProgress } from '../shared/contracts';
import inputCatalog from '../../resources/integrity/galaxy-rmge01-r0.json';
import cookedCatalog from '../../resources/integrity/galaxy-cooked-rmge01-r0.json';
import movieCatalog from '../../resources/integrity/galaxy-movies-q55-v1.json';

export function isInstalledGalaxyMovie(manifest: ReleaseManifest,
  source: { destination: string; bytes: number; sha256: string }, existing: { bytes: number; sha256: string }) {
  if (manifest.recipeId !== 'galaxy' || manifest.package?.runtimeSha256 !== cookedCatalog.runtimeSha256
    || !source.destination.startsWith(`${manifest.destinations.data}/`)) return false;
  const path = source.destination.slice(manifest.destinations.data.length + 1);
  const candidate = movieCatalog.files.find(file => file.path === path);
  const installed = cookedCatalog.files.find(file => file.path === path);
  return !!candidate && !!installed && candidate.sha256 === source.sha256 && candidate.bytes === source.bytes
    && installed.sha256 === existing.sha256 && installed.bytes === existing.bytes;
}

export function checkedGalaxyOutput(movieWorker: VerifiedTool) {
  requireThat(movieCatalog.runtimeSha256 === cookedCatalog.runtimeSha256
    && movieCatalog.quality === 55 && movieCatalog.workerSha256s.includes(movieWorker.sha256), 'unsupported_movie_recipe');
  const originalMovies = cookedCatalog.files.filter(file => file.path.startsWith('files/MovieData/'));
  requireThat(movieCatalog.files.length === originalMovies.length
    && new Set(movieCatalog.files.map(file => file.path)).size === originalMovies.length
    && movieCatalog.files.every(file => originalMovies.some(original => original.path === file.path)), 'invalid_preparation_recipe');
  return cookedCatalog.files.map(file => movieCatalog.files.find(movie => movie.path === file.path) ?? file);
}

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
  worker: VerifiedTool, movieWorker: VerifiedTool, signal?: AbortSignal, onProgress: (progress: PreparationProgress) => void = () => {}) {
  const expected = checkedGalaxyOutput(movieWorker);
  onProgress({ phase: 'checking', completed: 0, unit: 'files' });
  const root = await checkGalaxyExtraction(source, signal, true);
  const fst = await hashFile(join(root, 'sys/fst.bin'), signal);
  onProgress({ phase: 'converting', completed: 0, unit: 'files' });
  let cooked = 0;
  const result = await runWorker(worker, ['galaxy-cook', root, output], { cwd: source, signal, onStderrLine: line => {
    if (!line.startsWith('CBOC_PROGRESS ')) return;
    let progress; try { progress = JSON.parse(line.slice(14)); } catch { return; }
    if (progress && Number.isSafeInteger(progress.completed) && Number.isSafeInteger(progress.total)
      && progress.completed >= cooked && progress.completed <= progress.total && progress.total > 0 && progress.total <= limits.maxFiles) {
      cooked = progress.completed; onProgress({ phase: 'converting', completed: cooked, total: progress.total, unit: 'files' });
    }
  } });
  const receipt = JSON.parse(result.stdout);
  requireThat(receipt.prepared === true && receipt.movies === true && receipt.layoutVerified === true, 'galaxy_conversion_failed');
  const movies = expected.filter(f => f.path.startsWith('files/MovieData/'));
  for (const [index, movie] of movies.entries()) {
    const path = await checkedChild(output, movie.path), optimized = `${path}.optimized`;
    const handle = await open(path, 'r'), header = Buffer.alloc(24);
    let total: number | undefined;
    try {
      const read = await handle.read(header, 0, header.length, 0);
      const frames = header.readUInt32LE(0x14);
      if (read.bytesRead === header.length && header.subarray(0, 4).equals(Buffer.from('THP\0')) && frames > 0 && frames <= 10000000) total = frames;
    } finally { await handle.close(); }
    let completed = 0;
    const report = () => onProgress({ phase: 'optimizing', completed, total, unit: 'frames', item: index + 1, items: movies.length });
    report();
    await runWorker(movieWorker, [path, optimized, '55'], { cwd: output, signal, onStderrLine: line => {
      const match = /^(\d+) frames, [\d.]+ -> [\d.]+ KB\/frame$/.exec(line);
      if (match && total !== undefined && Number(match[1]) > completed && Number(match[1]) <= total) { completed = Number(match[1]); report(); }
    } });
    try { await verifyFile(optimized, movie, signal); }
    catch (error) {
      if (error instanceof FileIntegrityError) throw new InstallerError('optimization_verification_failed');
      throw error;
    }
    await rename(optimized, path);
    if (total !== undefined) { completed = total; report(); }
  }
  onProgress({ phase: 'checking', completed: 0, unit: 'files' });
  await checkGalaxyExtraction(root, signal, true);
  requireThat(await hashFile(join(root, 'sys/fst.bin'), signal) === fst, 'source_changed');
  await verifyInventory(output, expected, signal, true,
    (completed, total) => onProgress({ phase: 'hashing', completed, total, unit: 'bytes' }));
  await verifyFile(join(output, 'sys/fst.bin'), { sha256: fst }, signal);
  onProgress({ phase: 'hashing', completed: 0, unit: 'files' });
  const files = await inventoryTree(output, destination, limits, signal);
  requireThat(files.length === expected.length + 1, 'unexpected_conversion_output');
  return files;
}
