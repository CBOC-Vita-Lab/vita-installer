export const errorMessage = (code?: string, kind?: string): string => ({
  unsupported_game_revision: kind === 'infected-disc' || kind === 'infected' ? 'USA v1.02 (ULUS-10054) is required. Choose the matching game file.' : kind === 'strikers-disc' || kind === 'strikers' ? 'USA Rev 1 is required. Choose the matching game file.' : 'Choose the game revision listed for this release.',
  invalid_artwork: 'Choose a PNG, JPEG or still WebP under 20 MB.',
  unsupported_artwork_template: 'This release does not support that artwork layout.',
  connection_lost: 'Connection lost. Reconnect the same Vita to resume.',
  transport_closed: 'Connection lost. Reconnect the same Vita to resume.',
  device_changed: 'This is not the selected Vita or card. Reconnect the original.',
  device_not_ready: 'Connect your Vita before continuing.',
  existing_app: 'This game is already installed. An update needs your confirmation.',
  file_conflict: 'Existing files differ. Nothing will be replaced.',
  private_app_file_conflict: 'This installation has extra private files. Update is blocked to protect them.',
  insufficient_space: 'Not enough free space.', capacity_unknown: 'Free space could not be checked.',
  capacity_unverified: 'Free space could not be checked.',
  release_held: 'This release is not ready to install.', untrusted_release: 'This release is not verified.',
  invalid_release_signature: 'This release is not verified.', no_supported_release: 'No supported release found in that ZIP or folder.',
  package_hash_mismatch: 'The release files have changed. Download a fresh copy.',
  preparation_tool_missing: 'A preparation tool is missing from this app.', tool_download_failed: 'The file preparation tool could not be downloaded. Try again online.',
  source_changed: 'A selected file changed. Choose it again.', preparation_changed: 'Prepared files changed. Prepare them again.',
  unsupported_shader_compiler: 'That shader compiler is not the supported version.', missing_prerequisite: 'A required file is missing.',
  prerequisite_conflict: 'A different shader compiler is already installed. It will not be overwritten.',
  unsupported_bios: 'That BIOS file is not the supported version.',
  optional_pack_not_reviewed: 'This texture pack is not supported yet.',
  input_scope_too_broad: 'Choose the game folder, not your whole drive or home folder.',
  ambiguous_input_archive: 'More than one game was found. Choose one extracted game folder.',
  installed_bytes_unverified: 'Installation is not confirmed. Check the Vita, then check again.',
  promotion_needs_reconciliation: 'Installation is not confirmed. Check the Vita, then check again.',
  cancelled: 'Cancelled. Existing games and saves are unchanged.',
  marker_mismatch: 'The recovery marker does not match. No files were changed.',
  target_changed: 'Files on the destination changed. Nothing will be overwritten.',
  existing_cover_kept: 'Existing RetroFlow cover kept.', existing_bios_kept: 'Existing BIOS kept.',
  operation_in_progress: 'Wait for the current operation to finish.',
  not_on_local_subnet: 'Use a Vita address on this computer\'s local network.'
} as Record<string, string>)[code ?? ''] ?? 'The operation could not finish. Check your files and connection, then try again.';
export function bytes(value: number) {
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(1)} GB`;
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(1)} MB`;
  return `${Math.ceil(value / 1024)} KB`;
}
