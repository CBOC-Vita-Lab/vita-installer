export class InstallerError extends Error {
  constructor(public readonly code: string, message = code) { super(message); this.name = 'InstallerError'; }
}
export function requireThat(condition: unknown, code: string): asserts condition {
  if (!condition) throw new InstallerError(code);
}
export function cancelled(signal?: AbortSignal) {
  if (signal?.aborted) throw new InstallerError('cancelled');
}
