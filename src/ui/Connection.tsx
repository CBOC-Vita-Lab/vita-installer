import { useEffect, useRef, useState } from 'react';
import { Wifi, Usb, CreditCard, X, RefreshCw, Check } from 'lucide-react';
import type { UiRequest, UiSnapshot } from '../shared/ui';
import { IconButton } from './controls';
import { bytes } from './messages';
export function Connection({ state, run, close }: { state: UiSnapshot; run: (request: UiRequest) => Promise<void>; close: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null), [host, setHost] = useState(''), [port, setPort] = useState(1337), [confirm, setConfirm] = useState<string>();
  useEffect(() => { dialog.current?.showModal(); return () => dialog.current?.close(); }, []);
  const select = async (id: string, confirmUx0 = false) => { await run({ command: 'selectDevice', id, confirmUx0 }); close(); };
  return <dialog ref={dialog} onCancel={close} aria-labelledby="connect-title"><header className="dialog-header"><h2 id="connect-title">Connect Vita</h2><IconButton label="Close connection" onClick={close}><X /></IconButton></header>
    <div className="connection-content">
      {state.discoveryBusy ? <p className="pending" role="status"><RefreshCw /> Looking for your Vita</p> : !state.devices.length && <><h3>No Vita found</h3><p>Same Wi-Fi. VitaShell FTP running.</p></>}
      {state.discoveryStatus === 'usb_active' && <p>USB storage is active. FTP is paused.</p>}
      <div className="device-list">{state.devices.map(device => <div className="device-row" key={device.id}>
        {device.transport === 'ftp' ? <Wifi /> : device.transport === 'card' ? <CreditCard /> : <Usb />}
        <button type="button" disabled={device.state === 'offline'} onClick={() => device.storageRole === 'unknown' ? setConfirm(device.id) : void select(device.id)}>
          <strong>{device.label}</strong><span>{device.transport === 'ftp' ? device.host : device.transport === 'card' ? 'Memory card' : 'USB'}{device.freeBytes !== undefined ? ` · ${bytes(device.freeBytes)} free` : ''}</span>
        </button>{device.id === state.selectedDevice && <Check aria-label="Selected" />}
        {confirm === device.id && <div className="storage-confirm"><p>Is this your Vita's main storage (ux0)?</p><button className="primary" onClick={() => { void select(device.id, true); }}>Use as main storage</button></div>}
      </div>)}</div>
      <button onClick={() => { void run({ command: 'discover' }); }} disabled={state.discoveryBusy}><RefreshCw /> Retry</button>
      <details><summary>Use USB or a memory card</summary><p>Connect the Vita in USB mode, or insert its memory card.</p><button disabled={state.discoveryBusy} onClick={() => { void run({ command: 'discover' }); }}>Check connected storage</button></details>
      <details><summary>Manual address</summary><form onSubmit={event => { event.preventDefault(); void run({ command: 'discover', manual: { host, port } }); }}>
        <label>IP address<input value={host} onChange={event => setHost(event.target.value)} inputMode="decimal" placeholder="192.168.1.20" required /></label>
        <label>Port<input type="number" min="1" max="65535" value={port} onChange={event => setPort(Number(event.target.value))} required /></label>
        <button type="submit" disabled={state.discoveryBusy}>Connect</button>
      </form></details>
    </div>
  </dialog>;
}
