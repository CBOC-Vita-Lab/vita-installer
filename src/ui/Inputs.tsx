import { Check, FolderOpen, FilePlus2, X } from 'lucide-react';
import type { InputView, UiRequest, UiSnapshot } from '../shared/ui';
import { IconButton } from './controls';
import { errorMessage } from './messages';
export function Inputs({ state, kind, run, disabled }: { state: UiSnapshot; kind: 'game' | 'required'; run: (request: UiRequest) => Promise<void>; disabled: boolean }) {
  const gameKinds = ['strikers-disc', 'infected-disc', 'galaxy-disc'];
  const rows = state.inputs.filter(input => input.required && (kind === 'game' ? gameKinds.includes(input.kind) : !gameKinds.includes(input.kind)));
  const renderRow = (input: InputView) => <div className="input-row" key={input.id}>
    <div className="input-label"><strong>{input.label}</strong>{input.filename && <span className="filename" title={input.filename}>{input.filename}</span>}
      {input.message && <span className="error-text">{errorMessage(input.message, input.kind)}{input.problemFile && <span className="filename">{input.problemFile}</span>}</span>}</div>
    {['ready', 'unverified'].includes(input.status) ? <><span className="input-status"><Check />{input.onVita ? 'On Vita' : input.status === 'unverified' ? 'Ready to prepare' : 'Ready'}</span>
      <IconButton label={`Change ${input.label}`} disabled={disabled} onClick={() => { void run({ command: 'chooseInput', id: input.id, kind: 'file' }); }}><FolderOpen /></IconButton></>
      : <div className="input-actions"><button disabled={disabled} onClick={() => { void run({ command: 'chooseInput', id: input.id, kind: 'file' }); }} aria-label={`Choose ${input.label}`}><FilePlus2 /> Choose file</button>
        {['strikers-disc', 'roms'].includes(input.kind) && <IconButton label={`Choose ${input.label} folder`} disabled={disabled} onClick={() => { void run({ command: 'chooseInput', id: input.id, kind: 'folder' }); }}><FolderOpen /></IconButton>}</div>}
    {!input.required && <IconButton label={`Remove ${input.label}`} disabled={disabled} onClick={() => { void run({ command: 'removeInput', id: input.id }); }}><X /></IconButton>}
  </div>;
  return <div>
    {rows.length ? <div className="input-list">{rows.map(renderRow)}</div> : <p className="muted">{kind === 'game' ? 'No game file required.' : 'Required files are ready.'}</p>}
    <button className="text-button" disabled={disabled} onClick={() => { void run({ command: 'scanFolder' }); }}><FolderOpen /> Scan a folder</button>
    {kind === 'required' && state.optionalInputs.length > 0 && <details className="optional" open={state.inputs.some(i => !i.required && !['ready', 'unverified'].includes(i.status))}><summary>Optional extras</summary>
      {state.optionalInputs.map(input => renderRow(state.inputs.find(i => i.id === input.id) ?? { ...input, status: 'missing', required: false }))}
    </details>}
  </div>;
}
