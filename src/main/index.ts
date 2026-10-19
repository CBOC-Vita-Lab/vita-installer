import { app, BrowserWindow, dialog, ipcMain, Menu, shell, session } from 'electron';
import { Worker } from 'node:worker_threads';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { CSP, isAllowedExternal, parseRequest, rendererPreferences, repoUrl, trustedSender } from './security';
import { emptySnapshot, type UiRequest, type UiSnapshot } from '../shared/ui';
let window: BrowserWindow | undefined, worker: Worker | undefined, state = emptySnapshot(), serial = 0, closing = false, workerReady = false;
const pending = new Map<number, { resolve: () => void; reject: (error: Error) => void }>();
const appUrl = () => pathToFileURL(join(__dirname, '../renderer/index.html')).href;
function requestWorker(request: UiRequest): Promise<void> { return sendWorker({ request }); }
function sendWorker(message: { request?: UiRequest; paths?: string[]; dispose?: boolean }): Promise<void> {
  if (!workerReady || !worker) return Promise.reject(new Error('Worker unavailable'));
  const id = ++serial;
  return new Promise((resolve, reject) => { pending.set(id, { resolve, reject }); worker!.postMessage({ ...message, id }); });
}
function requireSender(event: Electron.IpcMainInvokeEvent) {
  if (!window || event.sender !== window.webContents || !trustedSender(event.senderFrame?.url ?? '', appUrl(), event.senderFrame === event.sender.mainFrame)) throw new Error('Invalid sender');
}
async function choosePaths(request: UiRequest): Promise<string[] | undefined> {
  let kind: 'file' | 'folder' | undefined, filters: Electron.FileFilter[] | undefined;
  if (request.command === 'import') { kind = request.kind; filters = [{ name: 'Release ZIP', extensions: ['zip'] }]; }
  else if (request.command === 'chooseInput') { kind = request.kind; filters = [{ name: 'Game and required files', extensions: ['iso', 'rvz', 'zip', 'bin', 'gba', 'gb', 'gbc', 'suprx'] }]; }
  else if (request.command === 'scanFolder') kind = 'folder';
  else if (request.command === 'artwork' && request.action === 'choose') { kind = 'file'; filters = [{ name: 'Artwork', extensions: ['png', 'jpg', 'jpeg', 'webp'] }]; }
  if (!kind) return undefined;
  if (state.busy) return [];
  const result = await dialog.showOpenDialog(window!, { properties: kind === 'folder' ? ['openDirectory'] : ['openFile'], filters: kind === 'file' ? filters : undefined });
  return result.canceled ? [] : result.filePaths;
}
function createWindow() {
  window = new BrowserWindow({ width: 1280, height: 800, minWidth: 800, minHeight: 600, title: 'CBOC Vita Installer', backgroundColor: '#141719', show: false,
    icon: join(app.isPackaged ? join(process.resourcesPath, 'installer-resources') : join(app.getAppPath(), 'resources'), 'branding/cboc-mark.png'),
    webPreferences: rendererPreferences(join(__dirname, 'preload.cjs')) });
  window.webContents.on('will-navigate', event => event.preventDefault());
  window.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
  window.webContents.on('will-attach-webview', event => event.preventDefault());
  window.once('ready-to-show', () => window?.show());
  window.on('close', event => {
    if (closing || !state.busy) return;
    event.preventDefault();
    const cancellable = !state.job || state.job.availableActions.includes('cancel');
    void dialog.showMessageBox(window!, { type: 'warning', title: 'Operation in progress',
      message: cancellable ? 'Cancel this operation and close?' : 'Installation is finishing. Keep the app open until it is verified.',
      buttons: cancellable ? ['Keep open', 'Cancel and close'] : ['Keep open'], defaultId: 0, cancelId: 0 }).then(async result => {
      if (result.response !== 1) return;
      await requestWorker({ command: 'cancel' });
      const deadline = Date.now() + 30000;
      while (state.busy && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 100));
      if (!state.busy) { closing = true; window?.close(); }
    });
  });
  void window.loadURL(appUrl());
}
if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on('second-instance', () => { if (window?.isMinimized()) window.restore(); window?.focus(); });
  void app.whenReady().then(() => {
    session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
    session.defaultSession.setPermissionCheckHandler(() => false);
    session.defaultSession.webRequest.onHeadersReceived((details, callback) => callback({ responseHeaders: { ...details.responseHeaders, 'Content-Security-Policy': [CSP] } }));
    Menu.setApplicationMenu(Menu.buildFromTemplate([{ role: 'appMenu' }, { role: 'editMenu' }, { role: 'windowMenu' }]));
    const root = app.getAppPath();
    worker = new Worker(join(__dirname, 'job-worker.mjs'), { workerData: { userData: app.getPath('userData'),
      resources: app.isPackaged ? join(process.resourcesPath, 'installer-resources') : join(root, 'resources'),
      workers: app.isPackaged ? join(process.resourcesPath, 'installer-workers') : join(root, 'workers/bin') } });
    worker.on('message', (message: { type: string; id?: number; state?: UiSnapshot; code?: string }) => {
      if (message.type === 'ready') { workerReady = true; void requestWorker({ command: 'discover' }).catch(() => {}); }
      else if (message.type === 'snapshot') { state = message.state!; window?.webContents.send('installer:state', state); }
      else if (message.type === 'fatal') { state = { ...state, error: 'worker_unavailable' }; window?.webContents.send('installer:state', state); }
      else if (message.id !== undefined) { const call = pending.get(message.id); pending.delete(message.id); if (message.type === 'error') call?.reject(new Error('Operation failed')); else call?.resolve(); }
    });
    worker.on('error', () => { workerReady = false; state = { ...state, busy: undefined, error: 'worker_unavailable' }; window?.webContents.send('installer:state', state); for (const call of pending.values()) call.reject(new Error('Worker unavailable')); pending.clear(); });
    ipcMain.handle('installer:snapshot', event => { requireSender(event); return state; });
    ipcMain.handle('installer:command', async (event, value: unknown) => {
      requireSender(event); const request = parseRequest(value);
      if (request.command === 'openRepo') { const url = repoUrl(request.repo); if (!isAllowedExternal(url)) throw new Error('Invalid URL'); await shell.openExternal(url); return; }
      const paths = await choosePaths(request); if (paths && !paths.length) return;
      await sendWorker({ request, paths });
    });
    createWindow(); app.on('activate', () => { if (BrowserWindow.getAllWindows().length === 0) { closing = false; createWindow(); } });
  });
  app.on('window-all-closed', () => { void worker?.terminate().finally(() => app.quit()); });
}
