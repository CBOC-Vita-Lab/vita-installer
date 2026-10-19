import { build } from 'esbuild';
import { readdir, readFile, readlink, realpath, writeFile, lstat } from 'node:fs/promises';
import { resolve, join, relative, sep } from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { createReadStream } from 'node:fs';
import { verifyMacSignature } from './verify-macos-signature.cjs';
await build({ entryPoints: ['src/packaging/policy.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/package-policy.mjs' });
const { validatePackagePolicy, NETWORK_POLICY } = await import('../dist/tools/package-policy.mjs');
await build({ entryPoints: ['src/packaging/native-notices.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/native-notices.mjs' });
const { verifyInstalledSharpNotices } = await import('../dist/tools/native-notices.mjs');
await build({ entryPoints: ['src/packaging/worker-catalog.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/worker-catalog.mjs' });
const { verifyCandidateReceipt } = await import('../dist/tools/worker-catalog.mjs');
await build({ entryPoints: ['src/packaging/application.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/application.mjs' });
const { packagedApplication } = await import('../dist/tools/application.mjs');
await build({ entryPoints: ['src/packaging/privacy.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/package-privacy.mjs' });
const { inspectPackagePrivacy } = await import('../dist/tools/package-privacy.mjs');
const platform = `${process.platform}-${process.arch}`;
const { app, resources, executable } = packagedApplication(process.cwd(), platform, process.argv[2]);
const inventory = [], reviewedUpstreamBuildPaths = [];
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
        const review = inspectPackagePrivacy(name, await readFile(path), ownerHome);
        if (review) reviewedUpstreamBuildPaths.push(review);
      }
      inventory.push({ path: name, bytes: size, sha256: await fileHash(path) });
    } else throw new Error(`Unsupported package member: ${name}`);
  }
}
await walk(resources);
const nativeNotices = await verifyInstalledSharpNotices(join(resources, 'installer-resources/notices/libvips'), join(resources, 'app/node_modules'), platform,
  await fileHash('resources/notices/libvips/manifest.json'));
const workerRoot = join(resources, 'installer-workers', platform), catalog = JSON.parse(await readFile(join(workerRoot, 'catalog.json'), 'utf8'));
if (catalog.platform !== platform) throw new Error('Worker catalog platform mismatch');
const candidateBytes = await readFile(join(resources, 'installer-resources/native-candidate.json')).catch(error => {
  if (error.code === 'ENOENT') return undefined; throw error;
});
const candidate = candidateBytes ? verifyCandidateReceipt(JSON.parse(candidateBytes), platform, await fileHash(join(workerRoot, 'catalog.json'))) : undefined;
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
if (await architecture(executable) !== process.arch) throw new Error('App architecture mismatch');
const signature = process.platform === 'darwin' ? verifyMacSignature(app)
  : { signing: 'unsigned-internal', signingSubject: null, signatureVerified: false };
const policy = { platform, host: platform, ...signature, files: inventory.map(x => x.path), helperArchitectures, networkPolicy: NETWORK_POLICY };
validatePackagePolicy(policy);
const receipt = { schemaVersion: 1, ...policy, files: inventory, executableSha256: await fileHash(executable),
  notarized: false, publicReleaseAccepted: false, deviceInstalled: false, thirdPartyRedistributionReview: 'pending',
  nativeCandidate: candidate ?? null,
  reviewedUpstreamBuildPaths,
  nativeImageNotices: { declaredComponents: nativeNotices.components.length,
    noticeFiles: nativeNotices.components.reduce((count, component) => count + component.notices.length, 0), completeRedistributionReview: false },
  checks: ['required-resources', 'native-architecture', 'helper-hashes', 'no-personal-signing-subject', 'payload-inventory', 'owner-path-and-token-scan', 'native-image-notice-hashes-and-versions',
    ...(process.platform === 'darwin' ? ['whole-app-deep-strict-signature'] : [])],
  skipped: ['GUI-and-device-acceptance', 'full-owned-input-comparisons', 'public-redistribution-license-audit'] };
await writeFile('out/package-verification.json', JSON.stringify(receipt, null, 2) + '\n');
console.log(`Verified internal ${platform} package: ${inventory.length} resource files. Public/hardware acceptance remains held.`);
