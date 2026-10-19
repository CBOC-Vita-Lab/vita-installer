const { existsSync } = require('node:fs');
const { join } = require('node:path');
const { verifyMacSignature } = require('./scripts/verify-macos-signature.cjs');
module.exports = {
  packagerConfig: {
    asar: false,
    appBundleId: 'org.cboc.vita-installer',
    appCategoryType: 'public.app-category.utilities',
    executableName: 'CBOC Vita Installer',
    icon: 'resources/branding/app-icon',
    osxSign: {
      identity: '-',
      identityValidation: false,
      continueOnError: false,
      preAutoEntitlements: false,
      preEmbedProvisioningProfile: false,
      // These native workers already have valid signatures bound to catalog hashes.
      ignore: path => /\/Contents\/Resources\/installer-workers(?:\/|$)/.test(path),
      optionsForFile: () => ({ hardenedRuntime: false, timestamp: 'none',
        entitlements: ['com.apple.security.cs.allow-jit'] })
    },
    ignore: path => {
      path = path.replaceAll('\\', '/');
      if (/(^|\/)\.[^/]+/.test(path)) return true;
      return !(path === '' || path === '/' || ['/package.json', '/LICENSE', '/dist'].includes(path)
        || /^\/(?:node_modules|dist\/(?:main|renderer))(?:\/|$)/.test(path));
    },
    extraResource: ['dist/installer-resources', 'dist/installer-workers', 'dist/installer-notices'],
    beforeCopy: [async ({ platform, arch }) => {
      if (`${platform}-${arch}` !== `${process.platform}-${process.arch}`) throw new Error('Build on the native target runner.');
      if (!existsSync(join('dist/installer-workers', `${platform}-${arch}`, 'catalog.json'))) throw new Error('Package native workers first.');
    }]
  },
  hooks: {
    postPackage: async (_config, result) => {
      if (result.platform === 'darwin') {
        for (const output of result.outputPaths) verifyMacSignature(join(output, 'CBOC Vita Installer.app'));
      }
    }
  },
  makers: [{ name: '@electron-forge/maker-zip', platforms: ['darwin'] },
    { name: '@electron-forge/maker-squirrel', platforms: ['win32'], config: { name: 'CBOCVitaInstaller', authors: 'CBOC-Vita', description: 'CBOC Vita Installer private preview' } }]
};
