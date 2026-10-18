import type { ImportedRelease, InputCheck, PreparationResult } from '../shared/contracts';
import type { ReleaseManifest } from '../releases/manifest';
import type { VerifiedTool } from './runner';
import { runWorker } from './runner';
import { lstat, mkdtemp, readdir, realpath, rm } from 'node:fs/promises';
import { basename, join, parse } from 'node:path';
import { homedir, tmpdir } from 'node:os';
import { checkedChild, extractArchive } from '../releases/import';
import { cancelled, InstallerError, requireThat } from '../shared/errors';
import { digest, hashFile } from '../shared/files';
import { inventoryTree } from './inventory';

export const STRIKERS_DOL = '8f811628ee6646c60620a9c599ef08ed929d2eccc98a779c211847f1500f81fd';
export const INFECTED_BOOT = '952d4bb2c08209f4cbc0020d95c4cc601e9944cad5c7ade4c1d060fe8daab1bc';
export const SHADER_COMPILER = '188adb8c34c3983d63244c32d981fd7b518a8dcfd200f5886233fface6b5fa40';
export const GBA_BIOS = 'fd2547724b505f487e6dcb29ec2ecff3af35a841a77ab2e85fd87350abd36570';
const ROM = /\.(gba|gb|gbc)$/i;
export interface InputSelections { roots: string[]; chosen: Record<string, string[]> }
export interface PreparationTools { nod?: VerifiedTool; dataWorker?: VerifiedTool }
export interface PreparationProgress { phase: 'checking' | 'extracting' | 'hashing'; files: number }
type Input = ReleaseManifest['requiredInputs'][number];

export function checkStrikersInfo(info: string) {
  const header = info.split(/^Partition\s+\d+/m, 1)[0];
  requireThat(/^Game ID: R4QE01\s*$/m.test(header) && /^Disc 1, Revision 1\s*$/m.test(header), 'unsupported_game_revision');
}

export async function checkStrikersExtraction(root: string, signal?: AbortSignal) {
  const canonical = await realpath(root);
  requireThat(!(await lstat(root)).isSymbolicLink(), 'unsafe_source');
  const dol = await checkedChild(canonical, 'sys/main.dol');
  requireThat(await hashFile(dol, signal) === STRIKERS_DOL, 'unsupported_game_revision');
  await checkedChild(canonical, 'sys/fst.bin');
  for (const path of [join(canonical, 'files'), join(canonical, 'files', 'Art')]) {
    const stat = await lstat(path); requireThat(stat.isDirectory() && !stat.isSymbolicLink(), 'incomplete_extraction');
  }
}

async function discover(roots: string[], signal?: AbortSignal) {
  requireThat(roots.length <= 8, 'input_root_limit');
  const files: string[] = [], extractions: string[] = []; let entries = 0;
  async function scan(path: string, depth: number) {
    cancelled(signal); const stat = await lstat(path);
    if (stat.isSymbolicLink()) return;
    if (stat.isFile()) { files.push(path); return; }
    requireThat(stat.isDirectory(), 'unsafe_source');
    const items = await readdir(path, { withFileTypes: true });
    entries += items.length; requireThat(entries <= 10000, 'input_scan_limit');
    if (items.some(e => e.name === 'sys' && e.isDirectory()) && items.some(e => e.name === 'files' && e.isDirectory())) {
      extractions.push(path); return;
    }
    for (const entry of items) {
      if (entry.isSymbolicLink() || entry.name.startsWith('.') || entry.name === 'node_modules') continue;
      if (entry.isFile()) files.push(join(path, entry.name));
      else if (entry.isDirectory() && depth < 4) await scan(join(path, entry.name), depth + 1);
    }
  }
  for (const root of roots) {
    requireThat(!(await lstat(root)).isSymbolicLink(), 'unsafe_source');
    const canonical = await realpath(root);
    requireThat(canonical !== parse(canonical).root && canonical !== homedir(), 'input_scope_too_broad');
    await scan(canonical, 0);
  }
  return { files: [...new Set(files)], extractions: [...new Set(extractions)] };
}

