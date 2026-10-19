import { expect, it } from 'vitest';
import { CSP, isAllowedExternal, parseRequest, rendererPreferences, safeError, trustedSender } from '../../src/main/security';
import { InstallerError } from '../../src/shared/errors';
it('allows only typed requests, not renderer paths or executable options', () => {
  expect(parseRequest({ command: 'importUrl', url: 'https://github.com/CBOC-Vita-Lab/vitadhoc' })).toHaveProperty('command', 'importUrl');
  for (const url of ['https://user:token@github.com/CBOC-Vita-Lab/vitadhoc', 'http://127.0.0.1/', 'https://github.com/CBOC-Vita-Lab/vitadhoc?token=secret'])
    expect(() => parseRequest({ command: 'importUrl', url })).toThrow();
  expect(parseRequest({ command: 'artwork', slot: 'bubble', action: 'adjust', fit: 'cover', x: 0.5, y: 0.5 })).toHaveProperty('slot', 'bubble');
  for (const value of [{ command: 'execute', id: '../../bad' }, { command: 'import', kind: 'file', path: '/private/key' },
    { command: 'artwork', slot: 'bubble', action: 'choose', source: '/secret' }, { command: 'artwork', slot: 'bubble', action: 'adjust', x: 2 },
    { command: 'discover', manual: { host: 'example.com\nreboot', port: 1337 } }, { command: 'exec', args: ['reboot'] }]) expect(() => parseRequest(value)).toThrow();
});
it('isolates the renderer and allows only its exact main-frame URL', () => {
  expect(rendererPreferences('/app/preload.cjs')).toMatchObject({ contextIsolation: true, sandbox: true, nodeIntegration: false, webSecurity: true, preload: '/app/preload.cjs' });
  expect(trustedSender('file:///app/index.html', 'file:///app/index.html', true)).toBe(true);
  expect(trustedSender('file:///app/index.html', 'file:///app/index.html', false)).toBe(false);
  expect(trustedSender('https://evil.test', 'file:///app/index.html', true)).toBe(false);
  expect(CSP).toContain("default-src 'none'"); expect(CSP).toContain("connect-src 'none'"); expect(CSP).not.toContain('unsafe-eval');
});
it('keeps external links allowlisted and diagnostics free of paths, tokens or raw exceptions', () => {
  expect(isAllowedExternal('https://github.com/CBOC-Vita-Lab/strikers-charged-vita')).toBe(true);
  for (const url of ['file:///secret', 'javascript:alert(1)', 'https://github.com.evil.test/CBOC-Vita-Lab/strikers-charged-vita',
    'https://github.com/CBOC-Vita-Lab/strikers-charged-vita?token=secret', 'https://user:secret@github.com/CBOC-Vita-Lab/strikers-charged-vita']) expect(isAllowedExternal(url)).toBe(false);
  expect(safeError(new Error('/Users/person/private/key TOKEN=secret'))).toBe('operation_failed');
  expect(safeError(new InstallerError('invalid_artwork'))).toBe('invalid_artwork');
});
