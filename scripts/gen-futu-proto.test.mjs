import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-futu-proto-"));

try {
  const scriptsDirectory = path.join(directory, "scripts");
  const sourceDirectory = path.join(directory, "futu-source");
  const stageDirectory = path.join(directory, "pkg", "futu", "proto");
  const outputDirectory = path.join(directory, "pkg", "futu", "pb");
  fs.mkdirSync(scriptsDirectory, { recursive: true });
  fs.mkdirSync(sourceDirectory, { recursive: true });
  fs.mkdirSync(stageDirectory, { recursive: true });
  fs.mkdirSync(outputDirectory, { recursive: true });

  const manifestName = "futu-proto-10.5.6508.sha256";
  const manifest = fs.readFileSync(path.join("scripts", manifestName), "utf8");
  fs.copyFileSync("scripts/gen-futu-proto.sh", path.join(scriptsDirectory, "gen-futu-proto.sh"));
  fs.writeFileSync(path.join(scriptsDirectory, manifestName), manifest);

  const filenames = manifest
    .split("\n")
    .filter((line) => line && !line.startsWith("#"))
    .map((line) => line.trim().split(/\s+/)[1]);
  for (const filename of filenames) {
    fs.writeFileSync(path.join(sourceDirectory, filename), `tampered ${filename}\n`);
  }

  const stageMarker = path.join(stageDirectory, "existing.proto");
  const outputMarker = path.join(outputDirectory, "existing.pb.go");
  fs.writeFileSync(stageMarker, "existing staged proto\n");
  fs.writeFileSync(outputMarker, "existing generated code\n");

  const result = spawnSync("bash", [path.join(scriptsDirectory, "gen-futu-proto.sh")], {
    encoding: "utf8",
    env: { ...process.env, FUTU_PROTO_SRC: sourceDirectory },
  });

  assert.notEqual(result.status, 0, "tampered Futu proto input should be rejected");
  assert.match(result.stderr, /Futu proto checksum mismatch for .*Common\.proto/);
  assert.equal(fs.readFileSync(stageMarker, "utf8"), "existing staged proto\n");
  assert.equal(fs.readFileSync(outputMarker, "utf8"), "existing generated code\n");
} finally {
  fs.rmSync(directory, { recursive: true, force: true });
}

console.log("Futu proto generation tests passed");
