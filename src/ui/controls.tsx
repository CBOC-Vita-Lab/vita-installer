import type { ButtonHTMLAttributes, ReactNode } from 'react';
export function IconButton({ label, children, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { label: string }) {
  return <button type="button" className="icon-button" aria-label={label} title={label} {...props}>{children}</button>;
}
export function Segmented({ label, options, value, onChange, disabled }: { label: string; options: { value: string; label: string }[]; value: string; onChange: (value: string) => void; disabled?: boolean }) {
  return <div className="segmented" role="radiogroup" aria-label={label}>{options.map(option => <button key={option.value} type="button" role="radio" aria-checked={option.value === value}
    disabled={disabled} onClick={() => onChange(option.value)}>{option.label}</button>)}</div>;
}
export function Status({ children }: { children: ReactNode }) { return <span className="status-text">{children}</span>; }
