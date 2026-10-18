import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const platform = `${process.platform}-${process.arch}`;
const output = join(root, 'workers', 'bin', platform), work = join(root, 'workers', 'build', platform);
await mkdir(output, { recursive: true }); await mkdir(work, { recursive: true });
const python = process.env.VITA_WORKER_PYTHON ?? join(root, '.venv-workers', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python');
for (const entry of ['prepare', 'volume']) {
  const extras = entry === 'prepare' ? ['--paths', join(root, 'workers/vendor/galaxy/cook'),
    '--paths', join(root, 'workers/vendor/galaxy'), '--copy-metadata', 'pycdlib'] : [];
  const build = spawnSync(python, ['-m', 'PyInstaller', '--noconfirm', '--clean', entry === 'volume' ? '--onedir' : '--onefile',
    '--name', `vita-${entry}`, '--distpath', output, '--workpath', work, '--specpath', work,
    ...extras, join(root, 'workers', `${entry}.py`)], { cwd: root, stdio: 'inherit' });
  if (build.error || build.status !== 0) throw build.error ?? new Error(`Worker build failed (${build.status})`);
}
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
  volumeWorker: { path: volumeName, sha256: sha256(await readFile(join(output, volumeName))) }, sources }, null, 2) + '\n');
console.log(`Packaged data and volume workers: ${platform}`);
