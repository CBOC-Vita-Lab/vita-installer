import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const movieBuild = spawnSync(process.execPath, [join(root, 'scripts/build-movie-worker.mjs')], { cwd: root, stdio: 'inherit' });
if (movieBuild.error || movieBuild.status !== 0) throw movieBuild.error ?? new Error('Movie helper validation failed.');
const pythonBuild = spawnSync(process.execPath, [join(root, 'scripts/build-python-workers.mjs'), join(root, 'workers/bin', `${process.platform}-${process.arch}`)], { cwd: root, stdio: 'inherit' });
if (pythonBuild.error || pythonBuild.status !== 0) throw pythonBuild.error ?? new Error('Python worker build failed.');
const platform = `${process.platform}-${process.arch}`;
const output = join(root, 'workers', 'bin', platform);
const name = `vita-prepare${process.platform === 'win32' ? '.exe' : ''}`;
const volumeName = `vita-volume/vita-volume${process.platform === 'win32' ? '.exe' : ''}`;
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const sources = [];
async function collect(directory, relative = '') {
  for (const entry of (await readdir(directory, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
    if (['bin', 'build', '__pycache__'].includes(entry.name)) continue;
    const path = join(directory, entry.name), name = relative + entry.name;
    if (entry.isDirectory()) await collect(path, `${name}/`);
    else if (entry.isFile()) sources.push({ path: name, sha256: sha256(await readFile(path)) });
  }
}
await collect(join(root, 'workers'));
await writeFile(join(output, 'catalog.json'), JSON.stringify({ schemaVersion: 1, platform,
  worker: { path: name, sha256: sha256(await readFile(join(output, name))) },
  volumeWorker: { path: volumeName, sha256: sha256(await readFile(join(output, volumeName))) },
  movieWorker: { path: 'vita-movies', sha256: sha256(await readFile(join(output, 'vita-movies'))) }, sources }, null, 2) + '\n');
console.log(`Packaged data and volume workers: ${platform}`);
