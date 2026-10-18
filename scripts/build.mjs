import { build } from 'esbuild';
import { build as viteBuild } from 'vite';
await build({ entryPoints: ['src/main/index.ts'], bundle: true, platform: 'node', format: 'cjs', packages: 'external', outfile: 'dist/main/index.cjs', sourcemap: false, target: 'node22' });
await build({ entryPoints: ['src/main/job-worker.ts'], bundle: true, platform: 'node', format: 'esm', packages: 'external', outfile: 'dist/main/job-worker.mjs', sourcemap: false, target: 'node22' });
await build({ entryPoints: ['src/main/preload.ts'], bundle: true, platform: 'node', format: 'cjs', external: ['electron'], outfile: 'dist/main/preload.cjs', sourcemap: false, target: 'node22' });
await viteBuild();
