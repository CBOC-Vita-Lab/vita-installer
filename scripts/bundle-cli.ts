import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { PROFILES } from '../src/releases/manifest';
import { buildInstallerBundle } from '../src/releases/bundle';
import { importRelease } from '../src/releases/import';
import { rm } from 'node:fs/promises';
const pub = process.env.VITA_PUBLISHING_ROOT, destination = process.argv[2];
if (!pub || !destination) throw new Error('Set VITA_PUBLISHING_ROOT and pass a new output directory.');
const output = resolve(destination); await mkdir(output, { recursive: false, mode: 0o700 });
const catalog = JSON.parse(await readFile('resources/profiles/bundles.json', 'utf8'));
const trust = { fingerprints: new Set<string>(JSON.parse(await readFile('resources/profiles/trust.json', 'utf8')).fingerprints), keys: new Map() };
const receipt: Record<string, unknown> = { deviceModified: false, published: false, releaseAccepted: false };
for (const [recipe, profile] of Object.entries(PROFILES)) {
  const config = catalog[recipe], path = join(output, config.output);
  const result = await buildInstallerBundle({ source: join(pub, 'artifacts', profile.repo, config.source), output: path,
    sourceSha256: config.sha256, manifest: await readFile(`resources/profiles/${recipe}.json`), guide: await readFile(`docs/games/${recipe}.md`), trust });
  const imported = await importRelease(path, trust);
  try {
    if (imported.length !== 1 || imported[0].manifest.recipeId !== recipe) throw new Error('Bundle readback mismatch');
    receipt[recipe] = { ...result, file: config.output, imported: true, held: imported[0].manifest.releaseState === 'held' };
    process.stdout.write(`${recipe}: verified, runtime unchanged${recipe === 'galaxy' ? ' (no VPK, held)' : ''}\n`);
  } finally { for (const item of imported) if (item.ownedWorkspace) await rm(item.ownedWorkspace, { recursive: true, force: true }); }
}
await writeFile(join(output, 'receipt.json'), `${JSON.stringify(receipt, null, 2)}\n`, { flag: 'wx', mode: 0o600 });
