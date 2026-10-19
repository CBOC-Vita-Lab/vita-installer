const { existsSync } = require('node:fs');
const { join } = require('node:path');
module.exports = {
  packagerConfig: {
    asar: false,
    appBundleId: 'org.cboc.vita-installer',
    appCategoryType: 'public.app-category.utilities',
    executableName: 'CBOC Vita Installer',
    icon: 'resources/branding/app-icon',
    osxSign: false,
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
  makers: [{ name: '@electron-forge/maker-zip', platforms: ['darwin'] },
    { name: '@electron-forge/maker-squirrel', platforms: ['win32'], config: { name: 'CBOCVitaInstaller', authors: 'CBOC-Vita', description: 'CBOC Vita Installer private preview' } }]
};
