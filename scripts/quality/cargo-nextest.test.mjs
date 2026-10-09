import assert from "node:assert/strict";
import test from "node:test";

import { nextestVersion, normalizePnpmArguments, releaseFor, sha256 } from "./cargo-nextest.mjs";

test("pins checksum-verified nextest archives for supported product hosts", () => {
  assert.equal(nextestVersion, "0.9.145");
  const expected = new Map([
    ["darwin-arm64", ["universal-apple-darwin", "52ecaedb4f5af9267ef7ed02bc937d2a15a94ff96cb663080e81311f798c9905"]],
    ["darwin-x64", ["universal-apple-darwin", "52ecaedb4f5af9267ef7ed02bc937d2a15a94ff96cb663080e81311f798c9905"]],
    ["linux-arm64", ["aarch64-unknown-linux-gnu", "0ad2815fd91a7ecec3a7e25c749b584f66729ac688fd26b2fd494f7ff94b7fd0"]],
    ["linux-x64", ["x86_64-unknown-linux-gnu", "32aa82416099eb12fffae9cf1a279ad201fecbd3f74826c613e32e9006b29867"]],
    ["win32-arm64", ["aarch64-pc-windows-msvc", "29614accc4b232b0fabdda1805d449bae3130a8de6cb343a5b2584e0e886cf0a"]],
    ["win32-x64", ["x86_64-pc-windows-msvc", "bfecd545af057adb83be7ddfe135fd067fb9c990544da1ea111e675f45d628a9"]],
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
