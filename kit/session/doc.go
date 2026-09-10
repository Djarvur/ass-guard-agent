// Package session implements the Session Core (D-17): the component that owns
// the durable transcript (the one audit artifact per D-20), projects the lean
// model-visible window at each turn boundary (D-01/D-02), and runs the turn loop
// (D-18).
//
// The Session Manager is the sole transcript writer (SESS-06 — append-only JSONL
// under `.ass-guard/`, with per-line LOG-03 redaction). The projector builds the
// lean window by mechanical extraction from the transcript — no model call (D-02
// lint). The turn loop wires the Phase-1 Shaper + Provider into the streaming
// cycle (real Session.Prompt lands in Plan 02-02; streaming wiring in Plan
// 02-05; subagent dispatch in Plan 02-06).
//
// This package replaces the Phase-1 test-harness loop (internal/loop). The
// Phase-1 AuditLogger folds into the TranscriptWriter here (D-20 — one artifact,
// one writer, no two-writer drift; Plan 02-07).
package session
