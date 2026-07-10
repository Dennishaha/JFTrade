import assert from "node:assert/strict";

import { withWindowsDesktopManifestVersion } from "./windows-resource-metadata.mjs";

const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1">
  <assemblyIdentity type="win32" name="com.jftrade.desktop" version="0.0.0" processorArchitecture="*"/>
</assembly>
`;
const updated = withWindowsDesktopManifestVersion(manifest, "0.1.2");
assert.match(updated, /<\?xml version="1\.0" encoding="UTF-8" standalone="yes"\?>/);
assert.match(
  updated,
  /name="com\.jftrade\.desktop" version="0\.1\.2\.0"/,
);
assert.throws(
  () => withWindowsDesktopManifestVersion(manifest, "dev"),
  /Invalid Windows desktop version/,
);
assert.throws(
  () => withWindowsDesktopManifestVersion("<assembly/>", "0.1.2"),
  /missing the com\.jftrade\.desktop assembly identity/,
);

console.log("windows resource metadata tests passed");
