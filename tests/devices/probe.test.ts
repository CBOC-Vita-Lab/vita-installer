import { afterEach, expect, it } from 'vitest';
import { spawn, type ChildProcess } from 'node:child_process';
import { createServer, type Server } from 'node:net';
import { once } from 'node:events';
import { mkdtemp, mkdir, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { createInterface } from 'node:readline';
import { probeFtp } from '../../src/devices/probe';
import { profileService } from '../../src/devices/profile';
const children: ChildProcess[] = [], roots: string[] = [], servers: Server[] = [];
afterEach(async () => {
  for (const child of children.splice(0)) { child.kill(); await once(child, 'close'); }
  for (const server of servers.splice(0)) await new Promise<void>(resolve => server.close(() => resolve()));
  for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true });
});
async function ftp(vita: boolean, prom = false) {
  const root = await mkdtemp(join(tmpdir(), 'ftp-test-')); roots.push(root);
  if (vita) for (const path of ['ux0/app', 'ux0/data', 'ur0/data']) await mkdir(join(root, path), { recursive: true });
  const python = resolve('.venv-workers', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python');
  const child = spawn(python, ['tests/fixtures/ftp_server.py', root, ...(prom ? ['--prom'] : [])], { stdio: ['ignore', 'pipe', 'ignore'] }); children.push(child);
  const lines = createInterface({ input: child.stdout! });
  try { const [line] = await once(lines, 'line'); return JSON.parse(line as string).port as number; } finally { lines.close(); }
}
async function companion(help: string) {
  const commands: string[] = [];
  const server = createServer(socket => {
    socket.on('data', chunk => { const command = chunk.toString().trim(); commands.push(command); socket.end(command === 'version' ? 'vitacompanion 1.02\n' : help); });
  }); servers.push(server); server.listen(0, '127.0.0.1'); await once(server, 'listening');
  return { port: (server.address() as import('node:net').AddressInfo).port, commands };
}
it('reads a Vita-shaped FTP namespace with an advertised capability, without writes', async () => {
  const port = await ftp(true, true), control = await companion('help\tDisplay help\n');
  const evidence = await probeFtp('127.0.0.1', port, new AbortController().signal, control.port);
  expect(evidence).toMatchObject({ ux0: true, ur0: true, features: ['PROM'] });
  expect(profileService(evidence!)).toMatchObject({ adapter: 'vitashell' });
  expect(control.commands.every(c => ['version', 'help'].includes(c))).toBe(true);
}, 10000);
it('rejects a misleading banner on unrelated FTP storage', async () => {
  const port = await ftp(false);
  expect(await probeFtp('127.0.0.1', port, new AbortController().signal, 1)).toBeNull();
}, 10000);
it('uses Companion version and command help, never an installation probe', async () => {
  const port = await ftp(true), control = await companion('help\tDisplay help\nversion\tDisplay version\npromote\tPromote an extracted app directory\n');
  const evidence = await probeFtp('127.0.0.1', port, new AbortController().signal, control.port);
  expect(profileService(evidence!)).toMatchObject({ adapter: 'companion', capabilities: { install: true } });
  expect(control.commands).toEqual(['version', 'help']);
}, 10000);
