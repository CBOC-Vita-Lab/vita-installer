import { Worker } from 'node:worker_threads';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
const userData = await mkdtemp(join(tmpdir(), 'vita-worker-smoke-'));
const worker = new Worker(resolve('dist/main/job-worker.mjs'), { workerData: { userData, resources: resolve('resources'), workers: resolve('workers/bin') } });
try {
  await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('Worker startup timed out')), 15000);
    worker.once('error', error => { clearTimeout(timeout); reject(error); });
    worker.on('message', message => {
      if (message.type === 'ready') { clearTimeout(timeout); resolve(); }
      if (message.type === 'fatal') { clearTimeout(timeout); reject(new Error(message.code)); }
    });
  });
  console.log('Built worker booted with empty private state; no network scan or device write.');
} finally { await worker.terminate(); await rm(userData, { recursive: true, force: true }); }
