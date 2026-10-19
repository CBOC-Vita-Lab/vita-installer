import { z } from 'zod';
import { isIPv4 } from 'node:net';
import { InstallerError } from '../shared/errors';
import type { UiRequest } from '../shared/ui';
import { parseGithubReleaseUrl } from '../releases/download';
const id = z.string().regex(/^[a-f0-9]{32}$/), inputId = z.string().regex(/^[a-z][a-z0-9-]{0,39}$/);
const kind = z.enum(['file', 'folder']);
const requestSchema = z.discriminatedUnion('command', [
  z.object({ command: z.literal('import'), kind }).strict(),
  z.object({ command: z.literal('importUrl'), url: z.string().max(1000).refine(value => { try { parseGithubReleaseUrl(value); return true; } catch { return false; } }) }).strict(),
  z.object({ command: z.literal('selectRelease'), id }).strict(),
  z.object({ command: z.literal('discover'), manual: z.object({ host: z.string().refine(isIPv4), port: z.number().int().min(1).max(65535) }).strict().optional() }).strict(),
  z.object({ command: z.literal('selectDevice'), id: z.string().min(1).max(200).regex(/^[^\x00-\x1f\x7f]+$/), confirmUx0: z.boolean().optional() }).strict(),
  z.object({ command: z.literal('chooseInput'), id: inputId, kind }).strict(),
  z.object({ command: z.literal('scanFolder') }).strict(),
  z.object({ command: z.literal('checkRelease') }).strict(),
  z.object({ command: z.enum(['checkInstallerUpdate', 'downloadInstallerUpdate']) }).strict(),
  z.object({ command: z.literal('removeInput'), id: inputId }).strict(),
  z.object({ command: z.literal('artwork'), slot: z.enum(['bubble', 'livearea', 'retroflow']), action: z.enum(['default', 'choose', 'adjust']),
    fit: z.enum(['cover', 'contain']).optional(), x: z.number().min(0).max(1).optional(), y: z.number().min(0).max(1).optional() }).strict(),
  z.object({ command: z.enum(['prepare', 'prepareUpdate', 'fullSetup']) }).strict(),
  z.object({ command: z.literal('plan'), acknowledgeUnknownSpace: z.boolean(), explicitUpdate: z.boolean(), replaceRetroflow: z.boolean() }).strict(),
  z.object({ command: z.literal('execute'), id }).strict(), z.object({ command: z.literal('resume'), id }).strict(),
  z.object({ command: z.enum(['pause', 'cancel', 'clearError', 'reset']) }).strict(),
  z.object({ command: z.literal('openRepo'), repo: z.enum(['installer', 'strikers', 'infected', 'vitadhoc', 'galaxy']) }).strict()
]);
export const parseRequest = (value: unknown): UiRequest => requestSchema.parse(value);
export const REPOSITORIES = { installer: 'vita-installer', strikers: 'strikers-charged-vita', infected: 'infected-vita', vitadhoc: 'vitadhoc', galaxy: 'galaxy-vita' } as const;
export const repoUrl = (repo: keyof typeof REPOSITORIES) => `https://github.com/CBOC-Vita-Lab/${REPOSITORIES[repo]}/releases`;
export const isAllowedExternal = (value: string): boolean => Object.keys(REPOSITORIES).some(key => {
  const url = repoUrl(key as keyof typeof REPOSITORIES); return url === value || url.slice(0, -9) === value;
});
export const rendererPreferences = (preload: string) => ({ preload, contextIsolation: true, sandbox: true,
  nodeIntegration: false, nodeIntegrationInWorker: false, nodeIntegrationInSubFrames: false, webSecurity: true,
  allowRunningInsecureContent: false, webviewTag: false, navigateOnDragDrop: false });
export const trustedSender = (frameUrl: string, appUrl: string, isMainFrame: boolean): boolean => isMainFrame && frameUrl === appUrl;
export function safeError(error: unknown): string {
  if (error instanceof Error && 'code' in error && error.code === 426) return 'ftp_transfer_interrupted';
  return error instanceof InstallerError && /^[a-z][a-z0-9_]{0,79}$/.test(error.code) ? error.code : 'operation_failed';
}
export const CSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-src 'none'; frame-ancestors 'none'";
