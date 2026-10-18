import { createWriteStream } from 'node:fs';
import { mkdtemp, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pipeline } from 'node:stream/promises';
import { PassThrough } from 'node:stream';
import { ZipFile } from 'yazl';
import { XMLParser, XMLValidator } from 'fast-xml-parser';
import type { ArtworkChoice, ArtworkSlot, ImportedRelease, PreparedPackage, PackageInventory } from '../shared/contracts';
import { verifyVpk } from '../releases/manifest';
import { walkZip, PACKAGE_LIMITS } from '../releases/archive';
import { checkedChild, extractArchive } from '../releases/import';
import { cancelled, requireThat } from '../shared/errors';
import { digest, hashFile } from '../shared/files';
import { readImage, imageSize, convertImage } from './images';

const MEMBERS = { bubble: 'sce_sys/icon0.png', livearea: 'sce_sys/livearea/contents/bg.png', retroflow: 'retroflow/covers.png' } as const;
const TEMPLATE = 'sce_sys/livearea/contents/template.xml';

async function defaultImages(path: string, inventory: PackageInventory, signal?: AbortSignal) {
  const wanted = new Set<string>([...Object.values(MEMBERS), TEMPLATE]), result = new Map<string, Buffer>();
  await walkZip(path, PACKAGE_LIMITS, async (entry, stream) => {
    if (!wanted.has(entry.fileName)) { for await (const _ of stream) { cancelled(signal); } return; }
    requireThat(entry.uncompressedSize <= (entry.fileName === TEMPLATE ? 65536 : 20 * 1024 ** 2), 'invalid_artwork');
    const chunks: Buffer[] = []; let bytes = 0;
    for await (const chunk of stream) {
      bytes += chunk.length; requireThat(bytes <= entry.uncompressedSize, 'archive_size_mismatch'); chunks.push(chunk);
    }
    const data = Buffer.concat(chunks);
    requireThat(bytes === entry.uncompressedSize && digest(data) === inventory.members.find(m => m.path === entry.fileName)?.sha256, 'source_changed');
    result.set(entry.fileName, data);
  }, signal);
  return result;
}

function validateTemplate(bytes: Buffer | undefined) {
  requireThat(bytes, 'unsupported_artwork_template');
  const xml = bytes.toString('utf8');
  requireThat(!/<!DOCTYPE|<!ENTITY/i.test(xml) && XMLValidator.validate(xml) === true, 'unsupported_artwork_template');
  const parsed = new XMLParser({ processEntities: false }).parse(xml);
  requireThat(parsed?.livearea?.['livearea-background']?.image === 'bg.png', 'unsupported_artwork_template');
}

async function repack(root: string, destination: string, inventory: PackageInventory,
  replacements: Map<string, Buffer>, signal?: AbortSignal) {
  const zip = new ZipFile(), target = createWriteStream(destination, { flags: 'wx', mode: 0o600 });
  const output = zip.outputStream as PassThrough;
  zip.on('error', error => output.destroy(error));
  const done = pipeline(output, target, { signal });
  // Observe rejection immediately while entries are being assembled.
  void done.catch(() => {});
  try {
    for (const member of inventory.members) {
      cancelled(signal);
      const file = await checkedChild(root, member.path);
      requireThat(await hashFile(file, signal) === member.sha256, 'source_changed');
      const options = { mtime: new Date('2000-01-01T00:00:00Z'), mode: 0o100644 };
      const custom = replacements.get(member.path);
      if (custom) zip.addBuffer(custom, member.path, options); else zip.addFile(file, member.path, options);
    }
    zip.end(); await done;
  } catch (error) { output.destroy(); target.destroy(); await done.catch(() => {}); throw error; }
}

export async function prepareArtwork(release: ImportedRelease,
  choices: Partial<Record<ArtworkSlot, ArtworkChoice>>, signal?: AbortSignal): Promise<PreparedPackage> {
  cancelled(signal);
  const { manifest, vpkPath } = release;
  requireThat(manifest.releaseState === 'ready' && manifest.package && vpkPath, 'release_held');
  requireThat(Object.keys(choices).every(key => Object.hasOwn(MEMBERS, key)), 'invalid_artwork_choice');
  const original = await verifyVpk(vpkPath, manifest, signal);
  const defaults = await defaultImages(vpkPath, original, signal);
  const workspace = await mkdtemp(join(await realpath(tmpdir()), 'vita-artwork-'));
  try {
    const images: PreparedPackage['images'] = {}, replacements = new Map<string, Buffer>();
    for (const slot of ['bubble', 'livearea', 'retroflow'] as const) {
      const defaultBytes = defaults.get(MEMBERS[slot]) ?? (slot === 'retroflow' ? defaults.get(MEMBERS.bubble) : undefined);
      requireThat(defaultBytes, 'unsupported_artwork_template');
      const size = await imageSize(defaultBytes), choice = choices[slot];
      let source = defaultBytes, output = defaultBytes;
      if (choice?.source) {
        if (slot === 'livearea') validateTemplate(defaults.get(TEMPLATE));
        source = await readImage(choice.source, signal);
        output = await convertImage(source, size, choice, slot !== 'retroflow', signal);
        if (slot !== 'retroflow') replacements.set(MEMBERS[slot], output);
      }
      const path = join(workspace, `${slot}.png`); await writeFile(path, output, { flag: 'wx', mode: 0o600 });
      images[slot] = { sourceSha256: digest(source), outputSha256: digest(output), path };
    }
    let path = vpkPath, inventory = original;
    if (replacements.size) {
      const unpacked = await extractArchive(vpkPath, workspace, PACKAGE_LIMITS, signal);
      path = join(workspace, `${manifest.titleId}.vpk`);
      await repack(unpacked, path, original, replacements, signal);
      const sha256 = await hashFile(path, signal);
      inventory = await verifyVpk(path, { ...manifest, package: { ...manifest.package, sha256 } }, signal);
      requireThat(inventory.members.length === original.members.length, 'artwork_package_mismatch');
      for (const member of inventory.members) {
        const expected = replacements.has(member.path) ? digest(replacements.get(member.path)!) : original.members.find(m => m.path === member.path)?.sha256;
        requireThat(member.sha256 === expected, 'artwork_package_mismatch');
      }
      await rm(unpacked, { recursive: true, force: true });
    }
    requireThat(await hashFile(vpkPath, signal) === original.sha256, 'source_changed'); cancelled(signal);
    return { originalSha256: original.sha256, sha256: inventory.sha256, runtimeSha256: inventory.runtimeSha256,
      path, images, inventory, workspace };
  } catch (error) { await rm(workspace, { recursive: true, force: true }); throw error; }
}
