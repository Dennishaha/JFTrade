import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-pine-proto-"));

try {
  const scriptsDirectory = path.join(directory, "scripts");
  const protoDirectory = path.join(directory, "pkg", "strategy", "pineworker", "proto");
  const outputDirectory = path.join(
    directory,
    "pkg",
    "strategy",
    "pineworker",
    "pineworkerpb",
  );
  const binDirectory = path.join(directory, "bin");
  for (const target of [scriptsDirectory, protoDirectory, outputDirectory, binDirectory]) {
    fs.mkdirSync(target, { recursive: true });
  }

  fs.copyFileSync(
    "scripts/gen-pineworker-proto.sh",
    path.join(scriptsDirectory, "gen-pineworker-proto.sh"),
  );
  for (const filename of [
    "pineworker_common.proto",
    "pineworker_types.proto",
    "pineworker.proto",
  ]) {
    fs.writeFileSync(path.join(protoDirectory, filename), "syntax = \"proto3\";\n");
  }

  const failingProtoc = path.join(binDirectory, "protoc");
  fs.writeFileSync(
    failingProtoc,
    `#!/usr/bin/env bash
if [ "\${1:-}" = "--version" ]; then
  echo 'libprotoc 34.1'
  exit 0
fi
echo synthetic protoc failure >&2
exit 42
`,
  );
  fs.chmodSync(failingProtoc, 0o755);
  for (const [plugin, version] of [
    ["protoc-gen-go", "protoc-gen-go v1.36.11"],
    ["protoc-gen-go-grpc", "protoc-gen-go-grpc 1.6.2"],
  ]) {
    const pluginPath = path.join(binDirectory, plugin);
    fs.writeFileSync(pluginPath, `#!/usr/bin/env bash\necho '${version}'\n`);
    fs.chmodSync(pluginPath, 0o755);
  }

  const marker = path.join(outputDirectory, "existing.pb.go");
  fs.writeFileSync(marker, "existing generated code\n");
  const result = spawnSync("bash", [path.join(scriptsDirectory, "gen-pineworker-proto.sh")], {
    encoding: "utf8",
    env: { ...process.env, PATH: `${binDirectory}:${process.env.PATH}` },
  });

  assert.equal(result.status, 42);
  assert.match(result.stderr, /synthetic protoc failure/);
  assert.equal(fs.readFileSync(marker, "utf8"), "existing generated code\n");

  fs.writeFileSync(
    failingProtoc,
    `#!/usr/bin/env bash
if [ "\${1:-}" = "--version" ]; then
  echo 'libprotoc 34.1'
  exit 0
fi
for argument in "$@"; do
  case "$argument" in
    --go_out=*) output="\${argument#*=}" ;;
  esac
done
mkdir -p "$output/proto"
printf 'first line\\nsecond line\\n' >"$output/proto/pineworker.pb.go"
`,
  );
  const oversizedResult = spawnSync(
    "bash",
    [path.join(scriptsDirectory, "gen-pineworker-proto.sh")],
    {
      encoding: "utf8",
      env: {
        ...process.env,
        MAX_GENERATED_LINES: "1",
        PATH: `${binDirectory}:${process.env.PATH}`,
      },
    },
  );

  assert.notEqual(oversizedResult.status, 0);
  assert.match(oversizedResult.stderr, /generated file exceeds 1 lines/);
  assert.equal(fs.readFileSync(marker, "utf8"), "existing generated code\n");
} finally {
  fs.rmSync(directory, { recursive: true, force: true });
}

console.log("Pineworker proto generation tests passed");
