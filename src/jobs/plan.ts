import type { ImportedRelease, PreparationResult, DeviceCandidate, PreparedPackage } from '../shared/contracts';
import type { Transport } from '../devices/transport';
import type { InstallPlan } from './types';
import { randomBytes } from 'node:crypto';
import { lstat } from 'node:fs/promises';
import { cancelled, requireThat } from '../shared/errors';
import { digest, hashFile } from '../shared/files';
import { verifyPreparation, SHADER_COMPILER } from '../preparation/inputs';
import { verifyVpk } from '../releases/manifest';
import { devicePath, stageRoot } from '../devices/transport';
import { observeFile, observeTrees, readSmallFile } from './observe';
import { planRetroflowCover, type RetroflowState } from '../artwork/retroflow';
import { readSfo } from '../releases/sfo';

export interface PlanOptions {
  transport: Transport; artwork: PreparedPackage; retroflow?: RetroflowState;
  acknowledgeUnknownSpace?: boolean; replaceRetroflow?: boolean;
}
export const planFingerprint = (plan: InstallPlan): string => digest(JSON.stringify({ ...plan, fingerprint: '' }));

export async function verifyPackage(release: ImportedRelease, pkg: PreparedPackage, signal?: AbortSignal) {
  requireThat(release.manifest.package && release.inventory && pkg.originalSha256 === release.manifest.package.sha256, 'package_identity_mismatch');
  requireThat(await hashFile(release.vpkPath!, signal) === pkg.originalSha256, 'package_hash_mismatch');
  const checked = await verifyVpk(pkg.path, { ...release.manifest, package: { ...release.manifest.package, sha256: pkg.sha256 } }, signal);
  requireThat(checked.members.length === release.inventory.members.length, 'package_identity_mismatch');
  for (const original of release.inventory.members) {
    const member = checked.members.find(m => m.path === original.path); requireThat(member, 'package_identity_mismatch');
    if (!['sce_sys/icon0.png', 'sce_sys/livearea/contents/bg.png'].includes(original.path))
      requireThat(member.sha256 === original.sha256, 'package_identity_mismatch');
  }
  requireThat(checked.runtimeSha256 === pkg.runtimeSha256, 'package_runtime_mismatch');
  for (const image of Object.values(pkg.images)) requireThat(await hashFile(image.path, signal) === image.outputSha256, 'artwork_changed');
}

