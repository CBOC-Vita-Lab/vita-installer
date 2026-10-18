import type { ReleaseManifest, PackageInventory, PackageMember } from '../releases/manifest';
export type { ReleaseManifest, PackageInventory, PackageMember };
export interface ImportedRelease {
  manifest: ReleaseManifest; manifestSha256: string; sourceRoot: string;
  vpkPath: string | null; inventory: PackageInventory | null; ownedWorkspace?: string;
}
export interface InputCheck {
  id: string; label: string; status: 'ready' | 'missing' | 'invalid' | 'unverified';
  required: boolean; source?: string; message?: string;
}
export interface PreparedFile { source: string; destination: string; bytes: number; sha256: string }
export interface PreparationResult {
  releaseId: string; inputFingerprints: Record<string, string>; recipeFingerprint: string;
  toolFingerprints?: Record<string, string>;
  files: PreparedFile[]; workspace: string;
}
export type ArtworkSlot = 'bubble' | 'livearea' | 'retroflow';
export interface ArtworkChoice { source?: string; fit: 'cover' | 'contain'; focalPoint?: { x: number; y: number } }
export interface PreparedPackage {
  originalSha256: string; sha256: string; runtimeSha256: string; path: string;
  images: Partial<Record<ArtworkSlot, { sourceSha256: string; outputSha256: string; path: string }>>;
  inventory: PackageInventory; workspace?: string;
}
export interface DeviceCandidate {
  id: string; transport: 'ftp' | 'usb' | 'card'; label: string;
  identity: string; root?: string; host?: string; port?: number;
  storageRole: 'ux0' | 'unknown'; adapter: 'vitashell' | 'companion' | 'ftp' | 'volume';
  capabilities: { transfer: boolean; install: boolean; ur0: boolean };
  evidence: string[]; state: 'ready' | 'offline' | 'needs_confirmation';
  freeBytes?: number;
}
export type CompletionMode = 'installed' | 'ready_on_vita' | 'ready_on_card' | 'needs_attention';
export interface JobSnapshot {
  jobId: string; phase: 'preparing' | 'checking' | 'transferring' | 'verifying' | 'installing' | 'paused' | 'cancelled' | 'complete' | 'failed';
  bytesDone: number; bytesTotal: number; verifiedFiles: number; totalFiles: number;
  errorCode?: string; availableActions: ('retry' | 'resume' | 'cancel' | 'done')[];
  completionMode?: CompletionMode; vpkDestination?: string;
  nextActions?: { kind: 'install_vpk' | 'copy_prerequisite' | 'import_cover' | 'eject_card'; from?: string; to?: string }[];
}
