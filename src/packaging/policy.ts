import { requireThat } from '../shared/errors';
export const NETWORK_POLICY = ['local-vita-discovery', 'fixed-public-github-release-metadata', 'authenticated-cboc-release-download', 'sha256-pinned-nod-release-download'] as const;
export function allowedPackagePath(path: string): boolean {
  path = path.replaceAll('\\', '/');
  if (/(^|\/)\.[^/]+/.test(path)) return false;
  return path === '' || path === '/' || ['/package.json', '/LICENSE', '/dist'].includes(path)
    || /^\/(?:node_modules|dist\/(?:main|renderer))(?:\/|$)/.test(path);
}
export interface PackagePolicy {
  platform: string; host: string; signing: string; signingSubject: string | null; files: string[];
  helperArchitectures: string[]; networkPolicy: readonly string[];
}
export function validatePackagePolicy(receipt: PackagePolicy) {
  requireThat(['darwin-arm64', 'darwin-x64', 'win32-x64'].includes(receipt.platform) && receipt.host === receipt.platform, 'native_platform_required');
  const arch = receipt.platform.split('-')[1], windows = receipt.platform.startsWith('win32-'), exe = windows ? '.exe' : '';
  requireThat(receipt.helperArchitectures.length === 3 && receipt.helperArchitectures.every(x => x === arch), 'wrong_helper_architecture');
  requireThat(receipt.signing === 'unsigned-internal' && receipt.signingSubject === null, 'unreviewed_signing_identity');
  requireThat(JSON.stringify(receipt.networkPolicy) === JSON.stringify(NETWORK_POLICY), 'unreviewed_network_policy');
  const worker = `installer-workers/${receipt.platform}`;
  const required = ['app/dist/main/index.cjs', 'app/dist/main/job-worker.mjs', 'app/dist/main/preload.cjs', 'app/dist/renderer/index.html', 'app/LICENSE',
    'installer-resources/profiles/trust.json', 'installer-resources/notices/libvips/manifest.json', 'installer-resources/notices/libvips/README.md',
    `${worker}/catalog.json`, `${worker}/vita-prepare${exe}`, `${worker}/vita-volume/vita-volume${exe}`, `${worker}/vita-movies${exe}`,
    `${worker}/vita-volume/_internal/${windows ? 'python314.dll' : 'Python'}`,
    ...['SBOM.cdx.json', 'THIRD_PARTY_NOTICES.md', 'Python-LICENSE.txt', 'pycdlib-LICENSE.txt', 'PyInstaller-COPYING.txt', 'Galaxy-LICENSE.txt',
      'libjpeg-turbo-LICENSE.md', 'libjpeg-turbo-README.ijg', 'Electron-LICENSE.txt', 'Chromium-LICENSES.html',
      'npm/@nodable__entities/LICENSE'].map(x => `installer-notices/${x}`)];
  requireThat(required.every(path => receipt.files.includes(path)), 'missing_package_resource');
  for (const path of receipt.files) {
    requireThat(!/(^|\/)(?:\.git|\.env|\.superpowers|auth|secrets|inputs|private)(\/|$)|\.(?:iso|rvz|vpk|suprx|spec|key|pem)$/i.test(path), 'private_package_payload');
    requireThat(path.startsWith('app/node_modules/') || !/(^|\/)build(\/|$)/.test(path), 'private_package_payload');
  }
}