async function inspectOne(input: Input, source: string, tools: PreparationTools, signal?: AbortSignal): Promise<'ready' | 'unverified'> {
  const stat = await lstat(source); requireThat(!stat.isSymbolicLink(), 'unsafe_source');
  switch (input.kind) {
    case 'strikers-disc':
      if (stat.isDirectory()) { await checkStrikersExtraction(source, signal); return 'ready'; }
      requireThat(stat.isFile() && tools.nod, 'preparation_tool_missing');
      checkStrikersInfo((await runWorker(tools.nod, ['info', source], { cwd: await realpath(tmpdir()), signal, timeoutMs: 120000 })).stdout);
      return 'unverified'; // The executable is checked after full validated extraction.
    case 'infected-disc': {
      requireThat(stat.isFile() && tools.dataWorker, 'preparation_tool_missing');
      const info = JSON.parse((await runWorker(tools.dataWorker, ['infected-inspect', source], { cwd: await realpath(tmpdir()), signal, timeoutMs: 120000 })).stdout);
      requireThat(info.bootSha256 === INFECTED_BOOT, 'unsupported_game_revision'); return 'ready';
    }
    case 'shader-compiler': requireThat(await hashFile(source, signal) === SHADER_COMPILER, 'unsupported_shader_compiler'); return 'ready';
    case 'gba-bios': requireThat(stat.size === 16384 && await hashFile(source, signal) === GBA_BIOS, 'unsupported_bios'); return 'ready';
    case 'roms': requireThat(stat.isDirectory() || (stat.isFile() && ROM.test(source)), 'unsupported_rom'); return 'ready';
    case 'galaxy-disc': throw new InstallerError('release_held');
    case 'hd-pack': throw new InstallerError('optional_pack_not_reviewed');
    default: throw new InstallerError('unsupported_input');
  }
}

export async function inspectInputs(release: ImportedRelease, selections: InputSelections,
  tools: PreparationTools = {}, signal?: AbortSignal): Promise<InputCheck[]> {
  cancelled(signal);
  if (release.manifest.releaseState === 'held' || release.manifest.recipeId === 'galaxy')
    return [{ id: 'release', label: release.manifest.displayName, required: true, status: 'invalid', message: 'This release is not ready to install.' }];
  const found = await discover(selections.roots, signal), checks: InputCheck[] = [];
  for (const input of [...release.manifest.requiredInputs, ...release.manifest.optionalInputs]) {
    const required = release.manifest.requiredInputs.includes(input), explicit = selections.chosen[input.id];
    if (!required && !explicit?.length) continue;
    let candidates = explicit;
    if (!candidates?.length) {
      candidates = input.kind === 'strikers-disc' ? [...found.extractions, ...found.files.filter(p => /\.(iso|rvz)$/i.test(p))]
        : input.kind === 'infected-disc' ? found.files.filter(p => /\.iso$/i.test(p))
        : input.kind === 'shader-compiler' ? found.files.filter(p => basename(p).toLowerCase() === 'libshacccg.suprx') : [];
    }
    const base = { id: input.id, label: input.label, required };
    if (!candidates.length) { checks.push({ ...base, status: 'missing' }); continue; }
    if (candidates.length > 1 && input.kind !== 'roms') { checks.push({ ...base, status: 'invalid', message: 'Choose one game file.' }); continue; }
    try {
      let status: 'ready' | 'unverified' = 'ready';
      for (const path of candidates) {
        requireThat(!(await lstat(path)).isSymbolicLink(), 'unsafe_source');
        status = await inspectOne(input, await realpath(path), tools, signal);
      }
      checks.push({ ...base, status, source: candidates[0] });
    } catch (error) {
      cancelled(signal);
      checks.push({ ...base, status: 'invalid', source: candidates[0], message: error instanceof InstallerError ? error.code : 'Could not read this file.' });
    }
  }
  return checks;
}

