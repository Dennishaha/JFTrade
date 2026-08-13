# Scripts library

`scripts/lib` contains reusable implementation modules for repository maintenance
scripts. Command-line entrypoints stay in `scripts/`; reusable logic and testable
contracts live here.

## Module responsibilities and stable APIs

The exports below are the stable, repository-internal API. Callers should import
these symbols instead of copying their logic or depending on unexported helpers.

| Module | Responsibility | Stable exports |
| --- | --- | --- |
| `desktop-release-metadata.mjs` | Resolve and validate desktop version, commit and reproducible build time metadata. | `desktopReleaseTagPattern`, `resolveDesktopBuildMetadata`, `requireDesktopReleaseMetadata` |
| `desktop-release-artifacts.mjs` | Define canonical Linux desktop artifact names, paths and package extensions. | `linuxPackageFormats`, `linuxReleaseArtifactName`, `linuxReleaseArtifactPaths`, `linuxPackageExtension` |
| `desktop-release-inputs.mjs` | Detect and validate prepared frontend, documentation and Pineworker release inputs. | `desktopReleaseInputPaths`, `usesPreparedDesktopReleaseInputs`, `assertPreparedDesktopReleaseInputs` |
| `spawn.mjs` | Run checked child processes with cross-platform pnpm command resolution. | `spawnChecked` |
| `pinets-package.mjs` | Verify the installed PineTS package and its license before bundling. | `checkPinetsPackageAndLicense` |
| `pineworker-rolldown-build.mjs` | Produce the Node ESM Pineworker bundle or its dry-run command. | `buildPineWorkerBundle`, `dryRunPineWorkerBundleCommand` |
| `openapi-quality.mjs` | Find OpenAPI quality gaps and maintain the quality allowlist. | `findOpenAPIQualityGaps`, `compareQualityGaps`, `buildQualityAllowlist` |
| `web-contract-index.mjs` | Validate the Web contract barrel exports. | `contractIndexViolations` |
| `web-contract-audit.mjs` | Audit wire contracts, view-model classification and exported declaration counts. | `normalizeRelativePath`, `generatedSchemaViolations`, `wireContractViolations`, `viewModelClassificationViolations`, `classifiedDeclarationCounts` |

Changing an exported symbol requires updating its direct callers and tests in the
same change. Unexported helpers are implementation details.

## Test convention

- Name Node maintenance tests `*.test.mjs`, use `node:assert/strict` and
  `node:test` for new cases, and keep tests deterministic and offline.
- Register every new test in `scriptTestSuites` in `scripts/test-scripts.mjs`.
  Its own regression test fails if a `*.test.mjs` file is missing from the
  registry.
- Put focused library tests beside their module in `scripts/lib`; integration
  and command tests stay in `scripts/`.
- Use temporary directories for filesystem cases and remove them in `finally`.
  Do not write generated outputs into the checkout.
- Run all Node maintenance tests with `pnpm run test:scripts`. Use
  `pnpm run test:scripts -- policy` or `pnpm run test:scripts -- desktop` for a
  focused suite. `pnpm run test:test-policy` also runs the `policy` suite, while
  desktop CI and release workflows run the `desktop` suite.
