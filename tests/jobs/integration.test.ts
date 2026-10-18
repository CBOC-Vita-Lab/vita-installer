import { afterEach, expect, it } from 'vitest';
import { mkdir, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { createInterface } from 'node:readline';
import { jobFixture } from './fixture';
import { openTransport, type Transport, type TransportOptions } from '../../src/devices/transport';
import { createInstallPlan } from '../../src/jobs/plan';
import { executeTransfer, writeScope } from '../../src/jobs/transfer';
import { FileJournal } from '../../src/jobs/journal';
import { digest, hashFile } from '../../src/shared/files';
import type { DeviceCandidate } from '../../src/shared/contracts';
const roots: string[] = [], transports: Transport[] = [], children: ChildProcess[] = [];
afterEach(async () => {
  for (const t of transports.splice(0)) await t.close();
  for (const c of children.splice(0)) if (c.exitCode === null && c.signalCode === null) { const done = once(c, 'close'); c.kill(); await done; }
  for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true });
});
it.each(['card', 'ftp'] as const)('runs the full coordinator with real %s IO and backup-backed artwork replacement', async kind => {
  const f = await jobFixture(); roots.push(f.root); const target = join(await realpath(f.root), 'target');
  const root = kind === 'ftp' ? join(target, 'ux0') : target;
  await mkdir(join(root, 'app'), { recursive: true }); await mkdir(join(root, 'data/RetroFlow/COVERS/Homebrew'), { recursive: true });
  const title = f.release.manifest.titleId, coverPath = `data/RetroFlow/COVERS/Homebrew/${title}.png`;
  await writeFile(join(root, coverPath), 'old cover'); const cover = join(f.root, 'custom.png'); await writeFile(cover, 'new cover');
  f.artwork.images.retroflow = { path: cover, sourceSha256: digest('new cover'), outputSha256: digest('new cover') };
  const pythonEntry = resolve('.venv-workers', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python');
  const python = await realpath(pythonEntry);
  let device: DeviceCandidate, options: TransportOptions;
  if (kind === 'ftp') {
    const child = spawn(pythonEntry, ['tests/fixtures/ftp_server.py', target, '--writable'], { stdio: ['ignore', 'pipe', 'ignore'] }); children.push(child);
    const lines = createInterface({ input: child.stdout! });
    const [line] = await Promise.race([once(lines, 'line'), once(child, 'close').then(() => { throw new Error('FTP fixture failed to start'); })]); lines.close();
    const { port } = JSON.parse(line as string);
    device = { ...f.device, host: '127.0.0.1', port }; options = {};
  } else {
    device = { ...f.device, id: 'volume:fixture', identity: 'volume:fixture', transport: 'card', adapter: 'volume', root,
      capabilities: { transfer: true, install: false, ur0: false } };
    const path = process.env.VITA_TEST_VOLUME_WORKER ?? python;
    options = { volumeWorker: { path, sha256: await hashFile(path), args: process.env.VITA_TEST_VOLUME_WORKER ? [] : [resolve('workers/volume.py')] },
      volumes: async () => [{ id: 'fixture', root, name: 'Test card', kind: 'card', readable: true, vitaLayout: true, role: 'unknown' }] };
  }
  const readonly = await openTransport(device, options); transports.push(readonly);
  const plan = await createInstallPlan(f.release, f.prepared, device, 'first_install', { transport: readonly, artwork: f.artwork,
    retroflow: { category: 'homebrew' }, replaceRetroflow: true, acknowledgeUnknownSpace: true });
  expect(plan.blockers).toEqual([]); await readonly.close();
  const transport = await openTransport(device, { ...options, scope: writeScope(plan) }); transports.push(transport);
  const journal = new FileJournal(join(f.root, 'jobs'));
  const result = await executeTransfer(plan, transport, journal, () => {});
  expect(result).toMatchObject({ phase: 'verifying', verifiedFiles: 3 });
  expect(await hashFile(join(root, plan.vpkDestination.slice(4)))).toBe(f.artwork.sha256);
  expect((await readFile(join(root, coverPath))).toString()).toBe('new cover');
  expect((await readFile((await journal.load(plan.jobId)).backups[0].local)).toString()).toBe('old cover');
}, 30000);
