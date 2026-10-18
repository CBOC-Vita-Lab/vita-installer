import type { PreparedFile, PreparedPackage } from '../shared/contracts';
import { lstat } from 'node:fs/promises';
import { hashFile } from '../shared/files';
import { requireThat } from '../shared/errors';
export interface RetroflowState {
  category: 'homebrew' | 'vita' | 'unknown'; existingSha256?: string; namedCoverOverride?: boolean; protectedPaths?: string[];
}
export type RetroflowPlan = { action: 'copy' | 'keep' | 'conflict'; file: PreparedFile }
  | { action: 'manual'; source: string; filename: string };
export async function planRetroflowCover(pkg: PreparedPackage, titleId: string,
  state: RetroflowState): Promise<RetroflowPlan> {
  requireThat(/^[A-Z0-9]{9}$/.test(titleId) && titleId === pkg.inventory.titleId, 'artwork_title_mismatch');
  const image = pkg.images.retroflow; requireThat(image, 'missing_artwork');
  requireThat(await hashFile(image.path) === image.outputSha256, 'artwork_changed');
  if (state.category === 'unknown' || state.namedCoverOverride)
    return { action: 'manual', source: image.path, filename: `${titleId}.png` };
  requireThat(state.category === 'homebrew' || state.category === 'vita', 'unknown_retroflow_category');
  const category = state.category === 'homebrew' ? 'Homebrew' : 'Sony - PlayStation Vita';
  const file: PreparedFile = { source: image.path, destination: `ux0:data/RetroFlow/COVERS/${category}/${titleId}.png`,
    bytes: (await lstat(image.path)).size, sha256: image.outputSha256 };
  return { action: state.existingSha256 ? state.existingSha256 === file.sha256 ? 'keep' : 'conflict' : 'copy', file };
}
