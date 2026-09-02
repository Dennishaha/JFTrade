import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const setupRustPath = new URL("../.github/actions/setup-rust/action.yml", import.meta.url);
const ciWorkflowPath = new URL("../.github/workflows/ci.yml", import.meta.url);

test("Rust bootstrap installs checksum-pinned protoc on every supported runner", async () => {
  const source = await readFile(setupRustPath, "utf8");
  const expectedArchives = new Map([
    [
      "protoc-${version}-linux-x86_64.zip",
      "af27ea66cd26938fe48587804ca7d4817457a08350021a1c6e23a27ccc8c6904",
    ],
    [
      "protoc-${version}-linux-aarch_64.zip",
      "31c5e9e3c7bf013cf41fb97765ee255c140024a6b175b6cc9b64beddd7c23ba7",
    ],
    [
      "protoc-${version}-osx-x86_64.zip",
      "ab124429c1f49951f03b6c0c0e911fec04e2c7c20de5c935e0cde7353bbd016c",
    ],
    [
      "protoc-${version}-osx-aarch_64.zip",
      "2c7e92b8b578916937df132b3032e2e8e6c170862ecf7a8333094a6f3d03650c",
    ],
    [
      "protoc-${version}-win64.zip",
      "6d7ebdc75e9c1f0026d4fb28f17ef1d0aae77d36744d83a9e052d79ba493724f",
    ],
  ]);

  assert.match(source, /version="34\.1"/);
  for (const runner of [
    "Linux-X64",
    "Linux-ARM64",
    "macOS-X64",
    "macOS-ARM64",
    "Windows-X64|Windows-ARM64",
  ]) {
    assert.ok(source.includes(`${runner})`), `missing protoc bootstrap for ${runner}`);
  }
  for (const [archive, digest] of expectedArchives) {
    assert.ok(source.includes(`archive_name="${archive}"`), `missing ${archive}`);
    assert.ok(source.includes(`expected_sha256="${digest}"`), `missing digest for ${archive}`);
  }
  assert.match(source, /actual_sha256.*expected_sha256/);
  assert.match(source, /libprotoc \$\{version\}/);
  assert.match(source, /GITHUB_PATH/);
  assert.match(source, /GITHUB_ENV/);
});

test("Rust CI provisions native headers and detaches compile-only checks from package resources", async () => {
  const source = await readFile(ciWorkflowPath, "utf8");
  const rustQuality = source.slice(
    source.indexOf("  rust-quality:"),
    source.indexOf("  rust-platform:"),
  );
  const rustPlatform = source.slice(
    source.indexOf("  rust-platform:"),
    source.indexOf("  web-quality:"),
  );
  const desktopLinuxSmoke = source.slice(
    source.indexOf("  desktop-linux-smoke:"),
    source.indexOf("  desktop-build:"),
  );
  const desktopBuild = source.slice(
    source.indexOf("  desktop-build:"),
    source.indexOf("  build-and-test:"),
  );
  const dependencyStep = rustQuality.indexOf(
    "run: bash scripts/install-linux-desktop-dependencies.sh",
  );
  const workspaceGate = rustQuality.indexOf("run: pnpm run check:rust");
  const compileOnlyConfig = `TAURI_CONFIG: '{"bundle":{"resources":[]}}'`;
  const plannedReleaseTag = `JFTRADE_DESKTOP_RELEASE_TAG: "v0.29.0"`;

  assert.match(rustQuality, /timeout-minutes: 90/);
  assert.match(rustQuality, /JFTRADE_STAGE9_PRODUCT_TIMEOUT_MS: "1800000"/);
  assert.ok(dependencyStep >= 0, "Rust quality must install Tauri Linux system headers");
  assert.ok(workspaceGate > dependencyStep, "system headers must be installed before check:rust");
  assert.ok(
    rustQuality.includes(compileOnlyConfig),
    "Rust quality must not require ignored release-package resources",
  );
  assert.ok(
    rustPlatform.includes(compileOnlyConfig),
    "native compile checks must not require ignored release-package resources",
  );
  for (const [name, job] of [
    ["PR desktop smoke", desktopLinuxSmoke],
    ["push desktop build", desktopBuild],
  ]) {
    const bindInputs = job.indexOf("node scripts/write-desktop-release-input-manifest.mjs");
    const prepareRuntime = job.indexOf("pnpm run prepare:tauri-release");
    const testDesktop = job.indexOf("cargo test -p jftrade-desktop");
    assert.ok(bindInputs >= 0, `${name} must bind its downloaded desktop inputs`);
    assert.ok(prepareRuntime > bindInputs, `${name} must bind inputs before preparing resources`);
    assert.ok(prepareRuntime >= 0, `${name} must prepare the Tauri runtime resources`);
    assert.ok(testDesktop > prepareRuntime, `${name} must prepare resources before desktop tests`);
    assert.ok(
      job.includes(plannedReleaseTag),
      `${name} must bind its unsigned build to the planned release version`,
    );
  }
});
