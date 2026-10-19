import { spawn } from 'node:child_process';
import { StringDecoder } from 'node:string_decoder';
import { hashFile } from '../shared/files';
import { cancelled, InstallerError, requireThat } from '../shared/errors';
export interface VerifiedTool { path: string; sha256: string }
export async function runWorker(tool: VerifiedTool, args: string[],
  options: { cwd: string; signal?: AbortSignal; timeoutMs?: number; maxOutputBytes?: number; onStderrLine?: (line: string) => void }): Promise<{ stdout: string; stderr: string }> {
  cancelled(options.signal);
  requireThat(await hashFile(tool.path, options.signal) === tool.sha256, 'tool_hash_mismatch');
  cancelled(options.signal);
  const env: NodeJS.ProcessEnv = { LANG: 'C', LC_ALL: 'C', PYTHONUTF8: '1', PYTHONDONTWRITEBYTECODE: '1' };
  for (const key of ['PATH', 'SystemRoot', 'SYSTEMROOT', 'WINDIR', 'TEMP', 'TMP', 'TMPDIR'])
    if (process.env[key]) env[key] = process.env[key];
  return new Promise((resolve, reject) => {
    const child = spawn(tool.path, args, { cwd: options.cwd, env, shell: false,
      windowsHide: true, detached: process.platform !== 'win32', stdio: ['ignore', 'pipe', 'pipe'] });
    const stdout: Buffer[] = [], stderr: Buffer[] = []; let bytes = 0, failure: string | undefined;
    const decoder = new StringDecoder('utf8'); let pending = '';
    let forceKill: NodeJS.Timeout | undefined;
    const kill = (signal: NodeJS.Signals) => {
      try { if (process.platform !== 'win32' && child.pid) process.kill(-child.pid, signal); else child.kill(signal); }
      catch { /* The child may have exited between the event and signal. */ }
    };
    const stop = (reason: string) => {
      if (failure) return; failure = reason; kill('SIGTERM');
      forceKill = setTimeout(() => kill('SIGKILL'), 1500);
    };
    const abort = () => stop('cancelled');
    const timer = setTimeout(() => stop('worker_timeout'), options.timeoutMs ?? 30 * 60 * 1000);
    const collect = (target: Buffer[], chunk: Buffer) => {
      bytes += chunk.length;
      if (bytes > (options.maxOutputBytes ?? 2 * 1024 ** 2)) stop('worker_output_limit');
      else target.push(chunk);
    };
    child.stdout.on('data', chunk => collect(stdout, chunk));
    child.stderr.on('data', chunk => {
      collect(stderr, chunk);
      if (failure || !options.onStderrLine) return;
      pending += decoder.write(chunk);
      let newline: number;
      while ((newline = pending.indexOf('\n')) >= 0 && !failure) {
        const line = pending.slice(0, newline).replace(/\r$/, ''); pending = pending.slice(newline + 1);
        try { options.onStderrLine(line); } catch { stop('worker_progress_failed'); }
      }
    });
    child.on('error', () => { failure ??= 'worker_start_failed'; });
    options.signal?.addEventListener('abort', abort, { once: true });
    if (options.signal?.aborted) abort();
    child.on('close', code => {
      clearTimeout(timer); clearTimeout(forceKill); options.signal?.removeEventListener('abort', abort);
      if (failure || code !== 0) reject(new InstallerError(failure ?? 'worker_failed'));
      else resolve({ stdout: Buffer.concat(stdout).toString('utf8'), stderr: Buffer.concat(stderr).toString('utf8') });
    });
  });
}