const recipeFingerprint = (release: ImportedRelease) => digest(`preparation-v1\n${release.manifestSha256}`);
export async function prepareInputs(release: ImportedRelease, selections: InputSelections,
  onProgress: (progress: PreparationProgress) => void, signal?: AbortSignal,
  tools: PreparationTools = {}): Promise<PreparationResult> {
  cancelled(signal);
  requireThat(release.manifest.releaseState === 'ready' && release.manifest.recipeId !== 'galaxy', 'release_held');
  onProgress({ phase: 'checking', files: 0 });
  const workspace = await mkdtemp(join(await realpath(tmpdir()), 'vita-prepare-'));
  const result: PreparationResult = { releaseId: release.manifest.releaseId, inputFingerprints: {}, recipeFingerprint: recipeFingerprint(release),
    toolFingerprints: Object.fromEntries(Object.entries(tools).filter(([, tool]) => !!tool).map(([name, tool]) => [name, tool!.sha256])), files: [], workspace };
  try {
    selections = { roots: [...selections.roots], chosen: structuredClone(selections.chosen) };
    const budget = { entries: 0, bytes: 0 };
    for (const [id, paths] of Object.entries(selections.chosen)) {
      const role = [...release.manifest.requiredInputs, ...release.manifest.optionalInputs].find(i => i.id === id);
      requireThat(role, 'unsupported_input');
      for (let index = 0; index < paths.length; index++) {
        const archive = paths[index]; if (!/\.zip$/i.test(archive)) continue;
        const before = await hashFile(archive, signal);
        const extracted = await extractArchive(archive, workspace, release.manifest.limits, signal, budget);
        requireThat(await hashFile(archive, signal) === before, 'source_changed');
        result.inputFingerprints[`archive:${id}:${index}`] = before;
        if (role.kind === 'roms') paths[index] = extracted;
        else {
          const found = await discover([extracted], signal);
          const candidates = role.kind === 'strikers-disc' ? [...found.extractions, ...found.files.filter(p => /\.(iso|rvz)$/i.test(p))]
            : role.kind === 'infected-disc' ? found.files.filter(p => /\.iso$/i.test(p)) : [];
          requireThat(candidates.length === 1, candidates.length ? 'ambiguous_input_archive' : 'no_supported_input');
          paths[index] = candidates[0];
        }
      }
    }
    const checks = await inspectInputs(release, selections, tools, signal);
    requireThat(checks.every(c => c.status === 'ready' || c.status === 'unverified'), 'invalid_input');
    for (const check of checks) {
      const input = [...release.manifest.requiredInputs, ...release.manifest.optionalInputs].find(i => i.id === check.id)!;
      const paths = selections.chosen[check.id]?.length ? selections.chosen[check.id] : [check.source!];
      for (const sourcePath of paths) {
        requireThat(!(await lstat(sourcePath)).isSymbolicLink(), 'unsafe_source');
        const source = await realpath(sourcePath), stat = await lstat(source); cancelled(signal);
        const data = release.manifest.destinations.data;
        const beforeHash = stat.isFile() ? await hashFile(source, signal) : undefined;
        await inspectOne(input, source, tools, signal);
        const start = result.files.length;
        if (input.kind === 'strikers-disc') {
          let extracted = source;
          if (stat.isFile()) {
            requireThat(tools.nod, 'preparation_tool_missing');
            extracted = join(workspace, 'strikers'); onProgress({ phase: 'extracting', files: 0 });
            await runWorker(tools.nod, ['extract', '--quiet', '--validate', '--partition', 'data', source, extracted], { cwd: workspace, signal });
          }
          await checkStrikersExtraction(extracted, signal);
          for (const dir of ['sys', 'files']) result.files.push(...await inventoryTree(join(extracted, dir), `${data}/${dir}`, release.manifest.limits, signal));
        } else if (input.kind === 'roms' && stat.isDirectory()) {
          result.files.push(...await inventoryTree(source, data, release.manifest.limits, signal,
            p => p.split('/').length <= 3 && !p.split('/').some(n => n.startsWith('.')) && ROM.test(p)));
        } else {
          requireThat(stat.isFile(), 'invalid_input');
          const destination = input.kind === 'infected-disc' ? `${data}/Infected.iso`
            : input.kind === 'shader-compiler' ? 'ur0:data/libshacccg.suprx'
            : input.kind === 'gba-bios' ? 'ux0:data/GBAdhoc/gba_bios.bin' : `${data}/${basename(source)}`;
          result.files.push({ source, destination, bytes: stat.size, sha256: beforeHash! });
        }
        if (beforeHash) requireThat(await hashFile(source, signal) === beforeHash, 'source_changed');
        const fingerprint = beforeHash ?? digest(JSON.stringify(result.files.slice(start).map(f => [f.destination, f.bytes, f.sha256])));
        result.inputFingerprints[`${check.id}:${paths.indexOf(sourcePath)}`] = fingerprint;
        onProgress({ phase: 'hashing', files: result.files.length });
      }
    }
    const seen = new Set<string>(); let bytes = 0;
    for (const file of result.files) {
      const key = file.destination.normalize('NFKC').toLowerCase(); requireThat(!seen.has(key), 'input_path_collision'); seen.add(key);
      bytes += file.bytes; requireThat(file.bytes <= release.manifest.limits.maxFileBytes, 'input_size_limit');
    }
    requireThat(bytes <= release.manifest.limits.maxTotalBytes && result.files.length <= release.manifest.limits.maxFiles, 'input_size_limit');
    cancelled(signal); return result;
  } catch (error) { await rm(workspace, { recursive: true, force: true }); throw error; }
}

export async function verifyPreparation(result: PreparationResult, release: ImportedRelease): Promise<boolean> {
  if (result.releaseId !== release.manifest.releaseId || result.recipeFingerprint !== recipeFingerprint(release)) return false;
  try { for (const file of result.files) if (await hashFile(file.source) !== file.sha256 || (await lstat(file.source)).size !== file.bytes) return false; }
  catch { return false; }
  return true;
}
