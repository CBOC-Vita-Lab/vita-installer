import { afterEach, expect, it } from 'vitest';
import { mkdtemp, mkdir, writeFile, rm, readFile, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { verifyNativeNotices, verifyInstalledSharpNotices } from '../../src/packaging/native-notices';

const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
const sha = (text: string) => createHash('sha256').update(text).digest('hex');
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'native-notices-')); roots.push(root);
  await mkdir(join(root, 'sample'));
  await writeFile(join(root, 'sample/LICENSE'), 'Copyright synthetic fixture\n');
  await writeFile(join(root, 'README.md'), 'Synthetic acknowledgments\n');
  const manifest = { schemaVersion: 1, packageVersion: '1.3.4', buildRecipeCommit: 'a'.repeat(40),
    readmeSha256: sha('Synthetic acknowledgments\n'),
    packages: { '@img/sharp-libvips-darwin-arm64': '1.3.4', '@img/sharp-libvips-darwin-x64': '1.3.4', '@img/sharp-win32-x64': '0.35.5' },
    completeRedistributionReview: false, unresolved: ['Transitive notices still require review.'],
    components: [{ name: 'sample', version: '1.0', source: { url: 'https://example.com/source.tar.gz', sha256: 'b'.repeat(64) },
      notices: [{ path: 'sample/LICENSE', sourceMember: 'sample-1.0/LICENSE', sha256: sha('Copyright synthetic fixture\n') }] }] };
  const save = () => writeFile(join(root, 'manifest.json'), JSON.stringify(manifest));
  await save(); return { root, manifest, save, digest: () => sha(JSON.stringify(manifest)) };
}
it('verifies the exact installed versions and unchanged notice bytes', async () => {
  const f = await fixture();
  const result = await verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' });
  expect(result.components).toHaveLength(1);
  expect(result.completeRedistributionReview).toBe(false);
});
it('rejects missing, changed and newly upgraded component notices', async () => {
  const f = await fixture();
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.5', { sample: '1.0' })).rejects.toThrow();
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '2.0' })).rejects.toThrow();
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { unknown: '1.0' })).rejects.toThrow();
  await expect(verifyNativeNotices(f.root, '@img/unreviewed', '1.3.4', { sample: '1.0' })).rejects.toThrow();
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', {})).rejects.toThrow();
  await writeFile(join(f.root, 'sample/LICENSE'), 'Changed');
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow();
  await rm(join(f.root, 'sample/LICENSE'));
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow();
});
it('rejects duplicate components, notice paths and unsafe destinations', async () => {
  const f = await fixture();
  f.manifest.components.push(f.manifest.components[0]); await f.save();
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow();
  f.manifest.components.pop();
  f.manifest.components[0].notices.push(f.manifest.components[0].notices[0]); await f.save();
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow();
  f.manifest.components[0].notices.pop();
  for (const path of ['../LICENSE', '/LICENSE', 'sample/../../LICENSE', 'other/LICENSE', 'sample\\LICENSE']) {
    f.manifest.components[0].notices[0].path = path; await f.save();
    await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow();
  }
});
it('does not treat the notice collection as complete redistribution clearance', async () => {
  const f = await fixture();
  f.manifest.completeRedistributionReview = true; await f.save();
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow();
});
it('reads the target native package, not metadata from another architecture', async () => {
  const f = await fixture(), modules = join(f.root, 'node_modules');
  for (const [platform, name, version] of [
    ['darwin-arm64', '@img/sharp-libvips-darwin-arm64', '1.3.4'],
    ['darwin-x64', '@img/sharp-libvips-darwin-x64', '1.3.4'],
    ['win32-x64', '@img/sharp-win32-x64', '0.35.5'],
  ]) {
    const target = join(modules, name);
    await mkdir(target, { recursive: true });
    await writeFile(join(target, 'package.json'), JSON.stringify({ name, version }));
    await writeFile(join(target, 'versions.json'), JSON.stringify({ sample: '1.0' }));
    expect((await verifyInstalledSharpNotices(f.root, modules, platform, f.digest())).components).toHaveLength(1);
    await writeFile(join(target, 'package.json'), JSON.stringify({ name, version: '99.0.0' }));
    await expect(verifyInstalledSharpNotices(f.root, modules, platform, f.digest())).rejects.toThrow();
    await writeFile(join(target, 'package.json'), JSON.stringify({ name: 'wrong-package', version }));
    await expect(verifyInstalledSharpNotices(f.root, modules, platform, f.digest())).rejects.toThrow();
  }
  await expect(verifyInstalledSharpNotices(f.root, modules, 'linux-x64', f.digest())).rejects.toThrow();
  await expect(verifyInstalledSharpNotices(f.root, join(f.root, 'missing'), 'win32-x64', f.digest())).rejects.toThrow();
});
it('rejects a copied manifest that authorizes its own altered notices', async () => {
  const f = await fixture(), expected = f.digest(), modules = join(f.root, 'node_modules');
  const target = join(modules, '@img/sharp-win32-x64');
  await mkdir(target, { recursive: true });
  await writeFile(join(target, 'package.json'), JSON.stringify({ name: '@img/sharp-win32-x64', version: '0.35.5' }));
  await writeFile(join(target, 'versions.json'), JSON.stringify({ sample: '1.0' }));
  await writeFile(join(f.root, 'sample/LICENSE'), 'Changed with matching checksum');
  f.manifest.components[0].notices[0].sha256 = sha('Changed with matching checksum');
  await f.save();
  await expect(verifyInstalledSharpNotices(f.root, modules, 'win32-x64', expected)).rejects.toThrow('manifest');
});
it('rejects changed collection acknowledgments', async () => {
  const f = await fixture();
  await writeFile(join(f.root, 'README.md'), 'Acknowledgments removed');
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow('README.md');
});
it.skipIf(process.platform === 'win32')('rejects a notice replaced by a symbolic link', async () => {
  const f = await fixture();
  await rm(join(f.root, 'sample/LICENSE'));
  await writeFile(join(f.root, 'original'), 'Copyright synthetic fixture\n');
  await symlink(join(f.root, 'original'), join(f.root, 'sample/LICENSE'));
  await expect(verifyNativeNotices(f.root, '@img/sharp-libvips-darwin-arm64', '1.3.4', { sample: '1.0' })).rejects.toThrow();
});
it('verifies the real notice collection against the installed native components', async () => {
  const expected = sha(await readFile('resources/notices/libvips/manifest.json', 'utf8'));
  const result = await verifyInstalledSharpNotices('resources/notices/libvips', 'node_modules', `${process.platform}-${process.arch}`, expected);
  expect(result.components).toHaveLength(28);
  expect(result.unresolved.length).toBeGreaterThan(0);
});
