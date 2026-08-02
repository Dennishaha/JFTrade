import {
  cpSync,
  lstatSync,
  readlinkSync,
  readdirSync,
  realpathSync,
  rmSync,
} from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

export function materializeDirectorySymlinks(rootDirectory) {
  const root = realpathSync(rootDirectory);
  const links = collectSymlinks(root).map((path) => ({
    path,
    target: resolveLinkTarget(path),
  }));

  for (const link of links) {
    assertTargetInsideRoot(root, link.path, link.target);
  }
  for (const link of links) {
    rmSync(link.path, { force: true, recursive: true });
    cpSync(link.target, link.path, {
      dereference: true,
      errorOnExist: true,
      recursive: lstatSync(link.target).isDirectory(),
    });
  }

  const remaining = collectSymlinks(root);
  if (remaining.length > 0) {
    throw new Error(
      `Failed to materialize bundle symlink: ${relative(root, remaining[0])}`,
    );
  }
  return links.length;
}

function collectSymlinks(root) {
  const links = [];
  const directories = [root];
  while (directories.length > 0) {
    const directory = directories.pop();
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name);
      if (entry.isSymbolicLink()) {
        links.push(path);
      } else if (entry.isDirectory()) {
        directories.push(path);
      }
    }
  }
  return links.sort((left, right) => left.localeCompare(right));
}

function resolveLinkTarget(path) {
  const rawTarget = readlinkSync(path);
  const target = isAbsolute(rawTarget)
    ? rawTarget
    : resolve(dirname(path), rawTarget);
  return realpathSync(target);
}

function assertTargetInsideRoot(root, link, target) {
  const relativeTarget = relative(root, target);
  if (
    relativeTarget === "" ||
    (!relativeTarget.startsWith(`..${sep}`) && relativeTarget !== "..")
  ) {
    return;
  }
  throw new Error(
    `Bundle symlink ${relative(root, link)} resolves outside the bundle`,
  );
}
