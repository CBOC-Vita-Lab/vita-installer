import { afterEach, expect, it, vi } from 'vitest';
import { mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { checkStrikersInfo, inspectInputs, prepareInputs, verifyPreparation } from '../../src/preparation/inputs';
import * as runner from '../../src/preparation/runner';
import type { ImportedRelease, PreparationResult } from '../../src/shared/contracts';
import { manifestSchema, PROFILES } from '../../src/releases/manifest';
import { digest, manifest, zip } from '../fixtures/releases';
const roots: string[] = [];
afterEach(async () => { vi.restoreAllMocks(); for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function directory() { const root = await realpath(await mkdtemp(join(tmpdir(), 'input-test-'))); roots.push(root); return root; }
function release(recipe: keyof typeof PROFILES = 'strikers'): ImportedRelease {
  const m = manifestSchema.parse({ ...manifest(), recipeId: recipe, titleId: PROFILES[recipe].title,
    destinations: { data: PROFILES[recipe].data }, requiredInputs: recipe === 'vitadhoc' ? []
      : [{ id: 'game', label: 'Game file', kind: `${recipe}-disc` }],
    optionalInputs: recipe === 'vitadhoc' ? [{ id: 'roms', label: 'Your games', kind: 'roms' }] : [],
    prerequisites: recipe === 'vitadhoc' ? [] : ['shader-compiler'] });
  return { manifest: m, manifestSha256: digest(JSON.stringify(m)), sourceRoot: '', vpkPath: null, inventory: null };
}
async function track(result: Promise<PreparationResult>) { const value = await result; roots.push(value.workspace); return value; }
it('prompts for missing required inputs but never makes optional ROMs necessary', async () => {
  expect(await inspectInputs(release(), { roots: [], chosen: {} })).toMatchObject([{ id: 'game', status: 'missing', required: true }]);
  expect(await inspectInputs(release('vitadhoc'), { roots: [], chosen: {} })).toEqual([]);
  const result = await track(prepareInputs(release('vitadhoc'), { roots: [], chosen: {} }, () => {}));
  expect(result.files).toEqual([]);
});
it('only discovers within selected roots and does not follow symlinks elsewhere', async () => {
  const root = await directory(), outside = await directory();
  await mkdir(join(outside, 'sys')); await mkdir(join(outside, 'files', 'Art'), { recursive: true });
  await writeFile(join(outside, 'sys', 'main.dol'), 'wrong game');
  await symlink(outside, join(root, 'game'));
  expect(await inspectInputs(release(), { roots: [root], chosen: {} })).toMatchObject([{ id: 'game', status: 'missing' }]);
});
it('rejects wrong-content extractions even with the correct file and folder names', async () => {
  const root = await directory(); await mkdir(join(root, 'sys')); await mkdir(join(root, 'files', 'Art'), { recursive: true });
  await writeFile(join(root, 'sys', 'main.dol'), 'wrong executable'); await writeFile(join(root, 'sys', 'fst.bin'), 'fst');
  expect(await inspectInputs(release(), { roots: [root], chosen: {} })).toMatchObject([{ id: 'game', status: 'invalid' }]);
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [root] } }, () => {})).rejects.toMatchObject({ code: 'invalid_input' });
});
it('reports ambiguity rather than silently selecting the first image', async () => {
  const root = await directory(); await writeFile(join(root, 'first.iso'), 'a'); await writeFile(join(root, 'second.rvz'), 'b');
  expect(await inspectInputs(release(), { roots: [root], chosen: {} })).toMatchObject([{ id: 'game', status: 'invalid', message: 'Choose one game file.' }]);
});
it('copies optional ROMs with exact extensions and relative folders, excluding adjacent saves and configs', async () => {
  const root = await directory(); await mkdir(join(root, 'GB')); await writeFile(join(root, 'Game.gba'), 'rom-gba');
  await writeFile(join(root, 'Game.sav'), 'private save'); await writeFile(join(root, 'vita.ini'), 'private settings');
  await writeFile(join(root, 'GB', 'Game.gb'), 'rom-gb'); await writeFile(join(root, '._Game.gba'), 'metadata');
  const result = await track(prepareInputs(release('vitadhoc'), { roots: [], chosen: { roms: [root] } }, () => {}));
  expect(result.files.map(f => f.destination).sort()).toEqual(['ux0:data/GBAdhoc/roms/GB/Game.gb', 'ux0:data/GBAdhoc/roms/Game.gba']);
  expect(await readFile(join(root, 'Game.sav'), 'utf8')).toBe('private save');
  expect(await verifyPreparation(result, release('vitadhoc'))).toBe(true);
  await writeFile(join(root, 'Game.gba'), 'modified ROM'); expect(await verifyPreparation(result, release('vitadhoc'))).toBe(false);
});
it('holds Galaxy regardless of an unverified ready flag or supplied files', async () => {
  await expect(prepareInputs(release('galaxy'), { roots: [], chosen: { game: ['anything'] } }, () => {})).rejects.toMatchObject({ code: 'release_held' });
});
it('stops before scanning or preparing when cancelled', async () => {
  const abort = new AbortController(); abort.abort();
  await expect(inspectInputs(release(), { roots: [], chosen: {} }, {}, abort.signal)).rejects.toMatchObject({ code: 'cancelled' });
  await expect(prepareInputs(release('vitadhoc'), { roots: [], chosen: {} }, () => {}, abort.signal)).rejects.toMatchObject({ code: 'cancelled' });
});
it('rejects incomplete same-name shader modules instead of trusting their filename', async () => {
  const r = release('vitadhoc'); r.manifest.requiredInputs = [{ id: 'compiler', label: 'Shader compiler', kind: 'shader-compiler' }];
  const root = await directory(); await writeFile(join(root, 'libshacccg.suprx'), 'wrong module');
  expect(await inspectInputs(r, { roots: [root], chosen: {} })).toMatchObject([{ id: 'compiler', status: 'invalid' }]);
});
it('validates an optional BIOS independently and never demands it for the built-in open BIOS', async () => {
  const r = release('vitadhoc');
  r.manifest.optionalInputs.push({ id: 'bios', label: 'Optional GBA BIOS', kind: 'gba-bios' });
  expect(await inspectInputs(r, { roots: [], chosen: {} })).toEqual([]);
  const root = await directory(), path = join(root, 'gba_bios.bin'); await writeFile(path, Buffer.alloc(16384));
  expect(await inspectInputs(r, { roots: [], chosen: { bios: [path] } })).toMatchObject([{ id: 'bios', required: false, status: 'invalid' }]);
});
it('reads the outer Strikers revision rather than a different partition header', () => {
  expect(() => checkStrikersInfo('Game ID: R4QE01\nDisc 1, Revision 1\nPartition 0:\nDisc 1, Revision 0')).not.toThrow();
  expect(() => checkStrikersInfo('Game ID: R4QE01\nDisc 1, Revision 0\nPartition 0:\nDisc 1, Revision 1')).toThrow('unsupported_game_revision');
  expect(() => checkStrikersInfo('Game ID: R4QP01\nDisc 1, Revision 1')).toThrow('unsupported_game_revision');
});
it('does not accept an explicitly selected symlink as an ordinary ROM', async () => {
  const root = await directory(), path = join(root, 'original.gba'), link = join(root, 'link.gba');
  await writeFile(path, 'game'); await symlink(path, link);
  expect(await inspectInputs(release('vitadhoc'), { roots: [], chosen: { roms: [link] } })).toMatchObject([{ status: 'invalid' }]);
});
it('binds disc validation to the same bytes that are prepared', async () => {
  const root = await directory(), path = join(root, 'game.iso'); await writeFile(path, 'original disc');
  let inspections = 0;
  vi.spyOn(runner, 'runWorker').mockImplementation(async (_tool, args) => {
    expect(args).toEqual(['infected-inspect', path]);
    if (++inspections === 2) await writeFile(path, 'changed disc');
    return { stdout: JSON.stringify({ bootSha256: '952d4bb2c08209f4cbc0020d95c4cc601e9944cad5c7ade4c1d060fe8daab1bc', bootBytes: 100 }), stderr: '' };
  });
  await expect(track(prepareInputs(release('infected'), { roots: [], chosen: { game: [path] } }, () => {}, undefined,
    { dataWorker: { path: 'bundled-reader', sha256: digest('worker') } }))).rejects.toMatchObject({ code: 'source_changed' });
});
it('removes partial worker output when extraction fails', async () => {
  const root = await directory(), path = join(root, 'game.rvz'); await writeFile(path, 'disc'); let staged = '';
  vi.spyOn(runner, 'runWorker').mockImplementation(async (_tool, args) => {
    if (args[0] === 'info') return { stdout: 'Game ID: R4QE01\nDisc 1, Revision 1\n', stderr: '' };
    expect(args.slice(0, 5)).toEqual(['extract', '--quiet', '--validate', '--partition', 'data']);
    staged = args[6]; await mkdir(staged); await writeFile(join(staged, 'partial'), 'not ready'); throw new Error('worker failed');
  });
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [path] } }, () => {}, undefined,
    { nod: { path: 'bundled-nod', sha256: digest('worker') } })).rejects.toThrow('worker failed');
  expect(staged).not.toBe(''); await expect(readFile(join(staged, 'partial'))).rejects.toMatchObject({ code: 'ENOENT' });
  expect(await readFile(path, 'utf8')).toBe('disc');
});
it('prepares a selected ZIP in its own workspace, preserving the archive and adjacent saves', async () => {
  const root = await directory(), path = join(root, 'games.zip');
  const original = await zip({ 'Game.gbc': 'game', 'Game.gbc.sav': 'private save' }); await writeFile(path, original);
  const result = await track(prepareInputs(release('vitadhoc'), { roots: [], chosen: { roms: [path] } }, () => {}));
  expect(result.files).toHaveLength(1); expect(result.files[0].destination).toBe('ux0:data/GBAdhoc/roms/Game.gbc');
  expect(result.files[0].source.startsWith(result.workspace + '/')).toBe(true);
  expect(await readFile(path)).toEqual(original);
  expect(result.inputFingerprints['archive:roms:0']).toBe(digest(original));
});
it('rejects ambiguous and over-budget game archives without executing bundled scripts', async () => {
  const root = await directory(), path = join(root, 'games.zip');
  await writeFile(path, await zip({ 'one.iso': 'one', 'two.rvz': 'two', 'install.sh': 'exit 0' }));
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [path] } }, () => {})).rejects.toMatchObject({ code: 'ambiguous_input_archive' });
  const r = release('vitadhoc'); r.manifest.limits.maxFileBytes = 2;
  await expect(prepareInputs(r, { roots: [], chosen: { roms: [path] } }, () => {})).rejects.toMatchObject({ code: 'archive_size_limit' });
});
