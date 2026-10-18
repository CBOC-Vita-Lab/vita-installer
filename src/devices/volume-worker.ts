import { spawn } from 'node:child_process';
import { Readable } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { tmpdir } from 'node:os';
import { hashFile } from '../shared/files';
import { cancelled, InstallerError, requireThat } from '../shared/errors';
import type { VolumeWorker } from './transport';

export async function volumeOperation(worker: VolumeWorker | undefined, args: string[],
  data?: AsyncIterable<Uint8Array>, signal?: AbortSignal) {
  requireThat(worker, 'volume_worker_missing'); cancelled(signal);
  requireThat(await hashFile(worker.path, signal) === worker.sha256, 'tool_hash_mismatch');
  const child = spawn(worker.path, [...(worker.args ?? []), ...args], { shell: false, cwd: tmpdir(),
    env: { LANG: 'C', ...(process.platform === 'win32' ? { SystemRoot: process.env.SystemRoot } : {}) },
    stdio: ['pipe', 'pipe', 'ignore'] });
  let output = '', startError: unknown, pipeError: unknown, timedOut = false, kill: NodeJS.Timeout | undefined;
  const stop = () => { child.kill('SIGTERM'); kill ??= setTimeout(() => child.kill('SIGKILL'), 1500); };
  const timer = setTimeout(() => { timedOut = true; stop(); }, 30 * 60 * 1000);
  signal?.addEventListener('abort', stop, { once: true });
  child.stdout.on('data', chunk => { output += chunk.toString(); if (output.length > 4096) stop(); });
  const closed = new Promise<number | null>(resolve => {
    child.on('error', e => { startError = e; }); child.on('close', code => resolve(code));
  });
  try {
    const input = pipeline(Readable.from(data ?? []), child.stdin).catch(e => { pipeError = e; stop(); });
    const code = await closed; await input;
    cancelled(signal); if (startError) throw startError;
    requireThat(!timedOut, 'volume_write_timeout');
    if (pipeError instanceof InstallerError) throw pipeError;
    let result: { error?: string };
    try { result = JSON.parse(output); } catch { throw pipeError ?? new InstallerError('volume_worker_failed'); }
    if (result.error) throw new InstallerError(result.error);
    requireThat(code === 0, 'volume_worker_failed'); if (pipeError) throw pipeError;
  } finally { clearTimeout(timer); if (kill) clearTimeout(kill); signal?.removeEventListener('abort', stop); }
}
