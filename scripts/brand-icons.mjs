import { mkdir, rm } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import sharp from 'sharp';
const source = 'resources/branding/cboc-mark.png', icons = 'dist/branding/app.iconset';
await mkdir(icons, { recursive: true });
for (const size of [16, 32, 128, 256, 512]) {
  for (const scale of [1, 2]) await sharp(source).resize(size * scale, size * scale)
    .png().toFile(`${icons}/icon_${size}x${size}${scale === 2 ? '@2x' : ''}.png`);
}
execFileSync('/usr/bin/iconutil', ['-c', 'icns', icons, '-o', 'resources/branding/app-icon.icns']);
execFileSync('magick', [source, '-define', 'icon:auto-resize=256,128,64,48,32,16', 'resources/branding/app-icon.ico']);
await rm(icons, { recursive: true });
console.log('Created macOS and Windows icon containers from the original transparent mark.');
