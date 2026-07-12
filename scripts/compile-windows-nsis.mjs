import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";
import { resolveWindowsNSISInvocation } from "./lib/windows-nsis.mjs";

const invocation = resolveWindowsNSISInvocation({
  arch: process.argv[2],
  installer: process.argv[3],
  makensis: process.env.MAKENSIS,
  rootDir: process.cwd(),
});
const status = spawnChecked(invocation.command, invocation.args, {
  cwd: invocation.cwd,
});
if (status !== 0) process.exit(status);
