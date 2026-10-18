import { afterEach, describe, expect, it } from 'vitest';
import { mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import sharp from 'sharp';
import { prepareArtwork } from '../../src/artwork/prepare';
import { manifestSchema, verifyVpk } from '../../src/releases/manifest';
import type { ImportedRelease } from '../../src/shared/contracts';
import { digest, manifest, runtime, sfo, zip } from '../fixtures/releases';

const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function png(width: number, height: number, background = '#37ad97') {
  return sharp({ create: { width, height, channels: 4, background } }).png().toBuffer();
}
async function fixture(extra: Record<string, Buffer | string> = {}) {
  const root = await mkdtemp(join(tmpdir(), 'art-test-')); roots.push(root);
  const members = {
    'eboot.bin': runtime, 'sce_sys/param.sfo': sfo(),
    'sce_sys/icon0.png': await png(128, 128),
    'sce_sys/livearea/contents/bg.png': await png(840, 500),
    'sce_sys/livearea/contents/startup.png': await png(280, 158),
    'sce_sys/livearea/contents/template.xml': '<livearea><livearea-background><image>bg.png</image></livearea-background></livearea>',
    'shaders/fixed.gxp': 'shader bytes', 'NOTICE': 'Original attribution',
    'retroflow/covers.png': await png(256, 320), ...extra,
  };
  const original = await zip(members), vpkPath = join(root, 'Strikers.vpk'); await writeFile(vpkPath, original);
  const m = manifestSchema.parse(manifest(digest(original)));
  const release: ImportedRelease = { manifest: m, manifestSha256: digest(JSON.stringify(m)),
    sourceRoot: root, vpkPath, inventory: await verifyVpk(vpkPath, m) };
  return { root, release, members, original };
}
async function prepare(release: ImportedRelease, choices: Parameters<typeof prepareArtwork>[1]) {
  const value = await prepareArtwork(release, choices); if (value.workspace) roots.push(value.workspace); return value;
}

describe('local artwork preparation', () => {
  it('keeps the exact original VPK when all artwork choices are default', async () => {
    const f = await fixture(), result = await prepare(f.release, {});
    expect(result.path).toBe(f.release.vpkPath);
    expect(result.sha256).toBe(digest(f.original));
    expect(result.originalSha256).toBe(result.sha256);
    expect(Object.keys(result.images).sort()).toEqual(['bubble', 'livearea', 'retroflow']);
    expect(await readFile(result.images.bubble!.path)).toEqual(f.members['sce_sys/icon0.png']);
  });
  it('replaces only bubble and background bytes, with exact trusted dimensions and an external cover', async () => {
    const f = await fixture(), source = join(f.root, 'custom.jpg');
    const custom = await sharp({ create: { width: 900, height: 700, channels: 3, background: '#da5678' } })
      .withExif({ IFD0: { Artist: 'Private Artist', Copyright: 'Private location' } }).jpeg().toBuffer();
    await writeFile(source, custom);
    const result = await prepare(f.release, {
      bubble: { source, fit: 'cover' }, livearea: { source, fit: 'contain' }, retroflow: { source, fit: 'cover' },
    });
    expect(result.path).not.toBe(f.release.vpkPath); expect(result.sha256).not.toBe(result.originalSha256);
    expect(result.originalSha256).toBe(digest(f.original)); expect(result.runtimeSha256).toBe(digest(runtime));
    expect(await readFile(f.release.vpkPath!)).toEqual(f.original); expect(await readFile(source)).toEqual(custom);
    expect(result.inventory.members.length).toBe(Object.keys(f.members).length);
    for (const item of result.inventory.members) {
      if (['sce_sys/icon0.png', 'sce_sys/livearea/contents/bg.png'].includes(item.path)) expect(item.sha256).not.toBe(digest(f.members[item.path as keyof typeof f.members]));
      else expect(item.sha256).toBe(digest(f.members[item.path as keyof typeof f.members]));
    }
    for (const [slot, width, height] of [['bubble', 128, 128], ['livearea', 840, 500], ['retroflow', 256, 320]] as const) {
      const image = result.images[slot]!;
      const meta = await sharp(image.path).metadata();
      expect(meta.width).toBe(width); expect(meta.height).toBe(height);
      expect(meta.exif).toBeUndefined(); expect(meta.icc).toBeUndefined(); expect(meta.xmp).toBeUndefined();
      expect(image.sourceSha256).toBe(digest(custom)); expect(image.outputSha256).toBe(digest(await readFile(image.path)));
    }
    expect((await sharp(result.images.bubble!.path).metadata()).isPalette).toBe(true);
  });
  it('does not rebuild the VPK for a cover-only customization', async () => {
    const f = await fixture(), source = join(f.root, 'cover.png'); await writeFile(source, await png(300, 400, '#dd2211'));
    const result = await prepare(f.release, { retroflow: { source, fit: 'cover' } });
    expect(result.path).toBe(f.release.vpkPath); expect(result.sha256).toBe(digest(f.original));
    expect(result.images.retroflow!.outputSha256).not.toBe(digest(f.members['retroflow/covers.png']));
  });
  it('letterboxes fit without cropping and makes cover fill the same dimensions', async () => {
    const f = await fixture(), source = join(f.root, 'wide.png'); await writeFile(source, await png(400, 100, '#ff0000'));
    const fit = await prepare(f.release, { bubble: { source, fit: 'contain' } });
    const crop = await prepare(f.release, { bubble: { source, fit: 'cover' } });
    const topFit = await sharp(fit.images.bubble!.path).ensureAlpha().raw().toBuffer();
    const topCrop = await sharp(crop.images.bubble!.path).ensureAlpha().raw().toBuffer();
    expect(topFit[3]).toBe(0); expect([...topCrop.subarray(0, 4)]).toEqual([255, 0, 0, 255]);
  });
  it('uses the selected crop focus and rejects invalid coordinates', async () => {
    const f = await fixture(), source = join(f.root, 'wide.png');
    await writeFile(source, await sharp({ create: { width: 400, height: 100, channels: 4, background: '#ff0000' } })
      .composite([{ input: await png(200, 100, '#0000ff'), left: 200, top: 0 }]).png().toBuffer());
    const left = await prepare(f.release, { bubble: { source, fit: 'cover', focalPoint: { x: 0, y: 0.5 } } });
    const right = await prepare(f.release, { bubble: { source, fit: 'cover', focalPoint: { x: 1, y: 0.5 } } });
    expect([...(await sharp(left.images.bubble!.path).ensureAlpha().raw().toBuffer()).subarray(0, 4)]).toEqual([255, 0, 0, 255]);
    expect([...(await sharp(right.images.bubble!.path).ensureAlpha().raw().toBuffer()).subarray(0, 4)]).toEqual([0, 0, 255, 255]);
    await expect(prepareArtwork(f.release, { bubble: { source, fit: 'cover', focalPoint: { x: NaN, y: 5 } } })).rejects.toMatchObject({ code: 'invalid_artwork_choice' });
  });
  it('refuses symlinked artwork and oversized encoded files', async () => {
    const f = await fixture(), path = join(f.root, 'huge.png'); await writeFile(path, Buffer.alloc(20 * 1024 ** 2 + 1));
    await expect(prepareArtwork(f.release, { bubble: { source: path, fit: 'cover' } })).rejects.toMatchObject({ code: 'invalid_artwork' });
    const linked = join(f.root, 'linked.png'); await symlink(path, linked);
    await expect(prepareArtwork(f.release, { bubble: { source: linked, fit: 'cover' } })).rejects.toThrow();
  });
  it.each(['broken', 'svg', 'animated', 'oversized'])('rejects %s images without changing the release', async kind => {
    const f = await fixture(), source = join(f.root, 'input.png');
    const data = kind === 'broken' ? Buffer.from('not an image')
      : kind === 'svg' ? Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>')
      : kind === 'oversized' ? await png(9000, 5000)
      : await sharp(Buffer.concat([Buffer.alloc(400, 30), Buffer.alloc(400, 190)]),
        { raw: { width: 10, height: 20, channels: 4, pageHeight: 10 } }).webp({ delay: [100, 100] }).toBuffer();
    if (kind === 'animated') expect((await sharp(data).metadata()).pages).toBe(2);
    await writeFile(source, data);
    await expect(prepareArtwork(f.release, { bubble: { source, fit: 'cover' } })).rejects.toMatchObject({ code: 'invalid_artwork' });
    expect(await readFile(f.release.vpkPath!)).toEqual(f.original);
  });
  it('rejects a changed package instead of deriving a new trusted hash', async () => {
    const f = await fixture(); await writeFile(f.release.vpkPath!, await zip({ 'eboot.bin': 'changed' }));
    await expect(prepareArtwork(f.release, {})).rejects.toMatchObject({ code: 'package_hash_mismatch' });
  });
  it('does not start artwork work on a cancelled request', async () => {
    const f = await fixture(); const abort = new AbortController(); abort.abort();
    await expect(prepareArtwork(f.release, {}, abort.signal)).rejects.toMatchObject({ code: 'cancelled' });
  });
  it('holds a held release even when a valid package was previously imported', async () => {
    const f = await fixture(); f.release.manifest.releaseState = 'held';
    await expect(prepareArtwork(f.release, {})).rejects.toMatchObject({ code: 'release_held' });
  });
});
