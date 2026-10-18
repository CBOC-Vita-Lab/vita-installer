import { afterEach, expect, it } from 'vitest';
import { createServer, type Server } from 'node:net';
import { once } from 'node:events';
import { promoteCompanion } from '../../src/devices/companion';
import { device } from '../jobs/fixture';
const servers: Server[] = [];
afterEach(async () => { for (const s of servers.splice(0)) await new Promise<void>(r => s.close(() => r())); });
it.each(['accepted', 'rejected', 'unknown'] as const)('sends only the exact Companion promote command and returns %s', async result => {
  const commands: string[] = [], id = 'a'.repeat(32);
  const server = createServer(socket => socket.on('data', chunk => {
    commands.push(chunk.toString());
    if (result === 'accepted') { socket.write('Promo'); socket.end('ted.\n'); }
    else if (result === 'rejected') socket.end('Error: Promotion service is busy.\n');
    else socket.destroy();
  })); servers.push(server); server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const candidate = { ...device, host: '127.0.0.1', adapter: 'companion' as const, capabilities: { ...device.capabilities, install: true } };
  expect(await promoteCompanion(candidate, id, { port: (server.address() as import('node:net').AddressInfo).port, timeoutMs: 1000 })).toBe(result);
  expect(commands).toEqual([`promote ux0:data/CBOC-Installer/jobs/${id}/app\n`]);
  await expect(promoteCompanion(candidate, `${id}\nreboot`)).rejects.toMatchObject({ code: 'invalid_job_id' });
});
