import { existsSync } from "node:fs";
import { delimiter, join } from "node:path";
import { spawnSync } from "node:child_process";

export function findBun() {
  const configured = process.env.JFTRADE_BUN_BINARY?.trim();
  if (configured && existsSync(configured)) {
    return configured;
  }
  const name = process.platform === "win32" ? "bun.exe" : "bun";
  for (const dir of (process.env.PATH ?? "").split(delimiter)) {
    if (!dir) continue;
    const candidate = join(dir, name);
    if (existsSync(candidate)) {
      return candidate;
    }
  }
  for (const candidate of fallbackCandidates(name)) {
    if (existsSync(candidate)) {
      return candidate;
    }
  }
  return "";
}

export function runBun(args, options = {}) {
  const bun = findBun();
  if (!bun) {
    console.error("bun is not installed or not discoverable. Set JFTRADE_BUN_BINARY or install Bun in ~/.bun/bin.");
    return { status: 127 };
  }
  const result = spawnSync(bun, args, { stdio: "inherit", ...options });
  if (result.error) {
    console.error(result.error.message);
    return { status: 1 };
  }
  return { status: result.status ?? 0 };
}

function fallbackCandidates(name) {
  const homes = [process.env.USERPROFILE, process.env.HOME].filter(Boolean);
  const candidates = homes.map((home) => join(home, ".bun", "bin", name));
  if (process.platform === "win32" && process.env.LOCALAPPDATA) {
    candidates.push(join(process.env.LOCALAPPDATA, "bun", "bun.exe"));
    candidates.push(join(process.env.LOCALAPPDATA, "Programs", "Bun", "bun.exe"));
  }
  return candidates;
}
