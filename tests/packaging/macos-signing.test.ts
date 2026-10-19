import { afterEach, expect, it } from 'vitest';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { createHash } from 'node:crypto';
import { build as buildPlist } from 'plist';

const forge = createRequire(import.meta.url)('../../forge.config.cjs');
const roots: string[] = [];
const macTest = it.skipIf(process.platform !== 'darwin');
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });

async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'cboc-signing-test-')); roots.push(root);
  const app = join(root, 'CBOC Vita Installer.app'), contents = join(app, 'Contents');
  await mkdir(join(contents, 'MacOS'), { recursive: true });
  await mkdir(join(contents, 'Resources/installer-workers/darwin-arm64'), { recursive: true });
  await writeFile(join(contents, 'Info.plist'), buildPlist({
    CFBundleIdentifier: 'org.cboc.vita-installer', CFBundleExecutable: 'CBOC Vita Installer',
    CFBundleName: 'CBOC Vita Installer', CFBundlePackageType: 'APPL', CFBundleVersion: '1.0',
  }));
  const binary = join(contents, 'MacOS/CBOC Vita Installer');
  execFileSync('/usr/bin/cc', ['-x', 'c', '-o', binary, '-'], { input: 'int main(void) { return 0; }' });
  execFileSync('/usr/bin/codesign', ['--force', '--sign', '-', binary], { stdio: 'pipe' });
  const worker = join(contents, 'Resources/installer-workers/darwin-arm64/vita-prepare');
  await writeFile(worker, await readFile(binary), { mode: 0o755 });
  const resource = join(contents, 'Resources/profile.json');
  await writeFile(resource, '{"reviewed":true}\n');
  return { root, app, worker, resource };
}

async function packageGate(root: string) {
  await forge.hooks?.postPackage?.({}, { platform: 'darwin', arch: process.arch, outputPaths: [root] });
}

macTest('rejects the old executable-only signature before creating a download', async () => {
  const { root } = await fixture();
  await expect(packageGate(root)).rejects.toThrow();
});

macTest('signs the whole app without changing checksum-pinned worker binaries', async () => {
  const { root, app, worker } = await fixture();
  const before = createHash('sha256').update(await readFile(worker)).digest('hex');
  if (forge.packagerConfig.osxSign) {
    const { sign } = await import('@electron/osx-sign');
    await sign({ ...forge.packagerConfig.osxSign, app, platform: 'darwin', version: '44.9.0' });
  }
  expect(() => execFileSync('/usr/bin/codesign', ['--verify', '--deep', '--strict', app], { stdio: 'pipe' })).not.toThrow();
  expect(createHash('sha256').update(await readFile(worker)).digest('hex')).toBe(before);
  await expect(packageGate(root)).resolves.toBeUndefined();
});

macTest('rejects a resource modified after the app was signed', async () => {
  const { root, app, resource } = await fixture();
  execFileSync('/usr/bin/codesign', ['--force', '--sign', '-', app], { stdio: 'pipe' });
  await writeFile(resource, '{"reviewed":false}\n');
  await expect(packageGate(root)).rejects.toThrow();
});
