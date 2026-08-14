---
phase: 08-slash-command-kickoff
plan: 05
subsystem: skills
tags: [skills, skill-tool, captured-listing, zcode-shape, dynamic-merge, per-session-copy]

# Dependency graph
requires:
  - phase: 08-01
    provides: ecosys skill discovery (AllSkills, precedence-resolved registry)
  - phase: 08-04
    provides: the startup ecosys registry field (loadCommandRegistry) + sessionFor wiring points
  - phase: 05 ecosystem-compat (v1.0)
    provides: the dynamic-merge + per-session-copy pattern (D-01/D-16)
provides:
  - ecosys.SkillListing/ResolveSkill/SkillExecute — the listing in the exact captured zcode shape; SKILL.md bodies as tool results; structured unknown/read errors
  - sessionFor wiring: the captured Skill tool becomes executable per session (schema untouched), the listing merges into the profile copy
  - golden-fixture pin of the captured listing format (header, entry shape, 249-char truncation, file suffix)
affects: [08-06 (E2E opsx flows rely on skills loading), Phase 9 re-capture (listing placement + Skill result shape are flagged divergences)]

# Tech tracking
tech-stack:
  added: []  # stdlib only
  patterns:
    - "override-Execute-only registration: the captured tool entry's Description/InputSchema stay byte-identical while Execute is replaced (mimicry integrity)"
    - "captured-shape golden fixture with the source artifact named in-test (D-06 evidence discipline)"
    - "hermetic skill tests: HOME pinned to an empty temp dir so user-scope discovery contributes nothing"

key-files:
  created:
    - internal/ecosys/skills.go
    - internal/ecosys/skills_test.go
  modified:
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go

key-decisions:
  - "Listing shape pinned from a REAL capture before code (D-06): rollout record model-io-sess_fb066d52-fab9-4726-a174-ac8f86874dad.jsonl messages[5] — a dedicated system-role message right after the first user message; entries '- <name>: <desc> (file: <abs path>)'; project skills carry no plugin alias; descriptions hard-cut at 249 chars + '...' (measured on the go-ultimate entry)"
  - "Placement approximation: the capture places the listing mid-conversation (after the first user message); ass-guard's Shaper supports only leading system blocks, so the listing rides as a trailing System TextBlock — documented in-code as a divergence for the Phase-9 re-capture"
  - "Skill result shape: no captured Skill tool_use result exists anywhere on this machine (listings present, invocations absent) — success shape follows ass-guard's established {\"content\": …} convention (WebFetch); flagged for Phase-9 re-capture"
  - "SkillExecute returns structured results for ALL outcomes (unknown skill → {\"error\",\"available\"}; read failure → {\"error\",\"skill\",\"file\"}) with nil Go errors — the D-10 discipline applied to skills"

patterns-established:
  - "resolution BY REGISTRY KEY only — Path comes from discovery, never from call input (T-8-20)"
  - "closure registers even on an empty registry (a Skill call then returns the unknown-skill structure — degradation is structured, never a nil-Execute panic)"

requirements-completed: [CMD-06]

# Metrics
duration: 45min
completed: 2026-08-14
---

# Phase 8 Plan 05: Skills, claude-code-compatibly Summary

**The model can now invoke Skill("openspec-explore") and get the real SKILL.md body through the captured tool entry, with every discovered skill listed in the exact captured zcode format — the prerequisite the opsx command bodies lean on.**

## Performance

- **Duration:** ~45 min
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Captured ground truth extracted FIRST (D-06): the listing format transcribed from a real zcode session on this machine — header line, `- <name>: <desc> (file: <path>)` entries, the 249-char mid-word description cutoff, placement as a dedicated system message after the first user message
- `ecosys.SkillListing` renders all discovered skills (D-04 — no filtering) in the pinned shape; empty registry yields "" (no merge)
- `ecosys.ResolveSkill` re-reads the SKILL.md body after frontmatter, by registry key only
- `ecosys.SkillExecute`: hit → `{"content": body}`; unknown → `{"error","available"}`; read failure → `{"error","skill","file"}` — all structured, nil Go errors
- `sessionFor`: the captured Skill entry gets the closure (Description/InputSchema byte-identical — asserted against the embedded coretools entry); the listing merges into the per-session profile copy as a trailing System TextBlock with fresh-slice appends (shared `r.profile` never mutated — asserted)
- Round-trip proven: a model-emitted Skill tool_call flows MCPExecutor → RealExecutor → catalog → closure, and the transcript records the tool_call + tool_result carrying the body; the expanded `/opsx:explore` context carries the listing (natural trigger, no auto-injection)

