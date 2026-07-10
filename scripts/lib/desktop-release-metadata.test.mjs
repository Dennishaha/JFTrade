import assert from "node:assert/strict";

import {
  requireDesktopReleaseMetadata,
  resolveDesktopBuildMetadata,
} from "./desktop-release-metadata.mjs";

const metadata = resolveDesktopBuildMetadata(
  {
    JFTRADE_DESKTOP_RELEASE_TAG: "v1.2.3",
    JFTRADE_DESKTOP_COMMIT: "abc123",
    SOURCE_DATE_EPOCH: "0",
  },
  new Date("2026-07-10T00:00:00Z"),
);
assert.deepEqual(metadata, {
  tag: "v1.2.3",
  version: "1.2.3",
  numericVersion: "1.2.3",
  commit: "abc123",
  buildTime: "1970-01-01T00:00:00.000Z",
});
assert.equal(requireDesktopReleaseMetadata(metadata), metadata);

const development = resolveDesktopBuildMetadata(
  { GITHUB_REF_NAME: "main", GITHUB_SHA: "def456" },
  new Date("2026-07-10T00:00:00Z"),
);
assert.equal(development.version, "dev");
assert.equal(development.numericVersion, "0.0.0");
assert.throws(
  () => requireDesktopReleaseMetadata(development),
  /vX\.Y\.Z/,
);

const zeroVersion = resolveDesktopBuildMetadata(
  { JFTRADE_DESKTOP_RELEASE_TAG: "v0.0.0" },
  new Date("2026-07-10T00:00:00Z"),
);
assert.throws(() => requireDesktopReleaseMetadata(zeroVersion), /v0\.0\.0/);

console.log("desktop release metadata tests passed");
