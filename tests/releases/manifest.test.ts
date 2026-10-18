import { describe, it, expect } from 'vitest';
import { generateKeyPairSync, sign } from 'node:crypto';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { authenticateManifest } from '../../src/releases/trust';
import { readSfo } from '../../src/releases/sfo';
import { verifyVpk } from '../../src/releases/manifest';
import { digest, manifest, runtime, sfo, zip } from '../fixtures/releases';

const trust = (bytes: Buffer) => ({ fingerprints: new Set([digest(bytes)]), keys: new Map() });
const bytes = (value: unknown) => Buffer.from(JSON.stringify(value));
describe('trusted release contract', () => {
  it('accepts_reviewed_fingerprint', () => {
    const b = bytes(manifest()); expect(authenticateManifest(b, trust(b)).titleId).toBe('MSCN00003');
  });
  it('rejects_self_signed_or_unknown_manifest', () => {
    const b = bytes(manifest()); expect(() => authenticateManifest(b, { fingerprints: new Set(), keys: new Map() })).toThrow();
  });
  it('accepts_trusted_signature_and_rejects_changed_signed_bytes', () => {
    const { publicKey, privateKey } = generateKeyPairSync('ed25519');
    const raw = bytes(manifest()), envelope = { keyId: 'test', manifest: raw.toString('base64'), signature: sign(null, raw, privateKey).toString('base64') };
    const store = { fingerprints: new Set<string>(), keys: new Map([['test', publicKey]]) };
    expect(authenticateManifest(bytes(envelope), store).releaseId).toBe('strikers-a8d4ad6');
    expect(() => authenticateManifest(bytes({ ...envelope, keyId: 'unknown' }), store)).toThrow();
    envelope.manifest = bytes({ ...manifest(), version: 'changed' }).toString('base64');
    expect(() => authenticateManifest(bytes(envelope), store)).toThrow();
  });
  it('rejects_unknown_schema_recipe_and_executable_fields', () => {
    for (const change of [{ schemaVersion: 2 }, { recipeId: 'shell' }, { script: 'echo bad' }, { minInstallerVersion: '99.0.0' }, { destinations: { data: 'ux0:tai' } }, { package: { ...manifest().package, path: '../runtime.vpk' } }]) {
      const b = bytes({ ...manifest(), ...change }); expect(() => authenticateManifest(b, trust(b))).toThrow();
    }
  });
  it('rejects_duplicate_keys_comments_and_oversized_documents', () => {
    for (const raw of ['{"schemaVersion":1,"schemaVersion":2}', '{/*comment*/"schemaVersion":1}', ' '.repeat(1048577)]) {
      const b = Buffer.from(raw); expect(() => authenticateManifest(b, trust(b))).toThrow();
    }
  });
  it('reads_title_and_rejects_truncated_sfo', () => {
    expect(readSfo(sfo())).toEqual({ titleId: 'MSCN00003' });
    expect(() => readSfo(sfo().subarray(0, 30))).toThrow();
  });
  it('rejects_wrong_title_runtime_or_vpk_hash', async () => {
    const root = await mkdtemp(join(tmpdir(), 'vita-contract-'));
    try {
      const payload = await zip({ 'eboot.bin': runtime, 'sce_sys/param.sfo': sfo() });
      const file = join(root, 'app.vpk'); await writeFile(file, payload);
      const raw = bytes(manifest(digest(payload))), checked = authenticateManifest(raw, trust(raw));
      expect((await verifyVpk(file, checked)).runtimeSha256).toBe(digest(runtime));
      for (const changed of [
        { ...checked, titleId: 'GBAD00001' as const },
        { ...checked, package: { ...checked.package!, sha256: '0'.repeat(64) } },
        { ...checked, package: { ...checked.package!, runtimeSha256: '0'.repeat(64) } }
      ]) await expect(verifyVpk(file, changed)).rejects.toThrow();
    } finally { await rm(root, { recursive: true, force: true }); }
  });
});
