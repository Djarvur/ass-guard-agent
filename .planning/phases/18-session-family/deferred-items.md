
## 18-06 (2026-09-03): golangci-lint 2.12.2 panics on go1.27-requiring deps (pre-existing, out of scope)

- **Found during:** 18-06 Task 1 lint gate. `golangci-lint run` (2.12.2, the `.mise.toml` pin resolving "2")
  panics with `file requires newer Go version go1.27 (application built with go1.26)` on UNTOUCHED packages
  (reproduced on ./internal/session/...). Cause: a dependency in the module cache now declares go 1.27 while
  the installed 2.12.2 binary embeds a go1.26 typechecker. System toolchain is go 1.27.1.
- **Not fixed (scope boundary):** no 18-06 change touches go.mod/go.sum; the panic pre-dates this plan.
  golangci-lint 2.13.2 installs and runs, but its exhaustruct_v5 behavior reports 500+ issues repo-wide on
  code that passes the project's pinned gate — a lint-config migration, not a plan-18-06 fix.
- **Workaround used for 18-06's gate:** `mise exec golangci-lint@2.13.2 -- golangci-lint run --new-from-rev=<red-commit>`
  over the changed packages — 0 issues introduced. vet/build/test all green on the standard toolchain.
- **Suggested owner action:** bump `.mise.toml` golangci-lint pin to 2.13.x + migrate exhaustruct settings,
  as a standalone tooling task.
