import { parse, type Expression } from 'luaparse';
import type { Transport } from '../devices/transport';
import type { RetroflowState } from './retroflow';
import { readSmallFile } from '../jobs/observe';
import { cancelled, requireThat } from '../shared/errors';
import { safeRelative } from '../shared/files';
export const RETROFLOW_CONFIG = ['CACHE/db_games.lua', 'CACHE/db_homebrews.lua', 'renamed_games.lua', 'overrides.dat'].map(p => `ux0:data/RetroFlow/${p}`);
type Value = string | number | boolean | null | { [key: string]: Value };
function literal(node: Expression, depth = 0): Value {
  requireThat(depth < 24, 'invalid_retroflow_cache');
  if (node.type === 'StringLiteral') return Buffer.from(node.value ?? '', 'latin1').toString('utf8');
  if (node.type === 'NumericLiteral' || node.type === 'BooleanLiteral') return node.value;
  if (node.type === 'NilLiteral') return null;
  requireThat(node.type === 'TableConstructorExpression', 'invalid_retroflow_cache');
  const result: Record<string, Value> = Object.create(null); let index = 0;
  for (const field of node.fields) {
    const key = field.type === 'TableValue' ? String(++index) : field.type === 'TableKeyString' ? field.key.name : literal(field.key, depth + 1);
    requireThat((typeof key === 'string' || typeof key === 'number') && !Object.hasOwn(result, String(key)), 'invalid_retroflow_cache');
    result[String(key)] = literal(field.value, depth + 1);
  }
  return result;
}
function records(data: Buffer): Record<string, Value>[] {
  let count = 0;
  const ast = parse(data.toString('latin1'), { encodingMode: 'pseudo-latin1', comments: false, onCreateNode: () => { requireThat(++count <= 100000, 'invalid_retroflow_cache'); } });
  requireThat(ast.body.length === 1 && ast.body[0].type === 'ReturnStatement' && ast.body[0].arguments.length === 1, 'invalid_retroflow_cache');
  const table = literal(ast.body[0].arguments[0]); requireThat(table !== null && typeof table === 'object', 'invalid_retroflow_cache');
  const rows = Object.values(table); requireThat(rows.every(row => row !== null && typeof row === 'object'), 'invalid_retroflow_cache');
  return rows as Record<string, Value>[];
}
export async function detectRetroflow(transport: Transport, titleId: string, signal?: AbortSignal): Promise<RetroflowState> {
  requireThat(/^[A-Z0-9]{9}$/.test(titleId), 'invalid_title');
  try {
    const read = async (path: string) => await transport.stat(path) ? readSmallFile(transport, path, 1024 * 1024, signal) : undefined;
    const matches: { row: Record<string, Value>; type: number }[] = [];
    for (const [index, path] of RETROFLOW_CONFIG.slice(0, 2).entries()) {
      const data = await read(path); if (!data) continue;
      for (const row of records(data)) if (row.name === titleId) matches.push({ row, type: index === 0 ? 1 : 0 });
    }
    if (matches.length !== 1) return { category: 'unknown' };
    const { row, type } = matches[0]; if (row.app_type !== type) return { category: 'unknown' };
    const overrides = await read(RETROFLOW_CONFIG[3]);
    if (overrides) {
      const entries = overrides.toString('utf8').split(/\r?\n/).map(line => line.trim()).filter(Boolean), found: number[] = [];
      for (const entry of entries) { const match = /^([^=\x00-\x1f]+)=([0-4])$/.exec(entry); requireThat(match, 'invalid_retroflow_cache'); if (match[1] === titleId) found.push(Number(match[2])); }
      if (found.length > 1 || found.some(n => ![0, 1, 4].includes(n) || (n === 1 ? 1 : 0) !== type)) return { category: 'unknown' };
    }
    const names = [row.apptitle, row.title].filter((value): value is string => typeof value === 'string' && value.length > 0);
    const renamed = await read(RETROFLOW_CONFIG[2]);
    if (renamed) for (const record of records(renamed)) if (record.name === titleId && typeof record.title === 'string') names.push(record.title);
    if (!names.length || names.some(name => !safeRelative(`${name}.png`) || name.includes('/'))) return { category: 'unknown' };
    const folder = type === 1 ? 'Sony - PlayStation Vita' : 'Homebrew'; let namedCoverOverride = false;
    for (const name of new Set(names)) if (name !== titleId && await transport.stat(`ux0:data/RetroFlow/COVERS/${folder}/${name}.png`)) namedCoverOverride = true;
    return { category: type === 1 ? 'vita' : 'homebrew', namedCoverOverride, protectedPaths: RETROFLOW_CONFIG };
  } catch { cancelled(signal); return { category: 'unknown' }; }
}
