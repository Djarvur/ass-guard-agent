---
phase: 06-distribution-polish
plan: 01
status: complete
requirements: [DIST-03]
decisions: [D-01, D-04]
key-files:
  created:
    - internal/defaults/defaults.go
    - internal/defaults/defaults_test.go
    - internal/defaults/seed/profiles/zcode/ (sanitized profile)
    - internal/defaults/seed/openspec.toml
    - internal/defaults/seed/scheduling.yaml
    - internal/defaults/seed/sync.sh
    - internal/firstrun/firstrun.go
    - internal/firstrun/firstrun_test.go
    - cmd/ass-guard/zeroconfig_test.go
  modified:
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/goconst_constants.go
    - internal/scheduler/load.go
---

# Plan 06-01 SUMMARY — go:embed defaults + zero-config first run (the TRACER)

## What was built

The thinnest end-to-end slice proving Phase 6's zero-config promise: drop the
ass-guard binary into an empty project, run `acp serve`, and the agent seeds
`.ass-guard/` with a working zcode profile + openspec.toml + scheduling.yaml,
then loads the seeded profile — no flags, no config, no network.

- **`internal/defaults`** (D-01) — embeds the seed tree via
  `//go:embed seed/profiles seed/openspec.toml seed/scheduling.yaml` (an
  `embed.FS` named `Seed`). `WriteTree(root, overwrite)` materializes it under
  root (non-clobbering when overwrite=false; overwrites when true).
- **`internal/firstrun`** (D-04) — `Ensure(workDir) (bool, error)`: resolves
  `<workDir>/.ass-guard`; on a missing dir creates it, writes the D-07
  self-gitignore (byte-identical to `internal/session/transcript.go`'s
  `selfGitignoreContent`), and lays down the embedded tree. An existing
  `.ass-guard/` (incl. partial/empty) returns (false, nil), mutating nothing.
- **`acp serve` wiring** — the RunE resolves workDir eagerly, calls
  `firstrun.Ensure`, and resolves `profilesDir` to `<workDir>/.ass-guard/profiles`
  when the flag is default and that dir exists (zero-config, DIST-03), else falls
  back to the dev `./profiles`. First-run log goes to stderr only (transport
  discipline). Logic extracted into `resolveWorkDir`/`seedACPGuard`/
  `resolveProfilesDir` helpers.
- **`sync.sh`** — the T-06-01 (HIGH) build-operator leak barrier: regenerates
  the seed from canonical sources, sanitizing the absolute paths
  (`/Users/nil/DiskD/...` → `<working_directory>` in block-2.txt; the meta.yaml
  rollout path → tilde-relative) and excluding coverage.yaml (dev-only, 6 abs
  paths, loader-unread).
- **`zeroconfig_test.go`** — the TRACER proof: builds the real binary by import
  path, runs `acp serve` in a fresh empty dir, sends an ACP `initialize` frame,
  and asserts seeding + handshake + transport discipline + idempotence.

## Decisions honored

- **D-01** embedded defaults via go:embed (zero network on first run; the binary
  IS the distribution).
- **D-04** first-run flow (detect missing `.ass-guard/` → create + gitignore →
  write embedded defaults → ready); non-clobbering; `acp serve` is the hook point.
- **D-07** self-gitignoring `.ass-guard/` (reuses the `internal/session` pattern).

## Notable findings / investigate-and-fix-ready

1. **Catalog drift (tools = 103, not the plan's 77).** The plan's acceptance
   criteria reference `jq 'length' == 77`, but the dev `profiles/zcode/tools.json`
   carries 103 tools (the PROF-04 drift event documented in STATE.md). The seed
   mirrors the actual source byte-for-byte (the drift-guard precedent), so the
   tests assert 103. The plan's "77" was stale; fudging it would mask the drift.
2. **T-06-04 (LOW) cross-phase openspec.toml path.** The seed writes
   `.ass-guard/openspec.toml`; the Phase-4 engine loads its embedded floor via
   `openspec.DefaultConfig()`. The operator overlay path (this file) is the
   reconciliation seam — flagged in the file's header comment for a future
   engine read-path alignment (degrades to "engine uses its embedded default",
   not a crash).

## Security

- **T-06-01 (HIGH, mitigated):** build-operator environment leak. The dev profile
  carried `/Users/nil/...` in block-2.txt + meta.yaml + coverage.yaml. sync.sh
  sanitizes the seed; the leak-guard test fails if `/Users/` or `/home/`
  re-appears in the embed; coverage.yaml is excluded entirely.
- **T-06-02 (MEDIUM, mitigated):** writes confined to the fixed `.ass-guard/`
  relative path; modes 0o644/0o755; content is trusted embedded seed; non-clobbering.
- **T-06-03 (LOW, mitigated):** first-run log routed to stderr; the E2E test
  asserts stdout carries ONLY ACP frames.

## Self-Check: PASSED

- `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...` all exit 0.
- `go test ./internal/defaults/... ./internal/firstrun/... ./cmd/...` exits 0
  (incl. the zero-config E2E under -trimpath).
- `grep '/Users/\|/home/' internal/defaults/seed/` (embeddable files) → empty.
- `scheduler.EmbeddedDefaultScheduling()` == `defaults.Seed` scheduling.yaml
  (embed-to-embed drift guard, -trimpath-safe).
- `goreleaser build --snapshot` binary performs the same zero-config first run
  (proven in Plan 06-02 T4 — embed survives cross-compile).
