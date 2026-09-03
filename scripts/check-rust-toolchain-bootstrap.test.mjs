import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const setupRustPath = new URL("../.github/actions/setup-rust/action.yml", import.meta.url);
const ciWorkflowPath = new URL("../.github/workflows/ci.yml", import.meta.url);
const packageJsonPath = new URL("../package.json", import.meta.url);
const nextestWrapperPath = new URL("./quality/cargo-nextest.mjs", import.meta.url);
const archivedReplayTests = new Map([
  [
    new URL("../crates/jftrade-engine/tests/api_transport_compatibility.rs", import.meta.url),
    "NEXTEST_BIN_EXE_jftrade_api_transport_replay",
  ],
  [
    new URL("../crates/jftrade-engine/tests/assistant_runtime_compatibility.rs", import.meta.url),
    "NEXTEST_BIN_EXE_jftrade_assistant_runtime_replay",
  ],
  [
    new URL("../crates/jftrade-engine/tests/provider_runtime_compatibility.rs", import.meta.url),
    "NEXTEST_BIN_EXE_jftrade_provider_runtime_replay",
  ],
  [
    new URL("../crates/jftrade-engine/tests/trading_strategy_compatibility.rs", import.meta.url),
    "NEXTEST_BIN_EXE_jftrade_trading_strategy_replay",
  ],
]);

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

