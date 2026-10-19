import { useEffect, useRef, useState } from 'react';
import { bytes } from './messages';
import type { PreparationProgress } from '../shared/contracts';

export const preparationLabel = (phase: PreparationProgress['phase']) => ({ checking: 'Checking game files', extracting: 'Extracting game files',
  converting: 'Preparing game files', optimizing: 'Optimizing movies', hashing: 'Checking prepared files' })[phase];
interface Sample { at: number; completed: number }
const elapsedText = (seconds: number) => `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}`;
const remainingText = (seconds: number) => seconds < 60 ? `${Math.max(5, Math.ceil(seconds / 5) * 5)} sec`
  : seconds < 3600 ? `${Math.ceil(seconds / 60)} min` : `${Math.floor(seconds / 3600)} hr ${Math.ceil(seconds % 3600 / 60)} min`;

export function Progress({ label, completed = 0, total, unit = 'bytes', measured = completed, item, items }: {
  label: string; completed?: number; total?: number; unit?: PreparationProgress['unit']; measured?: number; item?: number; items?: number;
}) {
  const [now, setNow] = useState(() => performance.now());
  const [samples, setSamples] = useState<Sample[]>(() => [{ at: now, completed: measured }]);
  const started = useRef(now);
  useEffect(() => { const timer = setInterval(() => setNow(performance.now()), 1000); return () => clearInterval(timer); }, []);
  useEffect(() => {
    const at = performance.now();
    setSamples(previous => {
      const last = previous.at(-1)!;
      if (measured === last.completed) return previous;
      // Retries must not inherit throughput from work which was discarded.
      if (measured < last.completed || !Number.isFinite(measured)) { started.current = at; return [{ at, completed: measured }]; }
      const recent = previous.filter((s, index) => s.at >= at - 60000 || index === previous.length - 1);
      // Bound by elapsed time, not event count: fast readbacks can emit hundreds of chunks a second.
      if (recent.length > 1 && Math.floor(last.at / 250) === Math.floor(at / 250)) recent.pop();
      return [...recent, { at, completed: measured }];
    });
    setNow(at);
  }, [measured]);
  const first = samples[0], last = samples.at(-1)!;
  const duration = (last.at - first.at) / 1000, amount = last.completed - first.completed;
  const known = Number.isSafeInteger(total) && total! > 0 && Number.isFinite(completed) && completed >= 0 && completed <= total!;
  const remaining = known && samples.length >= 3 && duration >= 5 && amount > 0 && now - last.at < 10000 && measured < total!
    ? (total! - measured) / (amount / duration) : undefined;
  const format = unit === 'bytes' ? bytes : (value: number) => value.toLocaleString('en-US');
  return <div className="work-progress">
    <progress aria-label={label} max={known ? total : 1} value={known ? completed : undefined} />
    <div className="progress-meta">
      {(known || completed > 0 || item) ? <span>{item && items ? `Movie ${item} of ${items} · ` : ''}{format(completed)}{known ? ` / ${format(total!)}` : ''}{unit !== 'bytes' ? ` ${unit}` : ''}</span> : null}
      <span className="muted">Elapsed {elapsedText(Math.max(0, now - started.current) / 1000)}{remaining !== undefined ? ` · About ${remainingText(remaining)} left in this ${item ? 'movie' : 'step'}` : ''}</span>
    </div>
  </div>;
}
