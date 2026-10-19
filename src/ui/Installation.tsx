import { Check, AlertCircle, Pause, Play, X, FolderOpen } from 'lucide-react';
import type { JobSnapshot } from '../shared/contracts';
import type { UiRequest } from '../shared/ui';
import { bytes, errorMessage } from './messages';
export function Installation({ job, run, busy }: { job: JobSnapshot; run: (request: UiRequest) => Promise<void>; busy: boolean }) {
  const title = job.completionMode ? { installed: 'Installed', ready_on_vita: 'Ready on Vita', ready_on_card: 'Ready on card', needs_attention: 'Check your Vita' }[job.completionMode]
    : { preparing: 'Preparing files', checking: 'Checking files', transferring: 'Copying to Vita', verifying: 'Verifying files', installing: 'Installing', paused: 'Transfer paused', cancelled: 'Cancelled', complete: 'Complete', failed: 'Could not finish' }[job.phase];
  const active = ['checking', 'transferring', 'verifying', 'installing'].includes(job.phase);
  return <section className="installation-result"><h2>{title}</h2>
    {job.completionMode === 'installed' && <Check className="result-icon success" />}
    {job.errorCode && <p className="error-text"><AlertCircle /> {errorMessage(job.errorCode)}</p>}
    {active && <><progress aria-label="Installation progress" max={job.bytesTotal || 1} value={job.phase === 'transferring' ? job.bytesDone : undefined} />
      {job.phase === 'transferring' && <p>{bytes(job.bytesDone)} / {bytes(job.bytesTotal)} <span className="muted"> · {job.verifiedFiles} / {job.totalFiles} files verified</span></p>}</>}
    {job.nextActions?.length ? <ol className="next-actions">{job.nextActions.map((action, index) => <li key={index}>
      {action.kind === 'eject_card' ? 'Safely eject the card. Return it to the powered-off Vita.' : action.kind === 'install_vpk' ? <>Open in VitaShell to finish installing:<code>{action.from}</code></>
        : action.kind === 'copy_prerequisite' ? <>In VitaShell, copy:<code>{action.from}</code><span>to</span><code>{action.to}</code></>
          : <>Import this cover in RetroFlow:<code>{action.from}</code></>}
    </li>)}</ol> : null}
    <div className="job-actions">
      {job.availableActions.includes('cancel') && ['source_missing', 'source_changed', 'source_unreadable', 'preparation_changed', 'artwork_changed'].includes(job.errorCode ?? '') &&
        <button disabled={busy} onClick={() => { void (async () => { await run({ command: 'cancel' }); await run({ command: 'reset' }); })(); }}><FolderOpen /> Choose files again</button>}
      {job.availableActions.some(a => a === 'resume' || a === 'retry') && <button className="primary" disabled={busy} onClick={() => { void run({ command: 'resume', id: job.jobId }); }}><Play /> {job.completionMode === 'needs_attention' ? 'Check again' : 'Resume'}</button>}
      {active && job.availableActions.includes('cancel') && <button onClick={() => { void run({ command: 'pause' }); }}><Pause /> Pause</button>}
      {job.availableActions.includes('cancel') && <button onClick={() => { void run({ command: 'cancel' }); }}><X /> Cancel</button>}
      {job.availableActions.includes('done') && <button className="primary" onClick={() => { void run({ command: 'reset' }); }}>Done</button>}
    </div>
  </section>;
}
