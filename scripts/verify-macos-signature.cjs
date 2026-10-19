const { execFileSync, spawnSync } = require('node:child_process');

function verifyMacSignature(app) {
  if (process.platform !== 'darwin') throw new Error('Mac signature verification requires macOS.');
  execFileSync('/usr/bin/codesign', ['--verify', '--deep', '--strict', '--verbose=2', app], { stdio: 'pipe' });
  const details = spawnSync('/usr/bin/codesign', ['-d', '--verbose=4', app], { encoding: 'utf8' });
  if (details.error || details.status !== 0) throw details.error ?? new Error('Cannot inspect the app signature.');
  const signature = details.stderr;
  if (!/^Signature=adhoc$/m.test(signature) || /^Authority=|^TeamIdentifier=(?!not set$)/m.test(signature))
    throw new Error('Unreviewed signing identity.');
  if (!/^Identifier=org\.cboc\.vita-installer$/m.test(signature) || !/^Sealed Resources version=2 /m.test(signature))
    throw new Error('Missing app identity or sealed resources.');
  return { signing: 'ad-hoc', signingSubject: null, signatureVerified: true };
}

module.exports = { verifyMacSignature };
