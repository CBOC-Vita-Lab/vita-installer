# CBOC identity

**Name:** CBOC Vita Installer. Use CBOC as the short wordmark.

The cyan C and lime insertion arrow identify the installer, not any game.
The logo uses no game artwork or third-party console mark. Keep the original
transparent image, its proportions and clear space. Use the system UI font;
the wordmark and logo sit beside the controls, not above a promotional banner.

Colors: charcoal `#141719`, surface `#1d2226`, mint `#67e8df`, lime `#b0ee52`,
text `#f5f7f8`, muted text `#a3acb5`. Coral `#ff8b83` is reserved for errors.

Source: `resources/branding/cboc-mark.png`, created for this project with the
built-in image-generation tool. No owner artwork or personal identifier was
used. This records provenance, not a trademark clearance claim.

Final prompt: "Refine this logo into a pristine, very simple flat mark. Keep the bold cyan angular C and the lime downward insertion arrow. REMOVE the controller, all gradients, texture, highlights, haze, stray pixels and ragged remnants. Only two opaque shapes: an angular cyan C and a centered lime down arrow inside its open center, with ample clean empty spacing between them. Solid colors #67e8df and #b0ee52 only with smoothly antialiased boundaries. Perfect transparent alpha everywhere else. Clean symmetric geometry, icon-readable at 32px, even padding on all sides. No text, no outline, no background tile, no glow or shadows."

On a Mac with the project's npm dependencies and ImageMagick installed,
`node scripts/brand-icons.mjs` derives the ICNS and ICO containers without
changing the source image. These tools are not needed by installer users.
