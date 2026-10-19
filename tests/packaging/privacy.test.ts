import { expect, it } from 'vitest';
import { existsSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { inspectPackagePrivacy } from '../../src/packaging/privacy';

const artifact = 'app/node_modules/@img/sharp-libvips-darwin-x64/lib/libvips-cpp.8.18.7.dylib';
const library = process.env.VITA_TEST_REVIEWED_LIBVIPS ?? resolve('node_modules/@img/sharp-libvips-darwin-x64/lib/libvips-cpp.8.18.7.dylib');

it('keeps ordinary resources and similar but different home names', () => {
  expect(inspectPackagePrivacy('app/example.js', Buffer.from('ordinary content'), '/Users/example')).toBeUndefined();
  expect(inspectPackagePrivacy('app/example.js', Buffer.from('/Users/example-other/work'), '/Users/example')).toBeUndefined();
  expect(inspectPackagePrivacy('app/example.js', Buffer.from('C:\\Users\\Example-other\\work'), 'C:/Users/Example')).toBeUndefined();
});
it.each([
  ['/Users/example', '/Users/example/work'],
  ['C:\\Users\\Example', 'C:\\Users\\Example\\work'],
  ['C:\\Users\\Example', 'C:/Users/Example/work'],
  ['C:\\Users\\Example', 'c:\\users\\example\\work'],
  ['C:\\Users\\Example', JSON.stringify('C:\\Users\\Example\\work')],
  ['C:/Users/Example', 'C:\\Users\\Example\\work'],
  ['C:/Users/Example', JSON.stringify('C:\\Users\\Example\\work')],
  ['C:/Users/Example', 'c:/users\\example/work'],
  ['C:\\Users/Example', 'C:/Users\\Example/work'],
  ['\\\\Server\\Users\\Example', JSON.stringify('\\\\server\\users\\example\\work')],
  ['//Server/Users/Example', '\\\\server\\users/example\\work'],
])('rejects owner paths from %s', (home, value) => {
  expect(() => inspectPackagePrivacy('app/example.js', Buffer.from(value), home)).toThrow('Private data');
});
it('rejects token patterns regardless of filename or home', () => {
  for (const prefix of ['github_pat_', 'ghp_', 'sk-ant-api03-'])
    expect(() => inspectPackagePrivacy(artifact, Buffer.from(prefix + 'x'.repeat(35)), '/Users/runner')).toThrow('Private data');
});
it('does not trust a generic runner path or an artifact name without matching bytes', () => {
  expect(() => inspectPackagePrivacy(artifact, Buffer.from('/Users/runner/work/sharp-libvips/source'), '/Users/runner')).toThrow('Private data');
});
it.skipIf(!existsSync(library))('allows only the reviewed upstream bytes at the exact artifact path and generic CI home', async () => {
  const data = await readFile(library);
  expect(inspectPackagePrivacy(artifact, data, '/Users/runner')).toMatchObject({
    path: artifact, sha256: 'fb2101ad991f683a46204ea0ac96a4269c4c93ecef023a0f841c93ebc9843353',
    package: '@img/sharp-libvips-darwin-x64@1.3.4',
  });
  expect(() => inspectPackagePrivacy('app/renamed.dylib', data, '/Users/runner')).toThrow('Private data');
  expect(() => inspectPackagePrivacy(artifact, data, '/Users')).toThrow('Private data');
  const changed = Buffer.from(data); changed[0] ^= 1;
  expect(() => inspectPackagePrivacy(artifact, changed, '/Users/runner')).toThrow('Private data');
  expect(() => inspectPackagePrivacy(artifact, Buffer.concat([data, Buffer.from('ghp_' + 'x'.repeat(35))]), '/Users/runner')).toThrow('Private data');
});
