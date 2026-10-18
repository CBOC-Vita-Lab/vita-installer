import { verify, type KeyObject } from 'node:crypto';
import { parseTree, getNodeValue, type Node, type ParseError } from 'jsonc-parser';
import { z } from 'zod';
import { digest } from '../shared/files';
import { requireThat } from '../shared/errors';
import { manifestSchema, type ReleaseManifest } from './manifest';

export interface TrustStore { fingerprints: ReadonlySet<string>; keys: ReadonlyMap<string, KeyObject> }
export function strictJson(bytes: Uint8Array): unknown {
  requireThat(bytes.byteLength > 0 && bytes.byteLength <= 1024 * 1024, 'invalid_manifest_size');
  const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes), errors: ParseError[] = [];
  const tree = parseTree(text, errors, { disallowComments: true, allowTrailingComma: false });
  requireThat(tree && errors.length === 0, 'invalid_json');
  function check(node: Node) {
    if (node.type === 'object') {
      const names = node.children!.map(p => p.children![0].value);
      requireThat(names.length === new Set(names).size, 'duplicate_json_key');
    }
    node.children?.forEach(check);
  }
  check(tree); return getNodeValue(tree);
}
const base64 = z.string().refine(s => /^[A-Za-z0-9+/]*={0,2}$/.test(s) && Buffer.from(s, 'base64').toString('base64') === s);
const envelopeSchema = z.object({ keyId: z.string().min(1).max(100), manifest: base64, signature: base64 }).strict();
export function authenticateManifest(bytes: Uint8Array, store: TrustStore): ReleaseManifest {
  let raw = Buffer.from(bytes);
  if (!store.fingerprints.has(digest(raw))) {
    const envelope = envelopeSchema.parse(strictJson(raw)), key = store.keys.get(envelope.keyId);
    requireThat(key && key.asymmetricKeyType === 'ed25519', 'untrusted_release');
    raw = Buffer.from(envelope.manifest, 'base64');
    requireThat(verify(null, raw, key, Buffer.from(envelope.signature, 'base64')), 'invalid_release_signature');
  }
  return manifestSchema.parse(strictJson(raw));
}
