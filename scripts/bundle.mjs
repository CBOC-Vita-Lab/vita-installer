import { build } from 'esbuild';
await build({ entryPoints: ['scripts/bundle-cli.ts'], bundle: true, platform: 'node', format: 'esm', packages: 'external', outfile: 'dist/tools/bundle.mjs' });
await import('../dist/tools/bundle.mjs');
