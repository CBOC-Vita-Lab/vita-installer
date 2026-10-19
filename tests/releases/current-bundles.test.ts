import { afterEach, expect, it } from 'vitest';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { manifestSchema, PROFILES } from '../../src/releases/manifest';
import { importRelease } from '../../src/releases/import';
import { digest } from '../../src/shared/files';
import { zip } from '../fixtures/releases';
const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
const catalog = async () => JSON.parse(await readFile('resources/profiles/trust.json', 'utf8')) as { fingerprints: string[] };
it('ships four hash-pinned data-only profiles without any owner path or download credential', async () => {
  const trust = await catalog();
  for (const recipe of Object.keys(PROFILES)) {
    const raw = await readFile(`resources/profiles/${recipe}.json`), profile = manifestSchema.parse(JSON.parse(raw.toString()));
    expect(trust.fingerprints).toContain(digest(raw)); expect(profile.recipeId).toBe(recipe); expect(profile.visibility).toBe(recipe === 'strikers' ? 'public' : 'private');
    expect(raw.toString()).not.toMatch(/\/Users\/|\/home\/|github_pat_|ghp_|token=|password/i);
    if (recipe === 'galaxy') { expect(profile.releaseState).toBe('held'); expect(profile.package).toBeNull(); }
    else expect(profile.package?.runtimeSha256).toMatch(/^[a-f0-9]{64}$/);
  }
});
it('never substitutes a package just because its title and filename look right', async () => {
  const raw = await readFile('resources/profiles/strikers.json'), root = await mkdtemp(join(tmpdir(), 'profile-check-')); roots.push(root);
  const m = manifestSchema.parse(JSON.parse(raw.toString())); await writeFile(join(root, 'cboc-release.json'), raw); await writeFile(join(root, m.package!.path), 'older validation build');
  await expect(importRelease(root, { fingerprints: new Set((await catalog()).fingerprints), keys: new Map() })).rejects.toMatchObject({ code: 'package_hash_mismatch' });
});
it('declares Galaxy saves, settings and shader prerequisites before it can be enabled', async () => {
  const profile = manifestSchema.parse(JSON.parse(await readFile('resources/profiles/galaxy.json', 'utf8')));
  expect(profile.protectedPaths).toEqual(expect.arrayContaining([
    'ux0:data/smg/saves', 'ux0:data/smg/settings.ini', 'ux0:data/smg/env.txt', 'ux0:data/smg/shadercache',
  ]));
  expect(profile.prerequisites).toContain('shader-compiler');
  expect(profile.requiredInputs).toContainEqual({ id: 'shader', label: 'Shader compiler (libshacccg.suprx)', kind: 'shader-compiler' });
});
it.skipIf(!process.env.VITA_PUBLISHING_ROOT)('imports all selected real packages and held Galaxy through the actual ZIP pipeline', async () => {
  const pub = process.env.VITA_PUBLISHING_ROOT!, candidates = JSON.parse(await readFile(join(pub, 'audit/installer-candidates-20261018.json'), 'utf8'));
  const root = await mkdtemp(join(tmpdir(), 'current-bundles-')); roots.push(root);
  for (const recipe of Object.keys(PROFILES) as (keyof typeof PROFILES)[]) {
    const raw = await readFile(`resources/profiles/${recipe}.json`), profile = manifestSchema.parse(JSON.parse(raw.toString()));
    const members: Record<string, Buffer> = { 'cboc-release.json': raw };
    if (profile.package) {
      const audit = candidates.projects[PROFILES[recipe].repo];
      expect(profile.package.sha256).toBe(audit.sha256); expect(profile.package.runtimeSha256).toBe(audit.runtime_sha256);
      members[profile.package.path] = await readFile(join(pub, audit.path));
    }
    const archive = join(root, `${recipe}.zip`); await writeFile(archive, await zip(members));
    const [release] = await importRelease(archive, { fingerprints: new Set((await catalog()).fingerprints), keys: new Map() });
    if (release.ownedWorkspace) roots.push(release.ownedWorkspace);
    expect(release.manifest.recipeId).toBe(recipe);
    if (profile.package) expect(release.inventory?.runtimeSha256).toBe(profile.package.runtimeSha256); else expect(release.vpkPath).toBeNull();
  }
});