export async function createInstallPlan(release: ImportedRelease, prepared: PreparationResult,
  device: DeviceCandidate, mode: 'first_install' | 'explicit_update', options: PlanOptions,
  signal?: AbortSignal): Promise<InstallPlan> {
  cancelled(signal); requireThat(release.manifest.releaseState === 'ready' && release.manifest.recipeId !== 'galaxy', 'release_held');
  requireThat(mode === 'first_install' || mode === 'explicit_update', 'invalid_install_mode');
  const valid = await verifyPreparation(prepared, release); cancelled(signal); requireThat(valid, 'preparation_changed');
  await verifyPackage(release, options.artwork, signal);
  const transport = options.transport; requireThat(device.state === 'ready' && await transport.identity() === device.identity, 'device_changed');
  const jobId = randomBytes(16).toString('hex'), root = stageRoot(jobId), title = release.manifest.titleId;
  const vpkDestination = `${root}/install.vpk`, appRoot = `ux0:app/${title}`, saveRoot = `ux0:user/00/savedata/${title}`;
  const appExists = !!await transport.stat(appRoot), blockers: string[] = [];
  if (appExists && mode === 'first_install') blockers.push('existing_app');
  if (appExists && mode === 'explicit_update') {
    try { if (readSfo(await readSmallFile(transport, `${appRoot}/sce_sys/param.sfo`, 65536, signal)).titleId !== title) blockers.push('installed_title_mismatch'); }
    catch { cancelled(signal); blockers.push('installed_title_mismatch'); }
  }
  const protectedRoots = [saveRoot, ...release.manifest.protectedPaths, ...(options.retroflow?.protectedPaths ?? [])];
  const protectedFiles = await observeTrees(transport, protectedRoots, signal);
  const appFiles = appExists ? await observeTrees(transport, [appRoot], signal) : [];
  for (const file of appFiles) {
    const path = file.path.slice(appRoot.length + 1), member = options.artwork.inventory.members.find(m => m.path === path);
    if ((!member && !path.startsWith('sce_sys/package/')) || (/bios/i.test(path) && member?.sha256 !== file.sha256)) blockers.push('private_app_file_conflict');
  }
  const backupFiles = mode === 'explicit_update' ? [...appFiles, ...protectedFiles] : [];
  const files: InstallPlan['files'] = [], warnings: string[] = []; let requiresUr0Action = false, coverDestination: string | undefined, observedCover;
  for (const file of prepared.files) {
    const path = file.destination, data = release.manifest.destinations.data;
    requireThat((path.startsWith(`${data}/`) || path === 'ur0:data/libshacccg.suprx'
      || (release.manifest.recipeId === 'vitadhoc' && path === 'ux0:data/GBAdhoc/gba_bios.bin'))
      && !release.manifest.protectedPaths.some(p => path === p || path.startsWith(`${p}/`)), 'protected_destination');
  }
  const sources = [...prepared.files, { source: options.artwork.path, destination: vpkDestination,
    bytes: (await lstat(options.artwork.path)).size, sha256: options.artwork.sha256 }];
  if (options.artwork.images.retroflow) {
    const cover = await planRetroflowCover(options.artwork, title, options.retroflow ?? { category: 'unknown' });
    const file = cover.action === 'manual' ? { source: cover.source, destination: `${root}/artwork/${cover.filename}`,
      bytes: (await lstat(cover.source)).size, sha256: options.artwork.images.retroflow.outputSha256 } : cover.file;
    sources.push(file);
    if (cover.action !== 'manual') coverDestination = file.destination;
  }
  const seen = new Set<string>();
  for (const source of sources) {
    cancelled(signal); devicePath(source.destination);
    const key = source.destination.normalize('NFKC').toLowerCase(); requireThat(!seen.has(key), 'input_path_collision'); seen.add(key);
    requireThat(!source.destination.startsWith('ux0:app/') && !source.destination.startsWith('ux0:user/'), 'protected_destination');
    let destination = source.destination, action: InstallPlan['files'][number]['action'] = 'copy';
    if (destination.startsWith('ur0:') && !device.capabilities.ur0) {
      requireThat(destination === 'ur0:data/libshacccg.suprx', 'unsupported_prerequisite');
      destination = `${root}/prerequisites/libshacccg.suprx`; action = 'manual'; requiresUr0Action = true;
    }
    const existing = await transport.stat(destination); let previous, backup;
    if (existing) {
      const observed = existing.kind === 'file' ? await observeFile(transport, destination, signal) : null;
      if (destination === coverDestination) observedCover = observed;
      if (observed?.sha256 === source.sha256) action = 'keep';
      else if (observed && destination === coverDestination && options.replaceRetroflow === true) {
        action = 'replace'; previous = observed; backup = `${root}/backup/retroflow.png`; backupFiles.push(observed);
      } else if (observed && (/\/gba_bios\.bin$/i.test(destination) || destination === coverDestination)) {
        action = 'preserve'; protectedRoots.push(destination); protectedFiles.push(observed);
        warnings.push(destination === coverDestination ? 'existing_cover_kept' : 'existing_bios_kept');
        if (mode === 'explicit_update') backupFiles.push(observed);
      } else blockers.push('file_conflict');
    }
    files.push({ ...source, destination, action, previous, backup, stage: `${root}/parts/${files.length}.part` });
  }
  if (release.manifest.prerequisites.includes('shader-compiler')) {
    const supplied = files.some(f => f.destination.endsWith('/libshacccg.suprx') && f.sha256 === SHADER_COMPILER);
    if (device.capabilities.ur0) {
      const path = 'ur0:data/libshacccg.suprx', existing = await transport.stat(path);
      if (existing && (existing.kind !== 'file' || (await observeFile(transport, path, signal)).sha256 !== SHADER_COMPILER)) blockers.push('prerequisite_conflict');
      else if (!existing && !supplied) blockers.push('missing_prerequisite');
    } else { requiresUr0Action = true; if (!supplied) blockers.push('missing_prerequisite'); }
  }
  const copied = files.filter(f => ['copy', 'manual', 'replace'].includes(f.action)).reduce((n, f) => n + f.bytes, 0);
  const appBytes = options.artwork.inventory.members.reduce((n, m) => n + m.bytes, 0);
  const requiredBytes = copied * 2 + appBytes * 2 + backupFiles.reduce((n, f) => n + f.bytes, 0) + 16 * 1024 ** 2;
  const freeBytes = await transport.freeSpace();
  if (freeBytes === undefined || !Number.isSafeInteger(freeBytes)) {
    warnings.push('capacity_unverified'); if (options.acknowledgeUnknownSpace !== true) blockers.push('capacity_unknown');
  }
  else if (freeBytes < requiredBytes) blockers.push('insufficient_space');
  protectedFiles.sort((a, b) => a.path.localeCompare(b.path)); backupFiles.sort((a, b) => a.path.localeCompare(b.path));
  const consentFingerprint = digest(JSON.stringify({ manifest: release.manifestSha256, target: device.identity,
    transport: device.transport, package: options.artwork.sha256, inputs: prepared.inputFingerprints,
    appFiles, coverDestination, observedCover, unknownSpace: freeBytes === undefined }));
  const plan: InstallPlan = { jobId, fingerprint: '', consentFingerprint, releaseId: release.manifest.releaseId, titleId: title, manifestSha256: release.manifestSha256,
    recipeFingerprint: prepared.recipeFingerprint, inputFingerprints: structuredClone(prepared.inputFingerprints),
    toolFingerprints: structuredClone(prepared.toolFingerprints ?? {}), target: structuredClone(device), mode,
    package: structuredClone(options.artwork), files, protectedRoots, protectedFiles, backupFiles, blockers: [...new Set(blockers)], warnings, requiredBytes, freeBytes,
    vpkDestination, requiresUr0Action, appExists };
  plan.fingerprint = planFingerprint(plan); cancelled(signal); return plan;
}
