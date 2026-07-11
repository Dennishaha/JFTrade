import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";
import {
  assertPreparedDesktopReleaseInputs,
  usesPreparedDesktopReleaseInputs,
} from "./lib/desktop-release-inputs.mjs";

if (usesPreparedDesktopReleaseInputs()) {
  assertPreparedDesktopReleaseInputs(process.cwd());
  console.log("Using prepared desktop release inputs.");
  process.exit(0);
}

for (const [command, args] of [
  ["npm", ["run", "prepare:desktop-release"]],
  ["npm", ["run", "generate:wails-bindings"]],
  ["npm", ["run", "build:pineworker"]],
]) {
  const status = spawnChecked(command, args, { cwd: process.cwd() });
  if (status !== 0) process.exit(status);
}
