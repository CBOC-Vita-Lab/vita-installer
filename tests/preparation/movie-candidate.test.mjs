import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve, join, dirname } from 'node:path';
import { spawnSync } from 'node:child_process';

const root = resolve(import.meta.dirname, '../..');
const fixtures = resolve(root, 'tests/fixtures/movies');
const worker = resolve(process.env.VITA_TEST_MOVIE_WORKER ?? join(root, 'out/movie-candidate', `${process.platform}-${process.arch}`, `vita-movies${process.platform === 'win32' ? '.exe' : ''}`));
const hash = data => createHash('sha256').update(data).digest('hex');
const reference = JSON.parse(await readFile(join(fixtures, 'reference.json')));
const run = args => spawnSync(worker, args, { encoding: 'utf8', timeout: 30_000, env: { ...process.env, PATH: '' } });

test('native candidate source bytes match the reviewed converter and decoder', async () => {
  const receipt = JSON.parse(await readFile(join(dirname(worker), 'candidate.json')));
  assert.equal(receipt.transcodeSourceSha256, reference.transcodeSourceSha256);
  assert.equal(receipt.decoderSourceSha256, reference.decoderSourceSha256);
  assert.equal(receipt.workerSha256, hash(await readFile(worker)));
  assert.equal(receipt.acceptedForPackaging, false);
});

function frames(data) {
  let offset = data.readUInt32LE(0x28), size = data.readUInt32LE(0x18);
  const result = [];
  for (let i = 0; i < data.readUInt32LE(0x14); i++) {
    assert.ok(size >= 16 && size % 32 === 0 && offset + size <= data.length);
    const frame = data.subarray(offset, offset + size);
    const videoSize = frame.readUInt32LE(8), audioSize = frame.readUInt32LE(12);
    assert.ok(16 + videoSize + audioSize <= size);
    result.push({ offset, size, next: frame.readUInt32LE(0), previous: frame.readUInt32LE(4), audio: frame.subarray(16 + videoSize, 16 + videoSize + audioSize) });
    offset += size; size = frame.readUInt32LE(0);
  }
  assert.equal(offset, data.length);
  assert.equal(data.readUInt32LE(0x1c), data.length);
  assert.equal(data.readUInt32LE(0x2c), result.at(-1).offset);
  for (let i = 0; i < result.length; i++) {
    assert.equal(result[i].next, result[(i + 1) % result.length].size);
    assert.equal(result[i].previous, result[(i + result.length - 1) % result.length].size);
  }
  return result;
}

test('native movie candidate exactly matches the reviewed helper on synthetic frames', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'movie comparison '));
  try {
    const input = await readFile(join(fixtures, 'synthetic.thp'));
    const expected = await readFile(join(fixtures, 'synthetic-q55.thp'));
    assert.equal(hash(input), reference.inputSha256);
    assert.equal(hash(expected), reference.outputSha256);
    const output = join(dir, 'converted movie.thp');
    const child = run([join(fixtures, 'synthetic.thp'), output, String(reference.quality)]);
    assert.ifError(child.error); assert.equal(child.status, 0, child.stderr);
    const actual = await readFile(output);
    assert.equal(hash(actual), reference.outputSha256);
    assert.deepEqual(actual, expected);
    const before = frames(input), after = frames(actual);
    assert.equal(after.length, reference.frames);
    assert.deepEqual(after.map(f => f.audio), before.map(f => f.audio));
    assert.equal(actual.readFloatLE(0x10), input.readFloatLE(0x10));
    assert.deepEqual(actual.subarray(48, 96), input.subarray(48, 96));
    assert.equal(hash(await readFile(join(fixtures, 'synthetic.thp'))), reference.inputSha256);
  } finally { await rm(dir, { recursive: true, force: true }); }
});

test('native movie candidate returns failure for missing and truncated inputs', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'movie invalid '));
  try {
    const truncated = join(dir, 'truncated.thp'); await writeFile(truncated, 'THP\0');
    const cases = [
      { args: [], status: 2 },
      { args: [join(dir, 'missing.thp'), join(dir, 'out.thp')], status: 1 },
      { args: [truncated, join(dir, 'out.thp')], status: 1 },
    ];
    for (const { args, status } of cases) {
      const child = run(args); assert.ifError(child.error); assert.equal(child.signal, null); assert.equal(child.status, status);
    }
  } finally { await rm(dir, { recursive: true, force: true }); }
});
