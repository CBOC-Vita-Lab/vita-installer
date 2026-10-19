import { build } from 'esbuild';
import { readdir, readFile, readlink, realpath, writeFile, lstat } from 'node:fs/promises';
import { resolve, join, relative, sep } from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { createReadStream } from 'node:fs';
await build({ entryPoints: ['src/packaging/policy.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/package-policy.mjs' });
const { validatePackagePolicy, NETWORK_POLICY } = await import('../dist/tools/package-policy.mjs');
const platform = `${process.platform}-${process.arch}`;
const app = resolve(process.argv[2] ?? `out/CBOC Vita Installer-${platform}/${process.platform === 'darwin' ? 'CBOC Vita Installer.app' : ''}`);
const resources = join(app, process.platform === 'darwin' ? 'Contents/Resources' : 'resources'), inventory = [];
const ownerHome = process.env.HOME ?? process.env.USERPROFILE;
async function fileHash(path) { const hash = createHash('sha256'); for await (const chunk of createReadStream(path)) hash.update(chunk); return hash.digest('hex'); }
async function walk(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name), name = relative(resources, path).split(sep).join('/');
    if (entry.isDirectory()) await walk(path);
    else if (entry.isSymbolicLink()) {
      const target = await realpath(path); if (!target.startsWith(resources + sep)) throw new Error(`Escaping package link: ${name}`);
      inventory.push({ path: name, link: await readlink(path) });
    } else if (entry.isFile()) {
      const size = (await lstat(path)).size;
      if (size < 20 * 1024 ** 2) {
        const data = await readFile(path), text = data.toString('utf8');
        if (ownerHome && text.includes(ownerHome + '/') || /(?:github_pat_|ghp_|sk-ant-api03-)[A-Za-z0-9_-]{30,}/.test(text)) throw new Error(`Private data in package: ${name}`);
      }
      inventory.push({ path: name, bytes: size, sha256: await fileHash(path) });
    } else throw new Error(`Unsupported package member: ${name}`);
  }
}
await walk(resources);
const workerRoot = join(resources, 'installer-workers', platform), catalog = JSON.parse(await readFile(join(workerRoot, 'catalog.json'), 'utf8'));
if (catalog.platform !== platform) throw new Error('Worker catalog platform mismatch');
async function architecture(path) {
  if (process.platform === 'darwin') { const value = execFileSync('/usr/bin/lipo', ['-archs', path], { encoding: 'utf8' }).trim().split(/\s+/); return value.includes('arm64') && process.arch === 'arm64' ? 'arm64' : value.includes('x86_64') ? 'x64' : 'unknown'; }
  const data = await readFile(path), offset = data.readUInt32LE(0x3c);
  if (data.subarray(offset, offset + 4).toString('binary') !== 'PE\0\0') return 'unknown';
  return data.readUInt16LE(offset + 4) === 0x8664 ? 'x64' : 'unknown';
}
const helperArchitectures = [];
for (const item of [catalog.worker, catalog.volumeWorker, catalog.movieWorker]) {
  const path = resolve(workerRoot, item.path); if (!path.startsWith(workerRoot + sep) || await fileHash(path) !== item.sha256) throw new Error('Packaged helper hash mismatch');
  helperArchitectures.push(await architecture(path));
}
const executable = join(app, process.platform === 'darwin' ? 'Contents/MacOS/CBOC Vita Installer' : 'CBOC Vita Installer.exe');
if (await architecture(executable) !== process.arch) throw new Error('App architecture mismatch');
let signature = 'Not authenticated for public distribution';
if (process.platform === 'darwin') {
  try { signature = execFileSync('/usr/bin/codesign', ['-d', '--verbose=4', executable], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }); }
  catch (error) { signature = String(error.stderr ?? 'unsigned'); }
  // codesign describes identity on stderr, including on success.
  const result = (await import('node:child_process')).spawnSync('/usr/bin/codesign', ['-d', '--verbose=4', executable], { encoding: 'utf8' });
  signature = result.stderr;
  if (/^Authority=|^TeamIdentifier=(?!not set)/m.test(signature)) throw new Error('Unreviewed signing identity');
}
const policy = { platform, host: platform, signing: 'unsigned-internal', signingSubject: null, files: inventory.map(x => x.path), helperArchitectures, networkPolicy: NETWORK_POLICY };
validatePackagePolicy(policy);
const receipt = { schemaVersion: 1, ...policy, files: inventory, executableSha256: await fileHash(executable),
  notarized: false, publicReleaseAccepted: false, deviceInstalled: false, thirdPartyRedistributionReview: 'pending',
  checks: ['required-resources', 'native-architecture', 'helper-hashes', 'no-personal-signing-subject', 'payload-inventory', 'owner-path-and-token-scan'],
  skipped: ['Windows', 'Intel-Mac', 'physical-Vita-install', 'public-redistribution-license-audit'] };
await writeFile('out/package-verification.json', JSON.stringify(receipt, null, 2) + '\n');
console.log(`Verified internal ${platform} package: ${inventory.length} resource files. Public/hardware acceptance remains held.`);
