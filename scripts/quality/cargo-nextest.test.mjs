import assert from "node:assert/strict";
import test from "node:test";

import { nextestVersion, normalizePnpmArguments, releaseFor, sha256 } from "./cargo-nextest.mjs";

test("pins checksum-verified nextest archives for supported product hosts", () => {
  assert.equal(nextestVersion, "0.9.143");
  const expected = new Map([
    ["darwin-arm64", ["universal-apple-darwin", "4830d430411148d17602a75cc880bfb4dc8dac153dea59a48a2ef4cc93577f07"]],
    ["darwin-x64", ["universal-apple-darwin", "4830d430411148d17602a75cc880bfb4dc8dac153dea59a48a2ef4cc93577f07"]],
    ["linux-arm64", ["aarch64-unknown-linux-gnu", "2a64b3566a92508550a7ab29c3e8db25472ca37730ecb4d22100b6aa440c2a68"]],
    ["linux-x64", ["x86_64-unknown-linux-gnu", "66786b9abe23920d022a182d1416b1bbc8130dd4872a9553d76985a1708dcd1e"]],
    ["win32-arm64", ["aarch64-pc-windows-msvc", "c89ca8168a6cb1aff6e38b3551bedc9b924477aa983d947b99038c5bed6438ba"]],
    ["win32-x64", ["x86_64-pc-windows-msvc", "c42a1dbde532da06dc9b4a43d44fd0ce668b836c2ab7388410f10ff9834476a2"]],
  ]);
  for (const [host, [target, digest]] of expected) {
    const [platform, arch] = host.split("-");
    const release = releaseFor(platform, arch);
    assert.equal(release.target, target);
    assert.equal(release.sha256, digest);
    assert.equal(release.archiveName, `cargo-nextest-${nextestVersion}-${target}.tar.gz`);
    assert.match(release.url, new RegExp(`/cargo-nextest-${nextestVersion}/${release.archiveName}$`));
  }
  assert.throws(() => releaseFor("freebsd", "x64"), /does not support/);
});

test("computes the SHA-256 used by the bootstrap verifier", () => {
  assert.equal(sha256(Buffer.from("jftrade-nextest")), "6ed1cc57c65bfb8c083f0d865c3555386f5f3ab621288a47ce6af7248f80fe10");
});

test("removes pnpm's argument separator without changing nextest options", () => {
  assert.deepEqual(
    normalizePnpmArguments(["archive", "--workspace", "--", "--archive-file", "/tmp/tests.tar.zst"]),
    ["archive", "--workspace", "--archive-file", "/tmp/tests.tar.zst"],
  );
  assert.deepEqual(normalizePnpmArguments(["run", "--workspace"]), ["run", "--workspace"]);
});
