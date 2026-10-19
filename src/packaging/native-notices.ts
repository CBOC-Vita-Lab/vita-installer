import { createHash } from 'node:crypto';
import { lstat, readFile, realpath } from 'node:fs/promises';
import { join, sep } from 'node:path';
import { z } from 'zod';

const digest = z.string().regex(/^[a-f0-9]{64}$/);
const manifestSchema = z.object({
  schemaVersion: z.literal(1), packageVersion: z.string().min(1), buildRecipeCommit: z.string().regex(/^[a-f0-9]{40}$/),
  readmeSha256: digest,
  packages: z.record(z.string().regex(/^@img\/sharp-(?:libvips-darwin-(?:arm64|x64)|win32-x64)$/), z.string().min(1)),
  completeRedistributionReview: z.literal(false), unresolved: z.array(z.string().min(1)).min(1),
  components: z.array(z.object({
    name: z.string().regex(/^[a-z0-9-]+$/), version: z.string().min(1),
    source: z.object({ url: z.string().url().startsWith('https://'), sha256: digest,
      sourceType: z.string().optional(), tag: z.string().optional(), tagObject: z.string().optional(),
      commit: z.string().optional(), tree: z.string().optional() }).strict(),
    notices: z.array(z.object({ path: z.string(), sourceMember: z.string().min(1),
      resolvedMember: z.string().optional(), sha256: digest }).strict()).min(1),
  }).strict()).min(1),
}).strict();

export async function verifyNativeNotices(root: string, packageName: string, packageVersion: string, installedVersions: Record<string, string>, expectedManifestSha256?: string) {
  const bytes = await readFile(join(root, 'manifest.json'));
  if (expectedManifestSha256 !== undefined && createHash('sha256').update(bytes).digest('hex') !== digest.parse(expectedManifestSha256))
    throw new Error('Native notice manifest differs from the reviewed source.');
  const manifest = manifestSchema.parse(JSON.parse(bytes.toString('utf8')));
  if (manifest.packages[packageName] !== packageVersion) throw new Error('Native notice package version needs review.');
  const components = new Map(manifest.components.map(c => [c.name, c]));
  if (components.size !== manifest.components.length) throw new Error('Duplicate native notice component.');
  if (Object.keys(installedVersions).length !== components.size) throw new Error('Native notice component inventory needs review.');
  for (const [name, version] of Object.entries(installedVersions)) {
    if (components.get(name)?.version !== version) throw new Error(`Native notice version needs review: ${name}`);
  }
  const realRoot = await realpath(root), seen = new Set<string>();
  async function verifyFile(relative: string, expected: string) {
    const path = join(root, relative), info = await lstat(path);
    if (!info.isFile() || info.isSymbolicLink() || !(await realpath(path)).startsWith(realRoot + sep))
      throw new Error('Native notice must be a contained regular file.');
    if (createHash('sha256').update(await readFile(path)).digest('hex') !== expected)
      throw new Error(`Native notice bytes changed: ${relative}`);
  }
  await verifyFile('README.md', manifest.readmeSha256);
  for (const component of manifest.components) for (const notice of component.notices) {
    if (!/^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(notice.path) || !notice.path.startsWith(component.name + '/')
        || notice.path.split('/').some(part => !part || part === '.' || part === '..') || seen.has(notice.path.toLowerCase()))
      throw new Error('Unsafe or duplicate native notice path.');
    seen.add(notice.path.toLowerCase());
    await verifyFile(notice.path, notice.sha256);
  }
  return manifest;
}

export async function verifyInstalledSharpNotices(root: string, modulesRoot: string, platform: string, expectedManifestSha256: string) {
  digest.parse(expectedManifestSha256);
  const packages: Record<string, string> = {
    'darwin-arm64': '@img/sharp-libvips-darwin-arm64',
    'darwin-x64': '@img/sharp-libvips-darwin-x64',
    'win32-x64': '@img/sharp-win32-x64',
  };
  const name = packages[platform];
  if (!name) throw new Error('Native notice platform needs review.');
  const packageRoot = join(modulesRoot, name);
  const pkg = z.object({ name: z.literal(name), version: z.string().min(1) })
    .parse(JSON.parse(await readFile(join(packageRoot, 'package.json'), 'utf8')));
  const versions = z.record(z.string(), z.string().min(1))
    .parse(JSON.parse(await readFile(join(packageRoot, 'versions.json'), 'utf8')));
  return verifyNativeNotices(root, name, pkg.version, versions, expectedManifestSha256);
}
