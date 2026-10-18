import { afterEach, expect, it } from 'vitest';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { planRetroflowCover } from '../../src/artwork/retroflow';
import type { PreparedPackage } from '../../src/shared/contracts';
import { digest } from '../fixtures/releases';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function cover() {
  const root = await mkdtemp(join(tmpdir(), 'cover-test-')); roots.push(root);
  const path = join(root, 'cover.png'); await writeFile(path, 'prepared image');
  return { images: { retroflow: { path, outputSha256: digest('prepared image'), sourceSha256: digest('input') } },
    inventory: { titleId: 'MSCN00003' } } as PreparedPackage;
}
it('uses the observed category and title ID, not a display name or assumed Vita folder', async () => {
  const pkg = await cover();
  const homebrew = await planRetroflowCover(pkg, 'MSCN00003', { category: 'homebrew' });
  expect(homebrew).toMatchObject({ action: 'copy', file: { destination: 'ux0:data/RetroFlow/COVERS/Homebrew/MSCN00003.png' } });
  const vita = await planRetroflowCover(pkg, 'MSCN00003', { category: 'vita' });
  expect(vita).toMatchObject({ action: 'copy', file: { destination: 'ux0:data/RetroFlow/COVERS/Sony - PlayStation Vita/MSCN00003.png' } });
});
it('leaves an existing different cover alone and skips identical content', async () => {
  const pkg = await cover();
  expect(await planRetroflowCover(pkg, 'MSCN00003', { category: 'homebrew', existingSha256: digest('user image') })).toMatchObject({ action: 'conflict' });
  expect(await planRetroflowCover(pkg, 'MSCN00003', { category: 'homebrew', existingSha256: digest('prepared image') })).toMatchObject({ action: 'keep' });
});
it('exports for manual import when category is unknown or a named cover has priority', async () => {
  const pkg = await cover();
  expect(await planRetroflowCover(pkg, 'MSCN00003', { category: 'unknown' })).toEqual({ action: 'manual', source: pkg.images.retroflow!.path, filename: 'MSCN00003.png' });
  expect(await planRetroflowCover(pkg, 'MSCN00003', { category: 'homebrew', namedCoverOverride: true })).toMatchObject({ action: 'manual' });
});
it('rejects a changed prepared image and title-ID substitution', async () => {
  const pkg = await cover();
  await expect(planRetroflowCover(pkg, 'OTHER0001', { category: 'homebrew' })).rejects.toMatchObject({ code: 'artwork_title_mismatch' });
  await writeFile(pkg.images.retroflow!.path, 'changed');
  await expect(planRetroflowCover(pkg, 'MSCN00003', { category: 'homebrew' })).rejects.toMatchObject({ code: 'artwork_changed' });
});
