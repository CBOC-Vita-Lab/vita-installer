import { expect, it } from 'vitest';
import { join, resolve } from 'node:path';
import { packagedApplication, smokeEnvironment } from '../../src/packaging/application';

it('locates native app executables and helpers without a Mac bundle suffix on Windows', () => {
  const root = resolve('fixture');
  const windows = packagedApplication(root, 'win32-x64');
  expect(windows.app).toBe(join(root, 'out/CBOC Vita Installer-win32-x64'));
  expect(windows.executable).toBe(join(windows.app, 'CBOC Vita Installer.exe'));
  expect(windows.prepare).toBe(join(windows.app, 'resources/installer-workers/win32-x64/vita-prepare.exe'));
  expect(windows.volume).toBe(join(windows.app, 'resources/installer-workers/win32-x64/vita-volume/vita-volume.exe'));
  for (const platform of ['darwin-arm64', 'darwin-x64']) {
    const mac = packagedApplication(root, platform);
    expect(mac.executable).toBe(join(root, `out/CBOC Vita Installer-${platform}/CBOC Vita Installer.app/Contents/MacOS/CBOC Vita Installer`));
    expect(mac.prepare).toBe(join(mac.app, `Contents/Resources/installer-workers/${platform}/vita-prepare`));
  }
  expect(() => packagedApplication(root, 'linux-x64')).toThrow();
});
it('keeps Windows system access but removes tools, credentials and real user folders from smoke tests', () => {
  const env = smokeEnvironment('/isolated', 'win32-x64', { SystemRoot: 'C:\\Windows', PATH: 'private-tools', HOME: '/real-user', GH_TOKEN: 'fixture-secret' });
  expect(env.SystemRoot).toBe('C:\\Windows'); expect(env.WINDIR).toBe('C:\\Windows');
  expect(env.PATH).toBe(''); expect(env.ELECTRON_RUN_AS_NODE).toBe('1');
  for (const key of ['HOME', 'USERPROFILE', 'TMPDIR', 'TEMP', 'TMP']) expect(env[key]).toBe('/isolated');
  expect(env.GH_TOKEN).toBeUndefined();
  expect(() => smokeEnvironment('/isolated', 'win32-x64', {})).toThrow();
  expect(smokeEnvironment('/isolated', 'darwin-arm64', {}).SystemRoot).toBeUndefined();
});
