import type { DeviceCandidate, PreparedFile, PreparedPackage } from '../shared/contracts';
export interface ObservedFile { path: string; bytes: number; sha256: string }
export interface PlannedFile extends PreparedFile {
  stage: string; action: 'copy' | 'keep' | 'preserve' | 'manual' | 'replace';
  previous?: ObservedFile; backup?: string;
}
export interface InstallPlan {
  jobId: string; fingerprint: string; consentFingerprint: string; releaseId: string; manifestSha256: string; titleId: string;
  recipeFingerprint: string; inputFingerprints: Record<string, string>; target: DeviceCandidate;
  toolFingerprints: Record<string, string>;
  mode: 'first_install' | 'explicit_update'; package: PreparedPackage;
  files: PlannedFile[]; protectedRoots: string[]; protectedFiles: ObservedFile[]; backupFiles: ObservedFile[];
  blockers: string[]; warnings: string[]; requiredBytes: number; freeBytes?: number; vpkDestination: string;
  requiresUr0Action: boolean; appExists: boolean;
}
