import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";

const run = (platform, env = {}) =>
  spawnSync(
    process.execPath,
    ["scripts/sign-desktop-release.mjs", platform, "missing-artifact"],
    {
      encoding: "utf8",
      env: {
        ...process.env,
        JFTRADE_MACOS_SIGN_IDENTITY: "",
        JFTRADE_MACOS_NOTARY_PROFILE: "",
        JFTRADE_WINDOWS_CERTIFICATE: "",
        JFTRADE_WINDOWS_CERTIFICATE_PASSWORD: "",
        ...env,
      },
    },
  );

assert.equal(run("macos").status, 0, "unsigned macOS flow should be accepted");
assert.equal(
  run("windows").status,
  0,
  "unsigned Windows flow should be accepted",
);
const partialMac = run("macos", {
  JFTRADE_MACOS_SIGN_IDENTITY: "Developer ID",
});
assert.notEqual(partialMac.status, 0, "partial macOS credentials must fail");
assert(partialMac.stderr.includes("all set or all unset"));
const partialWindows = run("windows", {
  JFTRADE_WINDOWS_CERTIFICATE: "cert.pfx",
});
assert.notEqual(
  partialWindows.status,
  0,
  "partial Windows credentials must fail",
);
assert(partialWindows.stderr.includes("all set or all unset"));
