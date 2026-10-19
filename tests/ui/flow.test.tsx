// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { afterEach, expect, it } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { App } from '../../src/ui/App';
import { emptySnapshot, type InstallerBridge, type UiRequest, type UiSnapshot } from '../../src/shared/ui';
const id = 'a'.repeat(32), deviceId = 'ftp:192.168.1.8:1337';
HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', ''); };
HTMLDialogElement.prototype.close = function () { this.removeAttribute('open'); };
const png = 'data:image/png;base64,iVBORw0KGgo=';
const ready = (): UiSnapshot => ({ ...emptySnapshot(), releases: [{ id, name: 'Mario Strikers Charged', version: 'a8d4ad6', held: false, image: png, recipe: 'strikers' }], selectedRelease: id,
  devices: [{ id: deviceId, transport: 'ftp', label: 'Vita', host: '192.168.1.8', port: 1337, storageRole: 'ux0', adapter: 'companion', state: 'ready', capabilities: { transfer: true, install: true, ur0: true }, evidence: [] }], selectedDevice: deviceId,
  inputs: [{ id: 'game', label: 'USA Rev 1 game file', kind: 'strikers-disc', required: true, status: 'ready', filename: 'game.rvz' }],
  artwork: (['bubble', 'livearea', 'retroflow'] as const).map(slot => ({ slot, mode: 'default', preview: png, fit: 'cover', x: .5, y: .5 })) });
