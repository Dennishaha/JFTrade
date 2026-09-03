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
  assert.match(
    source,
    /actual_sha256="\$\{actual_sha256#\\\\\}"/,
    "Windows checksum parsing must strip the GNU escaped-filename marker",
  );
  assert.match(source, /libprotoc \$\{version\}/);
  assert.match(source, /GITHUB_PATH/);
  assert.match(source, /GITHUB_ENV/);
});

test("Rust CI separates static tests and native builds while sharing pinned toolchains", async () => {
  const source = await readFile(ciWorkflowPath, "utf8");
  const rustStatic = source.slice(
    source.indexOf("\n  rust-static:\n"),
    source.indexOf("\n  rust-tests:\n"),
  );
  const rustTests = source.slice(
    source.indexOf("\n  rust-tests:\n"),
    source.indexOf("\n  web:\n"),
  );
  const desktopLinuxSmoke = source.slice(
    source.indexOf("\n  desktop-linux-smoke:\n"),
    source.indexOf("\n  desktop-build:\n"),
  );
  const desktopBuild = source.slice(
    source.indexOf("\n  desktop-build:\n"),
    source.indexOf("\n  windows-arm64-compile:\n"),
  );
  const windowsArm = source.slice(
    source.indexOf("\n  windows-arm64-compile:\n"),
    source.indexOf("\n  build-and-test:\n"),
  );
  const dependencyStep = rustStatic.indexOf(
    "run: bash scripts/install-linux-desktop-dependencies.sh",
  );
  const staticGate = rustStatic.indexOf("run: pnpm run check:rust:static");
  const compileOnlyConfig = `TAURI_CONFIG: '{"bundle":{"resources":[]}}'`;
  const plannedReleaseTag = `JFTRADE_DESKTOP_RELEASE_TAG: "v0.29.0"`;

  assert.ok(dependencyStep >= 0, "Rust static checks must install Tauri Linux headers");
  assert.ok(staticGate > dependencyStep, "headers must be installed before static checks");
  assert.ok(rustStatic.includes(compileOnlyConfig));
  assert.ok(rustTests.includes(compileOnlyConfig));
  assert.match(rustTests, /Run the Rust workspace test suite once/);
  assert.match(rustTests, /Replay affected compatibility capabilities in parallel/);
  assert.doesNotMatch(rustStatic, /needs:.*rust-tests/);
  assert.doesNotMatch(rustTests, /needs:.*rust-static/);
  assert.match(windowsArm, /cargo check --workspace --all-targets --locked --target aarch64-pc-windows-msvc/);
  for (const [name, job] of [
    ["PR desktop smoke", desktopLinuxSmoke],
    ["push desktop build", desktopBuild],
  ]) {
    assert.match(
      job,
      /needs: \[gate-plan, contracts, web, pine\]/,
      `${name} must wait for every desktop build input`,
    );
    assert.match(
      job,
      /needs\.contracts\.result == 'success'/,
      `${name} must require contract validation`,
    );
    assert.match(job, /pnpm install --frozen-lockfile/, `${name} must install the pinned desktop toolchain`);
    const bindInputs = job.indexOf("node scripts/write-desktop-release-input-manifest.mjs");
    const prepareRuntime = job.indexOf("pnpm run prepare:tauri-release");
    assert.ok(bindInputs >= 0, `${name} must bind its downloaded desktop inputs`);
    assert.ok(prepareRuntime > bindInputs, `${name} must bind inputs before preparing resources`);
    assert.ok(prepareRuntime >= 0, `${name} must prepare the Tauri runtime resources`);
    assert.ok(
      job.includes(plannedReleaseTag),
      `${name} must bind its unsigned build to the planned release version`,
    );
    assert.ok(
      !job.includes("apps/desktop/src-tauri/target"),
      `${name} must not look for workspace artifacts below the Tauri crate`,
    );
  }
});

test("Rust caches are isolated by job target toolchain lockfile and commit", async () => {
  const source = await readFile(setupRustPath, "utf8");
  assert.match(source, /inputs\.target/);
  assert.match(source, /inputs\.cache-job/);
  assert.match(source, /1\.97\.1-\$\{\{ hashFiles\('Cargo\.lock'\) \}\}-\$\{\{ github\.sha \}\}/);
  assert.match(source, /key: protoc-34\.1-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}/);
  assert.match(source, /key: cargo-deny-0\.20\.2-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}/);
});
