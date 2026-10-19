import { spawnSync } from 'node:child_process';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from 'esbuild';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
await build({ entryPoints: ['src/packaging/worker-catalog.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/worker-catalog.mjs' });
const { writeWorkerCatalog } = await import('../dist/tools/worker-catalog.mjs');
const movieBuild = spawnSync(process.execPath, [join(root, 'scripts/build-movie-worker.mjs')], { cwd: root, stdio: 'inherit' });
if (movieBuild.error || movieBuild.status !== 0) throw movieBuild.error ?? new Error('Movie helper validation failed.');
const pythonBuild = spawnSync(process.execPath, [join(root, 'scripts/build-python-workers.mjs'), join(root, 'workers/bin', `${process.platform}-${process.arch}`)], { cwd: root, stdio: 'inherit' });
if (pythonBuild.error || pythonBuild.status !== 0) throw pythonBuild.error ?? new Error('Python worker build failed.');
const platform = `${process.platform}-${process.arch}`;
const output = join(root, 'workers', 'bin', platform);
await writeWorkerCatalog(output, join(root, 'workers'), platform);
console.log(`Packaged data and volume workers: ${platform}`);
