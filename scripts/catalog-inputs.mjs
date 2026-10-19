import { createReadStream } from 'node:fs';
import { lstat, readdir, writeFile, mkdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { resolve, join, dirname } from 'node:path';
const [source, reference, output] = process.argv.slice(2);
if (!source || !reference || !output) throw new Error('Expected source, independent reference and output paths');
async function inventory(root) {
  const files = [];
  async function walk(relative) {
    const path = join(root, relative), stat = await lstat(path);
    if (stat.isSymbolicLink()) throw new Error('Symlink in reference');
    if (stat.isDirectory()) { for (const name of (await readdir(path)).sort()) await walk(`${relative}/${name}`); }
    else if (stat.isFile()) {
      const hash = createHash('sha256'); for await (const chunk of createReadStream(path)) hash.update(chunk);
      files.push({ path: relative, bytes: stat.size, sha256: hash.digest('hex') });
    } else throw new Error('Unexpected reference member');
  }
  for (const root of ['sys', 'files']) await walk(root);
  return files.sort((a, b) => a.path < b.path ? -1 : a.path > b.path ? 1 : 0);
}
const files = await inventory(resolve(source)), independent = await inventory(resolve(reference));
if (JSON.stringify(files) !== JSON.stringify(independent)) throw new Error('Reference inventories differ');
if (files.length !== 2359 || files.find(f => f.path === 'sys/main.dol')?.sha256 !== '8f811628ee6646c60620a9c599ef08ed929d2eccc98a779c211847f1500f81fd') throw new Error('Unsupported reference');
await mkdir(dirname(output), { recursive: true });
await writeFile(output, JSON.stringify({ schemaVersion: 1, gameId: 'R4QE01', revision: 1, files }, null, 2) + '\n');
console.log(`Matched ${files.length} reference files; saved metadata only.`);
