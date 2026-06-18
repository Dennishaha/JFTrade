import { spawn } from "node:child_process";

const npmCommand = process.env.npm_execpath
  ? [process.execPath, [process.env.npm_execpath]]
  : [process.platform === "win32" ? "npm.cmd" : "npm", []];

const vitepressArgs = [
  ...npmCommand[1],
  "exec",
  "--",
  "vitepress",
  "dev",
  "docs",
  "--host",
  "127.0.0.1",
  "--port",
  "3001",
];

const commands = [
  [npmCommand[0], vitepressArgs],
  [npmCommand[0], [...npmCommand[1], "run", "dev:web"]],
];

const children = commands.map(([command, args]) =>
  spawn(command, args, {
    stdio: "inherit",
    shell: process.platform === "win32",
  }),
);

let shuttingDown = false;

for (const child of children) {
  child.on("exit", (code, signal) => {
    if (shuttingDown) {
      return;
    }
    shuttingDown = true;
    for (const other of children) {
      if (other !== child && !other.killed) {
        other.kill();
      }
    }
    if (signal) {
      process.kill(process.pid, signal);
    } else {
      process.exit(code ?? 0);
    }
  });
}

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    shuttingDown = true;
    for (const child of children) {
      if (!child.killed) {
        child.kill(signal);
      }
    }
    process.exit(0);
  });
}
