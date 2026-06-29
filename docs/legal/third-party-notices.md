# Third-Party Notices

## PineTS / pinets

- Package: `pinets`
- Version: `0.9.26`
- Upstream: <https://github.com/LuxAlgo/PineTS>
- npm: <https://www.npmjs.com/package/pinets>
- License: `AGPL-3.0-only`
- Integration: optional Node worker used by the experimental `pinets-shadow` engine.

JFTrade uses PineTS only when the external engine mode is explicitly enabled.
The worker process boundary exists for runtime isolation, crash recovery, and upgrade
control. It is not intended to avoid AGPL obligations.

When PineTS functionality is exposed to network users, JFTrade distributors and
operators must provide the corresponding source code, license notice, build
instructions, and any local modifications required by AGPL-3.0.

## Source Offer

The corresponding source for this integration is the JFTrade source tree that
contains:

- `scripts/pinets-worker.mjs`
- `pkg/strategy/pineengine`
- this notice file
- `package.json` dependency metadata

Publish deployments should expose this source location from the product
documentation or an equivalent license/about page.
