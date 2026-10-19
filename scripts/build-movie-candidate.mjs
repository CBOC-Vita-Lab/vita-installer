// Native validation only. This cannot replace packaged workers or trust pins.
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const platform = `${process.platform}-${process.arch}`;
if (!['darwin-arm64', 'darwin-x64', 'win32-x64'].includes(platform)) throw new Error('Unsupported movie candidate host.');
const cmake = process.env.CMAKE ?? 'cmake';
const version = '3.1.4.1';
const archiveHash = 'ecae8008e2cc9ade2f2c1bb9d5e6d4fb73e7c433866a056bd82980741571a022';
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const cache = join(root, 'out/movie-candidate-build', platform);
const output = join(root, 'out/movie-candidate', platform);
await mkdir(cache, { recursive: true });
await mkdir(output, { recursive: true });
const archive = join(cache, `libjpeg-turbo-${version}.tar.gz`);
let bytes;
try { bytes = await readFile(archive); } catch (error) { if (error.code !== 'ENOENT') throw error; }
if (!bytes) {
  const response = await fetch(`https://github.com/libjpeg-turbo/libjpeg-turbo/releases/download/${version}/libjpeg-turbo-${version}.tar.gz`, { signal: AbortSignal.timeout(60_000) });
  if (!response.ok) throw new Error(`libjpeg-turbo download failed (${response.status}).`);
  bytes = Buffer.from(await response.arrayBuffer());
  if (hash(bytes) !== archiveHash) throw new Error('libjpeg-turbo source hash mismatch.');
  await writeFile(archive, bytes);
}
if (hash(bytes) !== archiveHash) throw new Error('Cached libjpeg-turbo source hash mismatch.');
function run(args, cwd = root) {
  const child = spawnSync(cmake, args, { cwd, stdio: 'inherit', timeout: 600_000 });
  if (child.error || child.status !== 0) throw child.error ?? new Error(`CMake failed (${child.status}).`);
}
run(['-E', 'tar', 'xzf', archive], cache);
const source = join(cache, `libjpeg-turbo-${version}`), jpegBuild = join(cache, 'jpeg');
const generator = process.platform === 'win32' ? ['-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', 'ClangCL'] : ['-G', 'Unix Makefiles', '-DCMAKE_C_COMPILER=clang', '-DCMAKE_CXX_COMPILER=clang++'];
run(['-S', source, '-B', jpegBuild, ...generator, '-DCMAKE_BUILD_TYPE=Release', '-DENABLE_SHARED=OFF',
  '-DENABLE_STATIC=ON', '-DWITH_TURBOJPEG=OFF', '-DWITH_JPEG8=ON', '-DWITH_SIMD=OFF', '-DWITH_CRT_DLL=OFF']);
run(['--build', jpegBuild, '--config', 'Release', '--target', 'jpeg-static', '--parallel', '2']);
const jpegLibrary = process.platform === 'win32' ? join(jpegBuild, 'Release/jpeg-static.lib') : join(jpegBuild, 'libjpeg.a');
run(['-S', join(root, 'workers/movie-candidate'), '-B', join(cache, 'helper'), ...generator,
  '-DCMAKE_BUILD_TYPE=Release', `-DJPEG_SOURCE_DIR=${source}`, `-DJPEG_BUILD_DIR=${jpegBuild}`,
  `-DJPEG_STATIC_LIBRARY=${jpegLibrary}`, `-DCANDIDATE_OUTPUT_DIR=${output}`]);
run(['--build', join(cache, 'helper'), '--config', 'Release', '--parallel', '2']);
const executable = join(output, `vita-movies${process.platform === 'win32' ? '.exe' : ''}`);
const receipt = {
  platform, libjpegVersion: version, libjpegSourceSha256: archiveHash, simd: false,
  workerSha256: hash(await readFile(executable)),
  transcodeSourceSha256: hash(await readFile(join(root, 'workers/vendor/galaxy/tools/thp_transcode.cpp'))),
  decoderSourceSha256: hash(await readFile(join(root, 'workers/vendor/galaxy/platform/src/sdk/thp_video.cpp'))),
  acceptedForPackaging: false,
};
await writeFile(join(output, 'candidate.json'), JSON.stringify(receipt, null, 2) + '\n');
console.log(JSON.stringify(receipt));
