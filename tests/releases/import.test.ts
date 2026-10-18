import { beforeEach, afterEach, it, expect } from 'vitest';
import { mkdtemp, mkdir, writeFile, readFile, rm, symlink, readdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { importRelease, extractArchive } from '../../src/releases/import';
import { digest, manifest, runtime, sfo, zip } from '../fixtures/releases';
import { type TrustStore } from '../../src/releases/trust';
let root: string, raw: Buffer, vpk: Buffer, store: TrustStore;
beforeEach(async () => {
  root = await mkdtemp(join(tmpdir(), 'vita-import-'));
  vpk = await zip({ 'eboot.bin': runtime, 'sce_sys/param.sfo': sfo() });
  raw = Buffer.from(JSON.stringify(manifest(digest(vpk))));
  store = { fingerprints: new Set([digest(raw)]), keys: new Map() };
});
afterEach(async () => { await rm(root, { recursive: true, force: true }); });
async function folder(name: string) {
  const path = join(root, name); await mkdir(path, { recursive: true });
  await writeFile(join(path, 'cboc-release.json'), raw); await writeFile(join(path, 'Strikers.vpk'), vpk); return path;
}
it('imports_one_nested_release_without_modifying_originals', async () => {
  const file = join(root, 'release.zip'), payload = await zip({ 'release/cboc-release.json': raw, 'release/Strikers.vpk': vpk });
  await writeFile(file, payload);
  const releases = await importRelease(file, store);
  try {
    expect(releases).toHaveLength(1); expect(releases[0].manifest.displayName).toBe('Strikers');
    expect(releases[0].inventory?.runtimeSha256).toBe(digest(runtime));
    expect(await readFile(file)).toEqual(payload);
  } finally { for (const r of releases) if (r.ownedWorkspace) await rm(r.ownedWorkspace, { recursive: true, force: true }); }
});
it('returns_multiple_choices_from_parent_folder', async () => {
  await folder('one'); await folder('two'); const found = await importRelease(root, store);
  expect(found).toHaveLength(2); expect(found[0].sourceRoot).not.toBe(found[1].sourceRoot);
});
it('supports_parent_containing_release_zips', async () => {
  await writeFile(join(root, 'game.zip'), await zip({ 'cboc-release.json': raw, 'Strikers.vpk': vpk }));
  const found = await importRelease(root, store);
  try { expect(found).toHaveLength(1); } finally { for (const r of found) if (r.ownedWorkspace) await rm(r.ownedWorkspace, { recursive: true, force: true }); }
});
it('rejects_changed_payload_and_mislabeled_non_zip', async () => {
  const dir = await folder('bad'); await writeFile(join(dir, 'Strikers.vpk'), 'wrong');
  await expect(importRelease(dir, store)).rejects.toThrow();
  const fake = join(root, 'fake.zip'); await writeFile(fake, 'not a zip');
  await expect(importRelease(fake, store)).rejects.toThrow();
});
it('rejects_symbolic_links_and_missing_sources', async () => {
  const dir = await folder('linked'); await symlink('/etc', join(dir, 'escape'));
  await expect(importRelease(dir, store)).rejects.toThrow();
  await expect(importRelease(join(root, 'missing'), store)).rejects.toThrow();
});
it('cancels_before_extracting', async () => {
  const dir = await folder('cancelled'), abort = new AbortController(); abort.abort();
  await expect(importRelease(dir, store, abort.signal)).rejects.toThrow('cancelled');
});
it('rejects_case_and_unicode_collisions_without_leaving_output', async () => {
  for (const [a, b] of [['A.txt', 'a.txt'], ['caf\u00e9.txt', 'cafe\u0301.txt'], ['same/file', 'same']]) {
    const file = join(root, 'collision.zip'); await writeFile(file, await zip({ [a]: 'one', [b]: 'two' }));
    const output = join(root, 'output'); await mkdir(output, { recursive: true });
    await expect(extractArchive(file, output, { maxFiles: 10, maxFileBytes: 100, maxTotalBytes: 100 })).rejects.toThrow();
    expect(await readdir(output)).toEqual([]);
  }
});
it('rejects_declared_size_and_entry_budgets', async () => {
  const file = join(root, 'budget.zip'); await writeFile(file, await zip({ a: '12345', b: '12345' }));
  const output = join(root, 'output'); await mkdir(output);
  for (const limits of [{ maxFiles: 1, maxFileBytes: 100, maxTotalBytes: 100 }, { maxFiles: 10, maxFileBytes: 4, maxTotalBytes: 100 }, { maxFiles: 10, maxFileBytes: 100, maxTotalBytes: 9 }]) {
    await expect(extractArchive(file, output, limits)).rejects.toThrow(); expect(await readdir(output)).toEqual([]);
  }
});
it('rejects_windows_paths_and_control_characters', async () => {
  for (const name of ['CON', 'path/a:b', 'a\nfile', 'trailing.']) {
    // yazl itself rejects drive paths; these portable-name cases reach the importer.
    const file = join(root, 'unsafe.zip'); await writeFile(file, await zip({ [name]: 'bad' }));
    await expect(importRelease(file, store)).rejects.toThrow();
  }
});
it('rejects_unknown_manifest_without_executing_supplied_scripts', async () => {
  const file = join(root, 'untrusted.zip');
  await writeFile(file, await zip({ 'cboc-release.json': '{}', 'setup.js': 'throw new Error("never run")' }));
  await expect(importRelease(file, store)).rejects.toThrow();
});
it('rejects_traversal_absolute_unc_and_drive_paths', async () => {
  for (const name of ['../escape', '/absolute', '\\\\host\\file', 'C:/escape']) {
    const replacement = Buffer.from(name), payload = await zip({ ['a'.repeat(replacement.length)]: 'x' });
    const central = payload.indexOf(Buffer.from([0x50, 0x4b, 0x01, 0x02]));
    replacement.copy(payload, 30); replacement.copy(payload, central + 46);
    const path = join(root, 'traversal.zip'); await writeFile(path, payload);
    await expect(importRelease(path, store)).rejects.toThrow();
    expect(await readdir(root)).toEqual(['traversal.zip']);
  }
});
it('rejects_encrypted_and_symlink_members', async () => {
  for (const kind of ['encrypted', 'symlink']) {
    const payload = await zip({ file: 'x' }), central = payload.indexOf(Buffer.from([0x50, 0x4b, 0x01, 0x02]));
    if (kind === 'encrypted') {
      payload.writeUInt16LE(payload.readUInt16LE(6) | 1, 6);
      payload.writeUInt16LE(payload.readUInt16LE(central + 8) | 1, central + 8);
    } else payload.writeUInt32LE((0o120777 * 65536) >>> 0, central + 38);
    const file = join(root, 'unsupported.zip'); await writeFile(file, payload);
    await expect(importRelease(file, store)).rejects.toThrow();
  }
});
it('rejects_100000_entry_zip64_bomb_before_writing', async () => {
  const payload = await zip({ file: 'x' }, true);
  const record = payload.indexOf(Buffer.from([0x50, 0x4b, 0x06, 0x06])); expect(record).toBeGreaterThan(0);
  payload.writeBigUInt64LE(100000n, record + 24); payload.writeBigUInt64LE(100000n, record + 32);
  const file = join(root, 'bomb.zip'); await writeFile(file, payload);
  await expect(importRelease(file, store)).rejects.toThrow('archive_entry_limit');
});
it('accepts_one_nested_wrapper_and_rejects_deeper_archives', async () => {
  let payload = await zip({ 'cboc-release.json': raw, 'Strikers.vpk': vpk });
  payload = await zip({ 'inner.zip': payload }); const file = join(root, 'nested.zip'); await writeFile(file, payload);
  const found = await importRelease(file, store);
  try { expect(found).toHaveLength(1); } finally { for (const r of found) if (r.ownedWorkspace) await rm(r.ownedWorkspace, { recursive: true, force: true }); }
  await writeFile(file, await zip({ 'outer.zip': payload }));
  await expect(importRelease(file, store)).rejects.toThrow('archive_depth_limit');
});
