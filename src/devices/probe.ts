import type { ServiceEvidence } from './profile';
import { Client } from 'basic-ftp';
import { createConnection } from 'node:net';

async function companionQuery(host: string, port: number, command: 'version' | 'help', signal: AbortSignal): Promise<string> {
  if (signal.aborted) return '';
  return new Promise(resolve => {
    const socket = createConnection({ host, port }); let result = '', completed = false;
    const stop = () => socket.destroy();
    socket.setTimeout(800, stop); signal.addEventListener('abort', stop, { once: true });
    socket.on('connect', () => socket.write(`${command}\n`));
    socket.on('data', chunk => { result += chunk.toString('utf8'); if (Buffer.byteLength(result) > 4096) { result = ''; socket.destroy(); } });
    socket.on('end', () => { completed = true; });
    socket.on('error', () => {});
    socket.on('close', () => { signal.removeEventListener('abort', stop); resolve(completed && !signal.aborted ? result : ''); });
    if (signal.aborted) stop();
  });
}

export async function probeFtp(host: string, port: number, signal: AbortSignal,
  companionPort = 1338): Promise<ServiceEvidence | null> {
  if (signal.aborted) return null;
  const client = new Client(800), abort = () => client.close();
  signal.addEventListener('abort', abort, { once: true });
  try {
    const greeting = await client.connect(host, port); await client.login('anonymous', 'anonymous@');
    const features = [...(await client.features()).keys()];
    await client.cd('/ux0:/'); const listing = await client.list();
    const ux0 = ['app', 'data'].every(name => listing.some(f => f.name === name && f.isDirectory));
    if (!ux0 || signal.aborted) return null;
    let ur0 = false;
    try { await client.cd('/ur0:/'); ur0 = (await client.list()).some(f => f.name === 'data' && f.isDirectory); } catch { /* Some servers expose only ux0. */ }
    const evidence: ServiceEvidence = { host, port, banner: greeting.message, ux0, ur0, features };
    client.close();
    const version = await companionQuery(host, companionPort, 'version', signal);
    if (/^vitacompanion\s+\d+\.\d+/i.test(version.trim())) {
      const help = await companionQuery(host, companionPort, 'help', signal);
      evidence.companion = { version, help };
    }
    return signal.aborted ? null : evidence;
  } catch (error) {
    if (['EACCES', 'EPERM'].includes((error as NodeJS.ErrnoException).code ?? '')) throw error;
    return null;
  } finally { client.close(); signal.removeEventListener('abort', abort); }
}
