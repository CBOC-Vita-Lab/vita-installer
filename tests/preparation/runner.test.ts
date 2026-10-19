import { afterEach, beforeAll, expect, it } from 'vitest';
import { mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { runWorker, type VerifiedTool } from '../../src/preparation/runner';
import { hashFile } from '../../src/shared/files';
let tool: VerifiedTool;
const roots: string[] = [];
beforeAll(async () => { const path = await realpath(process.execPath); tool = { path, sha256: await hashFile(path) }; });
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
async function work() { const root = await mkdtemp(join(tmpdir(), 'worker-test-')); roots.push(root); return root; }
it('uses exact argument boundaries and does not inherit credentials', async () => {
  const old = process.env.ANTHROPIC_API_KEY; process.env.ANTHROPIC_API_KEY = 'test-secret';
  try {
    const result = await runWorker(tool, ['-e', 'console.log(JSON.stringify([process.argv[1],process.env.ANTHROPIC_API_KEY]))', 'a; touch no'], { cwd: await work() });
    expect(JSON.parse(result.stdout)).toEqual(['a; touch no', null]);
  } finally { if (old === undefined) delete process.env.ANTHROPIC_API_KEY; else process.env.ANTHROPIC_API_KEY = old; }
});
it('refuses an executable checksum mismatch before any child is started', async () => {
  await expect(runWorker({ ...tool, sha256: '0'.repeat(64) }, ['-e', 'console.log(1)'], { cwd: await work() })).rejects.toMatchObject({ code: 'tool_hash_mismatch' });
});
it('does not treat partial output as success when a worker exits nonzero', async () => {
  const cwd = await work();
  await expect(runWorker(tool, ['-e', 'require("fs").writeFileSync("partial","not ready");process.exit(1)'], { cwd })).rejects.toMatchObject({ code: 'worker_failed' });
  expect(await readFile(join(cwd, 'partial'), 'utf8')).toBe('not ready');
});
it('cancels a child and waits for it to stop before reporting cancellation', async () => {
  const controller = new AbortController(), cwd = await work();
  const task = runWorker(tool, ['-e', 'setInterval(()=>{},100)'], { cwd, signal: controller.signal });
  setTimeout(() => controller.abort(), 100);
  await expect(task).rejects.toMatchObject({ code: 'cancelled' });
});
it('bounds execution time and output size', async () => {
  const cwd = await work();
  await expect(runWorker(tool, ['-e', 'setInterval(()=>{},100)'], { cwd, timeoutMs: 30 })).rejects.toMatchObject({ code: 'worker_timeout' });
  await expect(runWorker(tool, ['-e', 'console.log("x".repeat(4096))'], { cwd, maxOutputBytes: 50 })).rejects.toMatchObject({ code: 'worker_output_limit' });
});
it('streams complete stderr lines before exit without changing the final receipt', async () => {
  const lines: string[] = [], abort = new AbortController();
  const task = runWorker(tool, ['-e', 'process.stderr.write("first\\npar");setTimeout(()=>process.stderr.write("tial\\n"),25);setInterval(()=>{},100)'], {
    cwd: await work(), signal: abort.signal, onStderrLine: line => { lines.push(line); if (line === 'partial') abort.abort(); }, timeoutMs: 3000,
  });
  await expect(task).rejects.toMatchObject({ code: 'cancelled' });
  expect(lines).toEqual(['first', 'partial']);
  const result = await runWorker(tool, ['-e', 'console.error("progress");console.log("{\\"prepared\\":true}")'], { cwd: await work(), onStderrLine: line => lines.push(line) });
  expect(JSON.parse(result.stdout)).toEqual({ prepared: true });
  expect(result.stderr).toBe('progress\n');
});
