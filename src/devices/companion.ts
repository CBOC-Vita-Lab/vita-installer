import type { DeviceCandidate } from '../shared/contracts';
import type { PromotionResult } from '../jobs/install';
import { createConnection, isIPv4 } from 'node:net';
import { requireThat } from '../shared/errors';
import { stageRoot } from './transport';
export async function promoteCompanion(device: DeviceCandidate, jobId: string,
  options: { port?: number; timeoutMs?: number } = {}): Promise<PromotionResult> {
  requireThat(/^[a-f0-9]{32}$/.test(jobId), 'invalid_job_id');
  requireThat(device.state === 'ready' && device.transport === 'ftp' && device.adapter === 'companion' &&
    device.capabilities.install && !!device.host && isIPv4(device.host), 'promotion_not_supported');
  const port = options.port ?? 1338;
  requireThat(Number.isInteger(port) && port > 0 && port < 65536, 'invalid_ftp_endpoint');
  return new Promise(resolve => {
    const socket = createConnection({ host: device.host!, port }); let response = '', settled = false;
    const finish = (result: PromotionResult) => {
      if (settled) return; settled = true; clearTimeout(deadline); socket.destroy(); resolve(result);
    };
    const deadline = setTimeout(() => finish('unknown'), Math.min(120000, Math.max(1, options.timeoutMs ?? 120000)));
    socket.on('connect', () => socket.write(`promote ${stageRoot(jobId)}/app\n`));
    socket.on('data', chunk => { response += chunk.toString('utf8'); if (Buffer.byteLength(response) > 4096) finish('unknown'); });
    socket.on('end', () => finish(response.trim() === 'Promoted.' ? 'accepted' : response.trim().startsWith('Error:') ? 'rejected' : 'unknown'));
    socket.on('error', () => finish('unknown'));
    socket.on('close', () => finish('unknown'));
  });
}
