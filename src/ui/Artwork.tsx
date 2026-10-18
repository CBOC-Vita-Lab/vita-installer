import { useEffect, useState } from 'react';
import { FolderOpen, RotateCcw } from 'lucide-react';
import type { ArtworkView, UiRequest } from '../shared/ui';
import { IconButton, Segmented } from './controls';
const labels = { bubble: 'Bubble icon', livearea: 'Bubble background', retroflow: 'RetroFlow cover' };
export function Artwork({ items, disabled, run }: { items: ArtworkView[]; disabled: boolean; run: (request: UiRequest) => Promise<void> }) {
  return <div className="artwork-grid">{items.map(item => <ArtworkSlot key={item.slot} item={item} disabled={disabled} run={run} />)}</div>;
}
function ArtworkSlot({ item, disabled, run }: { item: ArtworkView; disabled: boolean; run: (request: UiRequest) => Promise<void> }) {
  const label = labels[item.slot], [x, setX] = useState(item.x), [y, setY] = useState(item.y);
  useEffect(() => { setX(item.x); setY(item.y); }, [item.x, item.y]);
  const adjust = () => { void run({ command: 'artwork', slot: item.slot, action: 'adjust', fit: item.fit, x, y }); };
  return <section className="artwork-slot" role="group" aria-label={label}>
    <h3>{label}</h3>
    <div className={`art-preview ${item.slot}`}><img src={item.preview} alt={`${label} preview`} /></div>
    <div className="art-toolbar"><Segmented label={`${label} source`} value={item.mode} disabled={disabled} options={[{ value: 'default', label: 'Default' }, { value: 'custom', label: 'Custom' }]}
      onChange={value => { void run({ command: 'artwork', slot: item.slot, action: value === 'default' ? 'default' : 'choose' }); }} />
      <IconButton label={`Choose ${label.toLowerCase()}`} disabled={disabled} onClick={() => { void run({ command: 'artwork', slot: item.slot, action: 'choose' }); }}><FolderOpen /></IconButton>
      <IconButton label={`Reset ${label.toLowerCase()}`} disabled={disabled || item.mode === 'default'} onClick={() => { void run({ command: 'artwork', slot: item.slot, action: 'default' }); }}><RotateCcw /></IconButton>
    </div>
    {item.mode === 'custom' && <div className="crop-controls">
      <span className="filename" title={item.filename}>{item.filename}</span>
      <Segmented label={`${label} crop`} value={item.fit} disabled={disabled} options={[{ value: 'cover', label: 'Fill' }, { value: 'contain', label: 'Fit' }]}
        onChange={fit => { void run({ command: 'artwork', slot: item.slot, action: 'adjust', fit: fit as 'cover' | 'contain', x, y }); }} />
      {item.fit === 'cover' && <>{[['Horizontal position', x, setX], ['Vertical position', y, setY]].map(([name, value, setter]) => <label className="slider-row" key={name as string}>
        <span>{name as string}</span><input type="range" aria-label={`${label} ${name}`} min="0" max="1" step="0.01" value={value as number} disabled={disabled}
          onChange={event => (setter as (v: number) => void)(Number(event.target.value))} onPointerUp={adjust} onKeyUp={adjust} />
      </label>)}</>}
    </div>}
  </section>;
}
