#!/usr/bin/env node
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const child = spawn("pnpm", ["run", "dev:web"], {
  cwd: repositoryRoot,
  env: {
    ...process.env,
    JFTRADE_DESKTOP_MODE: "1",
    VITE_DEV_API_TARGET: "http://127.0.0.1:3008",
    WAILS_VITE_PORT: "3003"
  },
  stdio: "inherit"
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.once(signal, () => child.kill(signal));
}

child.once("error", (error) => {
  console.error(`Failed to start the Tauri Vue development server: ${error.message}`);
  process.exitCode = 1;
});
child.once("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exitCode = code ?? 1;
});
