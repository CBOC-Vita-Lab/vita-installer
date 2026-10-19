import { Check, AlertCircle, Pause, Play, X, FolderOpen, RefreshCw } from 'lucide-react';
import type { JobSnapshot } from '../shared/contracts';
import type { UiRequest } from '../shared/ui';
import { bytes, errorMessage } from './messages';
import { Progress } from './Progress';
export function Installation({ job, run, busy }: { job: JobSnapshot; run: (request: UiRequest) => Promise<void>; busy: boolean }) {
  const title = job.completionMode ? { installed: 'Installed', ready_on_vita: 'Ready on Vita', ready_on_card: 'Ready on card', needs_attention: 'Check your Vita' }[job.completionMode]
    : { preparing: 'Preparing files', checking: 'Checking files', transferring: 'Copying and checking files', verifying: 'Verifying files', installing: 'Installing', paused: 'Transfer paused', cancelled: 'Cancelled', complete: 'Complete', failed: 'Could not finish' }[job.phase];
  const active = ['checking', 'transferring', 'verifying', 'installing'].includes(job.phase);
  const needsReview = job.phase === 'paused' && job.errorCode === 'protected_files_changed' && job.availableActions.includes('cancel');
  return <section className="installation-result"><h2>{title}</h2>
    {job.completionMode === 'installed' && <Check className="result-icon success" />}
    {job.errorCode && <p className="error-text"><AlertCircle /> {errorMessage(job.errorCode)}</p>}
    {active && <><Progress key={`${job.jobId}:${job.phase}:${!!job.verification}`} label="Installation progress"
      completed={job.phase === 'transferring' ? job.verifiedBytes ?? 0 : job.verification?.completed ?? 0}
      total={job.phase === 'transferring' ? job.bytesTotal : job.verification?.total} />
      {job.phase === 'transferring' && <p className="muted">{bytes(job.bytesDone)} processed · {job.verifiedFiles} / {job.totalFiles} files verified</p>}</>}
    {job.nextActions?.length ? <ol className="next-actions">{job.nextActions.map((action, index) => <li key={index}>
      {action.kind === 'eject_card' ? 'Safely eject the card. Return it to the powered-off Vita.' : action.kind === 'install_vpk' ? <>Open in VitaShell to finish installing:<code>{action.from}</code></>
        : action.kind === 'copy_prerequisite' ? <>In VitaShell, copy:<code>{action.from}</code><span>to</span><code>{action.to}</code></>
          : <>Import this cover in RetroFlow:<code>{action.from}</code></>}
    </li>)}</ol> : null}
    <div className="job-actions">
      {job.availableActions.includes('cancel') && ['source_missing', 'source_changed', 'source_unreadable', 'preparation_changed', 'artwork_changed'].includes(job.errorCode ?? '') &&
        <button disabled={busy} onClick={() => { void (async () => { await run({ command: 'cancel' }); await run({ command: 'reset' }); })(); }}><FolderOpen /> Choose files again</button>}
      {needsReview && <button className="primary" disabled={busy} onClick={() => { void (async () => { await run({ command: 'cancel' }); await run({ command: 'reset' }); })(); }}><RefreshCw /> Review installation again</button>}
      {!needsReview && job.availableActions.some(a => a === 'resume' || a === 'retry') && <button className="primary" disabled={busy} onClick={() => { void run({ command: 'resume', id: job.jobId }); }}><Play /> {job.completionMode === 'needs_attention' ? 'Check again' : 'Resume'}</button>}
      {active && job.availableActions.includes('cancel') && <button onClick={() => { void run({ command: 'pause' }); }}><Pause /> Pause</button>}
      {job.availableActions.includes('cancel') && <button onClick={() => { void run({ command: 'cancel' }); }}><X /> Cancel</button>}
      {job.availableActions.includes('done') && <button className="primary" onClick={() => { void run({ command: 'reset' }); }}>Done</button>}
    </div>
  </section>;
}
