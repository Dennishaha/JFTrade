import { spawnSync } from "node:child_process";

const windowsCommandExtensions = new Set(["pnpm", "pnpx"]);

export function spawnChecked(command, args, options = {}) {
  const resolved = resolveCommand(command, args);
  const result = spawnSync(resolved.command, resolved.args, { stdio: "inherit", ...options });
  if (result.error) {
    console.error(result.error.message);
    return result.status ?? 1;
  }
  return result.status ?? 0;
}

export function resolveCommand(
  command,
  args,
  { platform = process.platform, environment = process.env } = {},
) {
  if (platform === "win32" && windowsCommandExtensions.has(command)) {
    return {
      command: environment.ComSpec || "cmd.exe",
      args: ["/d", "/s", "/c", [command, ...args].map(windowsQuote).join(" ")],
    };
  }
  return { command, args };
}

function windowsQuote(value) {
  if (/^[A-Za-z0-9_@./:=+-]+$/.test(value)) {
    return value;
  }
  return `"${value.replace(/"/g, '\\"')}"`;
}
