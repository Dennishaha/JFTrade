# Pine v6 Native Public Surface Cleanup

## Summary

This cleanup keeps JFTrade helper names as internal lowering/runtime bindings while moving public authoring surfaces toward Pine v6-native syntax. Visual generation, editor hints, and direct Pine diagnostics should expose `ta.*`, `request.security(...)`, bracket history, and ternary-style Pine concepts instead of JFTrade-only helper names.

## Implementation Plan

- Make visual series-condition output use native `ta.rising`, `ta.falling`, `ta.barssince`, and `ta.valuewhen`.
- Keep reverse parsing for the new native `ta.*` form; old bare helper forms are rejected like other internal JFTrade helpers.
- Reject remaining public-only JFTrade helper calls that can leak through direct Pine input, including bare `barssince`, `valuewhen`, `previous`, `history`, `ifelse`, `cross_over`, `cross_under`, and `notify`.
- Update editor capability/intellisense wording so user-facing descriptions emphasize Pine v6 syntax and supported JFTrade runtime subsets, not internal binding names.
- Leave internal requirement keys and lowered expressions such as `ma:*`, `history(...)`, and `cross_over(...)` unchanged.

## Follow-Up Cleanup Status

- Pine snippet visual fallback has been removed from the visual block catalog, parser fallback, support summary, UI surfaces, and current docs/tests.
- Decide whether divergence helpers should be expanded into explicit Pine conditions or removed from public visual authoring.
- Keep visual no-op handling for native Pine visual APIs such as `plot` and `alertcondition`, but describe them as unsupported-for-trading warnings rather than compatibility helpers.

## Test Plan

- Frontend: verify series-condition visual generation emits `ta.*` state functions and reverse parsing still returns structured series-condition blocks.
- Backend: verify direct Pine rejects internal helper-style calls and still accepts native `ta.*` equivalents.
- Regression: run targeted strategy visual tests, Pine package tests, web typecheck, and diff whitespace checks.
