import { contextBridge, ipcRenderer } from 'electron';
import type { InstallerBridge, UiSnapshot } from '../shared/ui';
const bridge: InstallerBridge = {
  getSnapshot: () => ipcRenderer.invoke('installer:snapshot'),
  command: request => ipcRenderer.invoke('installer:command', request),
  subscribe: callback => {
    const listener = (_event: Electron.IpcRendererEvent, state: UiSnapshot) => callback(state);
    ipcRenderer.on('installer:state', listener);
    return () => { ipcRenderer.removeListener('installer:state', listener); };
  }
};
contextBridge.exposeInMainWorld('vitaInstaller', bridge);
