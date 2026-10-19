import { parentPort, workerData } from 'node:worker_threads';
import { readFile } from 'node:fs/promises';
import { join, resolve, sep } from 'node:path';
import { InstallerController } from './controller';
import { safeError } from './security';
import { nodDownload, obtainTool } from '../preparation/tools';
import { requireThat } from '../shared/errors';
import type { UiRequest } from '../shared/ui';
import { downloadGithubRelease } from '../releases/download';
const port = parentPort!;
async function start() {
  const { userData, resources, workers } = workerData as { userData: string; resources: string; workers: string };
  const trust = JSON.parse(await readFile(join(resources, 'profiles/trust.json'), 'utf8')) as { fingerprints: string[] };
  requireThat(Array.isArray(trust.fingerprints) && trust.fingerprints.every(f => /^[a-f0-9]{64}$/.test(f)), 'invalid_trust_store');
  const root = join(workers, `${process.platform}-${process.arch}`);
  const catalog = await readFile(join(root, 'catalog.json'), 'utf8').then(JSON.parse).catch(() => undefined);
  function tool(key: 'worker' | 'volumeWorker') {
    const entry = catalog?.[key]; if (!entry) return undefined;
    const path = resolve(root, entry.path); requireThat(path.startsWith(root + sep) && /^[a-f0-9]{64}$/.test(entry.sha256), 'invalid_tool_catalog');
    return { path, sha256: entry.sha256 as string };
  }
  const store = { fingerprints: new Set(trust.fingerprints), keys: new Map() };
  const controller = new InstallerController({ userData, trust: store, volumeWorker: tool('volumeWorker'),
    downloadRelease: (url, expectedManifestSha256, signal, onProgress) => downloadGithubRelease(url, {
      cache: join(userData, 'releases'), trust: store, expectedManifestSha256, signal, onProgress }),
    tools: async (recipe, signal) => recipe === 'strikers' ? { nod: await obtainTool(nodDownload(), join(userData, 'tools'), signal) }
      : recipe === 'infected' || recipe === 'galaxy' ? { dataWorker: tool('worker') } : {} }, state => port.postMessage({ type: 'snapshot', state }));
  await controller.initialize();
  port.on('message', async (message: { id: number; request?: UiRequest; paths?: string[]; dispose?: boolean }) => {
    try {
      if (message.dispose) await controller.dispose();
      else if (message.request) await controller.command(message.request, message.paths);
      port.postMessage({ type: 'done', id: message.id });
    } catch (error) { port.postMessage({ type: 'error', id: message.id, code: safeError(error) }); }
  });
  port.postMessage({ type: 'ready' });
}
void start().catch(error => port.postMessage({ type: 'fatal', code: safeError(error) }));
