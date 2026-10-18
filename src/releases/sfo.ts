import { requireThat } from '../shared/errors';

// Same bounded SFO layout used by the publishing package checker.
export function readSfo(bytes: Uint8Array): { titleId: string } {
  const b = Buffer.from(bytes);
  requireThat(b.length >= 20 && b.length <= 65536, 'invalid_sfo');
  const keys = b.readUInt32LE(8), values = b.readUInt32LE(12), count = b.readUInt32LE(16);
  requireThat(b.readUInt32LE(0) === 0x46535000 && b.readUInt32LE(4) === 0x101
    && count <= 256 && 20 + count * 16 <= keys && keys <= values && values <= b.length, 'invalid_sfo');
  let titleId: string | undefined;
  for (let i = 0; i < count; i++) {
    const offset = 20 + i * 16, start = keys + b.readUInt16LE(offset), end = b.indexOf(0, start);
    const kind = b.readUInt16LE(offset + 2), size = b.readUInt32LE(offset + 4), capacity = b.readUInt32LE(offset + 8), data = values + b.readUInt32LE(offset + 12);
    requireThat(start < values && end >= start && end < values && size <= capacity && data + capacity <= b.length, 'invalid_sfo');
    if (b.toString('ascii', start, end) === 'TITLE_ID') {
      requireThat(!titleId && kind === 0x204 && size === 10 && b[data + 9] === 0, 'invalid_title');
      titleId = b.toString('ascii', data, data + 9);
      requireThat(/^[A-Z0-9]{9}$/.test(titleId), 'invalid_title');
    }
  }
  requireThat(titleId, 'missing_title'); return { titleId };
}
