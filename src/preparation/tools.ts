import type { VerifiedTool } from './runner';
import { constants, createWriteStream } from 'node:fs';
import { chmod, copyFile, lstat, mkdir, mkdtemp, realpath, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { Readable, Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { cancelled, InstallerError, requireThat } from '../shared/errors';
import { hashFile, safeRelative } from '../shared/files';
export interface ToolDownload { name: string; url: string; sha256: string }
export async function obtainTool(tool: ToolDownload, cache: string, signal?: AbortSignal,
  fetcher: typeof fetch = fetch): Promise<VerifiedTool> {
  cancelled(signal);
  requireThat(safeRelative(tool.name) && !tool.name.includes('/'), 'unsafe_tool_name');
  requireThat(tool.url === `https://github.com/encounter/nod/releases/download/v2.0.0-alpha.12/${tool.name}`, 'untrusted_tool_url');
  await mkdir(cache, { recursive: true }); requireThat(!(await lstat(cache)).isSymbolicLink(), 'unsafe_workspace');
  const root = await realpath(cache), path = join(root, tool.name);
  const present = await lstat(path).catch(error => { if (error.code === 'ENOENT') return null; throw error; });
  if (present) { requireThat(await hashFile(path, signal) === tool.sha256, 'tool_hash_mismatch'); return { path, sha256: tool.sha256 }; }
  const workspace = await mkdtemp(join(root, '.download-')), partial = join(workspace, 'tool');
  const timeout = AbortSignal.timeout(90_000), combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
  try {
    const response = await fetcher(tool.url, { signal: combined });
    requireThat(response.ok && response.body, 'tool_download_failed');
    let bytes = 0;
    const meter = new Transform({ transform(chunk, _encoding, done) {
      bytes += chunk.length; done(bytes > 20 * 1024 ** 2 ? new InstallerError('tool_download_limit') : null, chunk);
    }});
    await pipeline(Readable.fromWeb(response.body as import('node:stream/web').ReadableStream), meter,
      createWriteStream(partial, { flags: 'wx', mode: 0o600 }), { signal: combined });
    requireThat(await hashFile(partial, signal) === tool.sha256, 'tool_hash_mismatch'); cancelled(signal);
    try { await copyFile(partial, path, constants.COPYFILE_EXCL); }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error; }
    requireThat(await hashFile(path, signal) === tool.sha256, 'tool_hash_mismatch');
    if (process.platform !== 'win32') await chmod(path, 0o700);
    return { path, sha256: tool.sha256 };
  } catch (error) {
    cancelled(signal); if (error instanceof InstallerError) throw error;
    throw new InstallerError('tool_download_failed');
  } finally { await rm(workspace, { recursive: true, force: true }); }
}

export function nodDownload(platform = process.platform, arch = process.arch): ToolDownload {
  const assets: Record<string, [string, string]> = {
    'darwin-arm64': ['nodtool-macos-arm64', '524c0b27c0c4289e435df4474d288ea7e63d6a34814cc7c650231688dd1e53b4'],
    'darwin-x64': ['nodtool-macos-x86_64', 'd1e7ba904c2e557fafbd3bf2d9b75a7901a2bf1568c0fb3a505fca0f30b0a833'],
    'win32-arm64': ['nodtool-windows-arm64.exe', '463d5f7fd7d753fde05c6772f8114ec6f8ab6eaf054790ed9b95a89ef8c63526'],
    'win32-x64': ['nodtool-windows-x86_64.exe', 'f65de62c44f0ad5d179eff85bd1e92099713ffacbf717f651fb761129a25c7bc'],
  };
  const asset = assets[`${platform}-${arch}`]; requireThat(asset, 'unsupported_platform');
  return { name: asset[0], sha256: asset[1], url: `https://github.com/encounter/nod/releases/download/v2.0.0-alpha.12/${asset[0]}` };
}
