// Package openspec implements the OpenSpec hosting adapter (Phase-4
// OPEN-01/02/03).
//
// ass-guard hosts the unmodified OpenSpec CLI as an external subprocess (D-13):
// the Adapter shells out to `openspec <command>`, captures stdout/stderr, and
// surfaces stdout into the model's context as a tool result. ass-guard does NOT
// reimplement OpenSpec logic — it is a thin subprocess host.
//
// A single TOML config (openspec.toml — D-14) declares:
//
//   - [[patterns]]     — text-regex → action (the dual-signal text half, D-02);
//   - [[handoff_tools]] — tool-call name → action (the dual-signal tool half);
//   - [commands]       — command name → mutability ("mutating" | "read-only",
//     D-15 / OPEN-03 — the single source of truth for context boundaries).
//
// The config is TOML (not YAML) to mirror the OpenSpec ecosystem idiom
// (RESEARCH §2.3 reconciliation). Each OpenSpec command's mutability drives
// toolcat.IsBoundary via the existing mutability floor — a mutating OpenSpec
// command IS a boundary (Phase-2 SESS-02), with NO boundary-engine change.
//
// Real-openspec tests are gated behind ASSGUARD_OPENSPEC_BIN=1 so CI never
// depends on the binary; unit tests use a stub script.
package openspec
