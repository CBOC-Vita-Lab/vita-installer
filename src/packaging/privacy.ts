import { createHash } from 'node:crypto';

const upstream = {
  path: 'app/node_modules/@img/sharp-libvips-darwin-x64/lib/libvips-cpp.8.18.7.dylib',
  sha256: 'fb2101ad991f683a46204ea0ac96a4269c4c93ecef023a0f841c93ebc9843353',
  package: '@img/sharp-libvips-darwin-x64@1.3.4',
  source: 'https://registry.npmjs.org/@img/sharp-libvips-darwin-x64/-/sharp-libvips-darwin-x64-1.3.4.tgz',
  integrity: 'sha512-iR2OKH80yi0U+dUplyh3/xdpFvps6YkCwsXenIJxqxR1v9o+xtKTGbS9H7cps+2Vxjc8B1j96p75NmTGjIhtpQ==',
} as const;

function containsOwnerPath(text: string, home: string) {
  const windows = /^(?:[a-z]:[\\/]|[\\/]{2})/i.test(home);
  if (windows) {
    // Normalize mixed and JSON-escaped separators without changing POSIX paths.
    const normalize = (value: string) => value.replace(/[\\/]+/g, '/').toLowerCase();
    return normalize(text).includes(normalize(home).replace(/\/+$/, '') + '/');
  }
  const roots = new Set([home, home.replaceAll('\\', '/')].map(value => value.replace(/[\\/]+$/, '')));
  const paths = [...roots].flatMap(root => [root + '/', root + '\\']);
  const needles = paths.flatMap(path => [path, JSON.stringify(path).slice(1, -1)]);
  return needles.some(path => text.includes(path));
}

export function inspectPackagePrivacy(path: string, data: Uint8Array, ownerHome?: string) {
  const text = Buffer.from(data).toString('utf8');
  if (/(?:github_pat_|ghp_|sk-ant-api03-)[A-Za-z0-9_-]{30,}/.test(text))
    throw new Error(`Private data in package: ${path}`);
  if (!ownerHome || !containsOwnerPath(text, ownerHome)) return undefined;
  // Upstream's generic CI home matches our Intel runner. Only its reviewed bytes qualify.
  if (ownerHome === '/Users/runner' && path === upstream.path
      && createHash('sha256').update(data).digest('hex') === upstream.sha256) return { ...upstream };
  throw new Error(`Private data in package: ${path}`);
}
