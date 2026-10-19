import { spawnSync } from 'node:child_process';
import { mkdir } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const platform = `${process.platform}-${process.arch}`;
const output = process.argv[2] ? resolve(process.argv[2]) : join(root, 'out/native-python-workers', platform);
const work = join(root, 'out/native-python-worker-build', platform);
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
console.log(`Built preparation and volume workers: ${platform}. Release catalog not generated.`);
