import { afterEach, describe, expect, it } from 'vitest';
import { realpath, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { manifest, digest } from '../fixtures/releases';
import { manifestSchema } from '../../src/releases/manifest';
import { checkStrikersInfo, prepareInputs } from '../../src/preparation/inputs';
import { inventoryTree } from '../../src/preparation/inventory';
import { nodDownload } from '../../src/preparation/tools';
import { runWorker } from '../../src/preparation/runner';
import type { PreparedFile } from '../../src/shared/contracts';

const { VITA_STRIKERS_EXTRACTION: extraction, VITA_STRIKERS_REFERENCE: reference,
  VITA_STRIKERS_DISC: disc, VITA_NODTOOL: nod } = process.env;
const owned: string[] = [];
afterEach(async () => { for (const path of owned.splice(0)) await rm(path, { recursive: true, force: true }); });
describe.runIf(extraction && reference && disc && nod)('owner-supplied Strikers data, read only', () => {
  it('recognizes the real outer disc header with the pinned extractor', async () => {
    const info = await runWorker({ path: nod!, sha256: nodDownload().sha256 }, ['info', disc!], { cwd: await realpath(extraction!) });
    expect(() => checkStrikersInfo(info.stdout)).not.toThrow();
  });
  it('prepares existing data without a copy and matches every retained reference file', async () => {
    const m = manifestSchema.parse(manifest());
    const release = { manifest: m, manifestSha256: digest(JSON.stringify(m)), sourceRoot: '', vpkPath: null, inventory: null };
    const result = await prepareInputs(release, { roots: [], chosen: { game: [extraction!] } }, () => {});
    owned.push(result.workspace);
    const expected: PreparedFile[] = [];
    for (const dir of ['sys', 'files']) expected.push(...await inventoryTree(join(reference!, dir), `${m.destinations.data}/${dir}`, m.limits));
    const comparable = (files: typeof expected) => files.map(f => [f.destination, f.bytes, f.sha256]).sort();
    expect(result.files.length).toBe(2359); expect(comparable(result.files)).toEqual(comparable(expected));
    for (const file of result.files) expect(file.source.startsWith(await realpath(extraction!))).toBe(true);
  }, 120000);
});
