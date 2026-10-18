import { constants } from 'node:fs';
import { open } from 'node:fs/promises';
import sharp from 'sharp';
import { cancelled, InstallerError, requireThat } from '../shared/errors';
import type { ArtworkChoice } from '../shared/contracts';

const MAX_IMAGE_BYTES = 20 * 1024 ** 2;
const MAX_PIXELS = 40_000_000;
const OPTIONS = { limitInputPixels: MAX_PIXELS, failOn: 'warning' as const };

export async function readImage(path: string, signal?: AbortSignal): Promise<Buffer> {
  cancelled(signal);
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const before = await file.stat();
    requireThat(before.isFile() && before.size > 0 && before.size <= MAX_IMAGE_BYTES, 'invalid_artwork');
    const bytes = await file.readFile({ signal });
    const after = await file.stat();
    requireThat(bytes.length === before.size && before.size === after.size && before.mtimeMs === after.mtimeMs, 'source_changed');
    return bytes;
  } finally { await file.close(); }
}

export async function imageSize(bytes: Buffer): Promise<{ width: number; height: number }> {
  try {
    const png = bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]));
    const jpeg = bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255;
    const webp = bytes.toString('ascii', 0, 4) === 'RIFF' && bytes.toString('ascii', 8, 12) === 'WEBP';
    requireThat(bytes.length <= MAX_IMAGE_BYTES && (png || jpeg || webp), 'invalid_artwork');
    // libvips can read an APNG as a still image. Reject its animation control chunk.
    if (png) {
      for (let p = 8; p < bytes.length;) {
        requireThat(p + 12 <= bytes.length, 'invalid_artwork');
        const length = bytes.readUInt32BE(p), type = bytes.toString('ascii', p + 4, p + 8);
        requireThat(length <= bytes.length - p - 12 && type !== 'acTL', 'invalid_artwork');
        p += length + 12;
      }
    }
    const meta = await sharp(bytes, OPTIONS).metadata();
    requireThat((meta.pages ?? 1) === 1 && meta.width && meta.height && meta.width * meta.height <= MAX_PIXELS, 'invalid_artwork');
    return { width: meta.width, height: meta.height };
  } catch { throw new InstallerError('invalid_artwork'); }
}

export async function convertImage(bytes: Buffer, size: { width: number; height: number },
  choice: ArtworkChoice, palette: boolean, signal?: AbortSignal): Promise<Buffer> {
  cancelled(signal); const inputSize = await imageSize(bytes);
  requireThat(choice.fit === 'cover' || choice.fit === 'contain', 'invalid_artwork_choice');
  const focus = choice.focalPoint;
  requireThat(!focus || [focus.x, focus.y].every(v => Number.isFinite(v) && v >= 0 && v <= 1), 'invalid_artwork_choice');
  try {
    let image = sharp(bytes, OPTIONS).autoOrient();
    if (focus && choice.fit === 'cover') {
      const meta = await sharp(bytes, OPTIONS).metadata(), rotated = (meta.orientation ?? 1) >= 5;
      const w = rotated ? inputSize.height : inputSize.width, h = rotated ? inputSize.width : inputSize.height;
      const scale = Math.max(size.width / w, size.height / h);
      const width = Math.min(w, Math.max(1, Math.round(size.width / scale)));
      const height = Math.min(h, Math.max(1, Math.round(size.height / scale)));
      const left = Math.max(0, Math.min(w - width, Math.round(focus.x * w - width / 2)));
      const top = Math.max(0, Math.min(h - height, Math.round(focus.y * h - height / 2)));
      image = image.extract({ left, top, width, height });
    }
    const result = await image.resize({ ...size, fit: choice.fit,
      position: 'centre', background: { r: 0, g: 0, b: 0, alpha: 0 } })
      .toColourspace('srgb').png({ palette, colours: 256, compressionLevel: 9 }).timeout({ seconds: 15 }).toBuffer();
    cancelled(signal); return result;
  } catch (error) {
    cancelled(signal); if (error instanceof InstallerError) throw error;
    throw new InstallerError('invalid_artwork');
  }
}
