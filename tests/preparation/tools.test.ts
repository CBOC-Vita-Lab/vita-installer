import { afterEach, expect, it } from 'vitest';
import { mkdtemp, readFile, readdir, rm, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { obtainTool } from '../../src/preparation/tools';
import { digest } from '../fixtures/releases';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function cache() { const root = await mkdtemp(join(tmpdir(), 'tool-test-')); roots.push(root); return root; }
const spec = { name: 'nodtool-macos-arm64', url: 'https://github.com/encounter/nod/releases/download/v2.0.0-alpha.12/nodtool-macos-arm64', sha256: digest('valid tool') };
it('only installs a completed, checksum-verified download and rechecks a cached file', async () => {
  const dir = await cache(); const tool = await obtainTool(spec, dir, undefined, async () => new Response('valid tool'));
  expect(await readFile(tool.path, 'utf8')).toBe('valid tool'); expect(tool.sha256).toBe(spec.sha256);
  await writeFile(tool.path, 'changed');
  const repaired = await obtainTool(spec, dir, undefined, async () => new Response('valid tool'));
  expect(await readFile(repaired.path, 'utf8')).toBe('valid tool');
});
it('keeps a damaged cache intact on failed repair and never follows a cache symlink', async () => {
  const dir = await cache(), path = join(dir, spec.name); await writeFile(path, 'damaged cache');
  await expect(obtainTool(spec, dir, undefined, async () => new Response('bad'))).rejects.toMatchObject({ code: 'tool_hash_mismatch' });
  expect(await readFile(path, 'utf8')).toBe('damaged cache'); expect(await readdir(dir)).toEqual([spec.name]);
  const outside = join(await cache(), 'outside'); await writeFile(outside, 'private'); await rm(path); await symlink(outside, path);
  await expect(obtainTool(spec, dir, undefined, async () => new Response('valid tool'))).rejects.toMatchObject({ code: 'unsafe_tool_cache' });
  expect(await readFile(outside, 'utf8')).toBe('private');
});
it('cleans partial downloads on a checksum mismatch, network failure or cancellation', async () => {
  const dir = await cache();
  await expect(obtainTool(spec, dir, undefined, async () => new Response('bad'))).rejects.toMatchObject({ code: 'tool_hash_mismatch' });
  expect(await readdir(dir)).toEqual([]);
  await expect(obtainTool(spec, dir, undefined, async () => new Response('bad', { status: 500 }))).rejects.toMatchObject({ code: 'tool_download_failed' });
  expect(await readdir(dir)).toEqual([]);
  const abort = new AbortController(); abort.abort();
  await expect(obtainTool(spec, dir, abort.signal)).rejects.toMatchObject({ code: 'cancelled' });
});
it('refuses non-allowlisted origins and cache traversal', async () => {
  const dir = await cache();
  await expect(obtainTool({ ...spec, url: 'http://localhost/tool' }, dir)).rejects.toMatchObject({ code: 'untrusted_tool_url' });
  await expect(obtainTool({ ...spec, name: '../outside' }, dir)).rejects.toMatchObject({ code: 'unsafe_tool_name' });
});
