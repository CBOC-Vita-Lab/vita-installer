import { cp, mkdir, readdir, readFile, writeFile, rm } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
import { join, resolve, dirname } from 'node:path';
import { createHash } from 'node:crypto';
const platform = `${process.platform}-${process.arch}`, root = process.cwd();
const python = process.env.VITA_WORKER_PYTHON ?? join(root, '.venv-workers', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python');
const query = spawnSync(python, ['-c', 'import sys,sysconfig,importlib.metadata,json; print(json.dumps({"stdlib":sysconfig.get_path("stdlib"),"prefix":sys.base_prefix,"site":str(importlib.metadata.distribution("pycdlib").locate_file("")),"pycdlib":str(importlib.metadata.distribution("pycdlib")._path),"pyinstaller":str(importlib.metadata.distribution("pyinstaller")._path)}))'], { encoding: 'utf8' });
if (query.status !== 0) throw new Error('Build-time worker environment unavailable.');
const paths = JSON.parse(query.stdout), notices = resolve('dist/installer-notices');
for (const dir of ['installer-resources', 'installer-workers', 'installer-notices']) await rm(join('dist', dir), { recursive: true, force: true });
await mkdir(notices, { recursive: true });
await cp('resources', 'dist/installer-resources', { recursive: true });
// Packager rewrites resource symlinks to the staging directory; materialize this runtime first.
await cp(`workers/bin/${platform}`, `dist/installer-workers/${platform}`, { recursive: true, dereference: true });
const catalog = JSON.parse(await readFile(`workers/bin/${platform}/catalog.json`, 'utf8'));
if (catalog.platform !== platform) throw new Error('Worker architecture mismatch.');
const sha = b => createHash('sha256').update(b).digest('hex');
for (const entry of [catalog.worker, catalog.volumeWorker, catalog.movieWorker]) if (!entry || sha(await readFile(join('workers/bin', platform, entry.path))) !== entry.sha256) throw new Error('Worker catalog mismatch.');
async function firstCopy(candidates, target) {
  for (const path of candidates) { try { await cp(path, join(notices, target)); return; } catch (error) { if (error.code !== 'ENOENT') throw error; } }
  throw new Error(`Missing license: ${target}`);
}
await firstCopy([join(paths.stdlib, 'LICENSE.txt'), join(paths.prefix, 'LICENSE.txt'), join(paths.prefix, 'LICENSE')], 'Python-LICENSE.txt');
await firstCopy([join(paths.pycdlib, 'licenses/COPYING')], 'pycdlib-LICENSE.txt');
await firstCopy([join(paths.pyinstaller, 'licenses/COPYING.txt')], 'PyInstaller-COPYING.txt');
await cp('workers/vendor/galaxy/LICENSE', join(notices, 'Galaxy-LICENSE.txt'));
await cp('workers/vendor/galaxy/ORIGIN.md', join(notices, 'Galaxy-ORIGIN.md'));
await cp('workers/vendor/libjpeg-turbo/LICENSE.md', join(notices, 'libjpeg-turbo-LICENSE.md'));
await cp('workers/vendor/libjpeg-turbo/README.ijg', join(notices, 'libjpeg-turbo-README.ijg'));
await cp('node_modules/electron/dist/LICENSE', join(notices, 'Electron-LICENSE.txt'));
await cp('node_modules/electron/dist/LICENSES.chromium.html', join(notices, 'Chromium-LICENSES.html'));
await cp(join(paths.site, 'pycdlib'), join(notices, 'source/pycdlib'), { recursive: true, filter: path => !path.includes('__pycache__') && !path.endsWith('.pyc') });
const lock = JSON.parse(await readFile('package-lock.json', 'utf8')), components = [], entries = [];
for (const [path, item] of Object.entries(lock.packages)) {
  if (!path || item.dev) continue;
  let pkg; try { pkg = JSON.parse(await readFile(join(path, 'package.json'), 'utf8')); } catch (error) { if (error.code === 'ENOENT' && item.optional) continue; throw error; }
  const names = (await readdir(path)).filter(name => /^(?:licen[cs]e|copying|copyrightnotice|notice)(?:\.|$)/i.test(name));
  const destination = join(notices, 'npm', pkg.name.replaceAll('/', '__')); await mkdir(destination, { recursive: true });
  for (const name of names) await cp(join(path, name), join(destination, name));
  if (pkg.name === '@nodable/entities') {
    const supplemental = 'resources/notices/nodable-entities';
    if (pkg.version !== '3.1.0' || sha(await readFile(join(supplemental, 'LICENSE'))) !== '750cb3fb6362804957ef52caaf9b5c824015be44d494637330d7cd8834d31d40')
      throw new Error('Nodable entities notice needs review.');
    await cp(join(supplemental, 'LICENSE'), join(destination, 'LICENSE'));
    await cp(join(supplemental, 'ORIGIN.md'), join(destination, 'ORIGIN.md'));
  } else if (!names.length) { await cp(join(path, 'README.md'), join(destination, 'README.md')); entries.push(`${pkg.name}: package supplies a license declaration/README, not a complete license text. Public redistribution review remains open.`); }
  if (pkg.name.startsWith('@img/sharp-libvips')) { await cp(join(path, 'versions.json'), join(destination, 'versions.json')); entries.push(`${pkg.name}: libvips and linked-library notices/versions retained. Complete corresponding source and notice review required before public distribution.`); }
  components.push({ type: 'library', name: pkg.name, version: pkg.version, purl: `pkg:npm/${pkg.name.replace('@', '%40')}@${pkg.version}`, licenses: [{ expression: typeof pkg.license === 'string' ? pkg.license : 'NOASSERTION' }] });
}
components.push({ type: 'framework', name: 'Electron', version: lock.packages['node_modules/electron'].version, licenses: [{ license: { id: 'MIT' } }] },
  { type: 'library', name: 'libjpeg-turbo', version: '3.1.4.1', licenses: [{ expression: 'IJG AND BSD-3-Clause AND Zlib' }] },
  { type: 'library', name: 'pycdlib', version: '1.22.0', licenses: [{ license: { id: 'LGPL-2.1-or-later' } }] },
  { type: 'library', name: 'Python', version: '3.14.4', licenses: [{ license: { id: 'PSF-2.0' } }] },
  { type: 'library', name: 'PyInstaller bootloader', version: '6.22.3', licenses: [{ expression: 'GPL-2.0-or-later WITH Bootloader-exception' }] });
await writeFile(join(notices, 'SBOM.cdx.json'), JSON.stringify({ bomFormat: 'CycloneDX', specVersion: '1.6', version: 1,
  metadata: { component: { type: 'application', name: lock.name, version: lock.version } }, components }, null, 2) + '\n');
await writeFile(join(notices, 'THIRD_PARTY_NOTICES.md'), '# Third-party components\n\nInternal unsigned preview. Not cleared for public distribution.\n\nElectron/Chromium, Python, PyInstaller (bootloader exception), PyCdlib, the Galaxy cooker and installed npm dependency notices are included. PyCdlib source is included for replacement/rebuilding; the app worker sources are in the matching installer repository.\n\n' + entries.map(x => `- ${x}`).join('\n') + '\n\nThe Python runtime also bundles native library dependencies. Their full notice/source audit remains a public-release blocker. No game data or proprietary module is included.\n');
console.log(`Staged ${platform}: native helpers, dependency inventory and internal-preview notices.`);
