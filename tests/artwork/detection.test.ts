import { expect, it } from 'vitest';
import { MemoryTransport } from '../jobs/fixture';
import { detectRetroflow } from '../../src/artwork/detect';
const root = 'ux0:data/RetroFlow';
it('reads the cached category and checks title-name cover precedence without writes', async () => {
  const t = new MemoryTransport();
  t.files.set(`${root}/CACHE/db_games.lua`, Buffer.from('return {{name="MSCN00003", app_type=1, apptitle="My Strikers"}}'));
  t.files.set(`${root}/overrides.dat`, Buffer.from('MSCN00003=1\n'));
  expect(await detectRetroflow(t, 'MSCN00003')).toMatchObject({ category: 'vita', namedCoverOverride: false });
  t.files.set(`${root}/COVERS/Sony - PlayStation Vita/My Strikers.png`, Buffer.from('existing'));
  expect(await detectRetroflow(t, 'MSCN00003')).toMatchObject({ category: 'vita', namedCoverOverride: true });
  expect(t.writes).toEqual([]);
});
it('fails closed for executable Lua, ambiguous caches, changed overrides and missing records', async () => {
  for (const value of ['os.execute("untrusted"); return {}', 'return { { name="MSCN00003", app_type=0 }, {name="MSCN00003",app_type=0} }']) {
    const t = new MemoryTransport(); t.files.set(`${root}/CACHE/db_homebrews.lua`, Buffer.from(value));
    expect(await detectRetroflow(t, 'MSCN00003')).toEqual({ category: 'unknown' });
  }
  const t = new MemoryTransport();
  expect(await detectRetroflow(t, 'MSCN00003')).toEqual({ category: 'unknown' });
  t.files.set(`${root}/CACHE/db_homebrews.lua`, Buffer.from('return {{name="MSCN00003",app_type=0,apptitle="Strikers"}}'));
  t.files.set(`${root}/overrides.dat`, Buffer.from('MSCN00003=1\n'));
  expect(await detectRetroflow(t, 'MSCN00003')).toEqual({ category: 'unknown' });
});
