import { expect, it } from 'vitest';
import { validatePackagePolicy, NETWORK_POLICY, allowedPackagePath } from '../../src/packaging/policy';
import { createRequire } from 'node:module';
const forge = createRequire(import.meta.url)('../../forge.config.cjs');
const receipt = () => ({ platform: 'darwin-arm64', host: 'darwin-arm64', signing: 'ad-hoc', signingSubject: null, signatureVerified: true,
  files: ['app/dist/main/index.cjs', 'app/dist/main/job-worker.mjs', 'app/dist/main/preload.cjs', 'app/dist/renderer/index.html', 'app/LICENSE',
    'installer-resources/profiles/trust.json', 'installer-resources/notices/libvips/manifest.json', 'installer-resources/notices/libvips/README.md',
    'installer-workers/darwin-arm64/catalog.json', 'installer-workers/darwin-arm64/vita-prepare',
    'installer-workers/darwin-arm64/vita-movies',
    'installer-workers/darwin-arm64/vita-volume/vita-volume', 'installer-workers/darwin-arm64/vita-volume/_internal/Python',
    'installer-notices/SBOM.cdx.json', 'installer-notices/THIRD_PARTY_NOTICES.md', 'installer-notices/Python-LICENSE.txt',
    'installer-notices/pycdlib-LICENSE.txt', 'installer-notices/PyInstaller-COPYING.txt', 'installer-notices/Galaxy-LICENSE.txt',
    'installer-notices/libjpeg-turbo-LICENSE.md', 'installer-notices/libjpeg-turbo-README.ijg',
    'installer-notices/Electron-LICENSE.txt', 'installer-notices/Chromium-LICENSES.html',
    'installer-notices/npm/@nodable__entities/LICENSE'],
  helperArchitectures: ['arm64', 'arm64', 'arm64'], networkPolicy: NETWORK_POLICY });
it('requires native helpers, runtime and notices for the exact native platform', () => {
  expect(() => validatePackagePolicy(receipt())).not.toThrow();
  expect(() => validatePackagePolicy({ ...receipt(), files: [...receipt().files, 'app/node_modules/lucide-react/dist/shared/src/build/icon.mjs'] })).not.toThrow();
  for (const missing of receipt().files) expect(() => validatePackagePolicy({ ...receipt(), files: receipt().files.filter(x => x !== missing) })).toThrow();
  expect(() => validatePackagePolicy({ ...receipt(), host: 'darwin-x64' })).toThrow();
  expect(() => validatePackagePolicy({ ...receipt(), helperArchitectures: ['x64', 'arm64'] })).toThrow();
});
it('rejects unexpected fetch policy, personal signatures and private payloads', () => {
  expect(() => validatePackagePolicy({ ...receipt(), networkPolicy: [...NETWORK_POLICY, 'https://unreviewed.test/tool'] })).toThrow();
  expect(() => validatePackagePolicy({ ...receipt(), signingSubject: 'Developer ID Application: Private Person' })).toThrow();
  expect(() => validatePackagePolicy({ ...receipt(), signatureVerified: false })).toThrow();
  expect(() => validatePackagePolicy({ ...receipt(), signing: 'unsigned-internal' })).toThrow();
  for (const path of ['app/auth/gh/hosts.yml', 'app/.env', 'app/private/game.iso', 'app/game.vpk', 'app/secrets/token.txt', 'app/.git/config', 'installer-workers/build/source.spec']) {
    expect(() => validatePackagePolicy({ ...receipt(), files: [...receipt().files, path] })).toThrow();
  }
});
it('uses a packaging allowlist rather than copying the workspace', () => {
  for (const path of ['/', '/package.json', '/LICENSE', '/node_modules/sharp/lib/index.js', '/dist/main/index.cjs', '/dist/renderer/index.html']) expect(allowedPackagePath(path)).toBe(true);
  for (const path of ['/src/main/index.ts', '/tests/test.ts', '/.venv-workers/bin/python', '/workers/build/log', '/.superpowers/private.md', '/out/app', '/dist/tools/bundle.mjs', '/node_modules/.vitest-cache/entry', '/node_modules/.bin/node']) {
    expect(allowedPackagePath(path)).toBe(false); expect(forge.packagerConfig.ignore(path)).toBe(true);
  }
});
