import { createWriteStream } from 'node:fs';
import { Readable, Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { cancelled, InstallerError, requireThat } from './errors';
import { hashFile } from './files';

export interface DownloadProgress { bytes: number; total?: number }
export async function downloadVerifiedFile(spec: { url: string; sha256: string; maxBytes: number; bytes?: number },
  path: string, signal?: AbortSignal, fetcher: typeof fetch = fetch, onProgress?: (progress: DownloadProgress) => void) {
  cancelled(signal);
  let url = new URL(spec.url);
  requireThat(url.origin === 'https://github.com' && !url.username && !url.password && !url.search && !url.hash
    && /^\/[^/]+\/[^/]+\/releases\/download\/.+/.test(url.pathname), 'untrusted_download_url');
  requireThat(/^[a-f0-9]{64}$/.test(spec.sha256), 'invalid_download_hash');
  const timeout = AbortSignal.timeout(5 * 60_000), combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
  try {
    let response: Response | undefined;
    for (let hop = 0; hop < 4; hop++) {
      response = await fetcher(url.href, { credentials: 'omit', redirect: 'manual', signal: combined });
      if (![301, 302, 303, 307, 308].includes(response.status)) break;
      const location = response.headers.get('location'); await response.body?.cancel();
      requireThat(location && hop < 3, 'untrusted_download_redirect');
      url = new URL(location, url);
      requireThat(url.protocol === 'https:' && !url.port && !url.username && !url.password && !url.hash
        && ['release-assets.githubusercontent.com', 'objects.githubusercontent.com'].includes(url.hostname), 'untrusted_download_redirect');
    }
    requireThat(response?.ok && response.body, 'download_failed');
    let bytes = 0; onProgress?.({ bytes, total: spec.bytes });
    const meter = new Transform({ transform(chunk, _encoding, done) {
      bytes += chunk.length;
      if (bytes > spec.maxBytes || spec.bytes !== undefined && bytes > spec.bytes) { done(new InstallerError('download_size_mismatch')); return; }
      onProgress?.({ bytes, total: spec.bytes }); done(null, chunk);
    }});
    await pipeline(Readable.fromWeb(response.body as import('node:stream/web').ReadableStream), meter,
      createWriteStream(path, { flags: 'wx', mode: 0o600 }), { signal: combined });
    requireThat(bytes > 0 && (spec.bytes === undefined || bytes === spec.bytes), 'download_size_mismatch');
    requireThat(await hashFile(path, combined) === spec.sha256, 'download_hash_mismatch'); cancelled(signal);
  } catch (error) {
    cancelled(signal); if (error instanceof InstallerError) throw error;
    throw new InstallerError('download_failed');
  }
}
