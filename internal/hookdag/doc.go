// Package hookdag implements the configurable hook-DAG executor (Phase-4
// HOOK-01..05) — the "forgotten routine" engine.
//
// After each SDD stage (post-implement, post-phase, or a custom stage), the
// engine (Plan 04-05) launches the matching hook from the declarative YAML
// config. The hand-rolled in-process executor (D-07 — ≤300 LOC core;
// Temporal/Argo/Windmill rejected as server products that would violate the
// single-binary guarantee) walks the hook's steps in declared order, dispatching
// each by kind:
//
//   - run-command  — exec a shell command; exit 0 ⇒ next step, non-zero ⇒ the
//     step's on-failure applies with stderr as the failure detail (the
//     Claudecourse exit-code contract);
//   - send-prompt  — inject a prompt as a REAL turn through the Session Core
//     (HOOK-05 — the hook orchestrates turns; it does not bypass them);
//   - fresh-context — open a new context window via the Phase-2 boundary
//     semantics (HOOK-05);
//   - wait        — delay for a Go duration (or until ctx cancels).
//
// Every step + hook declares on-failure (halt/continue/ask — HOOK-03): a
// failure is NEVER silent. Provenance (HOOK-04) structurally prevents loops —
// the executor refuses to launch a hook whose (Name, Trigger) is already
// in-flight unless the hook opts in via allow_reentrant. Every step boundary
// emits a HookProgress event so the routine is fully auditable
// (investigate-and-fix-ready, PROJECT.md).
//
// The executor is seam-driven: it depends only on small interfaces
// (CommandRunner / TurnRunner / BoundaryOpener) so the unit tests inject fakes
// and Plan 04-05 wires the real Session Core / exec / boundary implementations.
// This package has NO dependency on internal/engine, internal/session, or
// internal/provider — it is fully standalone + deterministic.
package hookdag
