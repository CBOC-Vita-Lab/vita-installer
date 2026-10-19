import { useEffect, useRef, useState } from 'react';
import { ArrowLeft, ArrowRight, Wifi, Usb, FolderOpen, FileArchive, ExternalLink, RefreshCw, X, Check, Download } from 'lucide-react';
import { emptySnapshot, type InstallerBridge, type Step, type UiRequest, type UiSnapshot } from '../shared/ui';
import { Artwork } from './Artwork';
import { Connection } from './Connection';
import { Inputs } from './Inputs';
import { Installation } from './Installation';
import { Progress, preparationLabel } from './Progress';
import { IconButton } from './controls';
import { bytes, errorMessage } from './messages';
const steps: { id: Step; label: string }[] = [{ id: 'release', label: 'Release' }, { id: 'game', label: 'Game files' }, { id: 'required', label: 'Required files' }, { id: 'artwork', label: 'Artwork' }, { id: 'install', label: 'Install' }];
export function App({ bridge = window.vitaInstaller }: { bridge?: InstallerBridge }) {
  const [state, setState] = useState<UiSnapshot>(emptySnapshot), [step, setStep] = useState<Step>('release'), [connecting, setConnecting] = useState(false);
  const [pending, setPending] = useState(false), [localError, setLocalError] = useState<string>();
  const [githubUrl, setGithubUrl] = useState('');
  const { space: ackSpace, update, cover: replaceCover } = state.plan?.consents ?? { space: false, update: false, cover: false };
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { if (!bridge) return; let alive = true; void bridge.getSnapshot().then(s => { if (alive) setState(s); }); const off = bridge.subscribe(setState); return () => { alive = false; off(); }; }, [bridge]);
  useEffect(() => { heading.current?.focus(); }, [step]);
  useEffect(() => { if (!state.selectedRelease && !state.job) setStep('release'); }, [state.selectedRelease, state.job]);
  useEffect(() => { if (state.repairStep && !state.job) setStep(state.repairStep); }, [state.repairStep, state.job]);
  const run = async (request: UiRequest) => {
    if (!bridge) return; setLocalError(undefined);
    const control = ['cancel', 'pause', 'discover', 'clearError'].includes(request.command);
    if (!control) setPending(true);
    try { await bridge.command(request); setState(await bridge.getSnapshot()); }
    catch { setLocalError('operation_failed'); }
    finally { if (!control) setPending(false); }
  };
  const selected = state.releases.find(r => r.id === state.selectedRelease), device = state.devices.find(d => d.id === state.selectedDevice);
  const visibleSteps = state.appUpdateSelected ? steps.filter(item => !['game', 'required'].includes(item.id)) : steps;
  const connected = device?.state === 'ready', disabled = pending || !!state.busy, current = visibleSteps.findIndex(s => s.id === step);
  const gameReady = state.inputs.filter(i => i.required && i.kind.endsWith('-disc')).every(i => ['ready', 'unverified'].includes(i.status));
  const allReady = state.inputs.every(i => ['ready', 'unverified'].includes(i.status));
  async function next() {
    if (step === 'release') {
      if (connected && selected && !selected.held) {
        if (state.appUpdateAvailable) { await run({ command: 'prepareUpdate' }); if ((await bridge?.getSnapshot())?.prepared) setStep('artwork'); }
        else setStep('game');
      }
    }
    else if (step === 'game') setStep('required');
    else if (step === 'required') { await run({ command: 'prepare' }); if ((await bridge?.getSnapshot())?.prepared) setStep('artwork'); }
    else if (step === 'artwork') { await run({ command: 'plan', acknowledgeUnknownSpace: false, explicitUpdate: false, replaceRetroflow: false }); if ((await bridge?.getSnapshot())?.plan) setStep('install'); }
    else if (state.plan) await run({ command: 'execute', id: state.plan.id });
  }
  const replan = (space: boolean, upd: boolean, cover: boolean) => { void run({ command: 'plan', acknowledgeUnknownSpace: space, explicitUpdate: upd, replaceRetroflow: cover }); };
  const nextDisabled = disabled || !connected || !selected || selected.held || state.repairStep === 'release' || (step === 'game' && !gameReady) || (step === 'required' && !allReady)
    || (step === 'install' && (!state.plan || state.plan.blockers.length > 0));
  if (!bridge) return <main className="unavailable"><h1>CBOC Vita Installer</h1><p>Open the desktop app to install a release.</p></main>;
  return <div className={`app-shell ${state.busy && !state.job ? 'with-progress' : ''}`}>
    <header className="app-header"><div className="brand"><img src={new URL('../../resources/branding/cboc-mark.png', import.meta.url).href} alt="CBOC logo" width="48" height="48" /><div><strong>CBOC</strong><h1 aria-label="CBOC Vita Installer">Vita Installer</h1></div></div><button className={`connection-button ${connected ? 'connected' : ''}`} disabled={!!state.busy && state.busy !== 'installing'} onClick={() => { setConnecting(true); if (!state.discoveryBusy) void run({ command: 'discover' }); }} aria-label={connected ? 'Vita connected' : 'Connect Vita'}>
      {device && device.transport !== 'ftp' ? <Usb /> : <Wifi />}<span>{connected ? device.transport === 'card' ? 'Card connected' : 'Vita connected' : 'Connect Vita'}</span></button></header>
    <nav className="steps" aria-label="Installation steps"><ol>{visibleSteps.map((item, i) => <li key={item.id} className={i < current ? 'done' : i === current ? 'current' : ''} aria-current={step === item.id ? 'step' : undefined}>
      <span className="step-dot">{i < current && <Check />}</span><span>{item.label}</span></li>)}</ol></nav>
    <main className="workspace">
      {(state.error || localError) && <div role="alert" className="error-banner"><span>{errorMessage(state.error ?? localError, selected?.recipe)}{state.errorFile && <strong className="filename">{state.errorFile}</strong>}</span><IconButton label="Dismiss error" onClick={() => { setLocalError(undefined); void run({ command: 'clearError' }); }}><X /></IconButton></div>}
      {state.notice === 'release_repaired' && <p role="status" className="repair-notice"><Check />Release files repaired.</p>}
      {state.job ? <Installation job={state.job} busy={disabled} run={run} /> : <>
        <div className="section-heading"><div><h2 ref={heading} tabIndex={-1}>{step === 'release' ? selected ? 'Confirm release' : 'Choose release' : steps.find(item => item.id === step)!.label}</h2>{selected && step !== 'release' && <p>{selected.name}</p>}</div>
          {selected && <IconButton label="Open game releases" disabled={disabled} onClick={() => { void run({ command: 'openRepo', repo: selected.recipe as 'strikers' }); }}><ExternalLink /></IconButton>}
        </div>
        {step === 'release' && <>
          <form className="github-source" onSubmit={event => { event.preventDefault(); void run({ command: 'importUrl', url: githubUrl.trim() }); }}>
            <label htmlFor="github-url">GitHub URL<input id="github-url" type="url" autoComplete="off" spellCheck={false} maxLength={1000} value={githubUrl}
              placeholder="https://github.com/CBOC-Vita-Lab/..." disabled={disabled} onChange={event => setGithubUrl(event.target.value)} /></label>
            <button type="submit" className="icon-button primary" aria-label="Download release" title="Download release" disabled={disabled || !githubUrl.trim()}><Download /></button>
          </form>
          {!state.releases.length ? <div className="release-drop"><FileArchive className="release-symbol" /><div className="release-actions"><button className="primary" disabled={disabled} onClick={() => { void run({ command: 'import', kind: 'file' }); }}><FileArchive /> Choose release ZIP</button>
            <IconButton label="Choose release folder" disabled={disabled} onClick={() => { void run({ command: 'import', kind: 'folder' }); }}><FolderOpen /></IconButton></div></div>
            : <div className={`release-list ${state.releases.length === 1 ? 'single' : ''}`}>{state.releases.map(release => <button key={release.id} className={`release-choice ${state.selectedRelease === release.id ? 'selected' : ''}`} disabled={disabled || release.held} onClick={() => { void run({ command: 'selectRelease', id: release.id }); }}>
              {release.image ? <img src={release.image} alt={`${release.name} artwork`} /> : <FileArchive className="release-symbol" />}<span><strong>{release.name}</strong><small>{release.held ? 'Not ready to install' : release.version}</small>{release.installed && <small className="installed-badge">Already installed</small>}</span>{release.id === state.selectedRelease && <Check />}</button>)}</div>}
          {state.releases.length > 0 && <div className="release-actions"><button className="text-button" disabled={disabled} onClick={() => { void run({ command: 'import', kind: 'file' }); }}><FileArchive /> Choose another release ZIP</button>
            <IconButton label="Choose another release folder" disabled={disabled} onClick={() => { void run({ command: 'import', kind: 'folder' }); }}><FolderOpen /></IconButton></div>}
          {selected && <div className="release-metadata"><IconButton label="Check public releases" disabled={disabled} onClick={() => { void run({ command: 'checkRelease' }); }}><RefreshCw /></IconButton>
            {state.publicRelease && <span className="muted">{state.publicRelease.status === 'available' ? `Latest public release: ${state.publicRelease.tag}` : 'Public release information unavailable. Your local bundle can still be used.'}</span>}</div>}
          {state.appUpdateAvailable && <button className="text-button" disabled={disabled || !connected} onClick={() => {
            void run({ command: 'fullSetup' }).then(() => setStep('game'));
          }}><FolderOpen />Full setup</button>}
          {state.recoverableJobs.map(job => <button className="recovery-row" key={job.id} disabled={disabled} onClick={() => { void run({ command: 'resume', id: job.id }); }}><RefreshCw /> Resume {job.title}</button>)}
        </>}
        {(step === 'game' || step === 'required') && <Inputs state={state} kind={step} run={run} disabled={disabled} />}
        {step === 'artwork' && <Artwork items={state.artwork} run={run} disabled={disabled} />}
        {step === 'install' && state.plan && <section className="plan-summary"><dl><div><dt>Destination</dt><dd>{device?.label} · {device?.transport === 'ftp' ? 'FTP' : device?.transport === 'card' ? 'Memory card' : 'USB'}</dd></div><div><dt>Space needed</dt><dd>{bytes(state.plan.requiredBytes)}</dd></div>
          {state.plan.transferBytes !== undefined && <div><dt>To transfer</dt><dd>{bytes(state.plan.transferBytes)}</dd></div>}
          {state.plan.appOnly && <div><dt>Game files</dt><dd>Checked and unchanged</dd></div>}
          <div><dt>Artwork</dt><dd>{state.plan.customArtwork.length ? 'Custom artwork selected' : 'Default artwork'}</dd></div><div><dt>Existing saves</dt><dd>Protected</dd></div></dl>
          {state.plan.blockers.filter(code => !['existing_app', 'capacity_unknown'].includes(code)).map(code => <p className="error-text" key={code}>{errorMessage(code)}</p>)}
          {state.plan.warnings.filter(code => code !== 'capacity_unverified').map(code => <p key={code} className="muted">{errorMessage(code)}</p>)}
          {state.plan.appExists && <><p className="installed-notice">Already installed on this {device?.transport === 'card' ? 'card' : 'Vita'}. Manual update approval is required.</p><label className="checkbox-row"><input type="checkbox" checked={update} disabled={disabled} onChange={e => replan(ackSpace, e.target.checked, replaceCover)} />Update this game after backing up its existing files.</label></>}
          {state.plan.freeBytes === undefined && <label className="checkbox-row"><input type="checkbox" checked={ackSpace} disabled={disabled} onChange={e => replan(e.target.checked, update, replaceCover)} />Free space could not be checked. I have at least {bytes(state.plan.requiredBytes)} available.</label>}
          {(replaceCover || state.plan.warnings.includes('existing_cover_kept')) && <label className="checkbox-row"><input type="checkbox" checked={replaceCover} disabled={disabled} onChange={e => replan(ackSpace, update, e.target.checked)} />Replace my RetroFlow cover after backing it up.</label>}
          {state.plan.requiresUr0Action && <p className="muted">One shader file must be copied to ur0 in VitaShell.</p>}
          {device?.capabilities.install !== true && <p className="muted">Finish by opening the copied VPK in VitaShell.</p>}
        </section>}
      </>}
    </main>
      {state.busy && !state.job && <div role="status" className="busy-status"><RefreshCw /><div className="busy-detail">
        <span>{state.busy === 'repairing' ? 'Repairing release' : state.busy === 'downloading' ? 'Downloading release' : state.busy === 'artwork' ? 'Updating preview' : state.busy === 'preparing' ? state.preparation ? preparationLabel(state.preparation.phase) : 'Preparing files' : state.busy === 'planning' ? 'Checking destination' : state.busy === 'importing' ? 'Checking release' : state.busy === 'installing' ? 'Checking installation' : 'Checking files'}</span>
        {state.download && <Progress key={`download:${state.busy}`} label="Download progress" completed={state.download.bytes} total={state.download.total} />}
        {state.busy === 'preparing' && state.preparation && <Progress key={`${state.preparation.phase}:${state.preparation.unit}:${state.preparation.item ?? 0}`}
          label="Preparation progress" {...state.preparation} />}
        </div><button onClick={() => { void run({ command: 'cancel' }); }}>Cancel</button></div>}
    {!state.job && <footer className="app-footer"><button disabled={disabled || current <= 0} onClick={() => setStep(visibleSteps[current - 1].id)}><ArrowLeft /> Back</button>
      <button className="primary" disabled={nextDisabled} onClick={() => { void next(); }}>{step === 'required' ? 'Prepare files' : step === 'artwork' ? 'Review install' : step === 'install' ? 'Install' : step === 'release' && state.appUpdateAvailable ? 'Update app' : 'Continue'}<ArrowRight /></button></footer>}
    {connecting && <Connection state={state} run={run} close={() => setConnecting(false)} />}
  </div>;
}
