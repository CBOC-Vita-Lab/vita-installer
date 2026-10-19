// Build-only validation path. Never writes workers/bin or modifies release pins.
import { build } from 'esbuild';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { resolve, join } from 'node:path';

const root = resolve(import.meta.dirname, '..'), platform = `${process.platform}-${process.arch}`;
if (!['darwin-arm64', 'darwin-x64', 'win32-x64'].includes(platform)) throw new Error('Unsupported candidate target.');
function run(args, env = process.env) {
  const result = spawnSync(process.execPath, args, { cwd: root, env, stdio: 'inherit', timeout: 15 * 60_000 });
  if (result.error || result.status !== 0) throw result.error ?? new Error('Native candidate build or test failed.');
}
run(['scripts/build-python-workers.mjs']);
run(['scripts/build-movie-candidate.mjs']);
const output = join(root, 'out/native-package-workers', platform), movie = join(root, 'out/movie-candidate', platform);
const name = `vita-movies${process.platform === 'win32' ? '.exe' : ''}`;
run(['--test', 'tests/preparation/movie-candidate.test.mjs'], { ...process.env, VITA_TEST_MOVIE_WORKER: join(movie, name) });
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const receipt = JSON.parse(await readFile(join(movie, 'candidate.json'), 'utf8'));
if (receipt.acceptedForPackaging !== false || receipt.platform !== platform || receipt.workerSha256 !== hash(await readFile(join(movie, name))))
  throw new Error('Native movie candidate receipt mismatch.');
// Rebuild only this generated candidate; do not retain stale files or links into the build tree.
await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });
await cp(join(root, 'out/native-python-workers', platform), output, { recursive: true, dereference: true });
await cp(join(movie, name), join(output, name));
await build({ entryPoints: ['src/packaging/worker-catalog.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/worker-catalog.mjs' });
const { writeWorkerCatalog } = await import('../dist/tools/worker-catalog.mjs');
await writeWorkerCatalog(output, join(root, 'workers'), platform);
await writeFile(join(output, 'native-candidate.json'), JSON.stringify({ schemaVersion: 1, platform,
  acceptedForRelease: false, catalogSha256: hash(await readFile(join(output, 'catalog.json'))) }, null, 2) + '\n');
console.log(`Native candidate workers ready: ${platform}. Not accepted for release.`);
