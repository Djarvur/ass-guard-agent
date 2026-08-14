---
phase: 06-distribution-polish
plan: 02
status: complete
requirements: [DIST-01, DIST-02]
decisions: [D-02, D-03]
key-files:
  created:
    - internal/version/version.go
    - internal/version/version_test.go
    - .goreleaser.yml
    - agent.json
    - scripts/gen-agent-json.sh
    - docs/install.md
  modified:
    - cmd/ass-guard/main.go
    - .gitignore
---

# Plan 06-02 SUMMARY — goreleaser static binary + ACP registry agent.json

## What was built

The distribution layer that makes ass-guard installable: a single goreleaser
config produces four static binaries (darwin/linux × amd64/arm64, CGO_ENABLED=0),
plus a canonical ACP registry `agent.json` so `zed: acp registry` spawns
`ass-guard acp serve` correctly.

- **`internal/version`** — `var Version = "dev"` (ldflags-overridable via
  `//nolint:gochecknoglobals`) + `String()` accessor. The root command gains a
  `--version` flag printing `ass-guard version <v>` to STDERR (transport
  discipline; stdout stays byte-clean) and exiting 0.
- **`.goreleaser.yml`** (D-02) — goreleaser v2.17; one build (`main:
  ./cmd/ass-guard`, `env: [CGO_ENABLED=0]`, `goos: [linux, darwin]`,
  `goarch: [amd64, arm64]` — 4 targets, no windows). ldflags inject
  `internal/version.Version` via the capital-D `github.com/Djarvur/...` path.
  Archives are tar.gz bundling `agent.json` + `README.md`. `goreleaser check`
  exits 0.
- **`agent.json`** (D-03, Tier-A corrected) — the canonical ACP registry
  manifest: `id`/`name`/`version`/`description`/`distribution.binary.<target>`
  with per-target `{archive, cmd, args}` for the 4 ACP targets
  (`darwin-aarch64`, `darwin-x86_64`, `linux-aarch64`, `linux-x86_64`).
  `cmd: "ass-guard"`, `args: ["acp", "serve"]`. NO `command`/`command_args`/
  `cwd` (the superseded D-03 wording). license MIT (matches LICENSE).
- **`scripts/gen-agent-json.sh`** — regenerates `agent.json` from `VERSION` +
  `CHECKSUMS_FILE` (fills per-target sha256 at release; writes 0.0.0-dev
  placeholders for snapshots). Maps ACP aarch64/x86_64 naming to goreleaser's
  arm64/amd64 archive filenames. Fixture-tested.
- **`docs/install.md`** — the team install flow (download/extract → PATH →
  `zed: acp registry` → zero-config first launch), the registry-PR submission
  path (fork `agentclientprotocol/registry`, add `ass-guard/` folder +
  agent.json + icon.svg), and the local snapshot build. Documents `cmd`/`args`
  (not the superseded names).
- `.gitignore` gains `/dist/` (goreleaser output; the agent.json dev placeholder
  stays tracked).

## Decisions honored

- **D-02** goreleaser 4 targets, CGO_ENABLED=0, `.goreleaser.yml` as single source.
- **D-03** ACP registry manifest — the INTENT (declare spawn command + args;
  goreleaser ships the binary) is preserved; only the field NAMES corrected to
  the canonical schema (`cmd`/`args`, not `command`/`command_args`/`cwd`).
  time/tzdata is already blank-imported in `internal/scheduler/init.go` (D-03).

## Notable findings / investigate-and-fix-ready

1. **Tier-A: the ldflags -X target MUST use capital-D `github.com/Djarvur/...`.**
   The go.mod module path is `github.com/Djarvur/ass-guard-agent` (capital D).
   The plan's lowercase `github.com/djarvur/...` silently fails to inject
   (verified — lowercase leaves Version="dev"). `.goreleaser.yml` uses the
   correct capital-D path.
2. **Tier-A: D-03 field names corrected.** The CONTEXT/STACK phrasing
   `command`/`command_args`/`cwd` does NOT match the canonical ACP schema
   (`agent.schema.json`, fetched 2026-08-09). The real schema requires
   `distribution.binary.<target>.{archive, cmd, args, sha256}`. Implemented the
   REAL schema; acceptance asserts the superseded fields are ABSENT.
3. **goreleaser v2.17 schema deprecations.** `archives.format` → `formats: [tar.gz]`;
   `archives.extra_files` → `archives.files`. Fixed per `goreleaser check`.
4. **goreleaser not preinstalled; `go install @latest` failed** (its pinned
   `golang.org/x/tools v0.21.0` doesn't compile under Go 1.26). Installed the
   prebuilt v2.17.1 binary (built with go1.26.5) — local tooling, not a project dep.
5. **`go mod tidy` + `gen-agent-json.sh` run as goreleaser before.hooks** —
   verified the working tree stays clean after a snapshot build (only `dist/`
  untracked, which is now gitignored).

## DIST-01 / DIST-02 evidence (the snapshot build proof)

`goreleaser build --snapshot --clean` produced 4 binaries under `dist/`:
- `file` on the **linux_amd64** binary reports `ELF 64-bit LSB executable,
  x86-64, ... statically linked` — **CGO_ENABLED=0 proven in the real goreleaser
  build** (no shared-object deps).
- `--version` on the snapshot binary prints `ass-guard version 0.0.1-dev`
  (ldflags injection confirmed end-to-end through goreleaser).
- The goreleaser-built binary performs the Plan 06-01 zero-config first run in
  an empty temp dir: seeds `.ass-guard/`, logs to stderr, returns a valid ACP
  `initialize` response on stdout — **the embed survived the cross-compile**.
- No GitHub token / API key required (snapshot is local-only).

## Security

- **T-06-05 (MEDIUM, mitigated):** registry manifest spawn. Implemented the REAL
  schema; acceptance asserts `command`/`command_args`/`cwd` ABSENT.
- **T-06-06 (MEDIUM, mitigated):** supply-chain — go.sum pins deps, CGO_ENABLED=0
  removes C-toolchain surface, goreleaser builds from the tagged commit, per-target
  sha256 lets installers verify. (Full reproducible builds/SLSA out of scope for v1.)
- **T-06-07 (LOW):** version spoofing via ldflags — informational; archives are
  authoritative (sha256-pinned).
- **T-06-08 (LOW):** `cwd` spawn scope inherited from D-03 — bounded by 06-01's
  T-06-02 (writes confined to `.ass-guard/`).

## Self-Check: PASSED

- `goreleaser check` exits 0 (config valid against v2.17.1).
- `goreleaser build --snapshot --clean` produced 4 binaries; linux_amd64 is
  statically linked.
- `agent.json` structurally valid (4 targets, cmd/args canonical, no superseded
  fields, license MIT); gen-agent-json.sh fixture-tested (sha256 fill + omit).
- `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`,
  `go test -race ./...` all exit 0 (no regression from the --version flag).
- Full phase gate: `mise ci` exits 0 (vet + golangci-lint 0 issues + build + race).
