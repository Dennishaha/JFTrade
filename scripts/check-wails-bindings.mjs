import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";

const rootDir = path.resolve(import.meta.dirname, "..");
const bindingsDir = path.join(rootDir, "apps", "web", "src", "wails");
const before = snapshotDirectory(bindingsDir);

const generateStatus = spawnChecked(
  process.execPath,
  [
    "scripts/wails3.mjs",
    "generate",
    "bindings",
    "-ts",
    "-i",
    "-noevents",
    "-d",
    "apps/web/src/wails",
    "./cmd/jftrade-desktop",
  ],
  { cwd: rootDir },
);
if (generateStatus !== 0) process.exit(generateStatus);

const after = snapshotDirectory(bindingsDir);
if (!snapshotsEqual(before, after)) {
  console.error(
    "Wails bindings are stale. Run npm run generate:wails-bindings.",
  );
  process.exit(1);
}

const serviceFiles = [...after.keys()]
  .filter((file) => file.endsWith("service.ts"))
  .map((file) => path.basename(file))
  .sort();
const expectedServices = [
  "desktoplinkservice.ts",
  "desktoplogservice.ts",
  "desktopupdateservice.ts",
];
if (JSON.stringify(serviceFiles) !== JSON.stringify(expectedServices)) {
  console.error(
    `Unexpected Wails service surface: ${serviceFiles.join(", ") || "none"}`,
  );
  process.exit(1);
}

function snapshotDirectory(directory) {
  const snapshot = new Map();
  if (!fs.existsSync(directory)) return snapshot;
  for (const entry of fs.readdirSync(directory, {
    recursive: true,
    withFileTypes: true,
  })) {
    if (!entry.isFile()) continue;
    const filePath = path.join(entry.parentPath, entry.name);
    snapshot.set(
      path.relative(directory, filePath),
      fs.readFileSync(filePath, "utf8"),
    );
  }
  return snapshot;
}

function snapshotsEqual(left, right) {
  if (left.size !== right.size) return false;
  for (const [file, contents] of left) {
    if (right.get(file) !== contents) return false;
  }
  return true;
}
