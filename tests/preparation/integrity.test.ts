import { afterEach, expect, it } from 'vitest';
import { copyFile, mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { digest } from '../../src/shared/files';
import { verifyInventory } from '../../src/preparation/integrity';
import { checkStrikersExtraction } from '../../src/preparation/inputs';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'integrity-')); roots.push(root);
  await mkdir(join(root, 'files/Art'), { recursive: true }); await writeFile(join(root, 'files/Art/game.bin'), 'game');
  return { root, entries: [{ path: 'files/Art/game.bin', bytes: 4, sha256: digest('game') }] };
}
it('accepts a complete tree without changing its files', async () => {
  const f = await fixture(); await expect(verifyInventory(f.root, f.entries)).resolves.toBeUndefined();
});
it.each(['file', 'folder'] as const)('names the expected missing %s instead of accepting a partial extraction', async kind => {
  const f = await fixture(); await rm(join(f.root, kind === 'file' ? 'files/Art/game.bin' : 'files/Art'), { recursive: true });
  await expect(verifyInventory(f.root, f.entries)).rejects.toMatchObject({ code: 'source_missing', path: join(f.root, 'files/Art/game.bin') });
});
it('rejects truncated and same-size corrupt data', async () => {
  const f = await fixture();
  for (const value of ['ga', 'fake']) {
    await writeFile(join(f.root, 'files/Art/game.bin'), value);
    await expect(verifyInventory(f.root, f.entries)).rejects.toMatchObject({ code: 'source_changed' });
  }
});
it('rejects symlinked folders and path escapes', async () => {
  const f = await fixture(); await symlink(join(f.root, 'files'), join(f.root, 'alias'));
  await expect(verifyInventory(f.root, [{ ...f.entries[0], path: 'alias/Art/game.bin' }])).rejects.toMatchObject({ code: 'source_unreadable' });
  await expect(verifyInventory(f.root, [{ ...f.entries[0], path: '../outside' }])).rejects.toMatchObject({ code: 'unsafe_path' });
});
it.runIf(!!process.env.VITA_STRIKERS_EXTRACTION)('checks the real complete Strikers extraction and rejects its incomplete copy', async () => {
  const original = process.env.VITA_STRIKERS_EXTRACTION!;
  await checkStrikersExtraction(original, undefined, true);
  const root = await mkdtemp(join(tmpdir(), 'incomplete-strikers-')); roots.push(root);
  await mkdir(join(root, 'sys')); await mkdir(join(root, 'files/Art'), { recursive: true });
  await copyFile(join(original, 'sys/main.dol'), join(root, 'sys/main.dol'));
  await expect(checkStrikersExtraction(root)).rejects.toMatchObject({ code: 'source_missing' });
}, 120000);
