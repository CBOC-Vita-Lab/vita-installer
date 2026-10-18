import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { ZipFile } from 'yazl';
import { walkZip, PACKAGE_LIMITS } from './archive';
import { authenticateManifest, type TrustStore } from './trust';
import { verifyVpk } from './manifest';
import { digest, hashFile } from '../shared/files';
import { requireThat } from '../shared/errors';

// Publisher-only assembly: the inner VPK and every existing source/notice stay intact.
export async function buildInstallerBundle(options: { source: string; output: string; sourceSha256: string; manifest: Buffer; guide: Buffer; trust: TrustStore }) {
  requireThat(await hashFile(options.source) === options.sourceSha256, 'source_changed');
  const manifest = authenticateManifest(options.manifest, options.trust), files = new Map<string, { data: Buffer; mode: number }>();
  await walkZip(options.source, PACKAGE_LIMITS, async (entry, stream) => {
    const chunks: Buffer[] = []; for await (const chunk of stream) chunks.push(chunk);
    files.set(entry.fileName, { data: Buffer.concat(chunks), mode: (entry.externalFileAttributes >>> 16) || 0o100644 });
  });
  for (const name of files.keys()) requireThat(!['cboc-release.json', 'start-installer.md'].includes(name.toLowerCase()), 'bundle_manifest_conflict');
  const vpks = [...files.keys()].filter(name => /\.vpk$/i.test(name) && name !== 'native/platform/vita/Makefile.vpk');
  requireThat(manifest.package ? vpks.length === 1 && vpks[0] === manifest.package.path : vpks.length === 0, 'package_mismatch');
  const temp = await mkdtemp(join(tmpdir(), 'bundle-check-'));
  try {
    if (manifest.package) { const path = join(temp, 'app.vpk'); await writeFile(path, files.get(manifest.package.path)!.data); await verifyVpk(path, manifest); }
    files.set('cboc-release.json', { data: options.manifest, mode: 0o100644 }); files.set('START-INSTALLER.md', { data: options.guide, mode: 0o100644 });
    files.delete('SHA256SUMS');
    files.set('SHA256SUMS', { data: Buffer.from([...files].sort(([a], [b]) => a.localeCompare(b)).map(([name, item]) => `${digest(item.data)}  ${name}\n`).join('')), mode: 0o100644 });
    const archive = new ZipFile(), chunks: Buffer[] = [];
    const done = new Promise<Buffer>((resolve, reject) => {
      archive.outputStream.on('data', chunk => chunks.push(chunk)); archive.outputStream.once('error', reject);
      archive.outputStream.once('end', () => resolve(Buffer.concat(chunks))); archive.once('error', reject);
    });
    for (const [name, item] of [...files].sort(([a], [b]) => a.localeCompare(b))) archive.addBuffer(item.data, name, { mode: item.mode, mtime: new Date('2026-10-18T00:00:00Z') });
    archive.end(); const data = await done;
    requireThat(await hashFile(options.source) === options.sourceSha256, 'source_changed');
    await writeFile(options.output, data, { flag: 'wx', mode: 0o600 });
    return { sha256: digest(data), bytes: data.length, members: files.size, vpkSha256: manifest.package?.sha256 ?? null };
  } finally { await rm(temp, { recursive: true, force: true }); }
}