test("Rust CI builds once then partitions all tests with nextest", async () => {
  const source = await readFile(ciWorkflowPath, "utf8");
  const rustStatic = source.slice(
    source.indexOf("\n  rust-static:\n"),
    source.indexOf("\n  rust-test-build:\n"),
  );
  const rustTestBuild = source.slice(
    source.indexOf("\n  rust-test-build:\n"),
    source.indexOf("\n  rust-tests:\n"),
  );
  const rustTests = source.slice(
    source.indexOf("\n  rust-tests:\n"),
    source.indexOf("\n  compatibility:\n"),
  );
  const compatibility = source.slice(
    source.indexOf("\n  compatibility:\n"),
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
  const buildAndTest = source.slice(source.indexOf("\n  build-and-test:\n"));
  const dependencyStep = rustStatic.indexOf(
    "run: bash scripts/install-linux-desktop-dependencies.sh",
  );
  const staticGate = rustStatic.indexOf("run: pnpm run check:rust:static");
  const compileOnlyConfig = `TAURI_CONFIG: '{"bundle":{"resources":[]}}'`;
  const plannedReleaseTag = `JFTRADE_DESKTOP_RELEASE_TAG: "v0.29.0"`;

  assert.ok(dependencyStep >= 0, "Rust static checks must install Tauri Linux headers");
  assert.ok(staticGate > dependencyStep, "headers must be installed before static checks");
  assert.ok(rustStatic.includes(compileOnlyConfig));
  assert.ok(rustTestBuild.includes(compileOnlyConfig));
  assert.ok(rustTests.includes(compileOnlyConfig));
  assert.ok(compatibility.includes(compileOnlyConfig));
  assert.match(rustTestBuild, /Build one immutable nextest archive/);
  assert.match(rustTestBuild, /pnpm run test:rust:archive -- --archive-file/);
  assert.match(rustTestBuild, /actions\/upload-artifact@v7/);
  assert.match(rustTestBuild, /compression-level: 0/);
  assert.match(rustTests, /name: Rust Tests \(\$\{\{ matrix\.shard \}\}\/4\)/);
  assert.match(rustTests, /needs: \[gate-plan, rust-test-build\]/);
  assert.match(rustTests, /fail-fast: false/);
  assert.match(rustTests, /shard: \[1, 2, 3, 4\]/);
  assert.match(rustTests, /actions\/download-artifact@v8/);
  assert.match(rustTests, /--partition "slice:\$\{\{ matrix\.shard \}\}\/4"/);
  assert.doesNotMatch(rustTests, /--partition count:/);
  assert.doesNotMatch(rustTests, /cargo (?:test|nextest archive)/);
  assert.match(compatibility, /Replay affected compatibility capabilities in parallel/);
  assert.match(compatibility, /Prebuild compatibility replay binaries without Cargo lock contention/);
  assert.match(compatibility, /cargo build --locked -p jftrade-engine --bins/);
  assert.match(compatibility, /JFTRADE_COMPATIBILITY_BIN_DIR: target\/debug/);
  for (const job of [rustStatic, rustTestBuild, compatibility]) {
    assert.match(job, /sccache: "true"/);
    assert.match(job, /fast-linker: "true"/);
    assert.match(job, /cache: true/);
    assert.match(job, /pnpm install --frozen-lockfile/);
    assert.ok(
      job.indexOf("pnpm install --frozen-lockfile") < job.indexOf("uses: ./.github/actions/setup-rust"),
      "Rust lanes must finish one deterministic pnpm install before concurrent commands start",
    );
    assert.doesNotMatch(job, /needs:.*rust-(?:static|unit-tests|integration-tests)/);
  }
  assert.match(windowsArm, /cargo check --workspace --all-targets --locked --target aarch64-pc-windows-msvc/);
  assert.doesNotMatch(windowsArm, /sccache: "true"/);
  assert.match(rustTests, /pnpm install --frozen-lockfile/);
  assert.doesNotMatch(rustTests, /sccache: "true"/);
  assert.doesNotMatch(rustTests, /fast-linker: "true"/);
  for (const jobName of ["rust-test-build", "rust-tests", "compatibility"]) {
    assert.ok(buildAndTest.includes(`- ${jobName}`), `aggregator must depend on ${jobName}`);
  }
  assert.match(buildAndTest, /RUST_TEST_BUILD_STATUS:.*needs\.rust-test-build\.result/);
  assert.match(buildAndTest, /RUST_TEST_STATUS:.*needs\.rust-tests\.result/);
  assert.match(buildAndTest, /COMPATIBILITY_STATUS:.*needs\.compatibility\.result/);
  assert.match(buildAndTest, /require_lane "\$RUST_TESTS_PLANNED" "\$RUST_TEST_BUILD_STATUS" RustTestBuild/);
  assert.match(buildAndTest, /require_lane "\$RUST_TESTS_PLANNED" "\$RUST_TEST_STATUS" RustTests/);
  assert.match(buildAndTest, /require_lane "\$COMPATIBILITY_PLANNED" "\$COMPATIBILITY_STATUS" Compatibility/);
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
    assert.match(
      job,
      /contracts_artifact: ""/,
      `${name} must not download a contract artifact after validating committed contracts`,
    );
    assert.match(job, /pnpm install --frozen-lockfile/, `${name} must install the pinned desktop toolchain`);
    assert.match(job, /sccache: "true"/, `${name} must use the shared compiler cache`);
    const bindInputs = job.indexOf("node scripts/write-desktop-release-input-manifest.mjs");
    const prepareRuntime = job.indexOf("pnpm run prepare:tauri-release");
    const linuxDependencies = job.indexOf("bash scripts/install-linux-desktop-dependencies.sh");
    const setupRust = job.indexOf("uses: ./.github/actions/setup-rust");
    assert.ok(linuxDependencies >= 0, `${name} must install the Linux mold package when applicable`);
    assert.ok(linuxDependencies < setupRust, `${name} must install mold before Rust setup enables it`);
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

test("Rust target caches are isolated while compiler objects are shared", async () => {
  const source = await readFile(setupRustPath, "utf8");
  assert.match(source, /inputs\.target/);
  assert.match(source, /inputs\.cache-job/);
  assert.match(source, /inputs\.sccache/);
  assert.match(source, /inputs\.fast-linker/);
  assert.match(source, /mozilla-actions\/sccache-action@v0\.0\.11/);
  assert.match(source, /version: "v0\.16\.0"/);
  assert.match(source, /RUSTC_WRAPPER=sccache/);
  assert.match(source, /SCCACHE_GHA_ENABLED=true/);
  assert.match(source, /SCCACHE_GHA_VERSION=jftrade-rust-1\.97\.1-v1/);
  assert.match(source, /CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER=clang/);
  assert.match(source, /RUSTFLAGS=-C link-arg=-fuse-ld=mold/);
  assert.match(source, /sccache-\$\{\{ inputs\.sccache \}\}-linker-\$\{\{ inputs\.fast-linker \}\}/);
  assert.match(source, /1\.97\.1-\$\{\{ hashFiles\('Cargo\.lock'\) \}\}-\$\{\{ github\.sha \}\}/);
  assert.match(source, /key: protoc-34\.1-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}/);
  assert.match(source, /key: cargo-deny-0\.20\.2-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}/);
});

test("local and CI nextest entrypoints preserve all-target coverage", async () => {
  const packageJson = JSON.parse(await readFile(packageJsonPath, "utf8"));
  const wrapper = await readFile(nextestWrapperPath, "utf8");
  assert.equal(
    packageJson.scripts["test:rust"],
    "node scripts/quality/cargo-nextest.mjs run --workspace --all-targets --locked",
  );
  assert.equal(
    packageJson.scripts["test:rust:archive"],
    "node scripts/quality/cargo-nextest.mjs archive --workspace --all-targets --locked",
  );
  assert.equal(
    packageJson.scripts["test:rust:archive-shard"],
    "node scripts/quality/cargo-nextest.mjs run",
  );
  assert.match(wrapper, /nextestVersion = "0\.9\.143"/);
  assert.match(wrapper, /archive checksum mismatch/);
  assert.doesNotMatch(JSON.stringify(packageJson.scripts), /run-rust-engine-unit-shard/);
});

test("archived replay tests resolve relocatable nextest runtime paths", async () => {
  for (const [path, runtimeBinaryVariable] of archivedReplayTests) {
    const source = await readFile(path, "utf8");
    assert.ok(
      source.includes(`std::env::var_os("${runtimeBinaryVariable}")`),
      `${path.pathname} must resolve its replay binary after archive extraction`,
    );
    assert.match(source, /std::env::var_os\("CARGO_MANIFEST_DIR"\)/);
    assert.doesNotMatch(source, /Command::new\(env!\("CARGO_BIN_EXE_/);
  }
});

test("baseline-aware CI jobs fetch merge-base history", async () => {
  const source = await readFile(ciWorkflowPath, "utf8");
  const contracts = source.slice(
    source.indexOf("\n  contracts:\n"),
    source.indexOf("\n  rust-static:\n"),
  );
  const web = source.slice(source.indexOf("\n  web:\n"), source.indexOf("\n  pine:\n"));

  for (const [name, job] of [
    ["Contracts", contracts],
    ["Web", web],
  ]) {
    assert.match(job, /uses: actions\/checkout@v7\s+with:\s+fetch-depth: 0/, `${name} must fetch its comparison baseline`);
  }
});
