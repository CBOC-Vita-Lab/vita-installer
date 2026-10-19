import { afterEach, expect, it, vi } from 'vitest';
import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { dirname, join, sep } from 'node:path';
import { tmpdir } from 'node:os';
import { inspectInputs, prepareInputs } from '../../src/preparation/inputs';
import * as runner from '../../src/preparation/runner';
import { manifestSchema } from '../../src/releases/manifest';
import { manifest } from '../fixtures/releases';
import type { ImportedRelease } from '../../src/shared/contracts';
import { errorMessage } from '../../src/ui/messages';

const fixtures = vi.hoisted(() => ({
  input: { 'sys/main.dol': 'galaxy executable', 'sys/boot.bin': 'RMGE01 header', 'sys/fst.bin': 'file table',
    'files/MovieData/opening.thp': 'movie source', 'files/AudioRes/music.ast': 'audio source' },
  output: { 'sys/fst.bin': 'file table', 'sys/StoryEvent.bcsv': 'cooked table',
    'files/MovieData/opening.thp': 'optimized movie', 'files/AudioRes/music.ast': 'cooked audio',
    'files/ObjectData/_bk00/model.arc': 'cooked model and texture' }
}));
vi.mock('../../resources/integrity/galaxy-rmge01-r0.json', async () => {
  const { createHash } = await import('node:crypto');
  const files = Object.entries(fixtures.input).map(([path, data]) => ({ path, bytes: Buffer.byteLength(data), sha256: createHash('sha256').update(data).digest('hex') }));
  return { default: { schemaVersion: 1, gameId: 'RMGE01', revision: 0,
    metadata: files.filter(f => ['sys/boot.bin', 'sys/fst.bin'].includes(f.path)).map(({ sha256, ...f }) => ({ ...f, sha256s: [sha256] })),
    files: files.filter(f => !['sys/boot.bin', 'sys/fst.bin'].includes(f.path)) } };
});
vi.mock('../../resources/integrity/galaxy-cooked-rmge01-r0.json', async () => {
  const { createHash } = await import('node:crypto');
  return { default: { schemaVersion: 1, gameId: 'RMGE01', revision: 0, runtimeSha256: '9'.repeat(64),
    files: Object.entries(fixtures.output).filter(([path]) => path !== 'sys/fst.bin')
      .map(([path, data]) => ({ path, bytes: Buffer.byteLength(data), sha256: createHash('sha256').update(data).digest('hex') })) } };
});
vi.mock('../../resources/integrity/galaxy-movies-q55-v1.json', async () => {
  const { createHash } = await import('node:crypto');
  return { default: { schemaVersion: 1, recipeId: 'galaxy-q55-portable-f32-v1', runtimeSha256: '9'.repeat(64),
    quality: 55, workerSha256s: ['d'.repeat(64)],
    files: Object.entries(fixtures.output).filter(([path]) => path.startsWith('files/MovieData/'))
      .map(([path, data]) => ({ path, bytes: Buffer.byteLength(data), sha256: createHash('sha256').update(data).digest('hex') })) } };
});
const roots: string[] = [];
afterEach(async () => { vi.restoreAllMocks(); for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function directory() { const path = await realpath(await mkdtemp(join(tmpdir(), 'galaxy-test-'))); roots.push(path); return path; }
async function tree(root: string, files: Record<string, string>) {
  for (const [name, data] of Object.entries(files)) { const path = join(root, name); await mkdir(dirname(path), { recursive: true }); await writeFile(path, data); }
}
function release(): ImportedRelease {
  const m = manifestSchema.parse({ ...manifest(), recipeId: 'galaxy', titleId: 'SMGV00001', destinations: { data: 'ux0:data/smg/game' },
    requiredInputs: [{ id: 'game', label: 'Galaxy USA Rev 0', kind: 'galaxy-disc' }] });
  m.package!.runtimeSha256 = '9'.repeat(64);
  return { manifest: m, manifestSha256: 'a'.repeat(64), sourceRoot: '', vpkPath: null, inventory: null };
}
const tools = { nod: { path: 'nod', sha256: 'b'.repeat(64) }, dataWorker: { path: 'worker', sha256: 'c'.repeat(64) },
  movieWorker: { path: 'movie-worker', sha256: 'd'.repeat(64) } };
it('allows a ready Galaxy release to ask for its game instead of remaining permanently held', async () => {
  expect(await inspectInputs(release(), { roots: [], chosen: {} }, tools)).toMatchObject([{ id: 'game', status: 'missing' }]);
  const held = release(); held.manifest.releaseState = 'held';
  await expect(prepareInputs(held, { roots: [], chosen: {} }, () => {}, undefined, tools)).rejects.toMatchObject({ code: 'release_held' });
});
it('validates the outer Galaxy region and revision before accepting a disc', async () => {
  const root = await directory(), iso = join(root, 'game.iso'); await writeFile(iso, 'disc');
  const worker = vi.spyOn(runner, 'runWorker');
  for (const [info, expected] of [
    ['Game ID: RMGE01\nDisc 1, Revision 0\nPartition 0\nDisc 1, Revision 1', 'unverified'],
    ['Game ID: RMGP01\nDisc 1, Revision 0', 'invalid'],
    ['Game ID: RMGE01\nDisc 1, Revision 1\nPartition 0\nDisc 1, Revision 0', 'invalid']
  ]) {
    worker.mockImplementation(async (_tool, args) => { expect(args).toEqual(['info', iso]); return { stdout: info, stderr: '' }; });
    expect(await inspectInputs(release(), { roots: [root], chosen: {} }, tools)).toMatchObject([{ id: 'game', status: expected }]);
  }
});
it('rejects missing movies during preflight and same-length corruption before cooking', async () => {
  const root = await directory(); await tree(root, fixtures.input);
  expect(await inspectInputs(release(), { roots: [], chosen: { game: [root] } }, tools)).toMatchObject([{ id: 'game', status: 'ready' }]);
  await rm(join(root, 'files/MovieData/opening.thp'));
  expect(await inspectInputs(release(), { roots: [], chosen: { game: [root] } }, tools)).toMatchObject([{ id: 'game', status: 'missing', problemFile: 'opening.thp' }]);
  await tree(root, fixtures.input); await writeFile(join(root, 'files/MovieData/opening.thp'), 'wrong source');
  const worker = vi.spyOn(runner, 'runWorker');
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [root] } }, () => {}, undefined, tools)).rejects.toMatchObject({ code: 'source_changed' });
  expect(worker).not.toHaveBeenCalled();
});
it('cooks the entire game into a separate workspace with movies and audio, never saves or settings', async () => {
  const root = await directory(); await tree(join(root, 'DATA'), fixtures.input); await writeFile(join(root, 'settings.ini'), 'private');
  let moviePasses = 0;
  vi.spyOn(runner, 'runWorker').mockImplementation(async (tool, args) => {
    if (tool.path === 'movie-worker') {
      expect(args[0].endsWith(sep + join('files', 'MovieData', 'opening.thp'))).toBe(true); expect(args.slice(2)).toEqual(['55']);
      expect(await readFile(args[0], 'utf8')).toBe('cooked movie');
      await writeFile(args[1], 'optimized movie'); moviePasses++; return { stdout: '', stderr: '' };
    }
    expect(tool).toEqual(tools.dataWorker); expect(args[0]).toBe('galaxy-cook'); expect(args[1]).toBe(join(root, 'DATA'));
    await tree(args[2], { ...fixtures.output, 'files/MovieData/opening.thp': 'cooked movie' });
    return { stdout: '{"prepared":true,"movies":true,"layoutVerified":true}', stderr: '' };
  });
  const result = await prepareInputs(release(), { roots: [], chosen: { game: [root] } }, () => {}, undefined, tools); roots.push(result.workspace);
  expect(result.files.map(f => f.destination).sort()).toEqual([
    'ux0:data/smg/game/files/AudioRes/music.ast', 'ux0:data/smg/game/files/MovieData/opening.thp',
    'ux0:data/smg/game/files/ObjectData/_bk00/model.arc',
    'ux0:data/smg/game/sys/StoryEvent.bcsv', 'ux0:data/smg/game/sys/fst.bin'
  ]);
  expect(result.files.every(f => f.source.startsWith(result.workspace + sep))).toBe(true);
  expect(result.inputFingerprints['game:0']).toMatch(/^[a-f0-9]{64}$/);
  expect(moviePasses).toBe(1);
  expect(await readFile(join(root, 'settings.ini'), 'utf8')).toBe('private');
});
it('does not fall back to unoptimized movies when the optimizer is missing', async () => {
  const root = await directory(); await tree(root, fixtures.input);
  const worker = vi.spyOn(runner, 'runWorker');
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [root] } }, () => {}, undefined,
    { ...tools, movieWorker: undefined })).rejects.toMatchObject({ code: 'preparation_tool_missing' });
  expect(worker).not.toHaveBeenCalled();
});
it('rejects an unreviewed optimizer before cooking or accepting its output', async () => {
  const root = await directory(); await tree(root, fixtures.input);
  const worker = vi.spyOn(runner, 'runWorker');
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [root] } }, () => {}, undefined,
    { ...tools, movieWorker: { ...tools.movieWorker, sha256: 'e'.repeat(64) } }))
    .rejects.toMatchObject({ code: 'unsupported_movie_recipe' });
  expect(worker).not.toHaveBeenCalled();
});
it('reports cooker and movie work while running, then hashes output before preparation completes', async () => {
  const root = await directory(); await tree(root, fixtures.input);
  const progress: import('../../src/preparation/inputs').PreparationProgress[] = [];
  vi.spyOn(runner, 'runWorker').mockImplementation(async (tool, args, options) => {
    if (tool.path === 'movie-worker') {
      options.onStderrLine?.('500 frames, 40.0 -> 22.0 KB/frame');
      expect(progress.at(-1)).toMatchObject({ phase: 'optimizing', completed: 500, total: 1000, unit: 'frames', item: 1, items: 1 });
      options.onStderrLine?.('999999 frames, 40.0 -> 22.0 KB/frame');
      expect(progress.at(-1)?.completed).toBe(500);
      await writeFile(args[1], 'optimized movie'); return { stdout: '', stderr: '' };
    }
    options.onStderrLine?.('CBOC_PROGRESS {"completed":2,"total":5}');
    expect(progress.at(-1)).toMatchObject({ phase: 'converting', completed: 2, total: 5, unit: 'files' });
    await tree(args[2], fixtures.output);
    const header = Buffer.alloc(48); header.write('THP\0'); header.writeUInt32LE(1000, 0x14);
    await writeFile(join(args[2], 'files/MovieData/opening.thp'), header);
    return { stdout: '{"prepared":true,"movies":true,"layoutVerified":true}', stderr: '' };
  });
  const result = await prepareInputs(release(), { roots: [], chosen: { game: [root] } }, p => progress.push(p), undefined, tools); roots.push(result.workspace);
  expect(progress.some(p => p.phase === 'hashing' && p.completed > 0)).toBe(true);
  expect(progress.some(p => p.phase === 'optimizing' && p.completed === 1000)).toBe(true);
});
it('never uses a preparation recipe belonging to a different game build', async () => {
  const root = await directory(); await tree(root, fixtures.input);
  const worker = vi.spyOn(runner, 'runWorker'), otherBuild = release();
  otherBuild.manifest.package!.runtimeSha256 = '8'.repeat(64);
  await expect(prepareInputs(otherBuild, { roots: [], chosen: { game: [root] } }, () => {}, undefined, tools))
    .rejects.toMatchObject({ code: 'unsupported_preparation_build' });
  expect(worker).not.toHaveBeenCalled();
});
it.each(['missing', 'optional', 'wrong-kind', 'duplicate'])('blocks a %s Galaxy game role before any preparation', async kind => {
  const invalid = release(), game = invalid.manifest.requiredInputs[0];
  if (kind === 'missing') invalid.manifest.requiredInputs = [];
  if (kind === 'optional') { invalid.manifest.requiredInputs = []; invalid.manifest.optionalInputs = [game]; }
  if (kind === 'wrong-kind') game.kind = 'roms';
  if (kind === 'duplicate') invalid.manifest.requiredInputs.push({ ...game, id: 'second-game' });
  const worker = vi.spyOn(runner, 'runWorker');
  await expect(prepareInputs(invalid, { roots: [], chosen: {} }, () => {}, undefined, {}))
    .rejects.toMatchObject({ code: 'invalid_preparation_recipe' });
  expect(worker).not.toHaveBeenCalled();
});
it('reports an optimization mismatch without asking for another copy of valid game data', async () => {
  const root = await directory(); await tree(root, fixtures.input);
  let staged = '';
  vi.spyOn(runner, 'runWorker').mockImplementation(async (tool, args) => {
    if (tool.path === 'movie-worker') { await writeFile(args[1], 'unoptimized mov'); return { stdout: '', stderr: '' }; }
    staged = args[2]; await tree(staged, fixtures.output);
    return { stdout: '{"prepared":true,"movies":true,"layoutVerified":true}', stderr: '' };
  });
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [root] } }, () => {}, undefined, tools))
    .rejects.toMatchObject({ code: 'optimization_verification_failed' });
  await expect(readFile(join(staged, 'sys/fst.bin'))).rejects.toMatchObject({ code: 'ENOENT' });
  expect(errorMessage('optimization_verification_failed')).toBe('Optimized files did not pass verification. Nothing was installed. Update the installer before retrying.');
});
it.each(['missing-movie', 'corrupt-audio', 'corrupt-texture', 'source-changed', 'bad-receipt', 'unexpected-output'])('blocks %s and removes incomplete Galaxy preparation', async failure => {
  const root = await directory(); await tree(root, fixtures.input); let staged = '';
  vi.spyOn(runner, 'runWorker').mockImplementation(async (_tool, args) => {
    if (_tool.path === 'movie-worker') {
      if (failure === 'missing-movie') throw new Error('missing movie');
      await writeFile(args[1], 'optimized movie'); return { stdout: '', stderr: '' };
    }
    staged = args[2]; await tree(staged, fixtures.output);
    if (failure === 'missing-movie') await rm(join(staged, 'files/MovieData/opening.thp'));
    if (failure === 'corrupt-audio') await writeFile(join(staged, 'files/AudioRes/music.ast'), 'broken audio');
    if (failure === 'corrupt-texture') await writeFile(join(staged, 'files/ObjectData/_bk00/model.arc'), 'broken model and texture');
    if (failure === 'source-changed') await writeFile(join(root, 'files/AudioRes/music.ast'), 'changed data');
    if (failure === 'unexpected-output') await writeFile(join(staged, 'settings.ini'), 'must not install');
    return { stdout: failure === 'bad-receipt' ? '{}' : '{"prepared":true,"movies":true,"layoutVerified":true}', stderr: '' };
  });
  await expect(prepareInputs(release(), { roots: [], chosen: { game: [root] } }, () => {}, undefined, tools)).rejects.toThrow();
  expect(staged).not.toBe(''); await expect(readFile(join(staged, 'sys/fst.bin'))).rejects.toMatchObject({ code: 'ENOENT' });
});
