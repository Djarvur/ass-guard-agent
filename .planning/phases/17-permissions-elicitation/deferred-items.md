# Phase 17 Deferred Items

Out-of-scope discoveries logged by the executor per the deviation-rule scope boundary (not fixed here; revisit when flagged).

## Watchlist

- **[2026-09-01, from 17-01] Transient `-race` gate flake (unreproduced, package unknown):** twice during 17-01's verification a full-suite run printed a bare `[test] FAIL` (once in `mise ci`, once in a later run); the failing package was lost to output truncation both times. Six dedicated follow-up runs were green (38/38 packages ok, including `mise test` ×3 and `mise ci` back-to-back). Not attributable to 17-01's changes (new leaf package `internal/perm`; zero existing files modified). **Action if it recurs:** run `mise ci 2>&1 | tee /tmp/ci.log` so the `--- FAIL` package line is captured before debugging.
