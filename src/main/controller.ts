import { emptySnapshot, type UiSnapshot, type UiRequest } from '../shared/ui';
import type { ArtworkChoice, ArtworkSlot, DeviceCandidate, ImportedRelease, PreparationResult, PreparedPackage, JobSnapshot, InputCheck } from '../shared/contracts';
import type { TrustStore } from '../releases/trust';
import type { InputSelections, PreparationTools } from '../preparation/inputs';
import type { InstallPlan } from '../jobs/types';
import type { Transport, TransportOptions, VolumeWorker } from '../devices/transport';
import { randomBytes } from 'node:crypto';
import { basename, join } from 'node:path';
import { mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import sharp from 'sharp';
import { importRelease } from '../releases/import';
import { prepareArtwork } from '../artwork/prepare';
import { convertImage, imageSize, readImage } from '../artwork/images';
import { inspectInputs, prepareInputs, SHADER_COMPILER, InputCheckError } from '../preparation/inputs';
import { createInstallPlan } from '../jobs/plan';
import { executeTransfer, writeScope } from '../jobs/transfer';
import { FileJournal } from '../jobs/journal';
import { finishInstall } from '../jobs/install';
import { nativeInstallationServices } from '../devices/installation';
import { inspectDevice, nativeDiscoveryIO } from '../devices/native';
import { discoverDevices } from '../devices/discover';
import { openTransport } from '../devices/transport';
import { readSmallFile } from '../jobs/observe';
import { digest, hashFile } from '../shared/files';
import { requireThat } from '../shared/errors';
import { safeError } from './security';
import { checkPublicRelease, releasePage } from '../releases/github';
import type { DownloadProgress } from '../releases/download';
import { detectRetroflow } from '../artwork/detect';
import { FileIntegrityError, verifyFile } from '../shared/integrity';

interface Options {
  userData: string; trust: TrustStore; volumeWorker?: VolumeWorker;
  tools(recipe: string, signal?: AbortSignal): Promise<PreparationTools>;
  discover?: (signal: AbortSignal, manual?: { host: string; port: number }) => AsyncIterable<DeviceCandidate>;
  inspect?: typeof inspectDevice; open?: (device: DeviceCandidate, options?: TransportOptions) => Promise<Transport>;
  checkRelease?: typeof checkPublicRelease;
  downloadRelease?: (url: string, expectedManifestSha256: string | undefined, signal: AbortSignal, onProgress: (progress: DownloadProgress) => void) => Promise<ImportedRelease>;
}
export class InstallerController {
  private state = emptySnapshot(); private releases = new Map<string, ImportedRelease>(); private devices = new Map<string, DeviceCandidate>();
  private selections: InputSelections = { roots: [], chosen: {} }; private choices: Partial<Record<ArtworkSlot, ArtworkChoice>> = {};
  private sourceHashes: Partial<Record<ArtworkSlot, string>> = {}; private previewHashes: Partial<Record<ArtworkSlot, string>> = {};
  private defaults?: PreparedPackage; private prepared?: PreparationResult; private plan?: InstallPlan;
  private abort?: AbortController; private discoveryAbort?: AbortController; private paused = false;
  private discoveryTask?: Promise<void>;
  private owned = new Set<string>(); private retained = new Set<string>(); private prerequisitePath?: string;
  private journal: FileJournal;
  constructor(private options: Options, private publish: (state: UiSnapshot) => void) { this.journal = new FileJournal(join(options.userData, 'jobs')); }
  snapshot() { return structuredClone(this.state); }
  private emit() { this.publish(this.snapshot()); }
  private release() { const release = this.releases.get(this.state.selectedRelease!); requireThat(release, 'unknown_release'); return release; }
  private selectedDevice() { const device = this.devices.get(this.state.selectedDevice!); requireThat(device?.state === 'ready', 'device_not_ready'); return device; }
  private async open(device: DeviceCandidate, options: TransportOptions = {}) {
    return (this.options.open ?? openTransport)(device, { volumeWorker: this.options.volumeWorker, ...options });
  }
  private remember(workspace?: string) { if (workspace) this.owned.add(workspace); }
  private invalidate(prepared = false) {
    this.plan = undefined; this.state.plan = undefined;
    if (prepared) { this.prepared = undefined; this.state.prepared = false; }
  }
  private inputIssue(check: InputCheck) {
    const role = this.state.inputs.find(i => i.id === check.id);
    this.state.inputs = this.state.inputs.map(i => i.id === check.id ? { ...i, status: check.status, message: check.message, problemFile: check.problemFile } : i);
    this.state.error = check.message ?? 'source_missing';
    this.state.errorFile = check.problemFile ?? role?.filename ?? (check.source ? basename(check.source) : undefined);
    this.state.repairStep = role?.kind.endsWith('-disc') ? 'game' : 'required';
    this.invalidate(true);
  }
  private async checkRelease(signal: AbortSignal) {
    const release = this.release();
    try {
      await verifyFile(join(release.sourceRoot, 'cboc-release.json'), { sha256: release.manifestSha256 }, signal);
      if (release.vpkPath && release.manifest.package) await verifyFile(release.vpkPath, { sha256: release.manifest.package.sha256 }, signal);
      return false;
    } catch (error) {
      if (!(error instanceof FileIntegrityError) || !['source_missing', 'source_changed'].includes(error.code) || !this.options.downloadRelease) throw error;
      const repaired = await this.download(release.downloadUrl ?? releasePage(release.manifest.recipeId), signal, release.manifestSha256);
      repaired.inputRoots = release.inputRoots ?? [release.sourceRoot];
      repaired.inputWorkspaces = [...new Set([...(release.inputWorkspaces ?? []), ...(release.ownedWorkspace ? [release.ownedWorkspace] : [])])];
      this.releases.set(this.state.selectedRelease!, repaired); this.remember(repaired.ownedWorkspace);
      this.invalidate(); this.state.repairStep = undefined; return true;
    }
  }
  private async download(url: string, signal: AbortSignal, expected?: string) {
    requireThat(this.options.downloadRelease, 'release_download_unavailable');
    const busy = this.state.busy; this.state.busy = expected ? 'repairing' : 'downloading'; this.state.download = { bytes: 0 }; this.emit();
    let last = 0;
    try {
      const release = await this.options.downloadRelease(url, expected, signal, progress => {
        this.state.download = progress;
        if (Date.now() - last > 100 || progress.bytes === progress.total) { last = Date.now(); this.emit(); }
      });
      if (expected) { requireThat(release.manifestSha256 === expected, 'repair_version_unavailable'); this.state.notice = 'release_repaired'; }
      return release;
    } catch (error) { if (expected) { this.invalidate(true); this.state.repairStep = 'release'; } throw error; }
    finally { this.state.busy = busy; this.state.download = undefined; this.emit(); }
  }
  private fileIssue(error: FileIntegrityError) {
    this.state.error = error.code; this.state.errorFile = basename(error.path);
    if (this.state.job) return;
    const release = this.releases.get(this.state.selectedRelease!);
    if (release && [release.vpkPath, join(release.sourceRoot, 'cboc-release.json')].includes(error.path)) {
      this.invalidate(true); this.state.repairStep = 'release';
      this.state.error = error.code === 'source_missing' ? 'release_files_missing' : 'release_files_changed'; return;
    }
    if (Object.values(this.choices).some(choice => choice.source === error.path)
      || Object.values(this.plan?.package.images ?? {}).some(image => image.path === error.path)
      || this.plan?.package.path === error.path) {
      this.invalidate(); this.state.repairStep = 'artwork';
      if (error.code === 'source_changed') this.state.error = 'artwork_changed'; return;
    }
    const file = this.prepared?.files.find(file => file.source === error.path);
    const input = this.state.inputs.find(input => input.id === file?.inputId)
      ?? this.state.inputs.find(input => this.selections.chosen[input.id]?.some(path => error.path === path || error.path.startsWith(path + '/')))
      ?? this.state.inputs.find(input => input.kind.endsWith('-disc'));
    if (input) this.inputIssue({ ...input, status: error.code === 'source_missing' ? 'missing' : 'invalid', message: error.code, problemFile: basename(error.path) });
    else { this.invalidate(true); this.state.repairStep = 'required'; }
  }
  private async preview(bytes: Buffer) { return `data:image/png;base64,${(await sharp(bytes).resize({ width: 480, height: 320, fit: 'inside', withoutEnlargement: true }).png().toBuffer()).toString('base64')}`; }
  async initialize() {
    await mkdir(this.options.userData, { recursive: true, mode: 0o700 });
    const files = await readdir(this.journal.root).catch(() => []);
    for (const file of files.filter(f => /^[a-f0-9]{32}\.json$/.test(f)).slice(0, 100)) {
      try {
        const record = await this.journal.load(file.slice(0, -5));
        if (record.snapshot.phase !== 'complete' && record.snapshot.phase !== 'cancelled') this.state.recoverableJobs.push({ id: record.plan.jobId, title: record.plan.titleId, phase: record.snapshot.phase });
      } catch { /* A corrupt journal is never resumed implicitly. */ }
    }
    this.emit();
  }
  async command(request: UiRequest, paths: string[] = []): Promise<void> {
    if (request.command === 'clearError') { this.state.error = undefined; this.state.errorFile = undefined; this.emit(); return; }
    if (request.command === 'pause' || request.command === 'cancel') {
      if (this.state.job && !this.state.job.availableActions.includes('cancel')) return;
      this.paused = request.command === 'pause'; this.abort?.abort(); this.discoveryAbort?.abort();
      if (!this.state.busy && this.state.job) {
        const record = await this.journal.load(this.state.job.jobId);
        if (this.paused && record.promotion !== 'not_started') return;
        record.snapshot = { ...record.snapshot, phase: this.paused ? 'paused' : 'cancelled', completionMode: undefined,
          availableActions: this.paused ? ['resume', 'cancel'] : ['done'] };
        await this.journal.save(record); this.state.job = record.snapshot; this.emit();
      }
      return;
    }
    if (this.state.busy || (this.state.job && !['resume', 'reset', 'discover', 'selectDevice', 'openRepo'].includes(request.command))) {
      this.state.error = 'operation_in_progress'; this.emit(); return;
    }
    if (request.command === 'discover') {
      if (this.discoveryTask) return this.discoveryTask;
      this.discoveryTask = this.discover(request.manual);
      try { await this.discoveryTask; } finally { this.discoveryTask = undefined; } return;
    }
    if (request.command === 'openRepo') return;
    if (this.discoveryTask) { this.discoveryAbort?.abort(); await this.discoveryTask; }
    this.state.error = undefined; this.state.errorFile = undefined; this.state.notice = undefined; this.abort = new AbortController(); const signal = this.abort.signal;
    try {
      if (request.command === 'import' || request.command === 'importUrl') {
        if (request.command === 'import' && !paths.length) return;
        this.state.busy = 'importing'; this.emit();
        const imported = request.command === 'importUrl' ? [await this.download(request.url, signal)]
          : await importRelease(paths[0], this.options.trust, signal, this.options.downloadRelease
            ? (manifest, sha) => this.download(releasePage(manifest.recipeId), signal, sha) : undefined);
        this.state.repairStep = undefined;
        this.releases.clear(); this.state.releases = []; this.state.selectedRelease = undefined; this.invalidate(true);
        for (const release of imported) {
          this.remember(release.ownedWorkspace); const id = randomBytes(16).toString('hex'); this.releases.set(id, release);
          for (const workspace of release.inputWorkspaces ?? []) this.remember(workspace);
          let image: string | undefined;
          if (release.manifest.releaseState === 'ready') {
            const artwork = await prepareArtwork(release, {}, signal); this.remember(artwork.workspace);
            image = await this.preview(await readFile(artwork.images.bubble!.path));
          }
          this.state.releases.push({ id, name: release.manifest.displayName, version: release.manifest.version,
            held: release.manifest.releaseState !== 'ready', recipe: release.manifest.recipeId, image });
        }
        await this.checkInstalled(signal);
        if (imported.length === 1 && !this.state.releases[0].held) await this.selectRelease(this.state.releases[0].id, signal);
      } else if (request.command === 'checkRelease') {
        this.state.busy = 'scanning'; this.emit();
        this.state.publicRelease = await (this.options.checkRelease ?? checkPublicRelease)(this.release().manifest.recipeId);
      } else if (request.command === 'selectRelease') {
        this.state.busy = 'scanning'; this.emit(); await this.selectRelease(request.id, signal);
      } else if (request.command === 'selectDevice') {
        const device = this.devices.get(request.id); requireThat(device, 'unknown_device');
        const selected = request.confirmUx0 && device.storageRole === 'unknown' ? { ...device, storageRole: 'ux0' as const, state: 'ready' as const } : device;
        requireThat(selected.state === 'ready', 'storage_role_unknown');
        this.state.busy = 'scanning'; this.emit();
        const current = await (this.options.inspect ?? inspectDevice)(selected, signal); requireThat(current.state === 'ready', 'device_not_ready');
        this.devices.set(current.id, current); this.state.selectedDevice = current.id; this.invalidate(); this.updateDeviceViews();
        await this.checkInstalled(signal);
        if (this.state.selectedRelease) { this.state.busy = 'scanning'; this.emit(); await this.scan(signal); }
      } else if (request.command === 'chooseInput' || request.command === 'scanFolder' || request.command === 'removeInput') {
        const release = this.release();
        if (request.command !== 'scanFolder') requireThat([...release.manifest.requiredInputs, ...release.manifest.optionalInputs].some(i => i.id === request.id), 'unsupported_input');
        if (request.command === 'removeInput') delete this.selections.chosen[request.id];
        else { if (!paths.length) return; if (request.command === 'scanFolder') this.selections.roots = [...new Set([...this.selections.roots, paths[0]])]; else this.selections.chosen[request.id] = paths; }
        this.invalidate(true); this.state.busy = 'scanning'; this.emit(); await this.scan(signal);
        if (this.state.repairStep) {
          const issue = this.state.inputs.find(input => !['ready', 'unverified'].includes(input.status));
          if (issue) this.inputIssue(issue); else this.state.repairStep = undefined;
        }
      } else if (request.command === 'artwork') {
        this.state.busy = 'artwork'; this.emit(); await this.updateArtwork(request, paths[0], signal); this.invalidate();
      } else if (request.command === 'prepare') {
        this.selectedDevice(); this.invalidate(true); this.state.busy = 'preparing'; this.emit();
        await this.checkRelease(signal); await this.scan(signal);
        const issue = this.state.inputs.find(input => !['ready', 'unverified'].includes(input.status));
        if (issue) { this.inputIssue(issue); return; }
        let lastProgress = 0;
        this.prepared = await prepareInputs(this.release(), this.selections, p => {
          const changed = this.state.preparation?.phase !== p.phase || this.state.preparation?.item !== p.item || this.state.preparation?.unit !== p.unit;
          this.state.preparation = p;
          if (changed || Date.now() - lastProgress > 100 || p.completed === p.total) { lastProgress = Date.now(); this.emit(); }
        }, signal,
          await this.options.tools(this.release().manifest.recipeId, signal));
        this.remember(this.prepared.workspace); this.state.prepared = true; this.state.repairStep = undefined;
      } else if (request.command === 'plan') {
        let reviewed = this.plan;
        this.invalidate(); requireThat(this.prepared, 'preparation_required'); this.state.busy = 'planning'; this.emit();
        if (await this.checkRelease(signal)) reviewed = undefined;
        for (const file of this.prepared.files) await verifyFile(file.source, file, signal);
        for (const [slot, choice] of Object.entries(this.choices)) if (choice.source)
          await verifyFile(choice.source, { sha256: this.sourceHashes[slot as ArtworkSlot] }, signal);
        const artwork = await prepareArtwork(this.release(), this.choices, signal); this.remember(artwork.workspace);
        for (const slot of Object.keys(this.previewHashes) as ArtworkSlot[]) requireThat(artwork.images[slot]?.outputSha256 === this.previewHashes[slot], 'artwork_changed');
        const device = await (this.options.inspect ?? inspectDevice)(this.selectedDevice(), signal); requireThat(device.state === 'ready', 'device_not_ready');
        this.devices.set(device.id, device); this.updateDeviceViews(); const transport = await this.open(device);
        let consents = { update: false, space: false, cover: false };
        try {
          const options = { transport, artwork, retroflow: await detectRetroflow(transport, this.release().manifest.titleId, signal) };
          const baseline = await createInstallPlan(this.release(), this.prepared, device, 'first_install', options, signal);
          this.plan = baseline;
          if (reviewed?.consentFingerprint === baseline.consentFingerprint) {
            consents = { update: request.explicitUpdate && baseline.appExists, space: request.acknowledgeUnknownSpace,
              cover: request.replaceRetroflow && baseline.warnings.includes('existing_cover_kept') };
            const approved = await createInstallPlan(this.release(), this.prepared, device, consents.update ? 'explicit_update' : 'first_install', {
              ...options, acknowledgeUnknownSpace: consents.space, replaceRetroflow: consents.cover }, signal);
            if (approved.consentFingerprint === baseline.consentFingerprint) this.plan = approved;
            else consents = { update: false, space: false, cover: false };
          }
        }
        finally { await transport.close(); }
        const p = this.plan; this.state.plan = { id: p.jobId, blockers: p.blockers, warnings: p.warnings, appExists: p.appExists,
          requiredBytes: p.requiredBytes, freeBytes: p.freeBytes, requiresUr0Action: p.requiresUr0Action, customArtwork: Object.keys(this.choices) as ArtworkSlot[], consents };
        this.state.repairStep = undefined;
      } else if (request.command === 'execute' || request.command === 'resume') {
        let plan: InstallPlan;
        if (request.command === 'execute') { requireThat(this.plan?.jobId === request.id && !this.plan.blockers.length, 'install_blocked'); plan = this.plan; }
        else {
          const record = await this.journal.load(request.id); plan = record.plan;
          requireThat(this.options.trust.fingerprints.has(plan.manifestSha256), 'untrusted_release');
        }
        this.state.busy = 'installing'; this.emit(); this.paused = false;
        if (request.command === 'execute' && await this.checkRelease(signal)) {
          this.state.repairStep = 'artwork'; requireThat(false, 'release_repaired_review');
        }
        const device = await (this.options.inspect ?? inspectDevice)(plan.target, signal); requireThat(device.state === 'ready' && device.identity === plan.target.identity, 'device_changed');
        this.devices.set(device.id, device); this.state.selectedDevice = device.id; this.updateDeviceViews();
        const transport = await this.open(device, { scope: writeScope(plan) });
        for (const workspace of this.owned) this.retained.add(workspace);
        const report = (snapshot: JobSnapshot) => { this.state.job = structuredClone(snapshot); this.emit(); };
        try {
          const previous = await this.journal.load(plan.jobId).catch(error => { if (error.code === 'ENOENT') return undefined; throw error; });
          let snapshot = previous?.snapshot;
          if (!previous || previous.promotion === 'not_started') snapshot = await executeTransfer(plan, transport, this.journal, report, signal);
          if (!signal.aborted && (snapshot?.phase === 'verifying' || previous?.promotion !== undefined && previous.promotion !== 'not_started'))
            await finishInstall(plan, transport, this.journal, report, nativeInstallationServices(plan, transport), signal);
        } catch (error) {
          if (!signal.aborted) {
            const record = await this.journal.load(plan.jobId).catch(() => undefined);
            if (record && record.promotion === 'not_started') {
              record.snapshot = { ...record.snapshot, phase: 'paused', completionMode: undefined,
                errorCode: safeError(error), availableActions: ['resume', 'cancel'] };
              await this.journal.save(record); report(record.snapshot);
            }
            throw error;
          }
        }
        finally {
          if (signal.aborted && this.state.job) {
            const record = await this.journal.load(plan.jobId); record.snapshot.phase = this.paused ? 'paused' : 'cancelled';
            record.snapshot.errorCode = this.paused ? undefined : 'cancelled'; record.snapshot.availableActions = this.paused ? ['resume', 'cancel'] : ['done'];
            await this.journal.save(record); report(record.snapshot);
          }
          await transport.close();
        }
      } else if (request.command === 'reset') {
        this.invalidate(true); this.state.job = undefined; this.state.selectedRelease = undefined; this.state.inputs = []; this.state.artwork = [];
        this.state.recoverableJobs = []; this.state.repairStep = undefined; this.choices = {}; this.selections = { roots: [], chosen: {} };
      }
    } catch (error) {
      if (error instanceof InputCheckError) this.inputIssue(error.check);
      else if (error instanceof FileIntegrityError) this.fileIssue(error);
      else {
        this.state.error = safeError(error);
        if (request.command === 'import' && ['ENOENT', 'ENOTDIR', 'package_hash_mismatch'].includes((error as { code?: string }).code ?? '')) {
          this.invalidate(true); this.state.repairStep = 'release';
          this.state.error = (error as { code?: string }).code === 'package_hash_mismatch' ? 'release_files_changed' : 'release_files_missing';
        }
      }
    }
    finally { this.state.busy = undefined; this.state.preparation = undefined; this.abort = undefined; this.emit(); }
  }
  private updateDeviceViews() {
    this.state.devices = [...this.devices.values()].map(({ root: _root, identity: _identity, ...device }) => device);
  }
  private async checkInstalled(signal: AbortSignal) {
    this.state.releases = this.state.releases.map(({ installed: _installed, ...release }) => release);
    const device = this.devices.get(this.state.selectedDevice!);
    if (device?.state !== 'ready' || !this.state.releases.length) return;
    const transport = await this.open(device);
    try {
      const apps = new Set((await transport.list('ux0:app')).map(entry => entry.name));
      if (signal.aborted) return;
      this.state.releases = this.state.releases.map(view => ({ ...view, installed: apps.has(this.releases.get(view.id)!.manifest.titleId) }));
    } finally { await transport.close(); }
  }
  private async discover(manual?: { host: string; port: number }) {
    if (this.state.discoveryBusy) return;
    this.discoveryAbort = new AbortController(); this.state.discoveryBusy = true; this.state.error = undefined; this.emit();
    const selected = this.state.selectedDevice;
    try {
      const source = this.options.discover ? this.options.discover(this.discoveryAbort.signal, manual) : discoverDevices({ io: nativeDiscoveryIO, manual,
        onStatus: status => { this.state.discoveryStatus = status; } }, this.discoveryAbort.signal);
      const found = new Map<string, DeviceCandidate>();
      for await (const candidate of source) { found.set(candidate.id, candidate); this.devices.set(candidate.id, candidate); this.updateDeviceViews(); this.emit(); }
      this.devices = found;
      const ready = [...found.values()].filter(d => d.state === 'ready');
      this.state.selectedDevice = selected && found.get(selected)?.state === 'ready' ? selected : ready.length === 1 ? ready[0].id : undefined;
      if (this.state.selectedDevice !== selected) this.invalidate();
      this.updateDeviceViews();
      await this.checkInstalled(this.discoveryAbort.signal);
      if (this.state.selectedRelease && this.state.selectedDevice && !this.state.job) await this.scan(this.discoveryAbort.signal);
    } catch (error) { this.state.error = safeError(error); }
    finally { this.state.discoveryBusy = false; this.discoveryAbort = undefined; this.emit(); }
  }
  private async selectRelease(id: string, signal: AbortSignal) {
    const release = this.releases.get(id); requireThat(release, 'unknown_release'); requireThat(release.manifest.releaseState === 'ready', 'release_held');
    this.state.selectedRelease = id; this.invalidate(true); this.state.inputs = []; this.state.artwork = []; this.defaults = undefined;
    this.state.publicRelease = undefined; this.state.repairStep = undefined;
    this.selections = { roots: [...(release.inputRoots ?? [release.sourceRoot])], chosen: {} }; this.choices = {}; this.sourceHashes = {}; this.previewHashes = {};
    await this.checkRelease(signal);
    this.defaults = await prepareArtwork(this.release(), {}, signal); this.remember(this.defaults.workspace);
    this.state.artwork = [];
    for (const slot of ['bubble', 'livearea', 'retroflow'] as const) this.state.artwork.push({ slot, mode: 'default', fit: 'cover', x: .5, y: .5,
      preview: await this.preview(await readFile(this.defaults.images[slot]!.path)) });
    this.state.optionalInputs = release.manifest.optionalInputs; await this.scan(signal);
  }
  private async scan(signal: AbortSignal) {
    const release = this.release(), tools = await this.options.tools(release.manifest.recipeId, signal);
    const shader = release.manifest.requiredInputs.find(i => i.kind === 'shader-compiler');
    const device = this.devices.get(this.state.selectedDevice!);
    if (shader && !this.selections.chosen[shader.id]?.length && device?.capabilities.ur0 && device.state === 'ready') {
      const transport = await this.open(device);
      try {
        const path = 'ur0:data/libshacccg.suprx', exists = await transport.stat(path);
        if (exists?.kind === 'file') {
          const data = await readSmallFile(transport, path, 8 * 1024 ** 2, signal);
          if (digest(data) === SHADER_COMPILER) {
            const cache = join(this.options.userData, 'prerequisites'); await mkdir(cache, { recursive: true, mode: 0o700 });
            this.prerequisitePath = join(cache, `${SHADER_COMPILER}.suprx`); await writeFile(this.prerequisitePath, data, { mode: 0o600 });
            this.selections.chosen[shader.id] = [this.prerequisitePath];
          }
        }
      } finally { await transport.close(); }
    }
    const checks = await inspectInputs(release, this.selections, tools, signal);
    this.state.inputs = checks.map(({ source, ...check }) => {
      const role = [...release.manifest.requiredInputs, ...release.manifest.optionalInputs].find(i => i.id === check.id);
      const selected = this.selections.chosen[check.id]?.[0];
      return { ...check, kind: role?.kind ?? '',
        filename: source ? basename(source) : selected ? basename(selected) : undefined, onVita: source !== undefined && source === this.prerequisitePath };
    });
  }
  private async updateArtwork(request: Extract<UiRequest, { command: 'artwork' }>, path: string | undefined, signal: AbortSignal) {
    this.release(); requireThat(this.defaults, 'missing_artwork'); const slot = request.slot;
    const original = await readFile(this.defaults.images[slot]!.path);
    if (request.action === 'choose' && !path) return;
    let choice: ArtworkChoice | undefined;
    if (request.action !== 'default') {
      const old = this.choices[slot]; requireThat(path || old?.source, 'missing_artwork');
      choice = { source: path ?? old!.source, fit: request.fit ?? old?.fit ?? 'cover', focalPoint: { x: request.x ?? old?.focalPoint?.x ?? .5, y: request.y ?? old?.focalPoint?.y ?? .5 } };
    }
    let image: Buffer = original, sourceHash: string | undefined;
    if (choice?.source) { const source = await readImage(choice.source, signal); sourceHash = digest(source); image = await convertImage(source, await imageSize(original), choice, slot !== 'retroflow', signal); }
    const preview = await this.preview(image);
    if (choice) { this.choices[slot] = choice; this.sourceHashes[slot] = sourceHash; this.previewHashes[slot] = digest(image); }
    else { delete this.choices[slot]; delete this.sourceHashes[slot]; delete this.previewHashes[slot]; }
    this.state.artwork = this.state.artwork.map(item => item.slot !== slot ? item : { slot, mode: choice ? 'custom' : 'default', preview,
      filename: choice?.source ? basename(choice.source) : undefined, fit: choice?.fit ?? 'cover', x: choice?.focalPoint?.x ?? .5, y: choice?.focalPoint?.y ?? .5 });
  }
  async dispose() {
    this.abort?.abort(); this.discoveryAbort?.abort();
    for (const workspace of this.owned) if (!this.retained.has(workspace)) await rm(workspace, { recursive: true, force: true });
  }
}
