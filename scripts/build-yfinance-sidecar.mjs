#!/usr/bin/env node

// Deprecated compatibility entrypoint. Keep external development and release
// commands working while the repository uses the generic market-data name.
await import("./build-marketdata-sidecar.mjs");
