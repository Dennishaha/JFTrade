import assert from "node:assert/strict";
import path from "node:path";

import {
  linuxPackageFormats,
  linuxReleaseArtifactName,
  linuxReleaseArtifactPaths,
} from "./desktop-release-artifacts.mjs";

assert.deepEqual(linuxPackageFormats, ["appimage", "deb", "rpm"]);
assert.equal(
  linuxReleaseArtifactName("1.2.3", "amd64", "appimage"),
  "JFTrade-1.2.3-linux-x64.AppImage",
);
assert.equal(
  linuxReleaseArtifactName("1.2.3", "amd64", "deb"),
  "JFTrade-1.2.3-linux-x64.deb",
);
assert.equal(
  linuxReleaseArtifactName("1.2.3", "amd64", "rpm"),
  "JFTrade-1.2.3-linux-x64.rpm",
);
assert.equal(
  linuxReleaseArtifactName("1.2.3", "arm64", "rpm"),
  "JFTrade-1.2.3-linux-arm64.rpm",
);
assert.deepEqual(
  linuxReleaseArtifactPaths("release", "1.2.3", "amd64"),
  [
    path.join("release", "JFTrade-1.2.3-linux-x64.AppImage"),
    path.join("release", "JFTrade-1.2.3-linux-x64.deb"),
    path.join("release", "JFTrade-1.2.3-linux-x64.rpm"),
  ],
);
assert.deepEqual(
  linuxReleaseArtifactPaths("release", "1.2.3", "amd64", "deb"),
  [path.join("release", "JFTrade-1.2.3-linux-x64.deb")],
);
assert.throws(
  () => linuxReleaseArtifactName("dev", "amd64", "deb"),
  /Invalid Linux release version/,
);
assert.throws(
  () => linuxReleaseArtifactName("1.2.3", "riscv64", "deb"),
  /Unsupported Linux release architecture/,
);
assert.throws(
  () => linuxReleaseArtifactName("1.2.3", "amd64", "archlinux"),
  /Unsupported Linux package format/,
);

console.log("desktop release artifact tests passed");
