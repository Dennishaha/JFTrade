import { spawn } from "node:child_process";

// Combined development entry point; individual services remain available via package scripts.

const npmCommand = process.env.npm_execpath
  ? [process.execPath, [process.env.npm_execpath]]
  : [process.platform === "win32" ? "npm.cmd" : "npm", []];

const docsArgs = [
  ...npmCommand[1],
  "run",
  "dev:docs",
];

const commands = [
  [npmCommand[0], docsArgs],
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
