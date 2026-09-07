---
phase: 20-built-in-commands-skills-per-agent-model
depth: standard
reviewed_at: 2026-09-08T00:55:00Z
status: clean
---

# Phase 20 Code Review

Scope (from SUMMARY key-files, production files only): internal/runtime/{commands.go,rescan.go,runtime.go}, internal/session/{subagent.go,session.go,transcript.go,manager.go,projector.go}, internal/acp/{types.go,emitter.go,server.go,handlers.go}, internal/acpserve/{acp_serve.go,command_source.go}, internal/ecosys/{types.go,loader.go}, internal/modelrouting/config.go, go.mod/go.sum.

Method: per-file analysis at standard depth (inline execution — the reviewer agent runtime is unavailable in this environment; the reviewer role was executed directly over the full diff `git log a2e9ac4..HEAD`).

## Findings

| ID | Severity | File | Finding | Status |
|----|----------|------|---------|--------|
| CR-01 | Warning | internal/session/projector.go | projectFullReset beat a NEWER compaction marker unconditionally — clear→compact→turn dropped the marker's durable summary (violates the last-reset-point-governs discipline) | FIXED (830d045, with the clear battery re-run green) |
| CR-02 | Info | internal/runtime/commands.go | builtinModel accepts any slug when schedCfg is nil (no validation possible) — unreachable in the serve composition (config always loads); documented behavior for bare runners | Accepted |
| CR-03 | Info | internal/runtime/rescan.go | maybeFreshRescan runs a synchronous Discover on drift INSIDE the resolution path — bounded by the plan's D-10 contract (synchronous-before-resolve); a hung filesystem delays freshness, not the session (matches T-20-20's accepted posture) | Accepted |
| CR-04 | Info | internal/runtime/runtime.go | dispatchAgentSlash runs DispatchSubagent synchronously under the turn mutex — same discipline as /compact (19-D-11); subagent streaming keeps the client live through the forwarder | Accepted (by design) |

## Security posture

- P-20-01 (no authority from discovered files): verified — frontmatter Tools/AllowedTools ride advisory paths only; the restricted set derives from the existing PARA defaults; no allow-granting path added.
- P-20-02 (cost source note): verified — every /cost output variant carries its producer note; battery-asserted.
- Credential hygiene (T-20-05): /doctor and /cost batteries assert the fixture secret never renders; Authorization headers carry values but output builders never interpolate them.
- T-20-SC (fsnotify dependency): audit-approved, pinned v1.10.1, no indirect surprises in go.sum.

## Verdict

**clean** — one Warning found and fixed in-phase (CR-01); the rest are Info-class accepted behaviors with their rationale named.
