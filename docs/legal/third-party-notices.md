# Third-Party Notices

## PineTS / pinets

- Package: `pinets`
- Version: `0.9.26`
- Upstream: <https://github.com/LuxAlgo/PineTS>
- npm: <https://www.npmjs.com/package/pinets>
- License: `AGPL-3.0-only`
- Integration: Node/PineTS worker runtime for `sourceFormat=pine-v6` with
  `runtime=pine-pinets`, plus the separate `pinets-shadow` compatibility report
  harness.

JFTrade uses public PineTS through worker processes under `workers/pineworker`
and the stdio shadow harness at `scripts/pinets-worker.mjs`. The worker process
boundary exists for runtime isolation, crash recovery, and upgrade control. It
is not intended to avoid AGPL obligations.

When PineTS functionality is exposed to network users, JFTrade distributors and
operators must provide the corresponding source code, license notice, build
instructions, and any local modifications required by AGPL-3.0.

## Source Offer

The corresponding source for this integration is the JFTrade source tree that
contains:

- `scripts/pinets-worker.mjs`
- `workers/pineworker`
- `scripts/build-pineworker-assets.mjs`
- `scripts/build-pineworker-dev.mjs`
- `scripts/check-pinets-release.mjs`
- `pkg/strategy/pineengine`
- `pkg/strategy/pineworker`
- this notice file
- `package.json` dependency metadata

Release and build commands for this integration include:

- `npm run build:pineworker`
- `npm run check:pinets-release`

Publish deployments should expose this source location, build instructions, and
license/about page from the product documentation or an equivalent in-product
entry.
