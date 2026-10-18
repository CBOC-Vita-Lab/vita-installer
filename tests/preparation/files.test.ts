import { expect, it } from 'vitest';
import { realpath } from 'node:fs/promises';
import { hashFile } from '../../src/shared/files';
it('normalizes cancellation while a large file hash is being streamed', async () => {
  const controller = new AbortController(), path = await realpath(process.execPath);
  const task = hashFile(path, controller.signal); setTimeout(() => controller.abort(), 1);
  await expect(task).rejects.toMatchObject({ code: 'cancelled' });
});
