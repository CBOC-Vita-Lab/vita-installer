import { PROFILES } from './manifest';
export type PublicRelease = { status: 'available'; tag: string } | { status: 'unavailable' };
export const releasePage = (recipe: keyof typeof PROFILES) => `https://github.com/CBOC-Vita-Lab/${PROFILES[recipe].repo}/releases`;
export async function checkPublicRelease(recipe: keyof typeof PROFILES, fetcher: typeof fetch = fetch): Promise<PublicRelease> {
  try {
    const response = await fetcher(`https://api.github.com/repos/CBOC-Vita-Lab/${PROFILES[recipe].repo}/releases/latest`, {
      redirect: 'error', credentials: 'omit', signal: AbortSignal.timeout(5000), headers: { Accept: 'application/vnd.github+json' }
    });
    if (!response.ok || !response.body) return { status: 'unavailable' };
    const reader = response.body.getReader(), chunks: Uint8Array[] = []; let bytes = 0;
    try {
      for (;;) { const next = await reader.read(); if (next.done) break; bytes += next.value.length;
        if (bytes > 256 * 1024) return { status: 'unavailable' }; chunks.push(next.value); }
    } finally { await reader.cancel(); }
    const data = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    if (data.draft !== false || data.prerelease !== false || typeof data.tag_name !== 'string' || !/^[\w./-]{1,100}$/.test(data.tag_name)) return { status: 'unavailable' };
    const url = new URL(data.html_url), expected = new URL(releasePage(recipe));
    if (url.origin !== expected.origin || url.username || url.password || url.search || url.hash ||
        decodeURIComponent(url.pathname) !== `${expected.pathname}/tag/${data.tag_name}`) return { status: 'unavailable' };
    return { status: 'available', tag: data.tag_name };
  } catch { return { status: 'unavailable' }; }
}
