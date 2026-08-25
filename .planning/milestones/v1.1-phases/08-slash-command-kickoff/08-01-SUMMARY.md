---
phase: 08-slash-command-kickoff
plan: 01
subsystem: ecosystem
tags: [slash-commands, discovery, yaml-frontmatter, zcode-parity, openspec]

# Dependency graph
requires:
  - phase: 05-ecosystem-compatibility (v1.0)
    provides: Registry/Load/discoverCommands flat scan + D-06 two-phase precedence
provides:
  - discoverCommands walks ONE level of subdirectories (commands/<ns>/<name>.md keyed "<ns>:<name>") — opsx:* layouts visible
  - zcode name-regex validation (^[a-z0-9][a-z0-9_:-]{0,63}$), description-fallback/drop rules, flat frontmatter semantics
  - Command.ArgumentHint/AllowedTools/Model parsed (not acted on — D-09)
  - stderr shadow warnings naming both file paths on every same-key precedence overwrite (CMD-05)
  - committed REAL openspec-init fixture tree + env-gated fixture-provenance test (ASSGUARD_OPENSPEC_BIN)
affects: [08-04 expansion seam, 08-05 skills, 08-06 e2e, ecosys consumers of Registry.Commands keys]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "flat single-line frontmatter semantics with strict-yaml-first + flat fallback + flat overlay (zcode parity)"
    - "swappable slog stderr seam for precedence-shadow observability"

key-files:
  created:
    - internal/ecosys/testdata/opsx-real/.claude/commands/opsx/*.md
    - internal/ecosys/testdata/opsx-real/.claude/skills/openspec-explore/SKILL.md
    - internal/ecosys/testdata/opsx-real/README.md
  modified:
    - internal/ecosys/loader.go
    - internal/ecosys/types.go
    - internal/ecosys/loader_test.go
    - internal/ecosys/precedence_test.go

key-decisions:
  - "Flat overlay applies even when strict YAML succeeds: multi-line/indented values zcode drops are dropped (accepting-what-zcode-drops would be mimicry divergence — PITFALLS 6)"
  - "Bare `key:` block-list openers count as absent in the flat view (no single-line value)"
  - "Shadow warnings via swappable package-level slog logger defaulting to stderr; mergeRegistries signature unchanged"

patterns-established:
  - "Real-fixture testdata + env-gated regeneration check (ASSGUARD_OPENSPEC_BIN) — fixture cannot rot"
  - "RED→GREEN commits per task under MVP+TDD"

requirements-completed: [CMD-01, CMD-05]

# Metrics
duration: 42min
completed: 2026-08-14
---

# Phase 8 Plan 01: ecosys namespaced discovery Summary

**`/opsx:*` layouts discovered with colon-joined keys under the existing precedence, proven against a real `openspec init --tools claude` fixture, with zcode parse semantics and stderr shadow warnings**

## Performance

- **Duration:** ~42 min
- **Started:** 2026-08-14T18:02Z
- **Completed:** 2026-08-14T18:44Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments
- `discoverCommands` walks one level of subdirectories (`commands/opsx/explore.md` → key `opsx:explore`, colon not slash); two-level trees are not flattened; flat and namespaced layouts coexist
- zcode ground-truth semantics: name regex with silent drops, description fallback to first non-empty body line, drop when neither description nor body, unknown frontmatter keys ignored, flat single-line frontmatter view (strict-yaml first, flat fallback on YAML error, flat overlay dropping multi-line values zcode drops)
- Every same-key Skills/Commands precedence overwrite emits one stderr warning naming both file paths — precedence direction unchanged (D-06)
- REAL fixture tree committed from actual `openspec init --tools claude` (v1.5.0) output with regeneration README; env-gated `TestOpsxFixtureMatchesRealInit` re-verifies against the live binary

## Task Commits

1. **Task 1: real fixture + failing discovery tests (RED)** - `deae022` (test)
2. **Task 2: failing shape/frontmatter tests (RED)** - `7d5ab3a` (test), then GREEN `fc553e8` (feat)
3. **Task 3: failing shadow-warning tests (RED)** - `d31fcea` (test), then GREEN `438722a` (feat)

**Plan metadata:** (this commit)

## Files Created/Modified
- `internal/ecosys/loader.go` - discoverCommands + namespaced walk + name regex + parseCommand flat semantics + shadow-warning merge
- `internal/ecosys/types.go` - Command gains ArgumentHint/AllowedTools/Model (parsed only, D-09)
- `internal/ecosys/loader_test.go` - Tests 1-10 + gated fixture-provenance test
- `internal/ecosys/precedence_test.go` - Tests 11-14 (shadow warnings)
- `internal/ecosys/testdata/opsx-real/` - real openspec-init fixture + README

## Decisions Made
- Flat-view overlay applies even on strict-YAML success so block-style lists (valid YAML) are dropped exactly as zcode drops them; a bare `key:` opener counts as absent
- Shadow logger is a swappable package-level slog var (default stderr) — mergeRegistries signature unchanged, per the plan's implementer's-choice

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- A concurrent planning session committed phase-10 docs commits interleaved with this plan's commits (observed between Task 3 RED and GREEN). No overlap in files; all plan commits remained atomic. Noted for STATE.md.

## TDD Gate Compliance

RED commits: `deae022`, `7d5ab3a`, `d31fcea` (all verified failing before implementation). GREEN commits: `fc553e8`, `438722a` (full package `-race` green after each).

## Next Phase Readiness
- `opsx:explore` … `opsx:sync` keys now resolvable from `Registry.Commands` — 08-04's expansion seam and 08-06's E2E can consume them
- `go test ./internal/ecosys/ -race`, `go vet`, `golangci-lint run internal/ecosys/...` all clean; `ASSGUARD_OPENSPEC_BIN=1` gated test passes with the installed v1.5.0 binary; `mise run build` green

## Self-Check: PASSED

- FOUND: internal/ecosys/testdata/opsx-real/.claude/commands/opsx/explore.md
- FOUND: internal/ecosys/loader.go (contains colon-join + shadows warning site)
- Commits deae022/7d5ab3a/fc553e8/d31fcea/438722a present on master

---
*Phase: 08-slash-command-kickoff*
*Completed: 2026-08-14*
