import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
const platform = `${process.platform}-${process.arch}`;
if (platform !== 'darwin-arm64') throw new Error('Movie helper needs native-platform validation before packaging.');
const jpeg = process.env.JPEG_TURBO_ROOT ?? '/opt/homebrew/opt/jpeg-turbo';
const config = await readFile(join(jpeg, 'include/jconfig.h'), 'utf8');
if (!/^#define\s+LIBJPEG_TURBO_VERSION\s+3\.1\.4\.1\s*$/m.test(config)) throw new Error('Use the reviewed libjpeg-turbo 3.1.4.1.');
const output = join('workers/bin', platform, 'vita-movies');
await mkdir(join('workers/bin', platform), { recursive: true });
const build = spawnSync('clang++', ['-O2', '-g0', '-std=c++17', '-U__ARM_NEON', '-U__ARM_NEON__', '-ffp-contract=off',
  `-I${join(jpeg, 'include')}`, 'workers/vendor/galaxy/tools/thp_transcode.cpp', 'workers/galaxy-host-log.cpp',
  join(jpeg, 'lib/libjpeg.a'), '-o', output], { stdio: 'inherit' });
if (build.error || build.status !== 0) throw build.error ?? new Error('Movie helper build failed.');
const sha256 = createHash('sha256').update(await readFile(output)).digest('hex');
const recipe = JSON.parse(await readFile('resources/integrity/galaxy-movies-q55-v1.json', 'utf8'));
if (!recipe.workerSha256s.includes(sha256)) throw new Error('New helper build needs full movie-output verification before inclusion.');
console.log(`Verified movie helper: ${platform} ${sha256}`);
