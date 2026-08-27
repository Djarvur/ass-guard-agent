# Phase 16 Deferred Items

| Item | Source | Status | Deferred At | Owner |
|------|--------|--------|-------------|-------|
| `TestAskPark_PromptResponsePrecedesResolution` (internal/runtime, Phase 12) tripped its 10s "near-instant" bound at 11.6s under one fully parallel `mise ci` run (2026-08-27, 16-01 execution) — scheduling delay under load, not semantics (the resolution path it guards against is a 1-hour timer). Passes standalone and in subsequent full-CI runs. Pre-existing test, out of 16-01 scope per the deviation scope boundary. | 16-01 execution (SUMMARY §Issues Encountered) | Open — verifier to consider a bound raise (10s→30s keeps the assertion's power) | 2026-08-27 | Phase 16 verifier / test-robustness pass |
