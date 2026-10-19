import { afterEach, expect, it, vi } from 'vitest';
import { emptySnapshot } from '../../src/shared/ui';
const originalPlatform = process.platform, originalArch = process.arch;
vi.mock('electron', async () => {
  const { EventEmitter } = await import('node:events');
  const app = Object.assign(new EventEmitter(), { isPackaged: true, requestSingleInstanceLock: () => true,
    whenReady: async () => {}, getVersion: () => '0.1.7', getAppPath: () => '/app', getPath: () => '/state', quit: vi.fn() });
  class BrowserWindow extends EventEmitter {
    static windows: BrowserWindow[] = [];
    static getAllWindows() { return this.windows; }
    webContents = Object.assign(new EventEmitter(), { mainFrame: {}, send: vi.fn(), setWindowOpenHandler: vi.fn() });
    constructor() { super(); BrowserWindow.windows.push(this); }
    loadURL = vi.fn(); isDestroyed = () => false;
  }
  return { app, BrowserWindow, ipcMain: { handle: vi.fn() }, shell: { openExternal: vi.fn(async () => {}) },
    dialog: { showMessageBox: vi.fn(async () => ({ response: 0 })) },
    Menu: { setApplicationMenu: vi.fn(), buildFromTemplate: vi.fn() },
    session: { defaultSession: { setPermissionRequestHandler: vi.fn(), setPermissionCheckHandler: vi.fn(), webRequest: { onHeadersReceived: vi.fn() } } } };
});
vi.mock('node:worker_threads', async () => {
  const { EventEmitter } = await import('node:events');
  class Worker extends EventEmitter {
    static instances: Worker[] = [];
    constructor() { super(); Worker.instances.push(this); }
    postMessage = vi.fn();
  }
  return { Worker };
});
afterEach(() => {
  vi.useRealTimers(); vi.unstubAllGlobals(); delete (process as any).resourcesPath;
  Object.defineProperty(process, 'platform', { value: originalPlatform });
  Object.defineProperty(process, 'arch', { value: originalArch });
});
it('keeps desktop state across worker snapshots and daily checks after a cancelled quit', async () => {
  vi.useFakeTimers(); vi.stubGlobal('__dirname', '/app/dist/main');
  Object.defineProperty(process, 'platform', { value: 'darwin' });
  Object.defineProperty(process, 'arch', { value: 'arm64' });
  (process as any).resourcesPath = '/resources';
  let published = false;
  const base = 'https://github.com/CBOC-Vita-Lab/vita-installer/releases';
  const name = `CBOC-Vita-Installer-0.2.0-${process.platform}-${process.arch}.${process.platform === 'win32' ? 'exe' : 'zip'}`;
  const url = `${base}/download/v0.2.0/${name}`;
  const fetcher = vi.fn(async () => published ? Response.json({ tag_name: 'v0.2.0', html_url: `${base}/tag/v0.2.0`, draft: false, prerelease: false,
    assets: [{ name, browser_download_url: url, state: 'uploaded', size: 200, digest: `sha256:${'a'.repeat(64)}` }] }) : new Response(null, { status: 404 }));
  vi.stubGlobal('fetch', fetcher);
  const electron = await import('electron'), workers = await import('node:worker_threads');
  await import('../../src/main/index'); await vi.advanceTimersByTimeAsync(0);
  const window = (electron.BrowserWindow as any).windows[0], worker = (workers.Worker as any).instances[0];
  const handlers = new Map((electron.ipcMain.handle as any).mock.calls);
  window.webContents.mainFrame.url = window.loadURL.mock.calls[0][0];
  const sender = { sender: window.webContents, senderFrame: window.webContents.mainFrame };
  const snapshot = () => (handlers.get('installer:snapshot') as Function)(sender);
  const command = (request: unknown) => (handlers.get('installer:command') as Function)(sender, request);
  expect(snapshot().desktopUpdate).toMatchObject({ currentVersion: '0.1.7', status: 'unavailable' });
  expect(fetcher).toHaveBeenCalledTimes(1);
  worker.emit('message', { type: 'snapshot', state: { ...emptySnapshot(), busy: 'preparing' } });
  expect(snapshot()).toMatchObject({ busy: 'preparing', desktopUpdate: { currentVersion: '0.1.7', status: 'unavailable' } });
  published = true; await command({ command: 'checkInstallerUpdate' });
  expect(snapshot().desktopUpdate.status).toBe('available');
  expect(worker.postMessage).not.toHaveBeenCalled();
  await expect(command({ command: 'downloadInstallerUpdate' })).rejects.toThrow();
  expect(electron.shell.openExternal).not.toHaveBeenCalled();
  await expect(command({ command: 'downloadInstallerUpdate', url: 'https://evil.test' })).rejects.toThrow();
  worker.emit('message', { type: 'snapshot', state: emptySnapshot() });
  await command({ command: 'downloadInstallerUpdate' });
  expect(electron.shell.openExternal).toHaveBeenCalledExactlyOnceWith(url);
  worker.emit('message', { type: 'snapshot', state: { ...emptySnapshot(), busy: 'preparing' } });
  electron.app.emit('before-quit', {});
  const preventDefault = vi.fn(); window.emit('close', { preventDefault });
  await vi.advanceTimersByTimeAsync(0); expect(preventDefault).toHaveBeenCalled();
  expect(fetcher).toHaveBeenCalledTimes(2);
  await vi.advanceTimersByTimeAsync(24 * 60 * 60 * 1000);
  expect(fetcher).toHaveBeenCalledTimes(3);
  electron.app.emit('will-quit');
  await vi.advanceTimersByTimeAsync(24 * 60 * 60 * 1000);
  expect(fetcher).toHaveBeenCalledTimes(3);
});
