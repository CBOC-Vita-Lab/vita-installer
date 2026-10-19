import { spawn } from 'node:child_process';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { build } from 'esbuild';
await build({ entryPoints: ['src/packaging/application.ts'], bundle: true, platform: 'node', format: 'esm', outfile: 'dist/tools/application.mjs' });
const { packagedApplication, smokeEnvironment } = await import('../dist/tools/application.mjs');
const platform = `${process.platform}-${process.arch}`;
const { resources, executable } = packagedApplication(process.cwd(), platform, process.argv[2]);
const root = await mkdtemp(join(tmpdir(), 'vita-packaged-smoke-'));
const bootstrap = `
const { Worker } = require('node:worker_threads');
const { join } = require('node:path');
const root = process.argv[2], resources = process.argv[3], bundle = process.argv[4];
const worker = new Worker(join(resources, 'app/dist/main/job-worker.mjs'), { workerData: {
  userData: join(root, 'state'), resources: join(resources, 'installer-resources'), workers: join(resources, 'installer-workers') } });
let state, id = 0, calls = new Map();
const command = request => new Promise((resolve, reject) => { const next = ++id; calls.set(next, { resolve, reject }); worker.postMessage({ id: next, ...request }); });
const timeout = setTimeout(() => { process.stderr.write('Packaged worker timed out\\n'); process.exit(1); }, 30000);
worker.on('error', error => { process.stderr.write(error.name + '\\n'); process.exit(1); });
worker.on('message', async message => {
  if (message.type === 'snapshot') state = message.state;
  else if (message.type === 'fatal') { process.stderr.write(message.code + '\\n'); process.exit(1); }
  else if (message.id) { const call = calls.get(message.id); calls.delete(message.id); message.type === 'done' ? call.resolve() : call.reject(new Error(message.code)); }
  else if (message.type === 'ready') {
    try {
      if (bundle) {
        await command({ request: { command: 'import', kind: 'file' }, paths: [bundle] });
        if (state.error || state.releases.length !== 1 || state.artwork.length !== 3) throw new Error(state.error || 'Import readback failed');
        await command({ request: { command: 'prepare' } });
        if (state.error !== 'device_not_ready') throw new Error('Missing-device gate failed');
      }
      await command({ dispose: true }); await worker.terminate(); clearTimeout(timeout);
      process.stdout.write(JSON.stringify({ packagedWorker: true, bundleImported: !!bundle, artworkPreviews: state?.artwork.length ?? 0, emptyPath: process.env.PATH === '', deviceWrites: false }) + '\\n');
    } catch (error) { process.stderr.write(error.message + '\\n'); process.exit(1); }
  }
});`;
try {
  const script = join(root, 'bootstrap.cjs'); await writeFile(script, bootstrap);
  const args = [script, root, resources]; if (process.argv[3]) args.push(resolve(process.argv[3]));
  const child = spawn(executable, args, { env: smokeEnvironment(root, platform, process.env), stdio: ['ignore', 'pipe', 'pipe'] });
  let output = '', error = ''; child.stdout.on('data', chunk => { output += chunk; }); child.stderr.on('data', chunk => { error += chunk; });
  await new Promise((resolve, reject) => { child.once('error', reject); child.once('close', code => code === 0 ? resolve() : reject(new Error(`Packaged smoke failed (${code}): ${error.slice(0, 1500)}`))); });
  const receipt = JSON.parse(output.trim()); await writeFile('out/packaged-smoke.json', JSON.stringify(receipt, null, 2) + '\n'); console.log(output.trim());
} finally { await rm(root, { recursive: true, force: true }); }
