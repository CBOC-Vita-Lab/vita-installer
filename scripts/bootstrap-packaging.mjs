import { spawnSync } from 'node:child_process';
import { copyFile, readFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { build } from 'esbuild';

const root = resolve(import.meta.dirname, '..');
const platform = `${process.platform}-${process.arch}`;
if (!['darwin-arm64', 'darwin-x64', 'win32-x64'].includes(platform)) throw new Error('Unsupported package target.');
// Electron's pinned package supplies the downloader and archive checksums.
await build({ entryPoints: [join(root, 'src/packaging/application.ts')], bundle: true, platform: 'node', format: 'esm', outfile: join(root, 'dist/tools/application.mjs') });
const { packagingEnvironment } = await import('../dist/tools/application.mjs');
const env = packagingEnvironment(platform, process.env);
const result = spawnSync(process.execPath, ['node_modules/electron/install.js'], {
  cwd: root, env, stdio: 'inherit', timeout: 5 * 60_000 });
if (result.error || result.status !== 0) throw result.error ?? new Error('Electron runtime download failed.');
for (const name of ['LICENSE', 'LICENSES.chromium.html']) {
  if (!(await readFile(join(root, 'node_modules/electron/dist', name))).length) throw new Error('Missing Electron notices.');
}
if (process.platform === 'win32') {
  // Select only the bundled native archiver, without enabling unrelated npm install hooks.
  const vendor = join(root, 'node_modules/electron-winstaller/vendor');
  for (const extension of ['exe', 'dll']) await copyFile(join(vendor, `7z-${process.arch}.${extension}`), join(vendor, `7z.${extension}`));
}
console.log(`Native packaging runtime ready: ${platform}.`);
