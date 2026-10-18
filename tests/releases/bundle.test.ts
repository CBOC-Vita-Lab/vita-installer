import { afterEach, expect, it } from 'vitest';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { buildInstallerBundle } from '../../src/releases/bundle';
import { manifest, digest, runtime, sfo, zip } from '../fixtures/releases';
import { walkZip, PACKAGE_LIMITS } from '../../src/releases/archive';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
it('adds only the trusted manifest and guide, retains every payload byte and regenerates checksums', async () => {
  const root = await mkdtemp(join(tmpdir(), 'bundle-test-')); roots.push(root);
  const vpk = await zip({ 'eboot.bin': runtime, 'sce_sys/param.sfo': sfo() }), raw = Buffer.from(JSON.stringify(manifest(digest(vpk))));
  const input = await zip({ 'Strikers.vpk': vpk, 'README.md': 'original guide', 'licenses/LICENSE': 'notice', 'native/platform/vita/Makefile.vpk': 'all:\n\tcompile', SHA256SUMS: 'old' });
  const source = join(root, 'original.zip'), output = join(root, 'new.zip'); await writeFile(source, input);
  const opts = { source, output, sourceSha256: digest(input), manifest: raw, guide: Buffer.from('New guide'), trust: { fingerprints: new Set([digest(raw)]), keys: new Map() } };
  await buildInstallerBundle(opts); expect(await readFile(source)).toEqual(input);
  const files = new Map<string, Buffer>(); await walkZip(output, PACKAGE_LIMITS, async (entry, stream) => {
    const chunks = []; for await (const chunk of stream) chunks.push(chunk); files.set(entry.fileName, Buffer.concat(chunks));
  });
  expect(files.get('Strikers.vpk')).toEqual(vpk); expect(files.get('README.md')?.toString()).toBe('original guide');
  expect(files.get('licenses/LICENSE')?.toString()).toBe('notice'); expect(files.get('cboc-release.json')).toEqual(raw);
  expect(files.get('SHA256SUMS')?.toString()).toContain(`${digest(raw)}  cboc-release.json`);
  await expect(buildInstallerBundle(opts)).rejects.toThrow(); expect(await readFile(source)).toEqual(input);
});
