import { useEffect, useRef } from 'react';
import { Download, ExternalLink, RefreshCw, X } from 'lucide-react';
import type { DesktopUpdateView, UiRequest } from '../shared/ui';
import { IconButton } from './controls';

export function Updates({ update, busy, run, close }: { update: DesktopUpdateView; busy: boolean;
  run: (request: UiRequest) => Promise<void>; close: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => { const element = dialog.current; element?.showModal(); return () => element?.close(); }, []);
  const checking = update.status === 'checking';
  const status = {
    idle: 'Ready to check.', checking: 'Checking for updates...', current: 'You are up to date.',
    available: `Version ${update.latestVersion} is ready.`,
    'no-build': `Version ${update.latestVersion} has no download for this computer yet.`,
    unavailable: 'Could not check public releases. You may be offline, or the release is still private.'
  }[update.status];
  return <dialog ref={dialog} onCancel={close} aria-labelledby="updates-title">
    <header className="dialog-header"><h2 id="updates-title">Installer updates</h2><IconButton label="Close updates" onClick={close}><X /></IconButton></header>
    <div className="updates-content">
      <p><strong>Version {update.currentVersion}</strong><span className="muted">{update.platform}</span></p>
      <p role="status">{status}</p>
      {update.status === 'available' && <>
        <button className="primary" disabled={busy} onClick={() => { void run({ command: 'downloadInstallerUpdate' }); }}><Download />Download update</button>
        <p className="muted">{busy ? 'Finish the current operation before updating.' : 'Download, quit, then replace the desktop app. Keep its app data; your install history stays in place.'}</p>
      </>}
      <div className="update-actions"><IconButton label="Check for updates" disabled={checking} onClick={() => { void run({ command: 'checkInstallerUpdate' }); }}><RefreshCw /></IconButton>
        <button onClick={() => { void run({ command: 'openRepo', repo: 'installer' }); }}><ExternalLink />Open installer releases</button></div>
    </div>
  </dialog>;
}
