import type { VerifiedTool } from './runner';
import { chmod, lstat, mkdir, mkdtemp, realpath, rename, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { cancelled, InstallerError, requireThat } from '../shared/errors';
import { hashFile, safeRelative } from '../shared/files';
import { downloadVerifiedFile } from '../shared/download';
export interface ToolDownload { name: string; url: string; sha256: string }
export async function obtainTool(tool: ToolDownload, cache: string, signal?: AbortSignal,
  fetcher: typeof fetch = fetch): Promise<VerifiedTool> {
  cancelled(signal);
  requireThat(safeRelative(tool.name) && !tool.name.includes('/'), 'unsafe_tool_name');
  requireThat(tool.url === `https://github.com/encounter/nod/releases/download/v2.0.0-alpha.12/${tool.name}`, 'untrusted_tool_url');
  await mkdir(cache, { recursive: true }); requireThat(!(await lstat(cache)).isSymbolicLink(), 'unsafe_workspace');
  const root = await realpath(cache), path = join(root, tool.name);
  const present = await lstat(path).catch(error => { if (error.code === 'ENOENT') return null; throw error; });
  if (present) {
    requireThat(present.isFile() && !present.isSymbolicLink(), 'unsafe_tool_cache');
    if (await hashFile(path, signal) === tool.sha256) return { path, sha256: tool.sha256 };
  }
  const workspace = await mkdtemp(join(root, '.download-')), partial = join(workspace, 'tool');
  try {
    await downloadVerifiedFile({ ...tool, maxBytes: 20 * 1024 ** 2 }, partial, signal, fetcher);
    const current = await lstat(path).catch(error => { if (error.code === 'ENOENT') return null; throw error; });
    requireThat(!current || current.isFile() && !current.isSymbolicLink(), 'unsafe_tool_cache');
    cancelled(signal); await rename(partial, path);
    requireThat(await hashFile(path, signal) === tool.sha256, 'tool_hash_mismatch');
    if (process.platform !== 'win32') await chmod(path, 0o700);
    return { path, sha256: tool.sha256 };
  } catch (error) {
    if (error instanceof InstallerError && error.code === 'download_hash_mismatch') throw new InstallerError('tool_hash_mismatch');
    if (error instanceof InstallerError && error.code === 'download_failed') throw new InstallerError('tool_download_failed');
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