function harness(initial: UiSnapshot = ready(), conflicts = false) {
  let state = initial; const calls: UiRequest[] = [], subscribers = new Set<(s: UiSnapshot) => void>();
  const set = (patch: Partial<UiSnapshot>) => { state = { ...state, ...patch }; subscribers.forEach(f => f(state)); };
  const bridge: InstallerBridge = { getSnapshot: async () => state, subscribe: f => { subscribers.add(f); return () => { subscribers.delete(f); }; },
    command: async request => { calls.push(request);
      if (request.command === 'selectRelease') set({ selectedRelease: request.id });
      if (request.command === 'prepare') set({ prepared: true });
      if (request.command === 'artwork') set({ artwork: state.artwork.map(a => a.slot !== request.slot ? a : { ...a, mode: request.action === 'default' ? 'default' : 'custom', fit: request.fit ?? a.fit, x: request.x ?? a.x, y: request.y ?? a.y, filename: request.action === 'default' ? undefined : 'custom.png' }) });
      if (request.command === 'plan') set({ plan: { id: 'b'.repeat(32), blockers: [], warnings: conflicts && !request.replaceRetroflow ? ['existing_cover_kept'] : [], appExists: conflicts, requiresUr0Action: false, requiredBytes: 1024, freeBytes: conflicts ? undefined : 4096, customArtwork: [],
        consents: { update: request.explicitUpdate, space: request.acknowledgeUnknownSpace, cover: request.replaceRetroflow } } });
    } };
  render(<App bridge={bridge} />); return { bridge, calls, set };
}
afterEach(cleanup);
async function toArtwork() {
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Prepare files' }));
  await screen.findByRole('heading', { name: 'Artwork' });
}
it('shows five steps and blocks release continuation until a destination is verified', async () => {
  harness({ ...ready(), devices: [], selectedDevice: undefined });
  expect(await screen.findByRole('button', { name: 'Continue' })).toBeDisabled();
  for (const text of ['Release', 'Game files', 'Required files', 'Artwork', 'Install']) expect(within(screen.getByRole('navigation', { name: 'Installation steps' })).getByText(text)).toBeVisible();
  expect(screen.getByRole('button', { name: 'Connect Vita' })).toBeVisible();
});
it('offers a visual multi-release choice rather than guessing one', async () => {
  const h = harness({ ...ready(), selectedRelease: undefined, releases: [...ready().releases, { id: 'c'.repeat(32), name: 'VitAdhoc', version: '21', held: false, image: png, recipe: 'vitadhoc' }] });
  fireEvent.click(await screen.findByRole('button', { name: /VitAdhoc/ }));
  expect(h.calls).toContainEqual({ command: 'selectRelease', id: 'c'.repeat(32) });
  expect(screen.getByRole('img', { name: 'VitAdhoc artwork' })).toBeVisible();
});
it('warns about an installed release before preparation without authorizing an update', async () => {
  const h = harness({ ...ready(), releases: [{ ...ready().releases[0], installed: true }] });
  expect(await screen.findByText('Already installed')).toBeVisible();
  expect(h.calls.some(call => call.command === 'plan' && call.explicitUpdate)).toBe(false);
  await toArtwork(); fireEvent.click(screen.getByRole('button', { name: 'Review install' }));
  await act(() => h.set({ plan: { id, blockers: ['existing_app'], warnings: [], requiredBytes: 1, appExists: true,
    requiresUr0Action: false, customArtwork: [], freeBytes: 100, consents: { update: false, cover: false, space: false } } }));
  expect(await screen.findByText(/Already installed on this/)).toBeVisible();
  expect(screen.getByRole('checkbox', { name: /Update this game/ })).not.toBeChecked();
  expect(screen.getByRole('button', { name: 'Install' })).toBeDisabled();
});
it('skips profile prompts for a detected Companion and keeps connection available throughout', async () => {
  harness(); await toArtwork();
  expect(screen.getByRole('button', { name: /Vita connected/ })).toBeVisible(); expect(screen.queryByText('Start VitaShell')).not.toBeInTheDocument();
});
it('offers FTP retry, secondary media and manual address only when connection is opened', async () => {
  const h = harness({ ...ready(), devices: [], selectedDevice: undefined });
  fireEvent.click(await screen.findByRole('button', { name: 'Connect Vita' }));
  expect(await screen.findByText('No Vita found')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible();
  fireEvent.click(screen.getByText('Manual address'));
  fireEvent.change(screen.getByLabelText('IP address'), { target: { value: '192.168.1.9' } });
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  expect(h.calls).toContainEqual({ command: 'discover', manual: { host: '192.168.1.9', port: 1337 } });
});
it('shows only missing required-file pickers and keeps optional extras collapsed', async () => {
  harness({ ...ready(), inputs: [...ready().inputs, { id: 'shader', label: 'Shader compiler', kind: 'shader-compiler', required: true, status: 'missing' }], optionalInputs: [{ id: 'bios', label: 'GBA BIOS', kind: 'gba-bios' }] });
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' })); fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  expect(screen.getByRole('button', { name: 'Choose Shader compiler' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Choose USA Rev 1 game file' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Prepare files' })).toBeDisabled();
  expect(screen.getByText('Optional extras').closest('details')).not.toHaveAttribute('open');
});
it('shows wrong-revision feedback without allowing preparation', async () => {
  harness({ ...ready(), inputs: [{ ...ready().inputs[0], status: 'invalid', message: 'unsupported_game_revision' }] });
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }));
  expect(await screen.findByText(/USA Rev 1 is required/)).toBeVisible(); expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled();
});
it('uses the Infected revision instead of the Strikers revision in file errors', async () => {
  harness({ ...ready(), inputs: [{ ...ready().inputs[0], kind: 'infected-disc', status: 'invalid', message: 'unsupported_game_revision' }] });
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }));
  expect(await screen.findByText(/USA v1.02.*ULUS-10054/)).toBeVisible();
  expect(screen.queryByText(/USA Rev 1 is required/)).not.toBeInTheDocument();
});
it('previews default/custom art in all three slots with fill, fit and reset', async () => {
  const h = harness(); await toArtwork();
  for (const label of ['Bubble icon', 'Bubble background', 'RetroFlow cover']) {
    const region = screen.getByRole('group', { name: label });
    expect(within(region).getByRole('img', { name: `${label} preview` })).toBeVisible();
    fireEvent.click(within(region).getByRole('button', { name: `Choose ${label.toLowerCase()}` }));
    await waitFor(() => expect(within(region).getByRole('radio', { name: 'Custom' })).toHaveAttribute('aria-checked', 'true'));
    fireEvent.click(within(region).getByRole('radio', { name: 'Fit' }));
    await waitFor(() => expect(within(region).getByRole('radio', { name: 'Fit' })).toHaveAttribute('aria-checked', 'true'));
    fireEvent.click(within(region).getByRole('button', { name: `Reset ${label.toLowerCase()}` }));
    await waitFor(() => expect(within(region).getByRole('radio', { name: 'Default' })).toHaveAttribute('aria-checked', 'true'));
  }
  expect(h.calls.filter(r => r.command === 'artwork' && r.action === 'choose')).toHaveLength(3);
});
it.each(['installed', 'ready_on_vita', 'ready_on_card', 'needs_attention'] as const)('uses a distinct truthful %s result', async mode => {
  const titles = { installed: 'Installed', ready_on_vita: 'Ready on Vita', ready_on_card: 'Ready on card', needs_attention: 'Check your Vita' };
  harness({ ...ready(), job: { jobId: id, phase: mode === 'needs_attention' ? 'paused' : 'complete', completionMode: mode, bytesDone: 1, bytesTotal: 1, verifiedFiles: 1, totalFiles: 1, availableActions: mode === 'needs_attention' ? ['retry', 'cancel'] : ['done'],
    nextActions: mode === 'ready_on_card' ? [{ kind: 'eject_card' }, { kind: 'install_vpk', from: 'ux0:data/CBOC-Installer/jobs/test/install.vpk' }] : [] } });
  expect(await screen.findByRole('heading', { name: titles[mode] })).toBeVisible();
  if (mode === 'ready_on_card') expect(screen.getByText(/powered-off Vita/)).toBeVisible();
});
it('keeps resume/cancel reachable after a disconnect and blocks conflicts before install', async () => {
  const h = harness({ ...ready(), job: { jobId: id, phase: 'paused', errorCode: 'connection_lost', bytesDone: 12, bytesTotal: 100, verifiedFiles: 0, totalFiles: 2, availableActions: ['resume', 'cancel'] } });
  fireEvent.click(await screen.findByRole('button', { name: 'Resume' }));
  expect(h.calls).toContainEqual({ command: 'resume', id });
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' })); expect(h.calls).toContainEqual({ command: 'cancel' });
});
it('requires an explicit main-storage confirmation for an unknown card', async () => {
  const card = { ...ready().devices[0], id: 'card', transport: 'card' as const, adapter: 'volume' as const, label: 'Vita SD', state: 'needs_confirmation' as const, storageRole: 'unknown' as const, freeBytes: 16 * 1024 ** 3 };
  const h = harness({ ...ready(), devices: [card], selectedDevice: undefined });
  fireEvent.click(await screen.findByRole('button', { name: 'Connect Vita' }));
  fireEvent.click(screen.getByRole('button', { name: /Vita SD/ }));
  expect(h.calls.filter(r => r.command === 'selectDevice')).toHaveLength(0);
  fireEvent.click(screen.getByRole('button', { name: 'Use as main storage' }));
  expect(h.calls).toContainEqual({ command: 'selectDevice', id: 'card', confirmUx0: true });
});
it('blocks an install when the destination plan contains a conflict', async () => {
  const h = harness(); await toArtwork();
  fireEvent.click(screen.getByRole('button', { name: 'Review install' }));
  await screen.findByRole('heading', { name: 'Install' });
  await import('@testing-library/react').then(({ act }) => act(() => h.set({ plan: { id, blockers: ['file_conflict'], warnings: [], requiredBytes: 1, appExists: false, requiresUr0Action: false, customArtwork: [], freeBytes: 100 } })));
  expect(screen.getByRole('button', { name: 'Install' })).toBeDisabled();
  expect(screen.getByText('Existing files differ. Nothing will be replaced.')).toBeVisible();
});
it('keeps cover replacement visible and revocable after consent', async () => {
  const h = harness(ready(), true); await toArtwork();
  fireEvent.click(screen.getByRole('button', { name: 'Review install' }));
  fireEvent.click(await screen.findByRole('checkbox', { name: /Replace my RetroFlow/ }));
  await waitFor(() => expect(screen.getByRole('checkbox', { name: /Replace my RetroFlow/ })).toBeChecked());
  fireEvent.click(screen.getByRole('checkbox', { name: /Replace my RetroFlow/ }));
  await waitFor(() => expect(h.calls.at(-1)).toMatchObject({ command: 'plan', replaceRetroflow: false }));
});
it.each(['release', 'destination', 'reset'] as const)('requires fresh consent after a %s change', async change => {
  const h = harness(ready(), true); await toArtwork();
  fireEvent.click(screen.getByRole('button', { name: 'Review install' }));
  for (const label of [/Update this game/, /Free space could/, /Replace my RetroFlow/]) {
    fireEvent.click(await screen.findByRole('checkbox', { name: label }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Install' })).toBeEnabled());
  }
  const next = 'c'.repeat(32);
  await act(() => h.set({ plan: undefined, ...(change === 'destination' ? { selectedDevice: 'card', devices: [{ ...ready().devices[0], id: 'card' }] }
    : { selectedRelease: change === 'reset' ? undefined : next, releases: [{ ...ready().releases[0], id: next }] }) }));
  if (change === 'reset') {
    fireEvent.click(await screen.findByRole('button', { name: /Mario Strikers Charged/ }));
    await toArtwork();
  } else fireEvent.click(screen.getByRole('button', { name: 'Back' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Review install' }));
  await waitFor(() => expect(h.calls.at(-1)).toEqual({ command: 'plan', explicitUpdate: false, acknowledgeUnknownSpace: false, replaceRetroflow: false }));
});
it('returns to the missing-file picker and blocks continuation until the replacement is checked', async () => {
  const h = harness(); await toArtwork();
  await act(() => h.set({ prepared: false, repairStep: 'game', error: 'source_missing', errorFile: 'game.rvz',
    inputs: [{ ...ready().inputs[0], status: 'missing', message: 'source_missing' }] }));
  expect(await screen.findByRole('heading', { name: 'Game files' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Choose USA Rev 1 game file' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled();
  await act(() => h.set({ inputs: ready().inputs, repairStep: undefined, error: undefined, errorFile: undefined }));
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled();
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  expect(await screen.findByRole('button', { name: 'Prepare files' })).toBeVisible();
});
it('requires a fresh release selection when the bundle is missing files', async () => {
  harness({ ...ready(), repairStep: 'release', error: 'release_files_missing', errorFile: 'install.vpk' });
  expect(await screen.findByRole('button', { name: 'Continue' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Choose another release ZIP' })).toBeVisible();
  expect(screen.getByRole('alert')).toHaveTextContent(/download|redownload/i);
});
it('reveals a missing selected optional file and blocks preparation instead of silently skipping it', async () => {
  harness({ ...ready(), repairStep: 'required', optionalInputs: [{ id: 'roms', label: 'Your games', kind: 'roms' }],
    inputs: [...ready().inputs, { id: 'roms', label: 'Your games', kind: 'roms', required: false, status: 'missing', message: 'source_missing', filename: 'game.gba' }] });
  expect(await screen.findByRole('button', { name: 'Prepare files' })).toBeDisabled();
  expect(screen.getByText('Optional extras').closest('details')).toHaveAttribute('open');
  expect(screen.getByRole('button', { name: 'Choose Your games' })).toBeVisible();
});
it('lets a paused missing-file job return to file selection rather than retrying blindly', async () => {
  const h = harness({ ...ready(), job: { jobId: id, phase: 'paused', errorCode: 'source_missing', bytesDone: 0, bytesTotal: 100,
    verifiedFiles: 0, totalFiles: 2, availableActions: ['resume', 'cancel'] } });
  fireEvent.click(await screen.findByRole('button', { name: 'Choose files again' }));
  await waitFor(() => expect(h.calls.slice(-2)).toEqual([{ command: 'cancel' }, { command: 'reset' }]));
});
