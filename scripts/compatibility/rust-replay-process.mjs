import fs from "node:fs";
import path from "node:path";

export function rustReplayInvocation({
  root,
  packageName,
  binaryName,
  args,
  env = process.env,
  platform = process.platform,
}) {
  const binaryDirectory = env.JFTRADE_COMPATIBILITY_BIN_DIR;
  if (!binaryDirectory) {
    return {
      command: "cargo",
      args: ["run", "--quiet", "-p", packageName, "--bin", binaryName, "--", ...args],
    };
  }

  const executableName = platform === "win32" ? `${binaryName}.exe` : binaryName;
  const command = path.resolve(root, binaryDirectory, executableName);
  if (!fs.existsSync(command)) {
    throw new Error(`prebuilt compatibility replay binary is missing: ${command}`);
  }
  return { command, args };
}
