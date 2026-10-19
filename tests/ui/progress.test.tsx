// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { afterEach, expect, it, vi } from 'vitest';
import { act, cleanup, render, screen } from '@testing-library/react';
import { Installation } from '../../src/ui/Installation';
import { Progress } from '../../src/ui/Progress';
import { App } from '../../src/ui/App';
import { emptySnapshot, type UiSnapshot } from '../../src/shared/ui';
import type { JobSnapshot } from '../../src/shared/contracts';

afterEach(() => { cleanup(); vi.useRealTimers(); });
const job = (): JobSnapshot => ({ jobId: 'a'.repeat(32), phase: 'transferring', bytesDone: 0, bytesTotal: 1000,
  verifiedFiles: 0, verifiedBytes: 0, totalFiles: 10, availableActions: ['cancel'] });
it('estimates from verified work, not queued upload bytes, and clears estimates while paused', () => {
  vi.useFakeTimers();
  const renderJob = (state: JobSnapshot) => <Installation job={state} run={async () => {}} busy={false} />;
  const view = render(renderJob(job()));
  act(() => vi.advanceTimersByTime(5000));
  view.rerender(renderJob({ ...job(), bytesDone: 900, verifiedBytes: 100, verifiedFiles: 1 }));
  expect(screen.queryByText(/About .* left/)).not.toBeInTheDocument();
  act(() => vi.advanceTimersByTime(5000));
  view.rerender(renderJob({ ...job(), bytesDone: 1000, verifiedBytes: 200, verifiedFiles: 2 }));
  expect(screen.getByText(/About 40 sec left in this step/)).toBeVisible();
  expect(screen.queryByRole('heading', { name: 'Complete' })).not.toBeInTheDocument();
  act(() => vi.advanceTimersByTime(11000));
  expect(screen.queryByText(/About .* left/)).not.toBeInTheDocument();
  view.rerender(renderJob({ ...job(), phase: 'paused', availableActions: ['resume', 'cancel'] }));
  expect(screen.queryByText(/Elapsed/)).not.toBeInTheDocument();
  view.rerender(renderJob({ ...job(), bytesDone: 200, verifiedBytes: 200 }));
  expect(screen.queryByText(/About .* left/)).not.toBeInTheDocument();
});
it('does not carry a transfer estimate into final verification or manual completion', () => {
  const view = render(<Installation job={{ ...job(), phase: 'verifying', bytesDone: 1000 }} run={async () => {}} busy={false} />);
  expect(screen.getByRole('progressbar', { name: 'Installation progress' })).not.toHaveAttribute('value');
  view.rerender(<Installation job={{ ...job(), phase: 'complete', completionMode: 'ready_on_vita', nextActions: [{ kind: 'install_vpk', from: 'ux0:data/install.vpk' }], availableActions: ['done'] }} run={async () => {}} busy={false} />);
  expect(screen.getByRole('heading', { name: 'Ready on Vita' })).toBeVisible();
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  expect(screen.getByText(/Open in VitaShell/)).toBeVisible();
});
it('uses final readback progress rather than the already copied byte total', () => {
  render(<Installation job={{ ...job(), phase: 'verifying', bytesDone: 1000, verification: { completed: 250, total: 1000 } }} run={async () => {}} busy={false} />);
  expect(screen.getByRole('progressbar', { name: 'Installation progress' })).toHaveAttribute('value', '250');
});
it('retains enough measured time for an estimate under frequent verification updates', () => {
  vi.useFakeTimers();
  const view = render(<Progress label="Verification progress" completed={0} total={10000} />);
  for (let completed = 1; completed <= 1200; completed++) {
    act(() => vi.advanceTimersByTime(5));
    view.rerender(<Progress label="Verification progress" completed={completed} total={10000} />);
  }
  expect(screen.getByText(/About 45 sec left in this step/)).toBeVisible();
});
it('shows movie frames and item counts from the preparation worker without claiming an install', async () => {
  const state: UiSnapshot = { ...emptySnapshot(), busy: 'preparing', preparation: {
    phase: 'optimizing', completed: 500, total: 2000, unit: 'frames', item: 2, items: 9 } };
  render(<App bridge={{ getSnapshot: async () => state, command: async () => {}, subscribe: () => () => {} }} />);
  expect(await screen.findByText('Optimizing movies')).toBeVisible();
  expect(screen.getByRole('progressbar', { name: 'Preparation progress' })).toHaveAttribute('value', '500');
  expect(screen.getByText(/500 \/ 2,000 frames/)).toBeVisible();
  expect(screen.getByText(/Movie 2 of 9/)).toBeVisible();
  expect(screen.queryByText(/About .* left/)).not.toBeInTheDocument();
});
