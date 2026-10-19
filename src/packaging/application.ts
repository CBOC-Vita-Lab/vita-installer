import { join, resolve } from 'node:path';

function checkedPlatform(platform: string) {
  if (!['darwin-arm64', 'darwin-x64', 'win32-x64'].includes(platform)) throw new Error('Unsupported native package platform.');
  return platform.startsWith('win32-');
}

export function packagingEnvironment(platform: string, original: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  checkedPlatform(platform);
  const blocked = new Set(['electron_override_dist_path', 'electron_use_remote_checksums',
    'npm_config_electron_use_remote_checksums', 'electron_install_platform', 'electron_install_arch', 'npm_config_arch']);
  const env = Object.fromEntries(Object.entries(original).filter(([key]) => !blocked.has(key.toLowerCase())));
  const [os, arch] = platform.split('-');
  return { ...env, ELECTRON_INSTALL_PLATFORM: os, ELECTRON_INSTALL_ARCH: arch, npm_config_arch: arch };
}

export function packagedApplication(root: string, platform: string, override?: string) {
  const windows = checkedPlatform(platform);
  const app = resolve(root, override ?? join('out', `CBOC Vita Installer-${platform}`, windows ? '' : 'CBOC Vita Installer.app'));
  const resources = join(app, windows ? 'resources' : 'Contents/Resources');
  const workers = join(resources, 'installer-workers', platform), exe = windows ? '.exe' : '';
  return { app, resources, workers,
    executable: join(app, windows ? 'CBOC Vita Installer.exe' : 'Contents/MacOS/CBOC Vita Installer'),
    prepare: join(workers, `vita-prepare${exe}`), volume: join(workers, `vita-volume/vita-volume${exe}`) };
}

export function smokeEnvironment(root: string, platform: string, original: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = { HOME: root, USERPROFILE: root, TMPDIR: root, TEMP: root, TMP: root,
    PATH: '', ELECTRON_RUN_AS_NODE: '1' };
  if (checkedPlatform(platform)) {
    const system = original.SystemRoot ?? original.SYSTEMROOT;
    if (!system) throw new Error('Windows SystemRoot is required for the packaged smoke test.');
    env.SystemRoot = system; env.WINDIR = system;
  }
  return env;
}
