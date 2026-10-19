// Developer-only synthetic images. No game frames or audio are used.
import sharp from 'sharp';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

const root = resolve(import.meta.dirname, '..');
const dir = resolve(root, 'tests/fixtures/movies');
const worker = resolve(process.argv[2] ?? '');
const recipe = JSON.parse(await readFile(resolve(root, 'resources/integrity/galaxy-movies-q55-v1.json')));
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const workerHash = hash(await readFile(worker));
if (!recipe.workerSha256s.includes(workerHash)) throw new Error('Generate reference outputs only with a reviewed movie helper.');

function unstuff(jpeg) {
  let offset = 2;
  while (offset < jpeg.length) {
    if (jpeg[offset] !== 255) throw new Error('Expected a JPEG marker.');
    const marker = jpeg[offset + 1];
    const next = offset + 2 + jpeg.readUInt16BE(offset + 2);
    if (marker === 0xda) {
      const scan = [];
      for (let i = next; i < jpeg.length; i++) {
        scan.push(jpeg[i]);
        if (jpeg[i] === 255 && jpeg[i + 1] === 0) i++;
      }
      return Buffer.concat([jpeg.subarray(0, next), Buffer.from(scan)]);
    }
    offset = next;
  }
  throw new Error('Missing JPEG scan.');
}

const frames = [];
for (let n = 0; n < 12; n++) {
  const pixels = Buffer.alloc(640 * 368 * 3);
  for (let y = 0; y < 368; y++) for (let x = 0; x < 640; x++) {
    const i = (y * 640 + x) * 3;
    const pattern = n % 4;
    if (pattern === 0) pixels.fill([0, 127, 255][Math.floor(n / 4)], i, i + 3);
    if (pattern === 1) { pixels[i] = x % 256; pixels[i + 1] = y % 256; pixels[i + 2] = (x + y) % 256; }
    if (pattern === 2) { const v = ((x >> 3) ^ (y >> 3)) & 1; pixels[i] = v * 255; pixels[i + 1] = (1 - v) * 255; pixels[i + 2] = 127; }
    if (pattern === 3) {
      let seed = (Math.floor(y / 8) * 80 + Math.floor(x / 8) + 1) >>> 0;
      for (let c = 0; c < 3; c++) { seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0; pixels[i + c] = seed >>> 24; }
    }
  }
  const jpeg = unstuff(await sharp(pixels, { raw: { width: 640, height: 368, channels: 3 } })
    .jpeg({ quality: [60, 80, 95][Math.floor(n / 4)], chromaSubsampling: '4:2:0', progressive: false }).toBuffer());
  const video = Buffer.alloc((jpeg.length + 3) & ~3); jpeg.copy(video);
  // Opaque binary audio payload tests that conversion leaves every byte untouched.
  const audio = Buffer.from(Array.from({ length: 512 + n * 32 }, (_, i) => (i + n) & 255));
  const frame = Buffer.alloc((16 + video.length + audio.length + 31) & ~31);
  if (frame.length > 128 * 1024) throw new Error(`Fixture frame ${n} (${frame.length} bytes) exceeds the converter input frame limit.`);
  frame.writeUInt32LE(video.length, 8); frame.writeUInt32LE(audio.length, 12);
  video.copy(frame, 16); audio.copy(frame, 16 + video.length); frames.push(frame);
}
for (let i = 0; i < frames.length; i++) {
  frames[i].writeUInt32LE(frames[(i + 1) % frames.length].length, 0);
  frames[i].writeUInt32LE(frames[(i + frames.length - 1) % frames.length].length, 4);
}
const header = Buffer.alloc(96);
header.write('THP\0'); header.writeUInt32LE(0x11000, 4);
header.writeUInt32LE(Math.max(...frames.map(f => f.length)), 8);
header.writeFloatLE(59.94, 0x10); header.writeUInt32LE(frames.length, 0x14);
header.writeUInt32LE(frames[0].length, 0x18);
header.writeUInt32LE(96 + frames.reduce((sum, f) => sum + f.length, 0), 0x1c);
header.writeUInt32LE(48, 0x20); header.writeUInt32LE(96, 0x28);
header.writeUInt32LE(header.readUInt32LE(0x1c) - frames.at(-1).length, 0x2c);
header.writeUInt32LE(2, 48); header.fill(255, 52, 68); header[52] = 0; header[53] = 1;
header.writeUInt32LE(640, 68); header.writeUInt32LE(368, 72);
header.writeUInt32LE(2, 80); header.writeUInt32LE(32000, 84); header.writeUInt32LE(1, 92);
await mkdir(dir, { recursive: true });
const input = resolve(dir, 'synthetic.thp');
await writeFile(input, Buffer.concat([header, ...frames]));
const output = resolve(dir, 'synthetic-q55.thp');
const child = spawnSync(worker, [input, output, '55'], { encoding: 'utf8', timeout: 30_000, env: { ...process.env, PATH: '' } });
if (child.error || child.status !== 0) throw child.error ?? new Error(child.stderr);
const manifest = {
  provenance: 'Twelve generated flat, gradient, checkerboard and seeded block-noise frames. Audio is a synthetic byte pattern, not playable audio. No game data.',
  recipeId: recipe.recipeId, quality: 55, frames: frames.length, referenceWorkerSha256: workerHash,
  inputSha256: hash(await readFile(input)), outputSha256: hash(await readFile(output)),
  transcodeSourceSha256: hash(await readFile(resolve(root, 'workers/vendor/galaxy/tools/thp_transcode.cpp'))),
  decoderSourceSha256: hash(await readFile(resolve(root, 'workers/vendor/galaxy/platform/src/sdk/thp_video.cpp'))),
  generatorSharpVersion: sharp.versions.sharp,
};
await writeFile(resolve(dir, 'reference.json'), JSON.stringify(manifest, null, 2) + '\n');
console.log(JSON.stringify(manifest));
