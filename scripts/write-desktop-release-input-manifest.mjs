#!/usr/bin/env node

import process from "node:process";

import { writeDesktopReleaseInputManifest } from "./lib/desktop-release-inputs.mjs";

const manifest = writeDesktopReleaseInputManifest(process.cwd());
console.log(
  `Wrote desktop release input manifest for ${manifest.files.length} shared input(s).`,
);
