import { afterEach, expect, it } from 'vitest';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { writeWorkerCatalog, packageWorkerSelection, verifyCandidateReceipt } from '../../src/packaging/worker-catalog';

const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function fixture(platform: string) {
  const root = await mkdtemp(join(tmpdir(), 'worker-catalog-')); roots.push(root);
  const output = join(root, 'output'), source = join(root, 'source'), exe = platform.startsWith('win32-') ? '.exe' : '';
  await mkdir(join(output, 'vita-volume'), { recursive: true }); await mkdir(source);
  for (const name of [`vita-prepare${exe}`, `vita-volume/vita-volume${exe}`, `vita-movies${exe}`]) await writeFile(join(output, name), 'abc');
  await writeFile(join(source, 'prepare.py'), 'abc');
  for (const name of ['bin', 'build', '__pycache__']) {
    await mkdir(join(source, name)); await writeFile(join(source, name, 'not-source'), 'excluded');
  }
  return { root, output, source };
}
it.each(['darwin-arm64', 'darwin-x64', 'win32-x64'])('writes the actual native executable names and hashes for %s', async platform => {
  const f = await fixture(platform), exe = platform.startsWith('win32-') ? '.exe' : '';
  await writeWorkerCatalog(f.output, f.source, platform);
  const result = JSON.parse(await readFile(join(f.output, 'catalog.json'), 'utf8'));
  const expectedHash = 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad';
  expect(result).toEqual({ schemaVersion: 1, platform,
    worker: { path: `vita-prepare${exe}`, sha256: expectedHash },
    volumeWorker: { path: `vita-volume/vita-volume${exe}`, sha256: expectedHash },
    movieWorker: { path: `vita-movies${exe}`, sha256: expectedHash },
    sources: [{ path: 'prepare.py', sha256: expectedHash }] });
});
it('does not write a successful catalog for missing helpers or unsupported targets', async () => {
  const f = await fixture('win32-x64');
  await rm(join(f.output, 'vita-movies.exe'));
  await expect(writeWorkerCatalog(f.output, f.source, 'win32-x64')).rejects.toThrow();
  await expect(readFile(join(f.output, 'catalog.json'))).rejects.toThrow();
  await expect(writeWorkerCatalog(f.output, f.source, 'linux-x64')).rejects.toThrow();
});
it('keeps native candidates opt-in, isolated and tied to their catalog', async () => {
  const f = await fixture('darwin-arm64');
  expect(await packageWorkerSelection(f.root, 'darwin-arm64', [])).toEqual({ root: join(f.root, 'workers/bin/darwin-arm64') });
  await expect(packageWorkerSelection(f.root, 'darwin-arm64', ['--unknown'])).rejects.toThrow();
  await expect(packageWorkerSelection(f.root, 'darwin-arm64', ['--native-candidate'])).rejects.toThrow();
  const candidate = join(f.root, 'out/native-package-workers/darwin-arm64');
  await mkdir(candidate, { recursive: true }); await writeFile(join(candidate, 'catalog.json'), 'catalog');
  const receipt = { schemaVersion: 1, platform: 'darwin-arm64', acceptedForRelease: false,
    catalogSha256: createHash('sha256').update('catalog').digest('hex') };
  await writeFile(join(candidate, 'native-candidate.json'), JSON.stringify(receipt));
  expect(await packageWorkerSelection(f.root, 'darwin-arm64', ['--native-candidate'])).toEqual({ root: candidate, candidate: receipt });
  await writeFile(join(candidate, 'catalog.json'), 'changed');
  await expect(packageWorkerSelection(f.root, 'darwin-arm64', ['--native-candidate'])).rejects.toThrow();
  expect(() => verifyCandidateReceipt({ ...receipt, acceptedForRelease: true }, 'darwin-arm64', receipt.catalogSha256)).toThrow();
  expect(() => verifyCandidateReceipt(receipt, 'win32-x64', receipt.catalogSha256)).toThrow();
});
