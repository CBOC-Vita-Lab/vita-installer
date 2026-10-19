import type { ArtworkSlot, InputCheck, JobSnapshot, DeviceCandidate, PreparationProgress } from './contracts';
export type Step = 'release' | 'game' | 'required' | 'artwork' | 'install';
export interface ReleaseView { id: string; name: string; version: string; held: boolean; image?: string; recipe: string; installed?: boolean }
export interface InputView extends Omit<InputCheck, 'source'> { filename?: string; kind: string; onVita?: boolean }
export interface ArtworkView { slot: ArtworkSlot; mode: 'default' | 'custom'; preview: string; fit: 'cover' | 'contain'; x: number; y: number; filename?: string }
export interface DeviceView extends Omit<DeviceCandidate, 'root' | 'identity'> {}
export interface PlanView { id: string; blockers: string[]; warnings: string[]; requiredBytes: number; freeBytes?: number; appExists: boolean; requiresUr0Action: boolean; customArtwork: ArtworkSlot[];
  appOnly?: boolean; transferBytes?: number;
  consents?: { update: boolean; space: boolean; cover: boolean } }
export interface UiSnapshot {
  releases: ReleaseView[]; selectedRelease?: string; inputs: InputView[];
  optionalInputs: { id: string; label: string; kind: string }[];
  devices: DeviceView[]; selectedDevice?: string; discoveryStatus?: string;
  artwork: ArtworkView[]; prepared: boolean; plan?: PlanView; job?: JobSnapshot;
  appUpdateAvailable?: boolean; appUpdateSelected?: boolean;
  busy?: 'importing' | 'scanning' | 'preparing' | 'planning' | 'installing' | 'artwork' | 'downloading' | 'repairing';
  download?: { bytes: number; total?: number }; notice?: 'release_repaired';
  error?: string; errorFile?: string; repairStep?: Step; discoveryBusy: boolean; preparation?: PreparationProgress;
  recoverableJobs: { id: string; title: string; phase: string }[];
  publicRelease?: { status: 'available' | 'unavailable'; tag?: string };
}
export const emptySnapshot = (): UiSnapshot => ({ releases: [], inputs: [], optionalInputs: [], devices: [], artwork: [], prepared: false, discoveryBusy: false, recoverableJobs: [] });
export type UiRequest =
  | { command: 'import'; kind: 'file' | 'folder' }
  | { command: 'importUrl'; url: string }
  | { command: 'selectRelease'; id: string }
  | { command: 'discover'; manual?: { host: string; port: number } }
  | { command: 'selectDevice'; id: string; confirmUx0?: boolean }
  | { command: 'chooseInput'; id: string; kind: 'file' | 'folder' }
  | { command: 'scanFolder' }
  | { command: 'checkRelease' }
  | { command: 'removeInput'; id: string }
  | { command: 'artwork'; slot: ArtworkSlot; action: 'default' | 'choose' | 'adjust'; fit?: 'cover' | 'contain'; x?: number; y?: number }
  | { command: 'prepare' | 'prepareUpdate' | 'fullSetup' }
  | { command: 'plan'; acknowledgeUnknownSpace: boolean; explicitUpdate: boolean; replaceRetroflow: boolean }
  | { command: 'execute'; id: string }
  | { command: 'resume'; id: string }
  | { command: 'pause' | 'cancel' | 'clearError' | 'reset' }
  | { command: 'openRepo'; repo: 'installer' | 'strikers' | 'infected' | 'vitadhoc' | 'galaxy' };
export interface InstallerBridge {
  getSnapshot(): Promise<UiSnapshot>;
  command(request: UiRequest): Promise<void>;
  subscribe(callback: (snapshot: UiSnapshot) => void): () => void;
}
declare global { interface Window { vitaInstaller?: InstallerBridge } }