## Task Commits

1. **Task 1: shape pin + ecosys implementation** — `57f44d3` (skills.go + golden/truncation/expose/resolve/execute tests; amend cycle for lint clean-up)
2. **Task 2: sessionFor wiring (RED→GREEN)** — `a7815aa` (test: 5 behavioral REDs, nil-Execute panic on the zero-skill case) → `3d8da37` (feat: closure registration + listing merge + hermetic HOME pinning)

**Plan metadata:** (this commit)

## Files Created/Modified
- `internal/ecosys/skills.go` — SkillListing / ResolveSkill / SkillExecute (+ captured-source documentation)
- `internal/ecosys/skills_test.go` — golden fixture (source named in-test), truncation cutoff, all-skills-exposed, body re-read, structured errors
- `cmd/ass-guard/acp_serve.go` — sessionFor wiring (skillToolName, override-Execute-only, System TextBlock merge)
- `cmd/ass-guard/acp_serve_test.go` — Tests 1-5

## Decisions Made
- See key-decisions: capture-first pinning, placement approximation, result-shape convention, structured errors

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2] Listing placement approximated (leading vs mid-conversation)**
- **Issue:** the capture places the listing as its own system message AFTER the first user message; the Shaper (out of this plan's file scope) supports only leading system blocks
- **Fix:** trailing System TextBlock merge — the same system-role channel, content byte-identical, position approximated; documented in-code + here, flagged for Phase-9 re-capture
- **Committed in:** `3d8da37`

**2. [Rule 3 - Blocking] Skill result shape unpinnable from capture**
- **Found during:** Task 1 ground-truth sweep (all rollout records searched — listings present, zero Skill invocations)
- **Fix:** success shape follows the repo's established `{"content": …}` content-result convention (WebFetch); unknown/read errors carry full structure; flagged for Phase-9 re-capture
- **Committed in:** `57f44d3`

**3. [Rule 2] Test hermeticity: HOME pinned in skill tests**
- **Found during:** Task 2 GREEN (zero-skill test failed against the operator's real ~/.claude skills — user-scope discovery is production-correct, the test wasn't hermetic)
- **Fix:** `newSkillRunner` pins HOME to an empty temp dir; the five skill tests give up t.Parallel() for t.Setenv
- **Committed in:** `3d8da37`

---

**Total deviations:** 3 auto-fixed (1 blocking, 2 sequencing/approximation)
**Impact on plan:** No scope creep; both unpinnable shapes documented for the Phase-9 re-capture instead of guessed silently.

## Issues Encountered
- None beyond the deviations. Full `go test ./... -race` suite green; `go vet` + `golangci-lint run ./...` (0 issues) + `CGO_ENABLED=0 go build ./...` clean.

## TDD Gate Compliance
RED commit: `a7815aa` (Tests 1-5 behavioral RED; the nil-Execute panic in Test 5 was the visible failure). GREEN commit: `3d8da37`. Task 1 is type `execute` (no RED mandate): implementation + golden fixture committed together (`57f44d3`), with the capture extracted before any code.

## Next Phase Readiness
- 08-06's E2E can rely on: /opsx:* expansion (08-04) + openspec:* tools (08-03) + skills loading (this plan) — the full kickoff chain
- Two flagged divergences (listing placement, Skill result shape) are queued for Phase-9's parity re-capture

## Self-Check: PASSED

- FOUND: internal/ecosys/skills.go (SkillListing + ResolveSkill + SkillExecute; captured source documented)
- FOUND: cmd/ass-guard/acp_serve.go (SkillExecute + SkillListing wiring in sessionFor; no writes to profiles/)
- Commits 57f44d3/a7815aa/3d8da37 present on master

---
*Phase: 08-slash-command-kickoff*
*Completed: 2026-08-14*
