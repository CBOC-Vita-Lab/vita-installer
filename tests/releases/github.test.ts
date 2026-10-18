import { expect, it, vi } from 'vitest';
import { checkPublicRelease, releasePage } from '../../src/releases/github';
const response = (extra = {}) => new Response(JSON.stringify({ tag_name: 'v1.0', draft: false, prerelease: false,
  html_url: 'https://github.com/CBOC-Vita-Lab/strikers-charged-vita/releases/tag/v1.0', ...extra }));
it('checks only the pinned repository without credentials, downloads or redirects', async () => {
  const fetcher = vi.fn().mockResolvedValue(response());
  expect(await checkPublicRelease('strikers', fetcher)).toEqual({ status: 'available', tag: 'v1.0' });
  expect(fetcher.mock.calls[0][0]).toBe('https://api.github.com/repos/CBOC-Vita-Lab/strikers-charged-vita/releases/latest');
  expect(fetcher.mock.calls[0][1]).toMatchObject({ redirect: 'error', credentials: 'omit' });
  expect(JSON.stringify(fetcher.mock.calls)).not.toMatch(/authorization|token/i);
  expect(releasePage('vitadhoc')).toBe('https://github.com/CBOC-Vita-Lab/vitadhoc/releases');
});
it('treats offline, inaccessible and mismatched metadata as unavailable, not a local import failure', async () => {
  for (const fetcher of [vi.fn().mockRejectedValue(new Error('offline')), vi.fn().mockResolvedValue(new Response('', { status: 404 })),
    vi.fn().mockResolvedValue(response({ html_url: 'https://github.com/other/strikers/releases/tag/v1.0' })),
    vi.fn().mockResolvedValue(response({ draft: true })), vi.fn().mockResolvedValue(response({ tag_name: '\nprivate' }))]) {
    expect(await checkPublicRelease('strikers', fetcher)).toEqual({ status: 'unavailable' });
  }
});
