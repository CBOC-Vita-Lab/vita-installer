import { afterEach, describe, expect, it } from 'vitest';
import { readFile, realpath, rm } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { manifest, digest } from '../fixtures/releases';
import { manifestSchema } from '../../src/releases/manifest';
import { checkStrikersInfo, prepareInputs } from '../../src/preparation/inputs';
import { inventoryTree } from '../../src/preparation/inventory';
import { nodDownload } from '../../src/preparation/tools';
import { runWorker } from '../../src/preparation/runner';
import type { PreparedFile } from '../../src/shared/contracts';
import { hashFile } from '../../src/shared/files';

const { VITA_STRIKERS_EXTRACTION: extraction, VITA_STRIKERS_REFERENCE: reference,
  VITA_STRIKERS_DISC: disc, VITA_NODTOOL: nod } = process.env;
const owned: string[] = [];
afterEach(async () => { for (const path of owned.splice(0)) await rm(path, { recursive: true, force: true }); });
describe('owner-supplied data, read only', () => {
  it.runIf(extraction && disc && nod)('recognizes the real outer disc header with the pinned extractor', async () => {
    const info = await runWorker({ path: nod!, sha256: nodDownload().sha256 }, ['info', disc!], { cwd: await realpath(extraction!) });
    expect(() => checkStrikersInfo(info.stdout)).not.toThrow();
  });
  it.runIf(extraction && reference)('prepares existing data without a copy and matches every retained reference file', async () => {
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
  it.runIf(process.env.VITA_INFECTED_DISC && process.env.VITA_TEST_WORKER)('prepares the real Infected disc without modifying it', async () => {
    const path = process.env.VITA_INFECTED_DISC!, worker = process.env.VITA_TEST_WORKER!;
    const m = manifestSchema.parse({ ...JSON.parse(await readFile('resources/profiles/infected.json', 'utf8')),
      requiredInputs: [{ id: 'game', label: 'Your game', kind: 'infected-disc' }], prerequisites: [] });
    const originalHash = await hashFile(path);
    const release = { manifest: m, manifestSha256: digest(JSON.stringify(m)), sourceRoot: '', vpkPath: null, inventory: null };
    const catalog = JSON.parse(await readFile(join(dirname(worker), 'catalog.json'), 'utf8'));
    const result = await prepareInputs(release, { roots: [], chosen: { game: [path] } }, () => {}, undefined,
      { dataWorker: { path: worker, sha256: catalog.worker.sha256 } });
    owned.push(result.workspace);
    expect(result.files).toHaveLength(1); expect(result.files[0].destination).toBe('ux0:data/infected/Infected.iso');
    expect(result.files[0].sha256).toBe(originalHash); expect(await hashFile(path)).toBe(originalHash);
  }, 120000);
});
