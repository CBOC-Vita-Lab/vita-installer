import { test } from 'node:test';
import assert from 'node:assert/strict';
import { constants } from 'node:fs';
import { cp, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';

const root = resolve(import.meta.dirname, '../..'), platform = `${process.platform}-${process.arch}`;
const app = resolve(process.env.VITA_TEST_PACKAGED_APP ?? join(root, `out/CBOC Vita Installer-${platform}`,
  process.platform === 'darwin' ? 'CBOC Vita Installer.app' : ''));
const verify = path => spawnSync(process.execPath, ['scripts/verify-release.mjs', path], { cwd: root, encoding: 'utf8', timeout: 60_000 });
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
function success(result) { assert.ifError(result.error); assert.equal(result.signal, null); assert.equal(result.status, 0, result.stderr); }

// Only mutate a disposable copy. Never change the packaged app being tested.
test('finished package rejects native notice and metadata damage', async t => {
  success(verify(app));
  const temporary = await mkdtemp(join(root, 'out/native-notice-check-')), copy = join(temporary, 'app');
  try {
    await cp(app, copy, { recursive: true, verbatimSymlinks: true, mode: constants.COPYFILE_FICLONE });
    success(verify(copy));
    const resources = join(copy, process.platform === 'darwin' ? 'Contents/Resources' : 'resources');
    const notices = join(resources, 'installer-resources/notices/libvips');
    const manifestFile = join(notices, 'manifest.json'), manifestBytes = await readFile(manifestFile);
    const manifest = JSON.parse(manifestBytes), notice = manifest.components.find(c => c.name === 'aom').notices.find(n => n.path === 'aom/LICENSE');
    const noticeFile = join(notices, notice.path), originalNotice = await readFile(noticeFile);
    const readmeFile = join(notices, 'README.md'), originalReadme = await readFile(readmeFile);
    const packageName = process.platform === 'win32' ? '@img/sharp-win32-x64' : `@img/sharp-libvips-${platform}`;
    const versionsFile = join(resources, 'app/node_modules', packageName, 'versions.json'), originalVersions = await readFile(versionsFile);
    const cases = [
      ['missing notice', async () => rm(noticeFile), /ENOENT.*aom[\\/]LICENSE/],
      ['changed notice', async () => writeFile(noticeFile, 'Changed notice'), /Native notice bytes changed: aom\/LICENSE/],
      ['self-authorized change', async () => {
        const changed = structuredClone(manifest), bytes = Buffer.from('Changed notice');
        changed.components.find(c => c.name === 'aom').notices.find(n => n.path === 'aom/LICENSE').sha256 = sha(bytes);
        await writeFile(noticeFile, bytes); await writeFile(manifestFile, JSON.stringify(changed));
      }, /Native notice manifest differs/],
      ['shortened manifest', async () => {
        const changed = structuredClone(manifest);
        changed.components.find(c => c.name === 'aom').notices = changed.components.find(c => c.name === 'aom').notices.filter(n => n.path !== 'aom/LICENSE');
        await rm(noticeFile); await writeFile(manifestFile, JSON.stringify(changed));
      }, /Native notice manifest differs/],
      ['changed acknowledgments', async () => writeFile(readmeFile, 'Acknowledgments removed'), /Native notice bytes changed: README.md/],
      ['changed installed component', async () => {
        const versions = JSON.parse(originalVersions); versions.aom = '99.0.0';
        await writeFile(versionsFile, JSON.stringify(versions));
      }, /Native notice version needs review: aom/],
    ];
    for (const [name, mutate, expected] of cases) await t.test(name, async () => {
      try {
        await mutate();
        const result = verify(copy);
        assert.ifError(result.error); assert.equal(result.signal, null); assert.equal(result.status, 1);
        assert.match(result.stderr, expected);
      } finally {
        await writeFile(manifestFile, manifestBytes); await writeFile(noticeFile, originalNotice);
        await writeFile(readmeFile, originalReadme); await writeFile(versionsFile, originalVersions);
      }
    });
    success(verify(copy));
  } finally { await rm(temporary, { recursive: true, force: true }); }
  success(verify(app));
});
